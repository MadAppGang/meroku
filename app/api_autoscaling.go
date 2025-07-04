package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	"github.com/aws/aws-sdk-go-v2/service/applicationautoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"gopkg.in/yaml.v2"
)

// ServiceAutoscalingInfo represents autoscaling configuration and status
type ServiceAutoscalingInfo struct {
	ServiceName         string  `json:"serviceName"`
	Enabled             bool    `json:"enabled"`
	CurrentDesiredCount int32   `json:"currentDesiredCount"`
	MinCapacity         int32   `json:"minCapacity"`
	MaxCapacity         int32   `json:"maxCapacity"`
	TargetCPU           float64 `json:"targetCPU"`
	TargetMemory        float64 `json:"targetMemory"`
	CPU                 int32   `json:"cpu"`
	Memory              int32   `json:"memory"`
	// Current metrics
	CurrentCPUUtilization    *float64              `json:"currentCPUUtilization,omitempty"`
	CurrentMemoryUtilization *float64              `json:"currentMemoryUtilization,omitempty"`
	LastScalingActivity      *ScalingActivityInfo  `json:"lastScalingActivity,omitempty"`
}

type ScalingActivityInfo struct {
	Time        string `json:"time"`
	Description string `json:"description"`
	Cause       string `json:"cause"`
}

type ServiceScalingHistory struct {
	ServiceName string         `json:"serviceName"`
	Events      []ScalingEvent `json:"events"`
}

type ScalingEvent struct {
	Timestamp    string `json:"timestamp"`
	ActivityType string `json:"activityType"`
	FromCapacity int32  `json:"fromCapacity"`
	ToCapacity   int32  `json:"toCapacity"`
	Reason       string `json:"reason"`
	StatusCode   string `json:"statusCode"`
}

type ServiceMetrics struct {
	ServiceName string        `json:"serviceName"`
	Metrics     MetricsData   `json:"metrics"`
}

type MetricsData struct {
	CPU          []MetricPoint `json:"cpu"`
	Memory       []MetricPoint `json:"memory"`
	TaskCount    []MetricPoint `json:"taskCount"`
	RequestCount []MetricPoint `json:"requestCount,omitempty"`
}

type MetricPoint struct {
	Timestamp string  `json:"timestamp"`
	Value     float64 `json:"value"`
}

