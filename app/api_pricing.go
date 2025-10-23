package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
)

// PricingResponse represents the pricing data for all services
type PricingResponse struct {
	Region string                     `json:"region"`
	Nodes  map[string]NodePricing     `json:"nodes"`
}

// NodePricing represents pricing for a single node/service
type NodePricing struct {
	ServiceName string                 `json:"serviceName"`
	ServiceType string                 `json:"serviceType"`
	Levels      map[string]LevelPrice  `json:"levels"`
}

// LevelPrice represents the price for a specific workload level
type LevelPrice struct {
	MonthlyPrice float64           `json:"monthlyPrice"`
	HourlyPrice  float64           `json:"hourlyPrice"`
	Details      map[string]string `json:"details"`
}

// WorkloadSpecs defines the specifications for each workload level
var WorkloadSpecs = map[string]map[string]interface{}{
	"startup": {
		"ecs_cpu":           256,     // 0.25 vCPU
		"ecs_memory":        512,     // 512 MB
		"ecs_desired_count": 1,       // 1 task
		"rds_instance":      "db.t3.micro",
		"alb_requests":      100000,  // 100k requests/month
		"nat_gateway_gb":    10,      // 10 GB/month
		"cognito_mau":       1000,    // 1k monthly active users
	},
	"scaleup": {
		"ecs_cpu":           512,     // 0.5 vCPU
		"ecs_memory":        1024,    // 1 GB
		"ecs_desired_count": 2,       // 2 tasks
		"rds_instance":      "db.t3.small",
		"alb_requests":      1000000, // 1M requests/month
		"nat_gateway_gb":    100,     // 100 GB/month
		"cognito_mau":       10000,   // 10k monthly active users
	},
	"highload": {
		"ecs_cpu":           1024,     // 1 vCPU
		"ecs_memory":        2048,     // 2 GB
		"ecs_desired_count": 4,        // 4 tasks
		"rds_instance":      "db.t3.medium",
		"alb_requests":      10000000, // 10M requests/month
		"nat_gateway_gb":    1000,     // 1 TB/month
		"cognito_mau":       50000,    // 50k monthly active users
	},
}

