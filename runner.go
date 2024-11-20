package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/sethvargo/go-githubactions"
)

type RunnerConfig struct {
	Mode             string
	GitHubToken      string
	EC2ImageID       string
	EC2InstanceType  string
	SubnetID         string
	SecurityGroupID  string
	IamInstanceRole  string
	AWSResourceTags  []types.Tag
	RepositoryURL    string
	SpotInstanceType bool
	SpotPrice        string
}

func main() {
	// Retrieve configuration from GitHub Actions inputs
	config := RunnerConfig{
		Mode:             githubactions.GetInput("mode"),
		GitHubToken:      githubactions.GetInput("github-token"),
		EC2ImageID:       githubactions.GetInput("ec2-image-id"),
		EC2InstanceType:  githubactions.GetInput("ec2-instance-type"),
		SubnetID:         githubactions.GetInput("subnet-id"),
		SecurityGroupID:  githubactions.GetInput("security-group-id"),
		IamInstanceRole:  githubactions.GetInput("iam-instance-role"),
		RepositoryURL:    os.Getenv("GITHUB_REPOSITORY"),
		SpotInstanceType: githubactions.GetInput("spot-instance") == "true",
		SpotPrice:        githubactions.GetInput("spot-price"),
	}

	// Parse resource tags if provided
	if tagsStr := githubactions.GetInput("aws-resource-tags"); tagsStr != "" {
		if err := json.Unmarshal([]byte(tagsStr), &config.AWSResourceTags); err != nil {
			githubactions.Fatalf("Failed to parse AWS resource tags: %v", err)
		}
	}

	// Use the config values
	fmt.Println("Mode:", config.Mode)
	fmt.Println("GitHub Token:", config.GitHubToken)
	fmt.Println("EC2 Image ID:", config.EC2ImageID)
	fmt.Println("EC2 Instance Type:", config.EC2InstanceType)
	fmt.Println("Subnet ID:", config.SubnetID)
	fmt.Println("Security Group ID:", config.SecurityGroupID)
	fmt.Println("IAM Instance Role:", config.IamInstanceRole)
	fmt.Println("AWS Resource Tags:", config.AWSResourceTags)
	fmt.Println("Repository URL:", config.RepositoryURL)
	fmt.Println("Spot Instance Type:", config.SpotInstanceType)
	fmt.Println("Spot Price:", config.SpotPrice)
}

// package main

// import (
// 	"context"
// 	"encoding/base64"
// 	"fmt"
// 	"log"
// 	"os"
// 	"strings"
// 	"time"

// 	"github.com/aws/aws-sdk-go-v2/aws"
// 	"github.com/aws/aws-sdk-go-v2/config"
// 	"github.com/aws/aws-sdk-go-v2/service/ec2"
// 	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
// 	"github.com/google/go-github/v66/github"
// 	"golang.org/x/oauth2"
// )

// type RunnerConfig struct {
// 	Mode             string
// 	GitHubToken      string
// 	EC2ImageID       string
// 	EC2InstanceType  string
// 	SubnetID         string
// 	SecurityGroupID  string
// 	AWSResourceTags  []types.Tag
// 	RepositoryURL    string
// 	SpotInstanceType bool
// 	SpotPrice        string
// 	IamInstanceRole  string
// }

// type EC2Runner struct {
// 	ec2Client    *ec2.Client
// 	githubClient *github.Client
// 	config       RunnerConfig
// 	instanceID   string
// 	runnerLabel  string
// }

// func NewEC2Runner(config RunnerConfig) (*EC2Runner, error) {
// 	// Load AWS configuration
// 	cfg, err := config.LoadAWSConfig()
// 	if err != nil {
// 		return nil, fmt.Errorf("unable to load AWS config: %v", err)
// 	}

// 	// Create AWS EC2 client
// 	ec2Client := ec2.NewFromConfig(cfg)

// 	// Create GitHub client
// 	ctx := context.Background()
// 	ts := oauth2.StaticTokenSource(
// 		&oauth2.Token{AccessToken: config.GitHubToken},
// 	)
// 	tc := oauth2.NewClient(ctx, ts)
// 	githubClient := github.NewClient(tc)

// 	return &EC2Runner{
// 		ec2Client:    ec2Client,
// 		githubClient: githubClient,
// 		config:       config,
// 	}, nil
// }

// func (c *RunnerConfig) LoadAWSConfig() (aws.Config, error) {
// 	return config.LoadDefaultConfig(context.TODO())
// }

// func (r *EC2Runner) StartRunner() error {
// 	ctx := context.TODO()

// 	if r.config.SpotInstanceType {
// 		return r.launchSpotInstance(ctx)
// 	}
// 	return r.launchOnDemandInstance(ctx)
// }

