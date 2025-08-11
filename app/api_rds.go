package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/aws"
)

type DatabaseInfo struct {
	Endpoint   string `json:"endpoint"`
	Port       int32  `json:"port"`
	IsAurora   bool   `json:"isAurora"`
	Status     string `json:"status"`
	Engine     string `json:"engine"`
	EngineVersion string `json:"engineVersion,omitempty"`
}

// GET /api/rds/endpoint?project=<project>&env=<env>&aurora=<true|false>
func getDatabaseEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	project := r.URL.Query().Get("project")
	env := r.URL.Query().Get("env")
	isAurora := r.URL.Query().Get("aurora") == "true"

	if project == "" || env == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "project and env parameters are required"})
		return
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(selectedAWSProfile),
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Failed to load AWS config: %v", err)})
		return
	}

	rdsClient := rds.NewFromConfig(cfg)
	
	var dbInfo DatabaseInfo
	
	if isAurora {
		// For Aurora, we need to describe the cluster
		clusterIdentifier := fmt.Sprintf("%s-aurora-%s", project, env)
		
		clusterOutput, err := rdsClient.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{
			DBClusterIdentifier: aws.String(clusterIdentifier),
		})
		
		if err != nil {
			// Try alternate naming convention
			clusterIdentifier = fmt.Sprintf("%s-%s-cluster", project, env)
			clusterOutput, err = rdsClient.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{
				DBClusterIdentifier: aws.String(clusterIdentifier),
			})
			
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Aurora cluster not found: %v", err)})
				return
			}
		}
		
		if len(clusterOutput.DBClusters) > 0 {
			cluster := clusterOutput.DBClusters[0]
			dbInfo = DatabaseInfo{
				Endpoint:   aws.ToString(cluster.Endpoint),
				Port:       aws.ToInt32(cluster.Port),
				IsAurora:   true,
				Status:     aws.ToString(cluster.Status),
				Engine:     aws.ToString(cluster.Engine),
				EngineVersion: aws.ToString(cluster.EngineVersion),
			}
		}
	} else {
		// For standard RDS, describe the instance
		instanceIdentifier := fmt.Sprintf("%s-postgres-%s", project, env)
		
		instanceOutput, err := rdsClient.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
			DBInstanceIdentifier: aws.String(instanceIdentifier),
		})
		
		if err != nil {
			// Try alternate naming convention
			instanceIdentifier = fmt.Sprintf("%s-%s-rds", project, env)
			instanceOutput, err = rdsClient.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
				DBInstanceIdentifier: aws.String(instanceIdentifier),
			})
			
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("RDS instance not found: %v", err)})
				return
			}
		}
		
		if len(instanceOutput.DBInstances) > 0 {
			instance := instanceOutput.DBInstances[0]
			dbInfo = DatabaseInfo{
				Endpoint:   aws.ToString(instance.Endpoint.Address),
				Port:       aws.ToInt32(instance.Endpoint.Port),
				IsAurora:   false,
				Status:     aws.ToString(instance.DBInstanceStatus),
				Engine:     aws.ToString(instance.Engine),
				EngineVersion: aws.ToString(instance.EngineVersion),
			}
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dbInfo)
}

// GET /api/rds/info?project=<project>&env=<env>
// This endpoint tries to detect whether RDS or Aurora is being used
func getDatabaseInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	project := r.URL.Query().Get("project")
	env := r.URL.Query().Get("env")

	if project == "" || env == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "project and env parameters are required"})
		return
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(selectedAWSProfile),
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Failed to load AWS config: %v", err)})
		return
	}

	rdsClient := rds.NewFromConfig(cfg)
	
	// First try Aurora
	clusterIdentifiers := []string{
		fmt.Sprintf("%s-aurora-%s", project, env),
		fmt.Sprintf("%s-%s-cluster", project, env),
	}
	
	for _, clusterID := range clusterIdentifiers {
		clusterOutput, err := rdsClient.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{
			DBClusterIdentifier: aws.String(clusterID),
		})
		
		if err == nil && len(clusterOutput.DBClusters) > 0 {
			cluster := clusterOutput.DBClusters[0]
			dbInfo := DatabaseInfo{
				Endpoint:   aws.ToString(cluster.Endpoint),
				Port:       aws.ToInt32(cluster.Port),
				IsAurora:   true,
				Status:     aws.ToString(cluster.Status),
				Engine:     aws.ToString(cluster.Engine),
				EngineVersion: aws.ToString(cluster.EngineVersion),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(dbInfo)
			return
		}
	}
	
	// If Aurora not found, try standard RDS
	instanceIdentifiers := []string{
		fmt.Sprintf("%s-postgres-%s", project, env),
		fmt.Sprintf("%s-%s-rds", project, env),
	}
	
	for _, instanceID := range instanceIdentifiers {
		instanceOutput, err := rdsClient.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
			DBInstanceIdentifier: aws.String(instanceID),
		})
		
		if err == nil && len(instanceOutput.DBInstances) > 0 {
			instance := instanceOutput.DBInstances[0]
			dbInfo := DatabaseInfo{
				Endpoint:   aws.ToString(instance.Endpoint.Address),
				Port:       aws.ToInt32(instance.Endpoint.Port),
				IsAurora:   false,
				Status:     aws.ToString(instance.DBInstanceStatus),
				Engine:     aws.ToString(instance.Engine),
				EngineVersion: aws.ToString(instance.EngineVersion),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(dbInfo)
			return
		}
	}
	
	// Neither found
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(ErrorResponse{Error: "No RDS instance or Aurora cluster found for this project/environment"})
}