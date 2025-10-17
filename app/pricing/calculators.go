package pricing

import "log"

// CalculateRDSPrice calculates monthly cost for RDS instance
// This calculation MUST match the frontend calculator exactly
//
// @param config - RDS configuration (instance class, storage, multi-AZ)
// @param rates - Current pricing rates from cache
// @return Monthly cost in USD
func CalculateRDSPrice(config RDSConfig, rates *PriceRates) float64 {
	// Get hourly instance price
	instanceHourly, exists := rates.RDS[config.InstanceClass]
	if !exists {
		log.Printf("[Pricing] Unknown RDS instance type: %s, using db.t4g.micro as fallback",
			config.InstanceClass)
		instanceHourly = rates.RDS["db.t4g.micro"] // Fallback to cheapest option
	}

	// Multi-AZ doubles instance cost (storage is already replicated)
	if config.MultiAZ {
		instanceHourly *= 2
	}

	// Calculate monthly costs
	instanceCostMonthly := instanceHourly * HoursPerMonth
	storageCostMonthly := float64(config.AllocatedStorage) * rates.Storage.GP3PerGBMonth

	totalMonthly := instanceCostMonthly + storageCostMonthly

	log.Printf("[Pricing] RDS cost: instance=%s hourly=%.4f, storage=%dGB, multiAZ=%v, total=%.2f/mo",
		config.InstanceClass, instanceHourly, config.AllocatedStorage, config.MultiAZ, totalMonthly)

	return totalMonthly
}

// CalculateAuroraPrice calculates monthly cost for Aurora Serverless v2
// Uses workload-level based ACU estimation with realistic utilization assumptions
// This calculation MUST match the frontend calculator exactly
//
// @param config - Aurora configuration (min/max capacity, workload level)
// @param rates - Current pricing rates from cache
// @return Monthly cost in USD
func CalculateAuroraPrice(config AuroraConfig, rates *PriceRates) float64 {
	avgACU := calculateAverageACU(config)
	hourlyACUCost := avgACU * rates.Aurora.ACUHourly
	monthlyPrice := hourlyACUCost * HoursPerMonth

	log.Printf("[Pricing] Aurora cost: level=%s, min=%d, max=%d, avgACU=%.2f, total=%.2f/mo",
		config.Level, config.MinCapacity, config.MaxCapacity, avgACU, monthlyPrice)

	return monthlyPrice
}

// calculateAverageACU estimates average ACU usage based on workload level
// Uses realistic utilization percentages and accounts for database pause time
//
// Utilization assumptions:
// - startup:  20% of (max - min) capacity, 75% active time if min=0
// - scaleup:  50% of (max - min) capacity, 90% active time if min=0
// - highload: 80% of (max - min) capacity, 100% active time (always on)
//
// This logic MUST be identical to the frontend implementation
func calculateAverageACU(config AuroraConfig) float64 {
	min := float64(config.MinCapacity)
	max := float64(config.MaxCapacity)

	// Determine utilization percentage based on workload level
	var utilizationPct float64
	switch config.Level {
	case "startup":
		utilizationPct = 0.20 // 20% average utilization for startup workloads
	case "scaleup":
		utilizationPct = 0.50 // 50% average utilization for scaleup workloads
	case "highload":
		utilizationPct = 0.80 // 80% average utilization for highload workloads
	default:
		utilizationPct = 0.50 // Default to scaleup if unknown
	}

	// Calculate average ACU: min + (range * utilization)
	avgACU := min + (max-min)*utilizationPct

	// If min capacity is 0, database can pause (scale to zero)
	// Apply active time percentage to account for pause periods
	if config.MinCapacity == 0 {
		var activeTimePct float64
		switch config.Level {
		case "startup":
			activeTimePct = 0.75 // Active 75% of the time
		case "scaleup":
			activeTimePct = 0.90 // Active 90% of the time
		case "highload":
			activeTimePct = 1.00 // Always active (100%)
		default:
			activeTimePct = 0.90
		}
		avgACU *= activeTimePct
	}

	return avgACU
}

// CalculateECSPrice calculates monthly cost for ECS Fargate tasks
// This calculation MUST match the frontend calculator exactly
//
// @param config - ECS configuration (CPU, memory, desired count)
// @param rates - Current pricing rates from cache
// @return Monthly cost in USD
func CalculateECSPrice(config ECSConfig, rates *PriceRates) float64 {
	// Convert CPU units to vCPU (256 units = 0.25 vCPU)
	vCPU := float64(config.CPU) / 1024.0
	memoryGB := float64(config.Memory) / 1024.0

	// Calculate hourly cost per task
	vCPUCostPerTask := vCPU * rates.Fargate.VCPUHourly
	memoryCostPerTask := memoryGB * rates.Fargate.MemoryGBHourly
	hourlyCostPerTask := vCPUCostPerTask + memoryCostPerTask

	// Multiply by desired count for total hourly cost
	totalHourlyCost := hourlyCostPerTask * float64(config.DesiredCount)

	// Calculate monthly cost
	monthlyPrice := totalHourlyCost * HoursPerMonth

	log.Printf("[Pricing] ECS cost: cpu=%d, memory=%d, count=%d, total=%.2f/mo",
		config.CPU, config.Memory, config.DesiredCount, monthlyPrice)

	return monthlyPrice
}

