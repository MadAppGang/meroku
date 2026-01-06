package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// checkDynamoDBTableForEnv checks if the DynamoDB state lock table exists and creates it if not
func checkDynamoDBTableForEnv(env Env) error {
	return checkDynamoDBTableForEnvWithRetry(env, false)
}

func checkDynamoDBTableForEnvWithRetry(env Env, isRetry bool) error {
	// Skip if state_lock_table is not configured
	if env.StateLockTable == "" {
		return nil
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(env.Region))
	if err != nil {
		return fmt.Errorf("failed to load AWS configuration: %v", err)
	}
	client := dynamodb.NewFromConfig(cfg)

	// Check if table exists by trying to describe it
	_, err = client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(env.StateLockTable),
	})

	if err != nil {
		// Check for SSO token expiration
		if !isRetry && strings.Contains(err.Error(), "unable to refresh SSO token") {
			fmt.Println("SSO token expired. Attempting to log in...")
			if _, err := runCommandWithOutput("aws", "sso", "login"); err != nil {
				return fmt.Errorf("failed to run 'aws sso login': %v", err)
			}
			return checkDynamoDBTableForEnvWithRetry(env, true)
		}

		// Check if table doesn't exist (ResourceNotFoundException)
		var notFoundErr *types.ResourceNotFoundException
		if errors.As(err, &notFoundErr) {
			// Table doesn't exist, create it
			return createDynamoDBLockTable(ctx, client, env)
		}

		return fmt.Errorf("failed to describe DynamoDB table %s: %v", env.StateLockTable, err)
	}

	fmt.Printf("✅ DynamoDB lock table %s already exists\n", env.StateLockTable)
	return nil
}

// createDynamoDBLockTable creates a new DynamoDB table for Terraform state locking
func createDynamoDBLockTable(ctx context.Context, client *dynamodb.Client, env Env) error {
	fmt.Printf("📝 Creating DynamoDB lock table: %s\n", env.StateLockTable)

	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:   aws.String(env.StateLockTable),
		BillingMode: types.BillingModePayPerRequest, // On-demand pricing
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("LockID"),
				KeyType:       types.KeyTypeHash,
			},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("LockID"),
				AttributeType: types.ScalarAttributeTypeS, // String type
			},
		},
		Tags: []types.Tag{
			{
				Key:   aws.String("Name"),
				Value: aws.String("Terraform State Locks"),
			},
			{
				Key:   aws.String("Project"),
				Value: aws.String(env.Project),
			},
			{
				Key:   aws.String("Environment"),
				Value: aws.String(env.Env),
			},
			{
				Key:   aws.String("ManagedBy"),
				Value: aws.String("meroku"),
			},
			{
				Key:   aws.String("Purpose"),
				Value: aws.String("Terraform state locking"),
			},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to create DynamoDB table %s: %v", env.StateLockTable, err)
	}

	// Wait for table to become active
	fmt.Printf("⏳ Waiting for table to become active...\n")
	waiter := dynamodb.NewTableExistsWaiter(client)
	err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(env.StateLockTable),
	}, 60*time.Second) // Wait up to 60 seconds

	if err != nil {
		return fmt.Errorf("timeout waiting for DynamoDB table %s to become active: %v", env.StateLockTable, err)
	}

	fmt.Printf("✅ DynamoDB lock table %s created successfully in region %s\n", env.StateLockTable, env.Region)
	return nil
}

// GenerateStateLockTableName generates the default state lock table name based on project and env
func GenerateStateLockTableName(project, env string) string {
	return fmt.Sprintf("%s-terraform-locks-%s", project, env)
}
