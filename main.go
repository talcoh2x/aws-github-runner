package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/sethvargo/go-githubactions"
)

type SpotConfig struct {
	ProvisioningMode string
	Region           string
}
type EC2RunnerConfig struct {
	EC2ImageID      string
	EC2InstanceType string
	SubnetID        string
	SecurityGroupID string
	IamInstanceRole string
	AWSResourceTags []types.Tag
	RepositoryURL   string
	SpotInstance    bool
	Spot            SpotConfig
}

type GithubRunnerConfig struct {
	GitHubToken     string
	GitHubTokenType string
	GitHubOrgRunner bool
}

type RunnerConfig struct {
	Mode               string
	GithubRunnerConfig *GithubRunnerConfig
	EC2RunnerConfig    *EC2RunnerConfig
}

type Runner struct {
	ec2Client    *AWSClient
	githubClient *GitHubClient
	config       RunnerConfig
	instanceID   *string
	runnerLabel  string
}

func NewRunner(config RunnerConfig) (*Runner, error) {
	// Initialize AWS client
	awsClient := &AWSClient{}
	awsClient = awsClient.NewAWSClient(config.EC2RunnerConfig)

	// Initialize Github client
	githubClient := &GitHubClient{}
	githubClient, err := githubClient.NewGithubClient(config.GithubRunnerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client: %v", err)
	}

	return &Runner{
		ec2Client:    awsClient,
		githubClient: githubClient,
		config:       config,
	}, nil
}

func (r *Runner) StartRunner(ctx context.Context) error {
	// create GitHub registration token
	registrationToken, err := r.githubClient.CreateGitHubRegistrationToken(ctx, r.config.GithubRunnerConfig.GitHubTokenType)
	if err != nil {
		log.Fatalf("Failed to create GitHub registration token: %v", err)
	}
	// set registration token
	r.githubClient.Action.RegistrationToken = registrationToken
	// launch instance
	r.instanceID, err = r.ec2Client.LaunchInstance(ctx, r.runnerLabel, *r.githubClient.Action.RegistrationToken.Token, r.githubClient.Action.OrgRunner)
	if err != nil {
		return fmt.Errorf("failed to launch instance: %v", err)
	}
	// runner created successfully
	fmt.Println("Runner created successfully " + *r.instanceID)

	// create a context with timeout for the waiting tasks
	waitCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()

	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Println("Waiting for instance to be ready...")
		if err := r.ec2Client.WaitForState(waitCtx, *r.instanceID); err != nil {
			errChan <- err
			cancel()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Println("Waiting for runner to register...")
		if err := r.githubClient.WaitForRunnerRegistered(waitCtx, r.runnerLabel); err != nil {
			errChan <- err
			cancel()
		}
	}()

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) StopRunner(ctx context.Context) error {
	if r.instanceID == nil {
		return fmt.Errorf("no instance to terminate")
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 2)
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()

	tasks := []func() error{
		func() error {
			if err := r.ec2Client.TerminateInstance(ctx, *r.instanceID); err != nil {
				return fmt.Errorf("failed to terminate instance: %v", err)
			}
			return nil
		},
		func() error {
			if err := r.githubClient.RemoveGithubRunner(ctx, r.runnerLabel); err != nil {
				return err
			}
			return nil
		},
	}

	for _, task := range tasks {
		wg.Add(1)
		go func(task func() error) {
			defer wg.Done()
			if err := task(); err != nil {
				errChan <- err
				cancel()
			}
		}(task)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

func main() {
	// Retrieve configuration from environment variables
	config := RunnerConfig{
		Mode: githubactions.GetInput("mode"),
		GithubRunnerConfig: &GithubRunnerConfig{
			GitHubToken:     githubactions.GetInput("github-token"),
			GitHubTokenType: githubactions.GetInput("github-token-type"),
			GitHubOrgRunner: githubactions.GetInput("github-org-runner") == "true",
		},
		EC2RunnerConfig: &EC2RunnerConfig{
			EC2ImageID:      githubactions.GetInput("ec2-image-id"),
			EC2InstanceType: githubactions.GetInput("ec2-instance-type"),
			SubnetID:        githubactions.GetInput("subnet-id"),
			SecurityGroupID: githubactions.GetInput("security-group-id"),
			IamInstanceRole: githubactions.GetInput("iam-instance-role"),
			RepositoryURL:   os.Getenv("GITHUB_REPOSITORY"),
			SpotInstance:    githubactions.GetInput("spot-instance") == "true",
			Spot: SpotConfig{
				ProvisioningMode: githubactions.GetInput("spot-provisioning-mode"),
				Region:           githubactions.GetInput("spot-region"),
			},
		},
	}

	// Parse resource tags if provided
	if tagsStr := githubactions.GetInput("aws-resource-tags"); tagsStr != "" {
		if err := json.Unmarshal([]byte(tagsStr), &config.EC2RunnerConfig.AWSResourceTags); err != nil {
			githubactions.Fatalf("Failed to parse AWS resource tags: %v", err)
		}
	}

	//TODO:  Validate configuration

	// Create a context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up a signal handler to cancel the context on interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create runner
	runner, err := NewRunner(config)
	if err != nil {
		log.Fatalf("Failed to create runner: %v", err)
	}

	// Handle different modes
	switch config.Mode {
	case "start":
		// Generate unique runner label
		runner.runnerLabel = fmt.Sprintf("ec2-runner-%s", time.Now().Format("20060102150405"))
		// Start runner
		err := runner.StartRunner(ctx)
		if err != nil {
			log.Fatalf("Failed to start runner: %v", err)
		}
		// Output instance details for GitHub Actions
		fmt.Printf("::set-output name=label::%s\n", runner.runnerLabel)
		fmt.Printf("::set-output name=ec2-instance-id::%s\n", *runner.instanceID)
	case "stop":
		if err := runner.StopRunner(ctx); err != nil {
			log.Fatalf("Failed to stop runner: %v", err)
		}
	default:
		log.Fatalf("Invalid mode: %s", config.Mode)
	}
}
