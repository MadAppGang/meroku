package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/charmbracelet/huh"
)

// Global variable to store the selected AWS profile
var selectedAWSProfile string

func selectAWSProfile() error {
	profiles, err := getLocalAWSProfiles()
	if err != nil {
		return fmt.Errorf("failed to get AWS profiles: %w", err)
	}

	if len(profiles) == 0 {
		return fmt.Errorf("no AWS profiles found")
	}

	var selectedProfile string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select an AWS Profile").
				Options(huh.NewOptions(profiles...)...).
				Value(&selectedProfile),
		),
	)

	err = form.Run()
	if err != nil {
		return fmt.Errorf("error running form: %w", err)
	}

	if selectedProfile != "" {
		// Store the selected profile globally
		selectedAWSProfile = selectedProfile
		
		err = os.Setenv("AWS_PROFILE", selectedProfile)
		if err != nil {
			return fmt.Errorf("failed to set AWS_PROFILE: %w", err)
		}
		// List S3 buckets for the selected profile
		s3Buckets, err := listS3Buckets()
		if err != nil {
			fmt.Printf("Error listing S3 buckets: %v\n", err)
			if strings.Contains(err.Error(), "the SSO session has expired or is invalid") || strings.Contains(err.Error(), "unable to refresh SSO token") {
				fmt.Println("SSO session has expired or is invalid. Attempting to log in...")
				_, err = runCommandWithOutput("aws", "sso", "login")
				if err != nil {
					return fmt.Errorf("failed to run 'aws sso login': %w", err)
				}
				fmt.Println("SSO login successful. Retrying S3 bucket listing...")
				return selectAWSProfile()
			}
			return err
		} else {
			var confirmProfile bool
			form := huh.NewForm(
				huh.NewGroup(
					huh.NewNote().
						Title("S3 buckets in your account").
						Description(formatS3BucketsList(s3Buckets)),
					huh.NewConfirm().
						Title("Is this the correct AWS profile?").
						Value(&confirmProfile),
				),
			)
			err = form.Run()
			if err != nil {
				fmt.Printf("Error running confirmation form: %v\n", err)
			} else if !confirmProfile {
				fmt.Println("Please run the selection again to choose a different profile.")
				return selectAWSProfile()
			}

			return nil
		}
	} else {
		fmt.Println("No profile selected")
	}

	return nil
}

func getLocalAWSProfiles() ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".aws", "config")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read AWS config file: %w", err)
	}

	var profiles []string
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[profile ") && strings.HasSuffix(line, "]") {
			profile := strings.TrimPrefix(line, "[profile ")
			profile = strings.TrimSuffix(profile, "]")
			profiles = append(profiles, profile)
		}
	}

	return profiles, nil
}

func listS3Buckets() ([]string, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	client := s3.NewFromConfig(cfg)
	fmt.Printf("Using AWS Region: %s\n", cfg.Region)

	result, err := client.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to list S3 buckets: %w", err)
	}

	var bucketNames []string
	for _, bucket := range result.Buckets {
		if bucket.Name != nil {
			bucketNames = append(bucketNames, *bucket.Name)
		}
	}

	return bucketNames, nil
}

// Add this function to format the S3 buckets list
func formatS3BucketsList(buckets []string) string {
	return "Your S3 Buckets:\n" + strings.Join(buckets, "\n")
}
