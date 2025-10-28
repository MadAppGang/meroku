package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	apigwtypes "github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
	"gopkg.in/yaml.v2"
)

type APIGatewayInfo struct {
	DefaultEndpoint     string `json:"defaultEndpoint"`
	APIGatewayID        string `json:"apiGatewayId"`
	CustomDomainEnabled bool   `json:"customDomainEnabled"`
	CustomDomain        string `json:"customDomain,omitempty"`
	Region              string `json:"region"`
	Error               string `json:"error,omitempty"`
}

func getAPIGatewayInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	envName := r.URL.Query().Get("env")
	if envName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env parameter is required"})
		return
	}

	// Load the environment config
	filename := fmt.Sprintf("%s.yaml", envName)
	content, err := os.ReadFile(filename)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "environment not found"})
		return
	}

	var envConfig Env
	if err := yaml.Unmarshal(content, &envConfig); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to parse environment config"})
		return
	}

	// Build the response
	info := APIGatewayInfo{
		CustomDomainEnabled: envConfig.Domain.Enabled,
	}

	// Get custom domain from config if enabled
	if envConfig.Domain.Enabled {
		// Build custom domain based on configuration
		prefix := envConfig.Domain.APIDomainPrefix
		if prefix == "" {
			prefix = "api"
		}
		info.CustomDomain = fmt.Sprintf("%s.%s", prefix, envConfig.Domain.DomainName)
	}

	// Get AWS config
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(envConfig.AWSProfile),
	)
	if err != nil {
		info.Error = fmt.Sprintf("Failed to load AWS config: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
		return
	}

	info.Region = cfg.Region

	// Create API Gateway v2 client
	client := apigatewayv2.NewFromConfig(cfg)

	// Construct API name based on project and environment
	apiName := fmt.Sprintf("%s-%s", envConfig.Project, envConfig.Env)

	// List APIs to find the one matching our naming convention
	listOutput, err := client.GetApis(ctx, &apigatewayv2.GetApisInput{})
	if err != nil {
		info.Error = fmt.Sprintf("Failed to list APIs: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
		return
	}

	// Find the API with matching name
	var foundAPI *apigwtypes.Api
	for _, api := range listOutput.Items {
		if aws.ToString(api.Name) == apiName {
			foundAPI = &api
			break
		}
	}

	if foundAPI == nil {
		info.Error = fmt.Sprintf("API Gateway '%s' not found. Run terraform apply first.", apiName)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
		return
	}

	info.APIGatewayID = aws.ToString(foundAPI.ApiId)

	// Get the stage to construct the full endpoint
	stageName := envConfig.Env
	stageOutput, err := client.GetStage(ctx, &apigatewayv2.GetStageInput{
		ApiId:     foundAPI.ApiId,
		StageName: aws.String(stageName),
	})

	if err != nil {
		info.Error = fmt.Sprintf("Failed to get stage: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
		return
	}

	// Construct the default endpoint URL
	info.DefaultEndpoint = fmt.Sprintf("%s/%s",
		aws.ToString(foundAPI.ApiEndpoint),
		aws.ToString(stageOutput.StageName),
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}
