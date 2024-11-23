## GitHub Action for AWS GitHub Runner

This repository contains a GitHub Action for managing AWS GitHub Runners.

### Usage

To use this action, create a workflow YAML file in your GitHub repository's `.github/workflows` directory. Here is an example:

```yaml
name: AWS GitHub Runner

on:
  push:
    branches:
      - main

jobs:
  start-runner:
    runs-on: ubuntu-latest
    outputs:
      label: ${{ steps.start-ec2-runner.outputs.label }}
      ec2-instance-id: ${{ steps.start-ec2-runner.outputs.ec2-instance-id }}
    steps:
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: ${{ secrets.AWS_REGION }}

      - name: Setup EC2 GitHub Runner
        uses: aws-github-runner@v1
        with:
          mode: start
          github-token: ${{ secrets.GITHUB_TOKEN }}
          ec2-image-id: ami-123
          ec2-instance-type: t3.nano
          subnet-id: subnet-123
          security-group-id: sg-123
          iam-role-name: my-role-name # optional, requires additional permissions
          aws-resource-tags: > # optional, requires additional permissions
            [
              {"Key": "Name", "Value": "ec2-github-runner"},
              {"Key": "GitHubRepository", "Value": "${{ github.repository }}"}
            ]

  # Job that runs on the self-hosted runner 
  run-build:
    needs:
      - start-runner
    runs-on: ${{ needs.start-runner.outputs.label }}
    steps:              
      - run: env

  stop-runner:
    name: Stop self-hosted EC2 runner
    needs:
      - start-runner # required to get output from the start-runner job
      - run-build # required to wait when the main job is done
    runs-on: ubuntu-latest
    if: ${{ always() }} # required to stop the runner even if the error happened in the previous jobs
    steps:
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: ${{ secrets.AWS_REGION }}

      - name: Stop EC2 runner
        uses: aws-github-runner@v1
        with:
          mode: stop
          github-token: ${{ secrets.GH_PERSONAL_ACCESS_TOKEN }}
          label: ${{ needs.start-runner.outputs.label }}
          ec2-instance-id: ${{ needs.start-runner.outputs.ec2-instance-id }}
```

### Inputs
| Name                  | Description                                                                 | Required | Default     |
|-----------------------|-----------------------------------------------------------------------------|----------|-------------|
| `mode`                | The mode of the action: `start` or `stop`.                                   | Yes      |             |
| `github-token`        | GitHub token for authentication.                                             | Yes      |             |
| `ec2-image-id`        | The ID of the EC2 AMI to use for the runner.                                 | Yes      |             |
| `ec2-instance-type`   | The type of EC2 instance to launch.                                          | Yes      |             |
| `subnet-id`           | The ID of the subnet in which to launch the instance.                        | Yes      |             |
| `security-group-id`   | The ID of the security group to associate with the instance.                 | Yes      |             |
| `iam-role-name`       | The name of the IAM role to attach to the instance.                          | No       |             |
| `aws-resource-tags`   | JSON-formatted list of tags to apply to the instance.                        | No       |             |
| `label`               | The label to assign to the self-hosted runner.                               | No       | `ec2-runner`|
| `ec2-instance-id`     | The ID of the EC2 instance to stop (required for `stop` mode).               | No       |             |


### License

This project is licensed under the MIT License.