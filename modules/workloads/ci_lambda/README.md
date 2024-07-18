# Golang lambda to listen for ECR and ECS events

This lambda listen for ECR events and for every new repo push it redeploys the service in dev cluster.

For pod deployment, it listens for event `action.production` only, which should be triggered outside of the system. See notes below.

## Build

The service is here in prebuild state as a binary for target platform, ready to be used by terraform. If you want to rebuild it, use the following command:

```bash
GOOS=linux GOARCH=amd64 go build -o bootstrap 
```


## Requirements

The lambda have to have IAM role to be able to do a outgoing HTTP request and two env variables should be set:

`PROJECT_NAME` - managed by terraform
`SLACK_WEBHOOK_URL` - slack webhook if you want to receive build messages
`PROJECT_ENV` - environment where the project is deployed, managed by terraform


## Deploy to Production

Dev deployments are automatic, every time the new ECR is published to repository. 

Production deployment is done explicitly only. You need to send event to prod EventBus.

`action.production` - should be a source.
And

You can send the event with aws cli:

```bash
aws events put-events --entries 'Source=action.production,DetailType=DEPLOY,Detail="{\"service\":\"backend\"}",EventBusName=default'
```

Where `backend` is a service name.

