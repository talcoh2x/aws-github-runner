package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type InstanceType = types.InstanceType

const (
	ProvisioningModeNone           = "None"           // Do not use spot instances
	ProvisioningModeSpotOnly       = "SpotOnly"       // Use spot instances only
	ProvisioningModeBestEffort     = "BestEffort"     // Use spot instances if available, otherwise use on-demand
	ProvisioningModeMaxPerformance = "MaxPerformance" // Use on-demand instances only
)

type EC2 struct {
	client *ec2.Client
	config EC2RunnerConfig
}

type AWSClient struct {
	cfg aws.Config
	Ec2 *EC2
}

func (c *AWSClient) NewAWSClient(config *EC2RunnerConfig) *AWSClient {
	// aws.Config could be initialized with some options
	return &AWSClient{
		cfg: aws.Config{},
		Ec2: &EC2{client: ec2.NewFromConfig(aws.Config{}), config: *config},
	}
}

func (r *AWSClient) LaunchInstance(ctx context.Context, label, registrationToken string, orgRunner bool) (*string, error) {
	switch r.Ec2.config.Spot.ProvisioningMode {
	case ProvisioningModeNone:
		return r.LaunchOnDemandInstance(ctx, label, registrationToken, orgRunner)
	case ProvisioningModeSpotOnly:
		return r.LaunchSpotInstance(ctx, label, registrationToken, orgRunner)
	// case ProvisioningModeBestEffort:
	// 	instanceType = types.InstanceType(config.EC2InstanceType)
	// 	maxPrice = spotPrice // Use spot price for best effort mode
	// case ProvisioningModeMaxPerformance:
	// 	return "", "", fmt.Errorf("provisioning mode is not supported " + config.Spot.ProvisioningMode)
	default:
		return nil, fmt.Errorf("unknown provisioning mode: %s", r.Ec2.config.Spot.ProvisioningMode)
	}
}

func (r *AWSClient) prepareUserData(label, registrationToken string, orgRunner bool) string {
	// TODO: add support for fine-grained token
	// if r.config.GitHubTokenType == "fine-grained" {
	// }
	owner, repo, _ := parseRepositoryURL(r.Ec2.config.RepositoryURL)
	url := fmt.Sprintf("https://github.com/%s/%s", owner, repo)
	if orgRunner {
		url = fmt.Sprintf("https://github.com/%s/", owner)
	}

	userData := fmt.Sprintf(`#!/bin/bash
	echo "Configuring GitHub Runner"
	mkdir -p /actions-runner
	cd /actions-runner
	case $(uname -m) in aarch64) ARCH="arm64" ;; amd64|x86_64) ARCH="x64" ;; esac && export RUNNER_ARCH=${ARCH}
	curl -O -L https://github.com/actions/runner/releases/download/v2.313.0/actions-runner-linux-${RUNNER_ARCH}-2.313.0.tar.gz
	tar xzf ./actions-runner-linux-${RUNNER_ARCH}-2.313.0.tar.gz
	export RUNNER_ALLOW_RUNASROOT=1
	./config.sh --url %s --token %s --name %s --work _work --labels %s
	./run.sh
	`, url, registrationToken, label, label)

	return base64.StdEncoding.EncodeToString([]byte(userData))
}

func (c *AWSClient) FetchSpotPrice(ctx context.Context, region, instanceType string) (string, error) {
	input := &ec2.DescribeSpotPriceHistoryInput{
		InstanceTypes:       []types.InstanceType{types.InstanceType(instanceType)},
		ProductDescriptions: []string{"Linux/UNIX"},
		StartTime:           aws.Time(time.Now().Add(-1 * time.Hour)),
		EndTime:             aws.Time(time.Now()),
	}

	result, err := c.Ec2.client.DescribeSpotPriceHistory(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to describe spot price history, %v", err)
	}

	if len(result.SpotPriceHistory) == 0 {
		return "", fmt.Errorf("no spot price history found for instance type %s in region %s", instanceType, region)
	}

	// Return the most recent spot price
	return *result.SpotPriceHistory[0].SpotPrice, nil
}

