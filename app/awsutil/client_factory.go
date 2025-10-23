package awsutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/amplify"
	"github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// ClientFactory creates and caches AWS service clients
// Thread-safe implementation with lazy initialization
type ClientFactory struct {
	cfg aws.Config
	mu  sync.RWMutex

	// Cached clients - initialized on first use
	ecsClient                 *ecs.Client
	rdsClient                 *rds.Client
	ec2Client                 *ec2.Client
	s3Client                  *s3.Client
	ssmClient                 *ssm.Client
	stsClient                 *sts.Client
	route53Client             *route53.Client
	ecrClient                 *ecr.Client
	lambdaClient              *lambda.Client
	sesClient                 *ses.Client
	eventbridgeClient         *eventbridge.Client
	amplifyClient             *amplify.Client
	autoscalingClient         *applicationautoscaling.Client
	serviceDiscoveryClient    *servicediscovery.Client
}

// NewClientFactory creates a new AWS client factory with the specified profile and region
func NewClientFactory(ctx context.Context, profile, region string) (*ClientFactory, error) {
	var cfg aws.Config
	var err error

	// Load AWS config with profile and region
	if profile != "" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithSharedConfigProfile(profile),
		)
	} else {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
		)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &ClientFactory{cfg: cfg}, nil
}

// ECS returns an ECS client (creates and caches if not already initialized)
func (f *ClientFactory) ECS() *ecs.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ecsClient == nil {
		f.ecsClient = ecs.NewFromConfig(f.cfg)
	}
	return f.ecsClient
}

// RDS returns an RDS client
func (f *ClientFactory) RDS() *rds.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.rdsClient == nil {
		f.rdsClient = rds.NewFromConfig(f.cfg)
	}
	return f.rdsClient
}

// EC2 returns an EC2 client
func (f *ClientFactory) EC2() *ec2.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ec2Client == nil {
		f.ec2Client = ec2.NewFromConfig(f.cfg)
	}
	return f.ec2Client
}

// S3 returns an S3 client
func (f *ClientFactory) S3() *s3.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.s3Client == nil {
		f.s3Client = s3.NewFromConfig(f.cfg)
	}
	return f.s3Client
}

// SSM returns an SSM Parameter Store client
func (f *ClientFactory) SSM() *ssm.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ssmClient == nil {
		f.ssmClient = ssm.NewFromConfig(f.cfg)
	}
	return f.ssmClient
}

// STS returns an STS client for identity and credentials
func (f *ClientFactory) STS() *sts.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.stsClient == nil {
		f.stsClient = sts.NewFromConfig(f.cfg)
	}
	return f.stsClient
}

// Route53 returns a Route53 DNS client
func (f *ClientFactory) Route53() *route53.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.route53Client == nil {
		f.route53Client = route53.NewFromConfig(f.cfg)
	}
	return f.route53Client
}

// ECR returns an ECR container registry client
func (f *ClientFactory) ECR() *ecr.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ecrClient == nil {
		f.ecrClient = ecr.NewFromConfig(f.cfg)
	}
	return f.ecrClient
}

// Lambda returns a Lambda function client
func (f *ClientFactory) Lambda() *lambda.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.lambdaClient == nil {
		f.lambdaClient = lambda.NewFromConfig(f.cfg)
	}
	return f.lambdaClient
}

// SES returns an SES email service client
func (f *ClientFactory) SES() *ses.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.sesClient == nil {
		f.sesClient = ses.NewFromConfig(f.cfg)
	}
	return f.sesClient
}

// EventBridge returns an EventBridge client
func (f *ClientFactory) EventBridge() *eventbridge.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.eventbridgeClient == nil {
		f.eventbridgeClient = eventbridge.NewFromConfig(f.cfg)
	}
	return f.eventbridgeClient
}

// Amplify returns an Amplify client
func (f *ClientFactory) Amplify() *amplify.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.amplifyClient == nil {
		f.amplifyClient = amplify.NewFromConfig(f.cfg)
	}
	return f.amplifyClient
}

// AutoScaling returns an Application Auto Scaling client
func (f *ClientFactory) AutoScaling() *applicationautoscaling.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.autoscalingClient == nil {
		f.autoscalingClient = applicationautoscaling.NewFromConfig(f.cfg)
	}
	return f.autoscalingClient
}

// ServiceDiscovery returns a Service Discovery client
func (f *ClientFactory) ServiceDiscovery() *servicediscovery.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.serviceDiscoveryClient == nil {
		f.serviceDiscoveryClient = servicediscovery.NewFromConfig(f.cfg)
	}
	return f.serviceDiscoveryClient
}

// Config returns the underlying AWS configuration
func (f *ClientFactory) Config() aws.Config {
	return f.cfg
}

// Region returns the configured AWS region
func (f *ClientFactory) Region() string {
	return f.cfg.Region
}
