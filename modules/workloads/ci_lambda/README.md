# Golang lambda to listen for ECR and ECS events

This lambda listen for ECR events and for every new repo push it redeploys the service in dev cluster.

For pod deployment, it listens for event `gihub.action.production` only, which should be triggered outside of the system by github action.

## Build

The service is here in prebuild state as a binary for target platform, ready to be used by terraform. If you want to rebuild it, use the following command:

```bash
GOOS=linux GOARCH=amd64 go build -o main 
```


## Requirements

The lambda have to have IAM role to be able to do a outgoing HTTP request and two env variables should be set:

`PROJECT_NAME` - managed by terraform
`SLACK_WEBHOOK_URL` - slack webhook if you want to receive build messages