func (r *AWSClient) LaunchSpotInstance(ctx context.Context, label, registrationToken string, orgRunner bool) (*string, error) {
	// Get the spot price based on the region and instance type
	maxPrice, err := r.FetchSpotPrice(ctx, r.Ec2.config.Spot.Region, r.Ec2.config.EC2InstanceType)
	if err != nil {
		return nil, err
	}

	spotInput := &ec2.RequestSpotInstancesInput{
		SpotPrice:     aws.String(maxPrice),
		InstanceCount: aws.Int32(1),
		LaunchSpecification: &types.RequestSpotLaunchSpecification{
			ImageId:      aws.String(r.Ec2.config.EC2ImageID),
			InstanceType: types.InstanceType(r.Ec2.config.EC2InstanceType),
			SubnetId:     aws.String(r.Ec2.config.SubnetID),
			SecurityGroupIds: []string{
				r.Ec2.config.SecurityGroupID,
			},
			UserData: aws.String(r.prepareUserData(label, registrationToken, orgRunner)),
		},
		Type: types.SpotInstanceTypeOneTime,
	}

	// Add IAM instance role if specified
	if r.Ec2.config.IamInstanceRole != "" {
		spotInput.LaunchSpecification.IamInstanceProfile = &types.IamInstanceProfileSpecification{
			Name: aws.String(r.Ec2.config.IamInstanceRole),
		}
	}

	// Add tags
	if len(r.Ec2.config.AWSResourceTags) > 0 {
		tagSpecs := types.TagSpecification{
			ResourceType: types.ResourceTypeSpotInstancesRequest,
			Tags:         r.Ec2.config.AWSResourceTags,
		}
		spotInput.TagSpecifications = []types.TagSpecification{tagSpecs}
	}

	// Request spot instance
	spotResult, err := r.Ec2.client.RequestSpotInstances(ctx, spotInput)
	if err != nil {
		return nil, fmt.Errorf("failed to request spot instance: %v", err)
	}

	if len(spotResult.SpotInstanceRequests) == 0 {
		return nil, fmt.Errorf("no spot instance requests returned")
	}

	// Wait for spot instance to be fulfilled
	spotRequestID := *spotResult.SpotInstanceRequests[0].SpotInstanceRequestId
	waiter := ec2.NewSpotInstanceRequestFulfilledWaiter(r.Ec2.client)
	err = waiter.Wait(ctx, &ec2.DescribeSpotInstanceRequestsInput{
		SpotInstanceRequestIds: []string{spotRequestID},
	}, 6*time.Minute)

	if err != nil {
		if r.Ec2.config.Spot.ProvisioningMode == ProvisioningModeBestEffort || r.Ec2.config.Spot.ProvisioningMode == ProvisioningModeMaxPerformance {
			fmt.Println("Spot instance request failed, falling back to on-demand instance")
			// Fallback to On-Demand instance if spot instance request fails
			return r.LaunchOnDemandInstance(ctx, label, registrationToken, orgRunner)
		}
		return nil, fmt.Errorf("error waiting for spot instance request to be fulfilled: %v", err)
	}

	// Describe spot instances to get instance ID
	describeInput := &ec2.DescribeSpotInstanceRequestsInput{
		SpotInstanceRequestIds: []string{spotRequestID},
	}
	describeResult, err := r.Ec2.client.DescribeSpotInstanceRequests(ctx, describeInput)
	if err != nil {
		return nil, fmt.Errorf("failed to describe spot instance request: %v", err)
	}

	if len(describeResult.SpotInstanceRequests) > 0 &&
		describeResult.SpotInstanceRequests[0].InstanceId != nil {
		return describeResult.SpotInstanceRequests[0].InstanceId, nil
	}
	return nil, fmt.Errorf("no instance ID found for spot instance")
}

func (r *AWSClient) WaitForState(ctx context.Context, instanceID string) error {
	// wait for the runner to be ready and in the requested state
	waiter := ec2.NewInstanceStatusOkWaiter(r.Ec2.client)
	err := waiter.Wait(ctx, &ec2.DescribeInstanceStatusInput{
		InstanceIds: []string{instanceID},
	}, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("error waiting for instance %v", err)
	}
	return nil
}

func (r *AWSClient) LaunchOnDemandInstance(ctx context.Context, label, registrationToken string, orgRunner bool) (*string, error) {
	runInput := &ec2.RunInstancesInput{
		ImageId:      aws.String(r.Ec2.config.EC2ImageID),
		InstanceType: types.InstanceType(r.Ec2.config.EC2InstanceType),
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
		SubnetId:     aws.String(r.Ec2.config.SubnetID),
		SecurityGroupIds: []string{
			r.Ec2.config.SecurityGroupID,
		},
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeInstance,
				Tags:         r.Ec2.config.AWSResourceTags,
			},
		},
		UserData: aws.String(r.prepareUserData(label, registrationToken, orgRunner)),
	}

	// Add IAM instance role if specified
	if r.Ec2.config.IamInstanceRole != "" {
		runInput.IamInstanceProfile = &types.IamInstanceProfileSpecification{
			Name: aws.String(r.Ec2.config.IamInstanceRole),
		}
	}

	result, err := r.Ec2.client.RunInstances(ctx, runInput)
	if err != nil {
		return nil, fmt.Errorf("failed to launch EC2 instance: %v", err)
	}

	if len(result.Instances) == 0 {
		return nil, fmt.Errorf("no instance ID found for on-demand instance")
	}

	instanceID := result.Instances[0].InstanceId

	// Wait for the instance to be in running state
	waiter := ec2.NewInstanceRunningWaiter(r.Ec2.client)
	err = waiter.Wait(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{*instanceID},
	}, 6*time.Minute)

	if err != nil {
		return nil, fmt.Errorf("error waiting for instance to be running: %v", err)
	}

	return instanceID, nil
}

func (r *AWSClient) TerminateInstance(ctx context.Context, instanceID string) error {
	// Terminate instance
	input := &ec2.TerminateInstancesInput{
		InstanceIds: []string{instanceID},
	}
	_, err := r.Ec2.client.TerminateInstances(ctx, input)

	return err
}
