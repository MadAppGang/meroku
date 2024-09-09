package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
		_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(env.StateBucket),
		})
		if err != nil {
			return fmt.Errorf("failed to create bucket %s: %v", env.StateBucket, err)
		}
		fmt.Printf("Bucket %s created successfully\n", env.StateBucket)
	}
	return nil
}
