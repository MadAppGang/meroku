package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func checkBucketStateForEnv(env Env) error {
	return checkBucketStateForEnvWithRetry(env, false)
}

func checkBucketStateForEnvWithRetry(env Env, isRetry bool) error {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(env.Region))
	if err != nil {
		return fmt.Errorf("failed to load AWS configuration: %v", err)
	}
	client := s3.NewFromConfig(cfg)

	result, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		if !isRetry && strings.Contains(err.Error(), "unable to refresh SSO token") {
			fmt.Println("SSO token expired. Attempting to log in...")
			if _, err := runCommandWithOutput("aws", "sso", "login"); err != nil {
				return fmt.Errorf("failed to run 'aws sso login': %v", err)
			}
			return checkBucketStateForEnvWithRetry(env, true)
		}
		return fmt.Errorf("failed to list buckets: %v", err)
	}

	// Check if the bucket exists
	bucketExists := false
	for _, b := range result.Buckets {
		if aws.ToString(b.Name) == env.StateBucket {
			bucketExists = true
			break
		}
	}

	// If bucket doesn't exist, create it
	if !bucketExists {
		createBucketInput := &s3.CreateBucketInput{
			Bucket: aws.String(env.StateBucket),
		}

		// For regions other than us-east-1, we need to specify the LocationConstraint
		if env.Region != "us-east-1" {
			createBucketInput.CreateBucketConfiguration = &s3types.CreateBucketConfiguration{
				LocationConstraint: s3types.BucketLocationConstraint(env.Region),
			}
		}

		_, err = client.CreateBucket(ctx, createBucketInput)
		if err != nil {
			return fmt.Errorf("failed to create bucket %s in region %s: %v", env.StateBucket, env.Region, err)
		}
		fmt.Printf("✅ Bucket %s created successfully in region %s\n", env.StateBucket, env.Region)
	} else {
		fmt.Printf("✅ Bucket %s already exists\n", env.StateBucket)
	}
	return nil
}