func getServiceAutoscaling(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	envName := r.URL.Query().Get("env")
	serviceName := r.URL.Query().Get("service")
	
	if envName == "" || serviceName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env and service parameters are required"})
		return
	}

	// Load environment config
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

	// Construct the full service name
	fullServiceName := getFullServiceName(envConfig.Project, serviceName, envConfig.Env)
	clusterName := fmt.Sprintf("%s_cluster_%s", envConfig.Project, envConfig.Env)

	// Get AWS config
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	// Get ECS service info
	ecsClient := ecs.NewFromConfig(cfg)
	serviceResult, err := ecsClient.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  &clusterName,
		Services: []string{fullServiceName},
	})

	autoscalingInfo := ServiceAutoscalingInfo{
		ServiceName: serviceName,
		Enabled:     false,
	}

	if err == nil && len(serviceResult.Services) > 0 {
		service := serviceResult.Services[0]
		autoscalingInfo.CurrentDesiredCount = service.DesiredCount

		// Get task definition to find CPU/Memory
		if service.TaskDefinition != nil {
			taskDefResult, err := ecsClient.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
				TaskDefinition: service.TaskDefinition,
			})
			if err == nil && taskDefResult.TaskDefinition != nil {
				cpu, _ := parseResourceValue(*taskDefResult.TaskDefinition.Cpu)
				memory, _ := parseResourceValue(*taskDefResult.TaskDefinition.Memory)
				autoscalingInfo.CPU = int32(cpu)
				autoscalingInfo.Memory = int32(memory)
			}
		}
	}

	// Get autoscaling configuration
	autoscalingClient := applicationautoscaling.NewFromConfig(cfg)
	resourceId := fmt.Sprintf("service/%s/%s", clusterName, fullServiceName)
	
	// Check if autoscaling target exists
	targetsResult, err := autoscalingClient.DescribeScalableTargets(ctx, &applicationautoscaling.DescribeScalableTargetsInput{
		ServiceNamespace: types.ServiceNamespaceEcs,
		ResourceIds:      []string{resourceId},
	})

	if err == nil && len(targetsResult.ScalableTargets) > 0 {
		target := targetsResult.ScalableTargets[0]
		autoscalingInfo.Enabled = true
		autoscalingInfo.MinCapacity = *target.MinCapacity
		autoscalingInfo.MaxCapacity = *target.MaxCapacity

		// Get scaling policies
		policiesResult, err := autoscalingClient.DescribeScalingPolicies(ctx, &applicationautoscaling.DescribeScalingPoliciesInput{
			ServiceNamespace: types.ServiceNamespaceEcs,
			ResourceId:       &resourceId,
		})

		if err == nil {
			for _, policy := range policiesResult.ScalingPolicies {
				if policy.TargetTrackingScalingPolicyConfiguration != nil {
					if policy.PolicyName != nil && strings.Contains(*policy.PolicyName, "cpu") {
						autoscalingInfo.TargetCPU = *policy.TargetTrackingScalingPolicyConfiguration.TargetValue
					} else if policy.PolicyName != nil && strings.Contains(*policy.PolicyName, "memory") {
						autoscalingInfo.TargetMemory = *policy.TargetTrackingScalingPolicyConfiguration.TargetValue
					}
				}
			}
		}

		// Get last scaling activity
		activitiesResult, err := autoscalingClient.DescribeScalingActivities(ctx, &applicationautoscaling.DescribeScalingActivitiesInput{
			ServiceNamespace: types.ServiceNamespaceEcs,
			ResourceId:       &resourceId,
			MaxResults:       aws.Int32(1),
		})

		if err == nil && len(activitiesResult.ScalingActivities) > 0 {
			activity := activitiesResult.ScalingActivities[0]
			autoscalingInfo.LastScalingActivity = &ScalingActivityInfo{
				Time:        activity.StartTime.Format(time.RFC3339),
				Description: *activity.Description,
				Cause:       *activity.Cause,
			}
		}
	}

	// Get current metrics
	cloudwatchClient := cloudwatch.NewFromConfig(cfg)
	now := time.Now()
	fiveMinutesAgo := now.Add(-5 * time.Minute)

	// Get CPU utilization
	cpuResult, err := cloudwatchClient.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/ECS"),
		MetricName: aws.String("CPUUtilization"),
		Dimensions: []cloudwatchtypes.Dimension{
			{Name: aws.String("ServiceName"), Value: &fullServiceName},
			{Name: aws.String("ClusterName"), Value: &clusterName},
		},
		StartTime:  &fiveMinutesAgo,
		EndTime:    &now,
		Period:     aws.Int32(300),
		Statistics: []cloudwatchtypes.Statistic{cloudwatchtypes.StatisticAverage},
	})

	if err == nil && len(cpuResult.Datapoints) > 0 {
		autoscalingInfo.CurrentCPUUtilization = cpuResult.Datapoints[0].Average
	}

	// Get Memory utilization
	memoryResult, err := cloudwatchClient.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/ECS"),
		MetricName: aws.String("MemoryUtilization"),
		Dimensions: []cloudwatchtypes.Dimension{
			{Name: aws.String("ServiceName"), Value: &fullServiceName},
			{Name: aws.String("ClusterName"), Value: &clusterName},
		},
		StartTime:  &fiveMinutesAgo,
		EndTime:    &now,
		Period:     aws.Int32(300),
		Statistics: []cloudwatchtypes.Statistic{cloudwatchtypes.StatisticAverage},
	})

	if err == nil && len(memoryResult.Datapoints) > 0 {
		autoscalingInfo.CurrentMemoryUtilization = memoryResult.Datapoints[0].Average
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(autoscalingInfo)
}

func getServiceScalingHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	envName := r.URL.Query().Get("env")
	serviceName := r.URL.Query().Get("service")
	hoursStr := r.URL.Query().Get("hours")
	
	if envName == "" || serviceName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env and service parameters are required"})
		return
	}

	hours := 24
	if hoursStr != "" {
		fmt.Sscanf(hoursStr, "%d", &hours)
	}

	// Load environment config
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

	fullServiceName := getFullServiceName(envConfig.Project, serviceName, envConfig.Env)
	clusterName := fmt.Sprintf("%s_cluster_%s", envConfig.Project, envConfig.Env)

	// Get AWS config
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	// Get scaling activities
	autoscalingClient := applicationautoscaling.NewFromConfig(cfg)
	resourceId := fmt.Sprintf("service/%s/%s", clusterName, fullServiceName)
	
	startTime := time.Now().Add(-time.Duration(hours) * time.Hour)
	activitiesResult, err := autoscalingClient.DescribeScalingActivities(ctx, &applicationautoscaling.DescribeScalingActivitiesInput{
		ServiceNamespace: types.ServiceNamespaceEcs,
		ResourceId:       &resourceId,
		MaxResults:       aws.Int32(100),
	})

	history := ServiceScalingHistory{
		ServiceName: serviceName,
		Events:      []ScalingEvent{},
	}

	if err == nil {
		for _, activity := range activitiesResult.ScalingActivities {
			if activity.StartTime.After(startTime) {
				event := ScalingEvent{
					Timestamp:  activity.StartTime.Format(time.RFC3339),
					StatusCode: string(activity.StatusCode),
				}

				// Parse activity description to determine type and capacity changes
				desc := *activity.Description
				if strings.Contains(desc, "Successfully set desired count") {
					// Extract from/to values from description
					// Format: "Successfully set desired count to X. Change successfully fulfilled by ecs."
					var toCapacity int32
					fmt.Sscanf(desc, "Successfully set desired count to %d", &toCapacity)
					event.ToCapacity = toCapacity
					
					// Determine if it's scale up or down based on cause
					if activity.Cause != nil && strings.Contains(*activity.Cause, "monitor alarm") {
						if strings.Contains(*activity.Cause, "High") {
							event.ActivityType = "ScaleUp"
						} else {
							event.ActivityType = "ScaleDown"
						}
						event.Reason = *activity.Cause
					}
				}

				history.Events = append(history.Events, event)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

func getServiceMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	envName := r.URL.Query().Get("env")
	serviceName := r.URL.Query().Get("service")
	
	if envName == "" || serviceName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env and service parameters are required"})
		return
	}

	// Load environment config
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

	fullServiceName := getFullServiceName(envConfig.Project, serviceName, envConfig.Env)
	clusterName := fmt.Sprintf("%s_cluster_%s", envConfig.Project, envConfig.Env)

	// Get AWS config
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	cloudwatchClient := cloudwatch.NewFromConfig(cfg)
	now := time.Now()
	oneDayAgo := now.Add(-24 * time.Hour)

	metrics := ServiceMetrics{
		ServiceName: serviceName,
		Metrics: MetricsData{
			CPU:       []MetricPoint{},
			Memory:    []MetricPoint{},
			TaskCount: []MetricPoint{},
		},
	}

	// Common dimensions
	dimensions := []cloudwatchtypes.Dimension{
		{Name: aws.String("ServiceName"), Value: &fullServiceName},
		{Name: aws.String("ClusterName"), Value: &clusterName},
	}

	// Get CPU metrics
	cpuResult, err := cloudwatchClient.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/ECS"),
		MetricName: aws.String("CPUUtilization"),
		Dimensions: dimensions,
		StartTime:  &oneDayAgo,
		EndTime:    &now,
		Period:     aws.Int32(300), // 5 minutes
		Statistics: []cloudwatchtypes.Statistic{cloudwatchtypes.StatisticAverage},
	})

	if err == nil {
		for _, datapoint := range cpuResult.Datapoints {
			metrics.Metrics.CPU = append(metrics.Metrics.CPU, MetricPoint{
				Timestamp: datapoint.Timestamp.Format(time.RFC3339),
				Value:     *datapoint.Average,
			})
		}
	}

	// Get Memory metrics
	memoryResult, err := cloudwatchClient.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/ECS"),
		MetricName: aws.String("MemoryUtilization"),
		Dimensions: dimensions,
		StartTime:  &oneDayAgo,
		EndTime:    &now,
		Period:     aws.Int32(300),
		Statistics: []cloudwatchtypes.Statistic{cloudwatchtypes.StatisticAverage},
	})

	if err == nil {
		for _, datapoint := range memoryResult.Datapoints {
			metrics.Metrics.Memory = append(metrics.Metrics.Memory, MetricPoint{
				Timestamp: datapoint.Timestamp.Format(time.RFC3339),
				Value:     *datapoint.Average,
			})
		}
	}

	// Get Task Count metrics
	taskCountResult, err := cloudwatchClient.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("ECS/ContainerInsights"),
		MetricName: aws.String("DesiredTaskCount"),
		Dimensions: dimensions,
		StartTime:  &oneDayAgo,
		EndTime:    &now,
		Period:     aws.Int32(300),
		Statistics: []cloudwatchtypes.Statistic{cloudwatchtypes.StatisticAverage},
	})

	if err == nil {
		for _, datapoint := range taskCountResult.Datapoints {
			metrics.Metrics.TaskCount = append(metrics.Metrics.TaskCount, MetricPoint{
				Timestamp: datapoint.Timestamp.Format(time.RFC3339),
				Value:     *datapoint.Average,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// Helper function to get full service name
func getFullServiceName(project, serviceName, env string) string {
	if serviceName == "backend" {
		return fmt.Sprintf("%s_service_%s", project, env)
	}
	return fmt.Sprintf("%s_service_%s_%s", project, serviceName, env)
}

// Helper function to parse resource values
func parseResourceValue(value string) (int, error) {
	var result int
	_, err := fmt.Sscanf(value, "%d", &result)
	return result, err
}