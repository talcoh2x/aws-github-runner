package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v66/github"
)

type Action struct {
	RegistrationToken *github.RegistrationToken
	OrgRunner         bool
}

type GitHubClient struct {
	Client *github.Client
	Token  string
	Repo   string
	Owner  string

	Action *Action
}

func (c *GitHubClient) NewGithubClient(config *GithubRunnerConfig) (*GitHubClient, error) {
	httpClient := &http.Client{}
	client := github.NewClient(httpClient).WithAuthToken(config.GitHubToken)
	//
	owner, repository, err := parseRepositoryURL(os.Getenv("GITHUB_REPOSITORY"))
	if err != nil {
		return nil, err
	}

	return &GitHubClient{
		Client: client,
		Token:  config.GitHubToken,
		Owner:  owner,
		Repo:   repository,
		Action: &Action{OrgRunner: config.GitHubOrgRunner},
	}, nil
}

func parseRepositoryURL(url string) (string, string, error) {
	// Basic repository URL parsing
	parts := strings.Split(strings.TrimPrefix(url, "https://github.com/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid repository URL")
	}
	return parts[0], parts[1], nil
}

// WaitForRunnerRegistered waits for a GitHub self-hosted runner to be registered and come online.
// It checks the runner's status every 10 seconds for up to 5 minutes.
//
// Parameters:
//
//	ctx - The context to control cancellation and timeout.
//	label - The label of the runner to wait for.
//
// Returns:
//
//	error - An error if the runner is not registered within the timeout period or if there is an issue listing the runners.
func (r *GitHubClient) WaitForRunnerRegistered(ctx context.Context, label string) error {
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return errors.New("timeout waiting for runner to register")
		case <-ticker.C:
			runners, _, err := r.Client.Actions.ListRunners(ctx, r.Owner, r.Repo, nil)
			if err != nil {
				return err
			}

			for _, runner := range runners.Runners {
				if runner.GetName() == label && runner.GetStatus() == "online" {
					return nil
				}
			}
		}
		fmt.Println("Waiting for runner to register...")
	}
}

// CreateGitHubRegistrationToken creates a registration token for adding a self-hosted runner.
//
// Parameters:
//
//	ctx - The context to control cancellation and timeout.
//	githubRunner - The type of runner ("org" for organization runner, otherwise repository runner).
//
// Returns:
//
//	*github.RegistrationToken - The registration token.
//	error - An error if the token creation fails.
func (r *GitHubClient) CreateGitHubRegistrationToken(ctx context.Context, githubRunner string) (*github.RegistrationToken, error) {
	var (
		token *github.RegistrationToken
		err   error
	)

	if r.Action.OrgRunner {
		token, _, err = r.Client.Actions.CreateOrganizationRegistrationToken(ctx, r.Owner)
	} else {
		token, _, err = r.Client.Actions.CreateRegistrationToken(ctx, r.Owner, r.Repo)
	}
	if err != nil {
		return nil, err
	}
	return token, nil
}

// getGithubRunnerID retrieves the ID of a GitHub self-hosted runner by its label.
//
// Parameters:
//
//	ctx - The context to control cancellation and timeout.
//	label - The label of the runner.
//
// Returns:
//
//	int64 - The ID of the runner, or 0 if not found.
func (r *GitHubClient) getGithubRunnerID(ctx context.Context, label string) int64 {
	runners, _, err := r.Client.Actions.ListRunners(ctx, r.Owner, r.Repo, nil)
	if err != nil {
		return 0
	}

	for _, runner := range runners.Runners {
		if runner.GetName() == label {
			return runner.GetID()
		}
	}
	return 0
}

// RemoveGithubRunner removes a self-hosted runner from the repository or organization.
//
// Parameters:
//
//	ctx - The context to control cancellation and timeout.
//	label - The label of the runner to remove.
//
// Returns:
//
//	error - An error if the runner removal fails.
func (r *GitHubClient) RemoveGithubRunner(ctx context.Context, label string) error {
	runnerID := r.getGithubRunnerID(ctx, label)
	if runnerID == 0 {
		return errors.New("runner not found")
	}

	_, err := r.Client.Actions.RemoveRunner(ctx, r.Owner, r.Repo, runnerID)
	if err != nil {
		return err
	}
	return nil
}
