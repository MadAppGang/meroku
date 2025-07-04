package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/aws-sdk-go-v2/aws"
)

type SSMParameter struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	ARN         string `json:"arn,omitempty"`
	Version     int64  `json:"version,omitempty"`
}

type SSMParameterRequest struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Overwrite   bool   `json:"overwrite,omitempty"`
}

// GET /api/ssm/parameter?name=<parameter-name>
func getSSMParameter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	paramName := r.URL.Query().Get("name")
	if paramName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name parameter is required"})
		return
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(selectedAWSProfile),
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	ssmClient := ssm.NewFromConfig(cfg)
	
	// Get parameter with decryption
	result, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(paramName),
		WithDecryption: aws.Bool(true),
	})
	
	if err != nil {
		if strings.Contains(err.Error(), "ParameterNotFound") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "parameter not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		}
		return
	}

	param := SSMParameter{
		Name:    *result.Parameter.Name,
		Value:   *result.Parameter.Value,
		Type:    string(result.Parameter.Type),
		Version: result.Parameter.Version,
	}
	
	if result.Parameter.ARN != nil {
		param.ARN = *result.Parameter.ARN
	}
	
	// Get parameter metadata for description
	descResult, err := ssmClient.DescribeParameters(ctx, &ssm.DescribeParametersInput{
		ParameterFilters: []types.ParameterStringFilter{
			{
				Key:    aws.String("Name"),
				Values: []string{paramName},
			},
		},
	})
	
	if err == nil && len(descResult.Parameters) > 0 && descResult.Parameters[0].Description != nil {
		param.Description = *descResult.Parameters[0].Description
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(param)
}

// PUT /api/ssm/parameter
func putSSMParameter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SSMParameterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid request body"})
		return
	}

	if req.Name == "" || req.Value == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name and value are required"})
		return
	}

	// Default to String type if not specified
	if req.Type == "" {
		req.Type = "String"
	}

	// Validate parameter type
	validTypes := map[string]bool{
		"String":       true,
		"StringList":   true,
		"SecureString": true,
	}
	if !validTypes[req.Type] {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid parameter type. Must be String, StringList, or SecureString"})
		return
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(selectedAWSProfile),
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	ssmClient := ssm.NewFromConfig(cfg)
	
	putInput := &ssm.PutParameterInput{
		Name:      aws.String(req.Name),
		Value:     aws.String(req.Value),
		Type:      types.ParameterType(req.Type),
		Overwrite: aws.Bool(req.Overwrite),
	}
	
	if req.Description != "" {
		putInput.Description = aws.String(req.Description)
	}

	result, err := ssmClient.PutParameter(ctx, putInput)
	if err != nil {
		if strings.Contains(err.Error(), "ParameterAlreadyExists") {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "parameter already exists. Set overwrite=true to update"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		}
		return
	}

	response := map[string]interface{}{
		"message": "parameter created/updated successfully",
		"version": result.Version,
		"tier":    string(result.Tier),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// DELETE /api/ssm/parameter?name=<parameter-name>
func deleteSSMParameter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	paramName := r.URL.Query().Get("name")
	if paramName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name parameter is required"})
		return
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(selectedAWSProfile),
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	ssmClient := ssm.NewFromConfig(cfg)
	
	_, err = ssmClient.DeleteParameter(ctx, &ssm.DeleteParameterInput{
		Name: aws.String(paramName),
	})
	
	if err != nil {
		if strings.Contains(err.Error(), "ParameterNotFound") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "parameter not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "parameter deleted successfully"})
}

// GET /api/ssm/parameters?prefix=<prefix>
func listSSMParameters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	prefix := r.URL.Query().Get("prefix")

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(selectedAWSProfile),
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	ssmClient := ssm.NewFromConfig(cfg)
	
	var filters []types.ParameterStringFilter
	if prefix != "" {
		filters = append(filters, types.ParameterStringFilter{
			Key:    aws.String("Name"),
			Option: aws.String("BeginsWith"),
			Values: []string{prefix},
		})
	}

	var parameters []SSMParameter
	var nextToken *string
	
	for {
		input := &ssm.DescribeParametersInput{
			MaxResults: aws.Int32(50),
		}
		if len(filters) > 0 {
			input.ParameterFilters = filters
		}
		if nextToken != nil {
			input.NextToken = nextToken
		}

		result, err := ssmClient.DescribeParameters(ctx, input)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
			return
		}

		for _, param := range result.Parameters {
			p := SSMParameter{
				Name:    *param.Name,
				Type:    string(param.Type),
				Version: param.Version,
			}
			if param.Description != nil {
				p.Description = *param.Description
			}
			parameters = append(parameters, p)
		}

		if result.NextToken == nil {
			break
		}
		nextToken = result.NextToken
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(parameters)
}