// func (r *EC2Runner) launchOnDemandInstance(ctx context.Context) error {
// 	runInput := &ec2.RunInstancesInput{
// 		ImageId:      aws.String(r.config.EC2ImageID),
// 		InstanceType: types.InstanceType(r.config.EC2InstanceType),
// 		MinCount:     aws.Int32(1),
// 		MaxCount:     aws.Int32(1),
// 		SubnetId:     aws.String(r.config.SubnetID),
// 		SecurityGroupIds: []string{
// 			r.config.SecurityGroupID,
// 		},
// 		TagSpecifications: []types.TagSpecification{
// 			{
// 				ResourceType: types.ResourceTypeInstance,
// 				Tags:         r.config.AWSResourceTags,
// 			},
// 		},
// 		UserData: aws.String(r.prepareUserData()),
// 	}

// 	// Add IAM instance role if specified
// 	if r.config.IamInstanceRole != "" {
// 		runInput.IamInstanceProfile = &types.IamInstanceProfileSpecification{
// 			Name: aws.String(r.config.IamInstanceRole),
// 		}
// 	}

// 	result, err := r.ec2Client.RunInstances(ctx, runInput)
// 	if err != nil {
// 		return fmt.Errorf("failed to launch EC2 instance: %v", err)
// 	}

// 	if len(result.Instances) > 0 {
// 		r.instanceID = *result.Instances[0].InstanceId
// 	}

// 	return r.registerGitHubRunner()
// }

// func (r *EC2Runner) launchSpotInstance(ctx context.Context) error {
// 	spotPrice := r.config.SpotPrice
// 	if spotPrice == "" {
// 		spotPrice = "0.10" // Default spot price
// 	}

// 	spotInput := &ec2.RequestSpotInstancesInput{
// 		SpotPrice: aws.String(spotPrice),
// 		InstanceCount: aws.Int32(1),
// 		LaunchSpecification: &types.LaunchSpecification{
// 			ImageId:      aws.String(r.config.EC2ImageID),
// 			InstanceType: types.InstanceType(r.config.EC2InstanceType),

// 			SubnetId: aws.String(r.config.SubnetID),
// 			SecurityGroupIds: []string{
// 				r.config.SecurityGroupID,
// 			},
// 			UserData: aws.String(r.prepareUserData()),
// 		},
// 		Type: types.SpotInstanceTypeOneTime,
// 	}

// 	// Add IAM instance role if specified
// 	if r.config.IamInstanceRole != "" {
// 		spotInput.LaunchSpecification.IamInstanceProfile = &types.IamInstanceProfileSpecification{
// 			Name: aws.String(r.config.IamInstanceRole),
// 		}
// 	}

// 	// Add tags
// 	if len(r.config.AWSResourceTags) > 0 {
// 		tagSpecs := types.TagSpecification{
// 			ResourceType: types.ResourceTypeSpotInstancesRequest,
// 			Tags:         r.config.AWSResourceTags,
// 		}
// 		spotInput.TagSpecifications = []types.TagSpecification{tagSpecs}
// 	}

// 	// Request spot instance
// 	spotResult, err := r.ec2Client.RequestSpotInstances(ctx, spotInput)
// 	if err != nil {
// 		return fmt.Errorf("failed to request spot instance: %v", err)
// 	}

// 	// Wait for spot instance to be fulfilled
// 	if len(spotResult.SpotInstanceRequests) > 0 {
// 		spotRequestID := *spotResult.SpotInstanceRequests[0].SpotInstanceRequestId

// 		// Wait for spot instance to be active
// 		waiter := ec2.NewSpotInstanceRequestFulfilledWaiter(r.ec2Client)
// 		err = waiter.Wait(ctx, &ec2.DescribeSpotInstanceRequestsInput{
// 			SpotInstanceRequestIds: []string{spotRequestID},
// 		}, 10*time.Minute)

// 		if err != nil {
// 			return fmt.Errorf("error waiting for spot instance: %v", err)
// 		}

// 		// Describe spot instances to get instance ID
// 		describeInput := &ec2.DescribeSpotInstanceRequestsInput{
// 			SpotInstanceRequestIds: []string{spotRequestID},
// 		}
// 		describeResult, err := r.ec2Client.DescribeSpotInstanceRequests(ctx, describeInput)
// 		if err != nil {
// 			return fmt.Errorf("failed to describe spot instance request: %v", err)
// 		}

// 		if len(describeResult.SpotInstanceRequests) > 0 &&
// 		   describeResult.SpotInstanceRequests[0].InstanceId != nil {
// 			r.instanceID = *describeResult.SpotInstanceRequests[0].InstanceId
// 		}
// 	}

// 	return r.registerGitHubRunner()
// }

