package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"gopkg.in/yaml.v2"
)

type ECSTaskInfo struct {
	TaskArn           string    `json:"taskArn"`
	TaskDefinitionArn string    `json:"taskDefinitionArn"`
	ServiceName       string    `json:"serviceName"`
	LaunchType        string    `json:"launchType"`
	LastStatus        string    `json:"lastStatus"`
	DesiredStatus     string    `json:"desiredStatus"`
	HealthStatus      string    `json:"healthStatus"`
	CreatedAt         time.Time `json:"createdAt"`
	StartedAt         *time.Time `json:"startedAt,omitempty"`
	StoppedAt         *time.Time `json:"stoppedAt,omitempty"`
	CPU               string    `json:"cpu"`
	Memory            string    `json:"memory"`
	AvailabilityZone  string    `json:"availabilityZone"`
	ConnectivityAt    *time.Time `json:"connectivityAt,omitempty"`
	PullStartedAt     *time.Time `json:"pullStartedAt,omitempty"`
	PullStoppedAt     *time.Time `json:"pullStoppedAt,omitempty"`
}

type ServiceTasksResponse struct {
	ServiceName string        `json:"serviceName"`
	Tasks       []ECSTaskInfo `json:"tasks"`
}

// Get tasks for a specific service
func getServiceTasks(w http.ResponseWriter, r *http.Request) {
	envName := r.URL.Query().Get("env")
	serviceName := r.URL.Query().Get("service")

	if envName == "" || serviceName == "" {
		http.Error(w, "Missing env or service parameter", http.StatusBadRequest)
		return
	}

	// Load AWS config
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(selectedAWSProfile),
	)
	if err != nil {
		http.Error(w, "Failed to load AWS config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	ecsClient := ecs.NewFromConfig(cfg)

	// Load environment config to get project name
	filename := fmt.Sprintf("%s.yaml", envName)
	content, err := os.ReadFile(filename)
	if err != nil {
		http.Error(w, "Environment not found: "+err.Error(), http.StatusNotFound)
		return
	}

	var envConfig Env
	if err := yaml.Unmarshal(content, &envConfig); err != nil {
		http.Error(w, "Failed to parse environment config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Get cluster name from config (must match format used in other APIs)
	clusterName := fmt.Sprintf("%s_cluster_%s", envConfig.Project, envConfig.Env)

	// Map frontend service name to actual ECS service name
	var actualServiceName string
	if serviceName == "backend" {
		// Backend service follows pattern: {project}_service_{env}
		actualServiceName = fmt.Sprintf("%s_service_%s", envConfig.Project, envConfig.Env)
	} else {
		// Other services follow pattern: {project}_service_{serviceName}_{env}
		actualServiceName = fmt.Sprintf("%s_service_%s_%s", envConfig.Project, serviceName, envConfig.Env)
	}

	// Get service ARN first
	listServicesInput := &ecs.ListServicesInput{
		Cluster: &clusterName,
	}

	listServicesOutput, err := ecsClient.ListServices(ctx, listServicesInput)
	if err != nil {
		http.Error(w, "Failed to list services: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Find the service we're looking for
	var targetServiceArn string
	if len(listServicesOutput.ServiceArns) > 0 {
		// Get service details to match by name
		describeServicesInput := &ecs.DescribeServicesInput{
			Cluster:  &clusterName,
			Services: listServicesOutput.ServiceArns,
		}

		describeServicesOutput, err := ecsClient.DescribeServices(ctx, describeServicesInput)
		if err != nil {
			http.Error(w, "Failed to describe services: "+err.Error(), http.StatusInternalServerError)
			return
		}

		for _, service := range describeServicesOutput.Services {
			if service.ServiceName != nil && *service.ServiceName == actualServiceName {
				targetServiceArn = *service.ServiceArn
				break
			}
		}
	}

	if targetServiceArn == "" {
		http.Error(w, fmt.Sprintf("Service not found: %s (looking for ECS service: %s)", serviceName, actualServiceName), http.StatusNotFound)
		return
	}

	// List tasks for the service
	listTasksInput := &ecs.ListTasksInput{
		Cluster:     &clusterName,
		ServiceName: &targetServiceArn,
		DesiredStatus: types.DesiredStatusRunning,
	}

	listTasksOutput, err := ecsClient.ListTasks(ctx, listTasksInput)
	if err != nil {
		http.Error(w, "Failed to list tasks: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(listTasksOutput.TaskArns) == 0 {
		// Return empty response if no tasks
		response := ServiceTasksResponse{
			ServiceName: serviceName,
			Tasks:       []ECSTaskInfo{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// Describe tasks to get detailed information
	describeTasksInput := &ecs.DescribeTasksInput{
		Cluster: &clusterName,
		Tasks:   listTasksOutput.TaskArns,
		Include: []types.TaskField{
			types.TaskFieldTags,
		},
	}

	describeTasksOutput, err := ecsClient.DescribeTasks(ctx, describeTasksInput)
	if err != nil {
		http.Error(w, "Failed to describe tasks: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert to our response format
	var tasks []ECSTaskInfo
	for _, task := range describeTasksOutput.Tasks {
		taskInfo := ECSTaskInfo{
			TaskArn:           *task.TaskArn,
			TaskDefinitionArn: *task.TaskDefinitionArn,
			ServiceName:       serviceName,
			LaunchType:        string(task.LaunchType),
			LastStatus:        *task.LastStatus,
			DesiredStatus:     *task.DesiredStatus,
			CreatedAt:         *task.CreatedAt,
		}

		if task.HealthStatus != "" {
			taskInfo.HealthStatus = string(task.HealthStatus)
		}

		if task.StartedAt != nil {
			taskInfo.StartedAt = task.StartedAt
		}

		if task.StoppedAt != nil {
			taskInfo.StoppedAt = task.StoppedAt
		}

		if task.Cpu != nil {
			taskInfo.CPU = *task.Cpu
		}

		if task.Memory != nil {
			taskInfo.Memory = *task.Memory
		}

		if task.AvailabilityZone != nil {
			taskInfo.AvailabilityZone = *task.AvailabilityZone
		}

		if task.ConnectivityAt != nil {
			taskInfo.ConnectivityAt = task.ConnectivityAt
		}

		if task.PullStartedAt != nil {
			taskInfo.PullStartedAt = task.PullStartedAt
		}

		if task.PullStoppedAt != nil {
			taskInfo.PullStoppedAt = task.PullStoppedAt
		}

		tasks = append(tasks, taskInfo)
	}

	response := ServiceTasksResponse{
		ServiceName: serviceName,
		Tasks:       tasks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}