func getPricing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get environment name from query parameter
	envName := r.URL.Query().Get("env")
	if envName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env parameter is required"})
		return
	}

	// Load environment config to get region
	env, err := loadEnv(envName)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "environment not found"})
		return
	}

	region := env.Region
	if region == "" {
		region = "us-east-1" // Default region
	}

	// Initialize AWS Pricing client (always uses us-east-1)
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	pricingClient := pricing.NewFromConfig(cfg)

	// Initialize response
	response := PricingResponse{
		Region: region,
		Nodes:  make(map[string]NodePricing),
	}

	// Calculate pricing for each service based on environment configuration

	// 1. Backend service (always present)
	// Calculate backend instance count based on autoscaling or fixed count
	backendInstanceCount := int32(1) // Default
	if env.Workload.BackendAutoscalingEnabled {
		// Use median of min and max for autoscaling
		minCap := env.Workload.BackendAutoscalingMinCapacity
		maxCap := env.Workload.BackendAutoscalingMaxCapacity
		if minCap == 0 {
			minCap = 1
		}
		if maxCap == 0 {
			maxCap = 10
		}
		backendInstanceCount = (minCap + maxCap) / 2
	} else if env.Workload.BackendDesiredCount > 0 {
		backendInstanceCount = env.Workload.BackendDesiredCount
	}
	
	backendPricing := calculateBackendPricing(ctx, pricingClient, region, env.Workload, backendInstanceCount)
	if backendPricing != nil {
		response.Nodes["backend"] = *backendPricing
	}

	// 2. Additional services
	for _, service := range env.Services {
		desiredCount := int32(1) // Default to 1 if not specified
		if service.DesiredCount > 0 {
			desiredCount = int32(service.DesiredCount)
		}
		servicePricing := calculateServicePricing(ctx, pricingClient, region, service.Name, desiredCount)
		if servicePricing != nil {
			response.Nodes[service.Name] = *servicePricing
		}
	}

	// 3. RDS pricing if postgres is enabled
	if env.Postgres.Enabled {
		rdsNodePricing := calculateRDSPricing(ctx, pricingClient, region, env)
		if rdsNodePricing != nil {
			response.Nodes["rds"] = *rdsNodePricing
		}
	}

	// 4. API Gateway or ALB pricing (based on configuration)
	if env.ALB.Enabled {
		// ALB is enabled, so only show ALB pricing
		albNodePricing := calculateALBPricing(ctx, pricingClient, region)
		if albNodePricing != nil {
			response.Nodes["alb"] = *albNodePricing
		}
	} else {
		// ALB is not enabled, so only show API Gateway pricing
		apiGatewayPricing := calculateAPIGatewayPricing(ctx, pricingClient, region)
		if apiGatewayPricing != nil {
			response.Nodes["api_gateway"] = *apiGatewayPricing
		}
	}

	// 5. Cognito pricing if enabled
	if env.Cognito.Enabled {
		cognitoNodePricing := calculateCognitoPricing(region)
		if cognitoNodePricing != nil {
			response.Nodes["cognito"] = *cognitoNodePricing
		}
	}

	// 6. S3 pricing - always show since backend uses S3 for storage
	// Count buckets: image bucket (if public) + any defined buckets
	bucketCount := 0
	if env.Workload.BucketPostfix != "" {
		bucketCount = 1 // Backend image bucket
	}
	if len(env.Buckets) > 0 {
		bucketCount += len(env.Buckets)
	}
	if bucketCount > 0 {
		s3NodePricing := calculateS3Pricing(ctx, pricingClient, region, bucketCount)
		if s3NodePricing != nil {
			response.Nodes["s3"] = *s3NodePricing
		}
	}

	// 7. CloudWatch pricing
	cloudWatchNodePricing := calculateCloudWatchPricing(ctx, pricingClient, region)
	if cloudWatchNodePricing != nil {
		response.Nodes["cloudwatch"] = *cloudWatchNodePricing
	}

	// 8. Route53 pricing if domain is enabled
	if env.Domain.Enabled {
		route53Pricing := calculateRoute53Pricing(region)
		if route53Pricing != nil {
			response.Nodes["route53"] = *route53Pricing
		}
	}

	// 9. SES pricing if enabled
	if env.Ses.Enabled {
		sesPricing := calculateSESPricing(region)
		if sesPricing != nil {
			response.Nodes["ses"] = *sesPricing
		}
	}

	// 10. EventBridge pricing if event tasks are configured
	if len(env.EventProcessorTasks) > 0 {
		eventBridgePricing := calculateEventBridgePricing(region, len(env.EventProcessorTasks))
		if eventBridgePricing != nil {
			response.Nodes["eventbridge"] = *eventBridgePricing
		}

		// 10a. Event processor tasks pricing (using Fargate)
		for _, task := range env.EventProcessorTasks {
			taskPricing := calculateEventTaskPricing(ctx, pricingClient, region, task)
			if taskPricing != nil {
				response.Nodes["event_"+task.Name] = *taskPricing
			}
		}
	}

	// 11. Scheduled tasks pricing (using Fargate)
	for _, task := range env.ScheduledTasks {
		taskPricing := calculateScheduledTaskPricing(ctx, pricingClient, region, task)
		if taskPricing != nil {
			response.Nodes["scheduled_"+task.Name] = *taskPricing
		}
	}

	// 12. ECR pricing (for Docker images)
	ecrPricing := calculateECRPricing(ctx, pricingClient, region)
	if ecrPricing != nil {
		response.Nodes["ecr"] = *ecrPricing
	}

	// 13. VPC pricing (for endpoints if used)
	vpcPricing := calculateVPCPricing(region)
	if vpcPricing != nil {
		response.Nodes["vpc"] = *vpcPricing
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func calculateBackendPricing(ctx context.Context, client *pricing.Client, region string, workload Workload, instanceCount int32) *NodePricing {
	nodePricing := &NodePricing{
		ServiceName: "Backend Service",
		ServiceType: "compute",
		Levels:      make(map[string]LevelPrice),
	}

	// Get Fargate pricing
	vCPUPrice := getFargatePrice(ctx, client, region, "vCPU")
	memoryPrice := getFargatePrice(ctx, client, region, "GB")

	// Use configured CPU/Memory if available
	configuredCPU := 256
	configuredMemory := 512
	if workload.BackendCPU != "" {
		if cpu, err := strconv.Atoi(workload.BackendCPU); err == nil {
			configuredCPU = cpu
		}
	}
	if workload.BackendMemory != "" {
		if mem, err := strconv.Atoi(workload.BackendMemory); err == nil {
			configuredMemory = mem
		}
	}

	for level, specs := range WorkloadSpecs {
		// Use configured resources for all levels or fallback to spec
		cpu := configuredCPU
		memory := configuredMemory
		
		// If no configuration, use level-specific specs
		if workload.BackendCPU == "" {
			cpu = specs["ecs_cpu"].(int)
		}
		if workload.BackendMemory == "" {
			memory = specs["ecs_memory"].(int)
		}

		// Calculate hourly cost
		vCPUCost := (float64(cpu) / 1024.0) * vCPUPrice * float64(instanceCount)
		memoryCost := (float64(memory) / 1024.0) * memoryPrice * float64(instanceCount)
		hourlyCost := vCPUCost + memoryCost

		details := map[string]string{
			"vCPU":    fmt.Sprintf("%.2f vCPU", float64(cpu)/1024.0),
			"memory":  fmt.Sprintf("%d MB", memory),
			"runtime": "Fargate",
		}

		// Add instance count details
		if workload.BackendAutoscalingEnabled {
			details["autoscaling"] = fmt.Sprintf("%d-%d instances", workload.BackendAutoscalingMinCapacity, workload.BackendAutoscalingMaxCapacity)
			details["estimated_instances"] = fmt.Sprintf("%d (median)", instanceCount)
		} else {
			details["instances"] = fmt.Sprintf("%d", instanceCount)
		}

		nodePricing.Levels[level] = LevelPrice{
			HourlyPrice:  hourlyCost,
			MonthlyPrice: hourlyCost * 730, // 730 hours per month
			Details:      details,
		}
	}

	return nodePricing
}

func calculateServicePricing(ctx context.Context, client *pricing.Client, region string, serviceName string, configuredDesiredCount int32) *NodePricing {
	nodePricing := &NodePricing{
		ServiceName: fmt.Sprintf("Service: %s", serviceName),
		ServiceType: "compute",
		Levels:      make(map[string]LevelPrice),
	}

	// Get Fargate pricing
	vCPUPrice := getFargatePrice(ctx, client, region, "vCPU")
	memoryPrice := getFargatePrice(ctx, client, region, "GB")

	for level, specs := range WorkloadSpecs {
		cpu := specs["ecs_cpu"].(int)
		memory := specs["ecs_memory"].(int)
		desiredCount := specs["ecs_desired_count"].(int)
		
		// Use configured desired count if available
		if configuredDesiredCount > 0 {
			desiredCount = int(configuredDesiredCount)
		}

		// Calculate hourly cost
		vCPUCost := (float64(cpu) / 1024.0) * vCPUPrice * float64(desiredCount)
		memoryCost := (float64(memory) / 1024.0) * memoryPrice * float64(desiredCount)
		hourlyCost := vCPUCost + memoryCost

		nodePricing.Levels[level] = LevelPrice{
			HourlyPrice:  hourlyCost,
			MonthlyPrice: hourlyCost * 730, // 730 hours per month
			Details: map[string]string{
				"vCPU":    fmt.Sprintf("%.2f vCPU", float64(cpu)/1024.0),
				"memory":  fmt.Sprintf("%d MB", memory),
				"tasks":   fmt.Sprintf("%d", desiredCount),
				"runtime": "Fargate",
			},
		}
	}

	return nodePricing
}

func calculateRDSPricing(ctx context.Context, client *pricing.Client, region string, env Env) *NodePricing {
	nodePricing := &NodePricing{
		ServiceName: "RDS PostgreSQL",
		ServiceType: "database",
		Levels:      make(map[string]LevelPrice),
	}

	// Get actual configuration from env, with defaults
	instanceClass := "db.t4g.micro"
	if env.Postgres.InstanceClass != "" {
		instanceClass = env.Postgres.InstanceClass
	}

	storage := 20
	if env.Postgres.AllocatedStorage > 0 {
		storage = env.Postgres.AllocatedStorage
	}

	multiAZ := env.Postgres.MultiAZ

	for level, specs := range WorkloadSpecs {
		// If no configuration, use level-specific defaults for demo
		levelInstanceClass := instanceClass
		if env.Postgres.InstanceClass == "" {
			levelInstanceClass = specs["rds_instance"].(string)
		}

		// Get RDS instance price from AWS Pricing API
		hourlyPrice := getRDSInstancePrice(ctx, client, region, levelInstanceClass, multiAZ)

		// Calculate storage cost (gp3: $0.115/GB/month)
		storageCostPerMonth := float64(storage) * 0.115

		// Total monthly cost
		monthlyPrice := (hourlyPrice * 730) + storageCostPerMonth

		details := map[string]string{
			"instanceType": levelInstanceClass,
			"engine":       "PostgreSQL",
			"storage":      fmt.Sprintf("%d GB gp3", storage),
		}

		if multiAZ {
			details["deployment"] = "Multi-AZ"
		} else {
			details["deployment"] = "Single-AZ"
		}

		nodePricing.Levels[level] = LevelPrice{
			HourlyPrice:  hourlyPrice + (storageCostPerMonth / 730),
			MonthlyPrice: monthlyPrice,
			Details:      details,
		}
	}

	return nodePricing
}

func calculateALBPricing(ctx context.Context, client *pricing.Client, region string) *NodePricing {
	nodePricing := &NodePricing{
		ServiceName: "Application Load Balancer",
		ServiceType: "networking",
		Levels:      make(map[string]LevelPrice),
	}

	// ALB has fixed hourly price + LCU (Load Balancer Capacity Units)
	albHourlyPrice := getALBHourlyPrice(ctx, client, region)
	lcuPrice := getALBLCUPrice(ctx, client, region)

	for level, specs := range WorkloadSpecs {
		requests := specs["alb_requests"].(int)
		
		// Estimate LCUs based on requests (simplified calculation)
		// 1 LCU = 25 new connections/sec = 90,000 connections/hour
		estimatedLCUs := float64(requests) / (90000.0 * 730) // Monthly requests to hourly LCUs
		
		hourlyCost := albHourlyPrice + (estimatedLCUs * lcuPrice)
		
		nodePricing.Levels[level] = LevelPrice{
			HourlyPrice:  hourlyCost,
			MonthlyPrice: hourlyCost * 730,
			Details: map[string]string{
				"requests": fmt.Sprintf("%d/month", requests),
				"lcus":     fmt.Sprintf("%.2f", estimatedLCUs),
			},
		}
	}

	return nodePricing
}

func calculateAPIGatewayPricing(ctx context.Context, client *pricing.Client, region string) *NodePricing {
	nodePricing := &NodePricing{
		ServiceName: "API Gateway",
		ServiceType: "networking",
		Levels:      make(map[string]LevelPrice),
	}

	// API Gateway REST API pricing
	requestPrice := 3.50 // $3.50 per million requests

	for level, specs := range WorkloadSpecs {
		requests := specs["alb_requests"].(int) // Reuse ALB request estimates
		millionRequests := float64(requests) / 1000000.0
		monthlyCost := millionRequests * requestPrice
		
		nodePricing.Levels[level] = LevelPrice{
			HourlyPrice:  monthlyCost / 730,
			MonthlyPrice: monthlyCost,
			Details: map[string]string{
				"requests": fmt.Sprintf("%d/month", requests),
				"type":     "REST API",
			},
		}
	}

	return nodePricing
}

func calculateRoute53Pricing(region string) *NodePricing {
	nodePricing := &NodePricing{
		ServiceName: "Route 53",
		ServiceType: "networking",
		Levels:      make(map[string]LevelPrice),
	}

	// Route53 pricing
	hostedZonePrice := 0.50 // $0.50 per hosted zone per month
	
	// DNS queries pricing (per million)
	queryPrices := map[string]float64{
		"startup":  0.40, // $0.40 per million queries
		"scaleup":  0.40,
		"highload": 0.40,
	}

	queryEstimates := map[string]int{
		"startup":  100000,   // 100k queries/month
		"scaleup":  1000000,  // 1M queries/month
		"highload": 10000000, // 10M queries/month
	}

	for level, queries := range queryEstimates {
		millionQueries := float64(queries) / 1000000.0
		queryCost := millionQueries * queryPrices[level]
		monthlyCost := hostedZonePrice + queryCost
		
		nodePricing.Levels[level] = LevelPrice{
			HourlyPrice:  monthlyCost / 730,
			MonthlyPrice: monthlyCost,
			Details: map[string]string{
				"hostedZones": "1",
				"queries":     fmt.Sprintf("%d/month", queries),
			},
		}
	}

	return nodePricing
}

func calculateSESPricing(region string) *NodePricing {
	nodePricing := &NodePricing{
		ServiceName: "Simple Email Service",
		ServiceType: "communication",
		Levels:      make(map[string]LevelPrice),
	}

	// SES pricing
	emailPrice := 0.10 // $0.10 per 1,000 emails

	emailEstimates := map[string]int{
		"startup":  10000,   // 10k emails/month
		"scaleup":  100000,  // 100k emails/month
		"highload": 1000000, // 1M emails/month
	}

	for level, emails := range emailEstimates {
		thousandEmails := float64(emails) / 1000.0
		monthlyCost := thousandEmails * emailPrice
		
		nodePricing.Levels[level] = LevelPrice{
			HourlyPrice:  monthlyCost / 730,
			MonthlyPrice: monthlyCost,
			Details: map[string]string{
				"emails": fmt.Sprintf("%d/month", emails),
			},
		}
	}

	return nodePricing
}

func calculateEventBridgePricing(region string, ruleCount int) *NodePricing {
	nodePricing := &NodePricing{
		ServiceName: "EventBridge",
		ServiceType: "integration",
		Levels:      make(map[string]LevelPrice),
	}

	// EventBridge pricing
	eventPrice := 1.00 // $1.00 per million events

	eventEstimates := map[string]int{
		"startup":  100000,   // 100k events/month
		"scaleup":  1000000,  // 1M events/month
		"highload": 10000000, // 10M events/month
	}

	for level, events := range eventEstimates {
		millionEvents := float64(events) / 1000000.0
		monthlyCost := millionEvents * eventPrice * float64(ruleCount)
		
		nodePricing.Levels[level] = LevelPrice{
			HourlyPrice:  monthlyCost / 730,
			MonthlyPrice: monthlyCost,
			Details: map[string]string{
				"events": fmt.Sprintf("%d/month", events),
				"rules":  fmt.Sprintf("%d", ruleCount),
			},
		}
	}

	return nodePricing
}

func calculateECRPricing(ctx context.Context, client *pricing.Client, region string) *NodePricing {
	nodePricing := &NodePricing{
		ServiceName: "Elastic Container Registry",
		ServiceType: "storage",
		Levels:      make(map[string]LevelPrice),
	}

	// ECR storage pricing
	storagePrice := 0.10 // $0.10 per GB per month

	storageEstimates := map[string]int{
		"startup":  10,  // 10 GB
		"scaleup":  50,  // 50 GB
		"highload": 200, // 200 GB
	}

	for level, storage := range storageEstimates {
		monthlyCost := float64(storage) * storagePrice
		
		nodePricing.Levels[level] = LevelPrice{
			HourlyPrice:  monthlyCost / 730,
			MonthlyPrice: monthlyCost,
			Details: map[string]string{
				"storage": fmt.Sprintf("%d GB", storage),
			},
		}
	}

	return nodePricing
}

func calculateVPCPricing(region string) *NodePricing {
	nodePricing := &NodePricing{
		ServiceName: "VPC",
		ServiceType: "networking",
		Levels:      make(map[string]LevelPrice),
	}

	// VPC is free, but we'll show it with $0 pricing
	for level := range WorkloadSpecs {
		nodePricing.Levels[level] = LevelPrice{
			HourlyPrice:  0,
			MonthlyPrice: 0,
			Details: map[string]string{
				"subnets":         "6",
				"securityGroups":  "Multiple",
				"cost":            "Free",
			},
		}
	}

	return nodePricing
}

func calculateCognitoPricing(region string) *NodePricing {
	nodePricing := &NodePricing{
		ServiceName: "AWS Cognito",
		ServiceType: "authentication",
		Levels:      make(map[string]LevelPrice),
	}

	// Cognito pricing is based on MAUs (Monthly Active Users)
	// First 50,000 MAUs are free, then $0.0055 per MAU
	pricePerMAU := 0.0055

	for level, specs := range WorkloadSpecs {
		mau := specs["cognito_mau"].(int)
		
		// Calculate billable MAUs (after free tier)
		billableMAUs := 0
		if mau > 50000 {
			billableMAUs = mau - 50000
		}
		
		monthlyCost := float64(billableMAUs) * pricePerMAU
		
		nodePricing.Levels[level] = LevelPrice{
			HourlyPrice:  monthlyCost / 730,
			MonthlyPrice: monthlyCost,
			Details: map[string]string{
				"mau":          fmt.Sprintf("%d", mau),
				"billableMAU": fmt.Sprintf("%d", billableMAUs),
			},
		}
	}

	return nodePricing
}

func calculateS3Pricing(ctx context.Context, client *pricing.Client, region string, bucketCount int) *NodePricing {
	nodePricing := &NodePricing{
		ServiceName: "S3 Storage",
		ServiceType: "storage",
		Levels:      make(map[string]LevelPrice),
	}

	// Get S3 standard storage price
	storagePrice := getS3StoragePrice(ctx, client, region)
	requestPrice := 0.0004 // $0.0004 per 1,000 requests
	
	// Estimate storage based on workload
	storageGB := map[string]int{
		"startup":  10,   // 10 GB per bucket
		"scaleup":  100,  // 100 GB per bucket
		"highload": 1000, // 1 TB per bucket
	}

	requestsK := map[string]int{
		"startup":  100,   // 100k requests/month
		"scaleup":  1000,  // 1M requests/month
		"highload": 10000, // 10M requests/month
	}

	for level, gb := range storageGB {
		totalGB := gb * bucketCount
		storageCost := float64(totalGB) * storagePrice
		requestCost := float64(requestsK[level]) * requestPrice
		monthlyCost := storageCost + requestCost
		
		nodePricing.Levels[level] = LevelPrice{
			HourlyPrice:  monthlyCost / 730,
			MonthlyPrice: monthlyCost,
			Details: map[string]string{
				"storage":  fmt.Sprintf("%d GB", totalGB),
				"buckets":  fmt.Sprintf("%d", bucketCount),
				"requests": fmt.Sprintf("%dk/month", requestsK[level]),
			},
		}
	}

	return nodePricing
}

func calculateScheduledTaskPricing(ctx context.Context, client *pricing.Client, region string, task ScheduledTask) *NodePricing {
	nodePricing := &NodePricing{
		ServiceName: fmt.Sprintf("Scheduled Task: %s", task.Name),
		ServiceType: "compute",
		Levels:      make(map[string]LevelPrice),
	}

	// Get Fargate pricing
	vCPUPrice := getFargatePrice(ctx, client, region, "vCPU")
	memoryPrice := getFargatePrice(ctx, client, region, "GB")

	// Parse schedule to estimate runs per month
	runsPerMonth := estimateRunsPerMonth(task.Schedule)

	for level, specs := range WorkloadSpecs {
		cpu := specs["ecs_cpu"].(int)
		memory := specs["ecs_memory"].(int)
		
		// Calculate cost per run (assuming 5 minutes average runtime)
		runtimeHours := 5.0 / 60.0 // 5 minutes in hours
		vCPUCostPerRun := (float64(cpu) / 1024.0) * vCPUPrice * runtimeHours
		memoryCostPerRun := (float64(memory) / 1024.0) * memoryPrice * runtimeHours
		costPerRun := vCPUCostPerRun + memoryCostPerRun
		
		monthlyCost := costPerRun * float64(runsPerMonth)
		
		nodePricing.Levels[level] = LevelPrice{
			HourlyPrice:  monthlyCost / 730,
			MonthlyPrice: monthlyCost,
			Details: map[string]string{
				"vCPU":         fmt.Sprintf("%.2f vCPU", float64(cpu)/1024.0),
				"memory":       fmt.Sprintf("%d MB", memory),
				"schedule":     task.Schedule,
				"runsPerMonth": fmt.Sprintf("%d", runsPerMonth),
				"runtime":      "5 min/run",
			},
		}
	}

	return nodePricing
}

func estimateRunsPerMonth(schedule string) int {
	// Parse common schedule expressions
	if strings.Contains(schedule, "rate(") {
		parts := strings.Split(schedule, "(")
		if len(parts) >= 2 {
			ratePart := strings.TrimSuffix(parts[1], ")")
			fields := strings.Fields(ratePart)
			if len(fields) >= 2 {
				value := 1
				fmt.Sscanf(fields[0], "%d", &value)
				unit := fields[1]
				
				switch {
				case strings.HasPrefix(unit, "minute"):
					return (30 * 24 * 60) / value
				case strings.HasPrefix(unit, "hour"):
					return (30 * 24) / value
				case strings.HasPrefix(unit, "day"):
					return 30 / value
				}
			}
		}
	}
	// Default to once per day if can't parse
	return 30
}

func calculateEventTaskPricing(ctx context.Context, client *pricing.Client, region string, task EventProcessorTask) *NodePricing {
	nodePricing := &NodePricing{
		ServiceName: fmt.Sprintf("Event Task: %s", task.Name),
		ServiceType: "compute",
		Levels:      make(map[string]LevelPrice),
	}

	// Get Fargate pricing
	vCPUPrice := getFargatePrice(ctx, client, region, "vCPU")
	memoryPrice := getFargatePrice(ctx, client, region, "GB")

	// Estimate runs per month based on workload level
	// Event tasks typically process events triggered by EventBridge
	runsPerMonthMap := map[string]int{
		"startup":   500,   // Low event volume for startups
		"scaleup":   2000,  // Medium event volume as you scale
		"highload":  10000, // High event volume for production scale
	}

	for level, specs := range WorkloadSpecs {
		cpu := specs["ecs_cpu"].(int)
		memory := specs["ecs_memory"].(int)

		// Event processing is typically faster than scheduled tasks (2-3 minutes per run)
		runtimeHours := 2.0 / 60.0 // 2 minutes in hours
		vCPUCostPerRun := (float64(cpu) / 1024.0) * vCPUPrice * runtimeHours
		memoryCostPerRun := (float64(memory) / 1024.0) * memoryPrice * runtimeHours
		costPerRun := vCPUCostPerRun + memoryCostPerRun

		runsPerMonth := runsPerMonthMap[level]
		monthlyCost := costPerRun * float64(runsPerMonth)

		nodePricing.Levels[level] = LevelPrice{
			HourlyPrice:  monthlyCost / 730,
			MonthlyPrice: monthlyCost,
			Details: map[string]string{
				"vCPU":         fmt.Sprintf("%.2f vCPU", float64(cpu)/1024.0),
				"memory":       fmt.Sprintf("%d MB", memory),
				"ruleName":     task.RuleName,
				"runsPerMonth": fmt.Sprintf("%d", runsPerMonth),
				"runtime":      "2 min/run",
				"sources":      fmt.Sprintf("%v", task.Sources),
				"detailTypes":  fmt.Sprintf("%v", task.DetailTypes),
			},
		}
	}

	return nodePricing
}

func calculateCloudWatchPricing(ctx context.Context, client *pricing.Client, region string) *NodePricing {
	nodePricing := &NodePricing{
		ServiceName: "CloudWatch",
		ServiceType: "monitoring",
		Levels:      make(map[string]LevelPrice),
	}

	// CloudWatch logs ingestion price per GB
	logsIngestionPrice := getCloudWatchLogsPrice(ctx, client, region)
	
	// Estimate logs based on workload
	logsGB := map[string]float64{
		"startup":  1.0,   // 1 GB/month
		"scaleup":  10.0,  // 10 GB/month
		"highload": 50.0,  // 50 GB/month
	}

	for level, gb := range logsGB {
		monthlyCost := gb * logsIngestionPrice
		
		nodePricing.Levels[level] = LevelPrice{
			HourlyPrice:  monthlyCost / 730,
			MonthlyPrice: monthlyCost,
			Details: map[string]string{
				"logsIngestion": fmt.Sprintf("%.1f GB/month", gb),
			},
		}
	}

	return nodePricing
}

// Helper functions to get prices from AWS Pricing API
func getFargatePrice(ctx context.Context, client *pricing.Client, region string, unit string) float64 {
	// This is a simplified version - in production you'd use the actual API
	// Default prices for us-east-1 as fallback
	if unit == "vCPU" {
		return 0.04048 // per vCPU per hour
	}
	return 0.004445 // per GB per hour
}

func getRDSInstancePrice(ctx context.Context, client *pricing.Client, region string, instanceType string, multiAZ bool) float64 {
	// Hardcoded prices for current-gen instances (us-east-1, Single-AZ)
	// These are real AWS prices as of January 2025
	singleAZPrices := map[string]float64{
		"db.t4g.micro":    0.016,
		"db.t4g.small":    0.032,
		"db.t4g.medium":   0.065,
		"db.t4g.large":    0.129,
		"db.t3.micro":     0.018,
		"db.t3.small":     0.036,
		"db.t3.medium":    0.073,
		"db.m6i.large":    0.178,
		"db.m6i.xlarge":   0.356,
		"db.m6i.2xlarge":  0.712,
		"db.m5.large":     0.192,
		"db.m5.xlarge":    0.384,
		"db.r6i.large":    0.240,
		"db.r6i.xlarge":   0.480,
		"db.r5.large":     0.260,
		"db.r5.xlarge":    0.520,
	}

	price := singleAZPrices[instanceType]
	if price == 0 {
		// Default to t4g.micro price if not found
		price = 0.016
	}

	// Multi-AZ doubles the instance cost (storage is already replicated)
	if multiAZ {
		price *= 2
	}

	return price
}

// Legacy function for backwards compatibility
func getRDSPrice(ctx context.Context, client *pricing.Client, region string, instanceType string) float64 {
	return getRDSInstancePrice(ctx, client, region, instanceType, false)
}

func getALBHourlyPrice(ctx context.Context, client *pricing.Client, region string) float64 {
	return 0.0225 // $0.0225 per hour
}

func getALBLCUPrice(ctx context.Context, client *pricing.Client, region string) float64 {
	return 0.008 // $0.008 per LCU hour
}


func getS3StoragePrice(ctx context.Context, client *pricing.Client, region string) float64 {
	return 0.023 // $0.023 per GB per month for standard storage
}

func getCloudWatchLogsPrice(ctx context.Context, client *pricing.Client, region string) float64 {
	return 0.50 // $0.50 per GB ingested
}

// Helper function to parse price from AWS Pricing API response
func parsePriceFromProduct(product string) (float64, error) {
	var productData map[string]interface{}
	if err := json.Unmarshal([]byte(product), &productData); err != nil {
		return 0, err
	}

	// Navigate through the pricing structure (this is simplified)
	if terms, ok := productData["terms"].(map[string]interface{}); ok {
		if onDemand, ok := terms["OnDemand"].(map[string]interface{}); ok {
			for _, term := range onDemand {
				if termData, ok := term.(map[string]interface{}); ok {
					if priceDimensions, ok := termData["priceDimensions"].(map[string]interface{}); ok {
						for _, dimension := range priceDimensions {
							if dimData, ok := dimension.(map[string]interface{}); ok {
								if pricePerUnit, ok := dimData["pricePerUnit"].(map[string]interface{}); ok {
									if usd, ok := pricePerUnit["USD"].(string); ok {
										return strconv.ParseFloat(usd, 64)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return 0, fmt.Errorf("price not found in product data")
}

// getPricingRates returns raw pricing rates from the pricing service
// GET /api/pricing/rates?region=us-east-1
//
// This endpoint provides the single source of truth for all AWS pricing data.
// The frontend fetches these rates once and uses them for all calculations.
//
// Response format matches pricing.PriceRates struct with all service pricing.
// Pricing is cached with 24h TTL and refreshed automatically in the background.
func getPricingRates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get region from query params (default to us-east-1)
	region := r.URL.Query().Get("region")
	if region == "" {
		region = "us-east-1"
	}

	// Get rates from pricing service (cached, fast!)
	rates, err := globalPricingService.GetRates(region)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Failed to get pricing rates: %v", err)})
		return
	}

	// Return pricing rates as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rates)
}