// func (r *EC2Runner) registerGitHubRunner() error {
// 	// Extract repository owner and name
// 	owner, repo, err := parseRepositoryURL(r.config.RepositoryURL)
// 	if err != nil {
// 		return err
// 	}

// 	ctx := context.Background()

// 	// Create runner registration request
// 	regReq := &github.CreateRegistrationTokenOptions{}
// 	registeredRunner, _, err := r.githubClient.Actions.CreateRegistrationToken(ctx, owner, repo, regReq)
// 	if err != nil {
// 		return fmt.Errorf("failed to get registration token: %v", err)
// 	}

// 	// Generate unique runner label
// 	r.runnerLabel = fmt.Sprintf("ec2-runner-%s", r.instanceID)

// 	log.Printf("Runner registered: %s", r.runnerLabel)
// 	return nil
// }

// func (r *EC2Runner) StopRunner() error {
// 	if r.instanceID == "" {
// 		return fmt.Errorf("no instance to terminate")
// 	}

// 	ctx := context.TODO()

// 	// Terminate instance
// 	input := &ec2.TerminateInstancesInput{
// 		InstanceIds: []string{r.instanceID},
// 	}
// 	_, err := r.ec2Client.TerminateInstances(ctx, input)

// 	return err
// }

// func (r *EC2Runner) prepareUserData() string {
// 	userData := fmt.Sprintf(`#!/bin/bash
// echo "Configuring GitHub Runner"
// mkdir -p /actions-runner
// cd /actions-runner
// curl -o actions-runner-linux-x64-2.294.0.tar.gz -L https://github.com/actions/runner/releases/download/v2.294.0/actions-runner-linux-x64-2.294.0.tar.gz
// tar xzf ./actions-runner-linux-x64-2.294.0.tar.gz
// ./config.sh --url %s --token RUNNER_TOKEN --name %s
// ./run.sh
// `, r.config.RepositoryURL, r.runnerLabel)

// 	return base64.StdEncoding.EncodeToString([]byte(userData))
// }

// func parseRepositoryURL(url string) (string, string, error) {
// 	// Basic repository URL parsing
// 	parts := strings.Split(strings.TrimPrefix(url, "https://github.com/"), "/")
// 	if len(parts) < 2 {
// 		return "", "", fmt.Errorf("invalid repository URL")
// 	}
// 	return parts[0], parts[1], nil
// }

// func main() {
// 	// Retrieve configuration from environment variables
// 	config := RunnerConfig{
// 		Mode:             os.Getenv("INPUT_MODE"),
// 		GitHubToken:      os.Getenv("INPUT_GITHUB_TOKEN"),
// 		EC2ImageID:       os.Getenv("INPUT_EC2_IMAGE_ID"),
// 		EC2InstanceType:  os.Getenv("INPUT_EC2_INSTANCE_TYPE"),
// 		SubnetID:         os.Getenv("INPUT_SUBNET_ID"),
// 		SecurityGroupID:  os.Getenv("INPUT_SECURITY_GROUP_ID"),
// 		RepositoryURL:    os.Getenv("GITHUB_REPOSITORY_URL"),
// 		SpotInstanceType: os.Getenv("INPUT_SPOT_INSTANCE") == "true",
// 		SpotPrice:        os.Getenv("INPUT_SPOT_PRICE"),
// 		IamInstanceRole:  os.Getenv("INPUT_IAM_INSTANCE_ROLE"),
// 	}

// 	// Parse resource tags if provided
// 	tagsStr := os.Getenv("INPUT_AWS_RESOURCE_TAGS")
// 	var tags []types.Tag
// 	if tagsStr != "" {
// 		// Implement JSON parsing for tags
// 		// This is a placeholder and would need proper JSON parsing
// 		tags = []types.Tag{
// 			{
// 				Key:   aws.String("Name"),
// 				Value: aws.String("GitHub-Runner"),
// 			},
// 		}
// 	}
// 	config.AWSResourceTags = tags

// 	// Create runner
// 	runner, err := NewEC2Runner(config)
// 	if err != nil {
// 		log.Fatalf("Failed to create runner: %v", err)
// 	}

// 	// Handle different modes
// 	switch config.Mode {
// 	case "start":
// 		if err := runner.StartRunner(); err != nil {
// 			log.Fatalf("Failed to start runner: %v", err)
// 		}
// 		// Output instance details for GitHub Actions
// 		fmt.Printf("::set-output name=label::%s\n", runner.runnerLabel)
// 		fmt.Printf("::set-output name=ec2-instance-id::%s\n", runner.instanceID)

// 	case "stop":
// 		if err := runner.StopRunner(); err != nil {
// 			log.Fatalf("Failed to stop runner: %v", err)
// 		}

// 	default:
// 		log.Fatalf("Invalid mode: %s", config.Mode)
// 	}
// }