// CalculateS3Price calculates monthly cost for S3 storage and requests
// This calculation MUST match the frontend calculator exactly
//
// @param config - S3 configuration (storage GB, requests per day)
// @param rates - Current pricing rates from cache
// @return Monthly cost in USD
func CalculateS3Price(config S3Config, rates *PriceRates) float64 {
	// Storage cost
	storageCost := config.StorageGB * rates.S3.StandardPerGBMonth

	// Request cost (convert daily to monthly)
	monthlyRequests := float64(config.RequestsPerDay) * 30
	requestCost := (monthlyRequests / 1000.0) * rates.S3.RequestsPer1000

	totalMonthly := storageCost + requestCost

	log.Printf("[Pricing] S3 cost: storage=%.1fGB, requests=%d/day, total=%.2f/mo",
		config.StorageGB, config.RequestsPerDay, totalMonthly)

	return totalMonthly
}

// CalculateALBPrice calculates monthly cost for Application Load Balancer
// Includes fixed hourly price + LCU-based pricing
//
// @param estimatedLCUs - Estimated Load Balancer Capacity Units per hour
// @param rates - Current pricing rates from cache
// @return Monthly cost in USD
func CalculateALBPrice(estimatedLCUs float64, rates *PriceRates) float64 {
	// Fixed hourly price
	fixedHourlyCost := rates.ALB.HourlyPrice

	// LCU-based cost
	lcuHourlyCost := estimatedLCUs * rates.ALB.LCUPrice

	// Total monthly cost
	totalHourlyCost := fixedHourlyCost + lcuHourlyCost
	monthlyPrice := totalHourlyCost * HoursPerMonth

	return monthlyPrice
}

// CalculateAPIGatewayPrice calculates monthly cost for API Gateway
// Based on number of requests
//
// @param monthlyRequests - Number of requests per month
// @param rates - Current pricing rates from cache
// @return Monthly cost in USD
func CalculateAPIGatewayPrice(monthlyRequests int, rates *PriceRates) float64 {
	millionRequests := float64(monthlyRequests) / 1000000.0
	monthlyPrice := millionRequests * rates.APIGateway.RequestsPerMillion

	return monthlyPrice
}

// CalculateCloudWatchPrice calculates monthly cost for CloudWatch logs
// Based on GB of logs ingested
//
// @param logsGB - GB of logs ingested per month
// @param rates - Current pricing rates from cache
// @return Monthly cost in USD
func CalculateCloudWatchPrice(logsGB float64, rates *PriceRates) float64 {
	monthlyPrice := logsGB * rates.CloudWatch.LogsIngestionPerGB
	return monthlyPrice
}

// CalculateRoute53Price calculates monthly cost for Route 53
// Includes hosted zone cost + query cost
//
// @param hostedZones - Number of hosted zones
// @param monthlyQueries - Number of DNS queries per month
// @param rates - Current pricing rates from cache
// @return Monthly cost in USD
func CalculateRoute53Price(hostedZones int, monthlyQueries int, rates *PriceRates) float64 {
	// Hosted zone cost
	hostedZoneCost := float64(hostedZones) * rates.Route53.HostedZonePerMonth

	// Query cost
	millionQueries := float64(monthlyQueries) / 1000000.0
	queryCost := millionQueries * rates.Route53.QueriesPerMillion

	totalMonthly := hostedZoneCost + queryCost
	return totalMonthly
}

// CalculateCognitoPrice calculates monthly cost for Cognito
// Includes free tier (first 50k MAUs free)
//
// @param monthlyActiveUsers - Number of monthly active users
// @param rates - Current pricing rates from cache
// @return Monthly cost in USD
func CalculateCognitoPrice(monthlyActiveUsers int, rates *PriceRates) float64 {
	// Calculate billable MAUs (after free tier)
	billableMAUs := monthlyActiveUsers - rates.Cognito.FreeMAUs
	if billableMAUs < 0 {
		billableMAUs = 0
	}

	monthlyPrice := float64(billableMAUs) * rates.Cognito.MAUPrice
	return monthlyPrice
}

// CalculateSESPrice calculates monthly cost for Simple Email Service
//
// @param monthlyEmails - Number of emails sent per month
// @param rates - Current pricing rates from cache
// @return Monthly cost in USD
func CalculateSESPrice(monthlyEmails int, rates *PriceRates) float64 {
	thousandEmails := float64(monthlyEmails) / 1000.0
	monthlyPrice := thousandEmails * rates.SES.Per1000Emails
	return monthlyPrice
}

// CalculateEventBridgePrice calculates monthly cost for EventBridge
//
// @param monthlyEvents - Number of events per month
// @param rates - Current pricing rates from cache
// @return Monthly cost in USD
func CalculateEventBridgePrice(monthlyEvents int, rates *PriceRates) float64 {
	millionEvents := float64(monthlyEvents) / 1000000.0
	monthlyPrice := millionEvents * rates.EventBridge.EventsPerMillion
	return monthlyPrice
}

// CalculateECRPrice calculates monthly cost for Elastic Container Registry
//
// @param storageGB - GB of container images stored
// @param rates - Current pricing rates from cache
// @return Monthly cost in USD
func CalculateECRPrice(storageGB float64, rates *PriceRates) float64 {
	monthlyPrice := storageGB * rates.ECR.StoragePerGBMonth
	return monthlyPrice
}
