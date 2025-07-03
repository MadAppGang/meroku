package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"gopkg.in/yaml.v2"
)

type Environment struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
}

type ConfigResponse struct {
	Content string `json:"content"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next(w, r)
	}
}

func getEnvironments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var environments []Environment
	
	// Read only files in the current directory (not subdirectories)
	files, err := os.ReadDir(".")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}
	
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".yaml") {
			name := strings.TrimSuffix(file.Name(), ".yaml")
			environments = append(environments, Environment{
				Name: name,
				Path: file.Name(),
			})
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(environments)
}

func getEnvironmentConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	envName := r.URL.Query().Get("name")
	if envName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name parameter is required"})
		return
	}
	
	filename := fmt.Sprintf("%s.yaml", envName)
	
	content, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "environment not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		}
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ConfigResponse{Content: string(content)})
}

func updateEnvironmentConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	envName := r.URL.Query().Get("name")
	if envName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name parameter is required"})
		return
	}
	
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read request body"})
		return
	}
	defer r.Body.Close()
	
	var req struct {
		Content string `json:"content"`
	}
	
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid JSON"})
		return
	}
	
	filename := fmt.Sprintf("%s.yaml", envName)
	
	err = os.WriteFile(filename, []byte(req.Content), 0644)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "configuration updated successfully"})
}

type AccountInfo struct {
	Profile   string `json:"profile"`
	AccountID string `json:"accountId"`
	Region    string `json:"region"`
}

func getCurrentAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Always use the profile selected at startup
	accountInfo := AccountInfo{
		Profile: selectedAWSProfile,
	}

	// Get AWS account ID and region using the selected profile
	if selectedAWSProfile != "" {
		ctx := context.Background()
		cfg, err := config.LoadDefaultConfig(ctx,
			config.WithSharedConfigProfile(selectedAWSProfile),
		)
		if err == nil {
			stsClient := sts.NewFromConfig(cfg)
			identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
			if err == nil && identity.Account != nil {
				accountInfo.AccountID = *identity.Account
			}
			accountInfo.Region = cfg.Region
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(accountInfo)
}

// ECS-related structures
type ECSClusterInfo struct {
	ClusterName      string   `json:"clusterName"`
	ClusterArn       string   `json:"clusterArn"`
	Status           string   `json:"status"`
	RegisteredTasks  int32    `json:"registeredTasks"`
	RunningTasks     int32    `json:"runningTasks"`
	ActiveServices   int32    `json:"activeServices"`
	CapacityProviders []string `json:"capacityProviders"`
	ContainerInsights string   `json:"containerInsights"`
}

type ECSNetworkInfo struct {
	VPC               VPCInfo          `json:"vpc"`
	AvailabilityZones []string         `json:"availabilityZones"`
	Subnets           []SubnetInfo     `json:"subnets"`
	ServiceDiscovery  ServiceDiscovery `json:"serviceDiscovery"`
}

type VPCInfo struct {
	VpcId     string `json:"vpcId"`
	CidrBlock string `json:"cidrBlock"`
	State     string `json:"state"`
}

type SubnetInfo struct {
	SubnetId           string `json:"subnetId"`
	AvailabilityZone   string `json:"availabilityZone"`
	CidrBlock          string `json:"cidrBlock"`
	AvailableIpCount   int64  `json:"availableIpCount"`
	Type               string `json:"type"` // public or private
}

type ServiceDiscovery struct {
	NamespaceId   string `json:"namespaceId"`
	NamespaceName string `json:"namespaceName"`
	ServiceCount  int    `json:"serviceCount"`
}

type ECSServicesInfo struct {
	Services       []ServiceInfo `json:"services"`
	ScheduledTasks []TaskInfo    `json:"scheduledTasks"`
	EventTasks     []TaskInfo    `json:"eventTasks"`
	TotalTasks     int          `json:"totalTasks"`
}

type ServiceInfo struct {
	ServiceName    string `json:"serviceName"`
	Status         string `json:"status"`
	DesiredCount   int32  `json:"desiredCount"`
	RunningCount   int32  `json:"runningCount"`
	PendingCount   int32  `json:"pendingCount"`
	LaunchType     string `json:"launchType"`
	TaskDefinition string `json:"taskDefinition"`
}

type TaskInfo struct {
	TaskName     string `json:"taskName"`
	TaskType     string `json:"taskType"`
	Schedule     string `json:"schedule,omitempty"`
	EventPattern string `json:"eventPattern,omitempty"`
	Enabled      bool   `json:"enabled"`
}

func getAWSProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to get user home directory"})
		return
	}

	configPath := filepath.Join(homeDir, ".aws", "config")
	content, err := os.ReadFile(configPath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to read AWS config file"})
		return
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profiles)
}

func getECSClusterInfo(w http.ResponseWriter, r *http.Request) {
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

	// Load the environment config to get project name
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

	// Construct cluster name based on the pattern
	clusterName := fmt.Sprintf("%s_cluster_%s", envConfig.Project, envConfig.Env)

	// Get AWS config
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	// Get ECS cluster information
	ecsClient := ecs.NewFromConfig(cfg)
	clusterResult, err := ecsClient.DescribeClusters(ctx, &ecs.DescribeClustersInput{
		Clusters: []string{clusterName},
		Include:  []types.ClusterField{types.ClusterFieldStatistics, types.ClusterFieldSettings},
	})

	clusterInfo := ECSClusterInfo{
		ClusterName: clusterName,
		CapacityProviders: []string{"FARGATE", "FARGATE_SPOT"},
	}

	if err == nil && len(clusterResult.Clusters) > 0 {
		cluster := clusterResult.Clusters[0]
		if cluster.ClusterArn != nil {
			clusterInfo.ClusterArn = *cluster.ClusterArn
		}
		if cluster.Status != nil {
			clusterInfo.Status = *cluster.Status
		}
		clusterInfo.RegisteredTasks = cluster.RegisteredContainerInstancesCount
		clusterInfo.RunningTasks = cluster.RunningTasksCount
		clusterInfo.ActiveServices = cluster.ActiveServicesCount
		if cluster.Settings != nil {
			for _, setting := range cluster.Settings {
				if setting.Name == "containerInsights" && setting.Value != nil {
					clusterInfo.ContainerInsights = *setting.Value
				}
			}
		}
		if cluster.CapacityProviders != nil {
			clusterInfo.CapacityProviders = cluster.CapacityProviders
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clusterInfo)
}

func getECSNetworkInfo(w http.ResponseWriter, r *http.Request) {
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

	// Load the environment config to get project name
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

	clusterName := fmt.Sprintf("%s_cluster_%s", envConfig.Project, envConfig.Env)

	// Get AWS config
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	networkInfo := ECSNetworkInfo{
		AvailabilityZones: []string{},
		Subnets:          []SubnetInfo{},
	}

	// First, get the ECS service to find the subnets it's using
	ecsClient := ecs.NewFromConfig(cfg)
	servicesResult, err := ecsClient.ListServices(ctx, &ecs.ListServicesInput{
		Cluster: &clusterName,
	})

	var vpcId string
	var subnetIds []string

	if err == nil && len(servicesResult.ServiceArns) > 0 {
		// Get the first service details to find network configuration
		describeResult, err := ecsClient.DescribeServices(ctx, &ecs.DescribeServicesInput{
			Cluster:  &clusterName,
			Services: []string{servicesResult.ServiceArns[0]},
		})

		if err == nil && len(describeResult.Services) > 0 {
			service := describeResult.Services[0]
			if service.NetworkConfiguration != nil && service.NetworkConfiguration.AwsvpcConfiguration != nil {
				subnetIds = service.NetworkConfiguration.AwsvpcConfiguration.Subnets
			}
		}
	}

	ec2Client := ec2.NewFromConfig(cfg)
	
	// If we found subnets, get their details and VPC info
	if len(subnetIds) > 0 {
		subnetResult, err := ec2Client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
			SubnetIds: subnetIds,
		})

		if err == nil && len(subnetResult.Subnets) > 0 {
			// Get VPC ID from the first subnet
			vpcId = *subnetResult.Subnets[0].VpcId

			// Get VPC details
			vpcResult, err := ec2Client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
				VpcIds: []string{vpcId},
			})

			if err == nil && len(vpcResult.Vpcs) > 0 {
				vpc := vpcResult.Vpcs[0]
				networkInfo.VPC = VPCInfo{
					VpcId:     *vpc.VpcId,
					CidrBlock: *vpc.CidrBlock,
					State:     string(vpc.State),
				}
			}

			// Process subnet information
			azMap := make(map[string]bool)
			for _, subnet := range subnetResult.Subnets {
				subnetType := "private"
				if subnet.MapPublicIpOnLaunch != nil && *subnet.MapPublicIpOnLaunch {
					subnetType = "public"
				}
				
				subnetInfo := SubnetInfo{
					SubnetId:         *subnet.SubnetId,
					AvailabilityZone: *subnet.AvailabilityZone,
					CidrBlock:        *subnet.CidrBlock,
					AvailableIpCount: int64(*subnet.AvailableIpAddressCount),
					Type:             subnetType,
				}
				networkInfo.Subnets = append(networkInfo.Subnets, subnetInfo)
				azMap[*subnet.AvailabilityZone] = true
			}
			
			// Extract unique AZs
			for az := range azMap {
				networkInfo.AvailabilityZones = append(networkInfo.AvailabilityZones, az)
			}
		}
	} else if vpcId == "" {
		// If no services found, try to get the default VPC
		vpcResult, err := ec2Client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
			Filters: []ec2types.Filter{
				{
					Name:   aws.String("isDefault"),
					Values: []string{"true"},
				},
			},
		})

		if err == nil && len(vpcResult.Vpcs) > 0 {
			vpc := vpcResult.Vpcs[0]
			networkInfo.VPC = VPCInfo{
				VpcId:     *vpc.VpcId,
				CidrBlock: *vpc.CidrBlock,
				State:     string(vpc.State),
			}
			vpcId = *vpc.VpcId

			// Get all subnets for this VPC
			subnetResult, err := ec2Client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
				Filters: []ec2types.Filter{
					{
						Name:   aws.String("vpc-id"),
						Values: []string{vpcId},
					},
				},
			})

			if err == nil {
				azMap := make(map[string]bool)
				for _, subnet := range subnetResult.Subnets {
					subnetType := "private"
					if subnet.MapPublicIpOnLaunch != nil && *subnet.MapPublicIpOnLaunch {
						subnetType = "public"
					}
					
					subnetInfo := SubnetInfo{
						SubnetId:         *subnet.SubnetId,
						AvailabilityZone: *subnet.AvailabilityZone,
						CidrBlock:        *subnet.CidrBlock,
						AvailableIpCount: int64(*subnet.AvailableIpAddressCount),
						Type:             subnetType,
					}
					networkInfo.Subnets = append(networkInfo.Subnets, subnetInfo)
					azMap[*subnet.AvailabilityZone] = true
				}
				
				// Extract unique AZs
				for az := range azMap {
					networkInfo.AvailabilityZones = append(networkInfo.AvailabilityZones, az)
				}
			}
		}
	}

	// Get service discovery namespace info
	sdClient := servicediscovery.NewFromConfig(cfg)
	nsResult, err := sdClient.ListNamespaces(ctx, &servicediscovery.ListNamespacesInput{})
	
	if err == nil && len(nsResult.Namespaces) > 0 {
		// Look for the local namespace
		for _, ns := range nsResult.Namespaces {
			if ns.Name != nil && *ns.Name == "local" {
				networkInfo.ServiceDiscovery = ServiceDiscovery{
					NamespaceId:   *ns.Id,
					NamespaceName: *ns.Name,
				}
				
				// Get service count in this namespace
				servicesResult, err := sdClient.ListServices(ctx, &servicediscovery.ListServicesInput{
					Filters: []sdtypes.ServiceFilter{
						{
							Name:   sdtypes.ServiceFilterNameNamespaceId,
							Values: []string{*ns.Id},
						},
					},
				})
				if err == nil {
					networkInfo.ServiceDiscovery.ServiceCount = len(servicesResult.Services)
				}
				break
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(networkInfo)
}

func getECSServicesInfo(w http.ResponseWriter, r *http.Request) {
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

	clusterName := fmt.Sprintf("%s_cluster_%s", envConfig.Project, envConfig.Env)

	// Get AWS config
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	ecsClient := ecs.NewFromConfig(cfg)
	
	// List all services in the cluster
	servicesResult, err := ecsClient.ListServices(ctx, &ecs.ListServicesInput{
		Cluster: &clusterName,
	})

	servicesInfo := ECSServicesInfo{
		Services:       []ServiceInfo{},
		ScheduledTasks: []TaskInfo{},
		EventTasks:     []TaskInfo{},
	}

	if err == nil && len(servicesResult.ServiceArns) > 0 {
		// Describe services to get details
		describeResult, err := ecsClient.DescribeServices(ctx, &ecs.DescribeServicesInput{
			Cluster:  &clusterName,
			Services: servicesResult.ServiceArns,
		})

		if err == nil {
			for _, service := range describeResult.Services {
				serviceInfo := ServiceInfo{
					ServiceName:  *service.ServiceName,
					Status:       *service.Status,
					DesiredCount: service.DesiredCount,
					RunningCount: service.RunningCount,
					PendingCount: service.PendingCount,
					LaunchType:   string(service.LaunchType),
				}
				if service.TaskDefinition != nil {
					// Extract just the task definition name from the ARN
					parts := strings.Split(*service.TaskDefinition, "/")
					if len(parts) > 0 {
						serviceInfo.TaskDefinition = parts[len(parts)-1]
					}
				}
				servicesInfo.Services = append(servicesInfo.Services, serviceInfo)
			}
		}
	}

	// Add scheduled tasks from config
	if envConfig.ScheduledTasks != nil {
		for _, task := range envConfig.ScheduledTasks {
			servicesInfo.ScheduledTasks = append(servicesInfo.ScheduledTasks, TaskInfo{
				TaskName: task.Name,
				TaskType: "scheduled",
				Schedule: task.Schedule,
				Enabled:  true,
			})
		}
	}

	// Add event tasks from config
	if envConfig.EventProcessorTasks != nil {
		for _, task := range envConfig.EventProcessorTasks {
			servicesInfo.EventTasks = append(servicesInfo.EventTasks, TaskInfo{
				TaskName:     task.Name,
				TaskType:     "event",
				EventPattern: task.RuleName,
				Enabled:      true,
			})
		}
	}

	// Calculate total tasks
	servicesInfo.TotalTasks = len(servicesInfo.Services) + len(servicesInfo.ScheduledTasks) + len(servicesInfo.EventTasks)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(servicesInfo)
}



