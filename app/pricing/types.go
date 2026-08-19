package pricing

import "time"

// PriceRates represents all AWS pricing data for a specific region
// This is the single source of truth for all pricing calculations
type PriceRates struct {
	Region      string    `json:"region"`
	LastUpdate  time.Time `json:"lastUpdate"`  // When data was cached/fetched
	Source      string    `json:"source"`      // "aws_api" or "fallback"
	PricingDate string    `json:"pricingDate"` // When pricing was sourced (e.g., "2025-01-15")

	// Compute pricing
	RDS     map[string]float64 `json:"rds"`     // Instance type -> hourly price
	Aurora  AuroraPricing      `json:"aurora"`  // Aurora Serverless v2
	Fargate FargatePricing     `json:"fargate"` // ECS Fargate
	EC2     EC2Pricing         `json:"ec2"`     // ECS on EC2 capacity pools

	// Storage pricing
	Storage StoragePricing `json:"storage"` // EBS, gp3
	S3      S3Pricing      `json:"s3"`      // S3 storage and requests

	// Networking pricing
	ALB        ALBPricing        `json:"alb"`
	APIGateway APIGatewayPricing `json:"apiGateway"`
	NATGateway NATGatewayPricing `json:"natGateway"`

	// Other services
	CloudWatch  CloudWatchPricing  `json:"cloudWatch"`
	Route53     Route53Pricing     `json:"route53"`
	Cognito     CognitoPricing     `json:"cognito"`
	SES         SESPricing         `json:"ses"`
	EventBridge EventBridgePricing `json:"eventBridge"`
	ECR         ECRPricing         `json:"ecr"`
}

// AuroraPricing holds Aurora Serverless v2 pricing
type AuroraPricing struct {
	ACUHourly      float64 `json:"acuHourly"`      // $/ACU/hour (e.g., 0.12)
	StorageGBMonth float64 `json:"storageGbMonth"` // $/GB/month
	IORequestsPerM float64 `json:"ioRequestsPerM"` // $/million I/O requests
}

// FargatePricing holds ECS Fargate pricing
type FargatePricing struct {
	VCPUHourly     float64 `json:"vcpuHourly"`     // $/vCPU/hour (e.g., 0.04048)
	MemoryGBHourly float64 `json:"memoryGbHourly"` // $/GB/hour (e.g., 0.004445)
}

// EC2Pricing holds the pricing for the container instances that back an ECS
// capacity pool.
//
// The unit is the whole point of this type, and it is different from every
// other compute price in this file: Fargate is billed per TASK (vCPU-hour plus
// GB-hour of the task's own reservation), EC2 is billed per INSTANCE-hour
// whether or not a task is running on it. A pool at min_size: 1 with zero tasks
// costs a full instance-hour every hour. Pricing an EC2-runtime service the way
// Fargate is priced bills one instance once per task and shows a number several
// times too high, which is why CalculateEC2PoolPrice takes a pool and not a
// task.
type EC2Pricing struct {
	// OnDemandHourly maps instance type -> $/instance-hour, Linux, shared
	// tenancy, on-demand list price.
	//
	// An absent key means "price unknown" and MUST NOT be read as free: a
	// missing entry returns ok=false from EC2PoolHourly so the caller can
	// render "price unknown" rather than $0. A zero value would instead say
	// the instance is free, which is never true.
	OnDemandHourly map[string]float64 `json:"onDemandHourly"`

	// SpotRatio is the spot price expressed as a FRACTION OF the on-demand
	// price for the same instance type, e.g. 0.35 means "spot typically costs
	// 35% of on-demand" (a 65% discount). It is an indicative planning figure
	// for the cost view, not a quote: real spot prices vary per type, per AZ
	// and by the minute, and the compute endpoints read them live from
	// DescribeSpotPriceHistory.
	//
	// A value <= 0 or > 1 is not usable and is treated as 1.0 (spot priced as
	// on-demand) rather than as a discount, so a malformed rate set can never
	// make capacity look free.
	SpotRatio float64 `json:"spotRatio"`
}

// StoragePricing holds EBS storage pricing
type StoragePricing struct {
	GP3PerGBMonth float64 `json:"gp3PerGbMonth"` // $/GB/month (e.g., 0.115)
	GP2PerGBMonth float64 `json:"gp2PerGbMonth"` // $/GB/month
}

// S3Pricing holds S3 pricing
type S3Pricing struct {
	StandardPerGBMonth float64 `json:"standardPerGbMonth"` // $/GB/month (e.g., 0.023)
	RequestsPer1000    float64 `json:"requestsPer1000"`    // $/1000 requests
}

// ALBPricing holds Application Load Balancer pricing
type ALBPricing struct {
	HourlyPrice float64 `json:"hourlyPrice"` // $/hour (e.g., 0.0225)
	LCUPrice    float64 `json:"lcuPrice"`    // $/LCU/hour (e.g., 0.008)
}

// APIGatewayPricing holds API Gateway pricing
type APIGatewayPricing struct {
	RequestsPerMillion float64 `json:"requestsPerMillion"` // $/million requests (e.g., 3.50)
}

// NATGatewayPricing holds NAT Gateway pricing
type NATGatewayPricing struct {
	HourlyPrice    float64 `json:"hourlyPrice"`    // $/hour
	DataPerGBMonth float64 `json:"dataPerGbMonth"` // $/GB processed
}

// CloudWatchPricing holds CloudWatch pricing
type CloudWatchPricing struct {
	LogsIngestionPerGB float64 `json:"logsIngestionPerGb"` // $/GB ingested (e.g., 0.50)
	MetricsPerMetric   float64 `json:"metricsPerMetric"`   // $/metric/month
}

// Route53Pricing holds Route 53 pricing
type Route53Pricing struct {
	HostedZonePerMonth float64 `json:"hostedZonePerMonth"` // $/zone/month (e.g., 0.50)
	QueriesPerMillion  float64 `json:"queriesPerMillion"`  // $/million queries (e.g., 0.40)
}

// CognitoPricing holds Cognito pricing
type CognitoPricing struct {
	MAUPrice float64 `json:"mauPrice"` // $/MAU (e.g., 0.0055)
	FreeMAUs int     `json:"freeMAUs"` // Free tier MAUs (e.g., 50000)
}

// SESPricing holds Simple Email Service pricing
type SESPricing struct {
	Per1000Emails float64 `json:"per1000Emails"` // $/1000 emails (e.g., 0.10)
}

// EventBridgePricing holds EventBridge pricing
type EventBridgePricing struct {
	EventsPerMillion float64 `json:"eventsPerMillion"` // $/million events (e.g., 1.00)
}

// ECRPricing holds Elastic Container Registry pricing
type ECRPricing struct {
	StoragePerGBMonth float64 `json:"storagePerGbMonth"` // $/GB/month (e.g., 0.10)
}

// Configuration types for calculators

// RDSConfig holds RDS instance configuration
type RDSConfig struct {
	InstanceClass    string `json:"instanceClass"`
	AllocatedStorage int    `json:"allocatedStorage"`
	MultiAZ          bool   `json:"multiAz"`
	Engine           string `json:"engine"` // "postgres", "mysql"
}

// AuroraConfig holds Aurora Serverless v2 configuration
type AuroraConfig struct {
	MinCapacity int    `json:"minCapacity"`
	MaxCapacity int    `json:"maxCapacity"`
	Level       string `json:"level"` // "startup", "scaleup", "highload"
}

// ECSConfig holds ECS Fargate configuration
type ECSConfig struct {
	CPU          int `json:"cpu"`          // CPU units (e.g., 256, 512, 1024)
	Memory       int `json:"memory"`       // Memory in MB (e.g., 512, 1024, 2048)
	DesiredCount int `json:"desiredCount"` // Number of tasks
}

// EC2PoolConfig holds the cost-relevant shape of one EC2 capacity pool.
//
// It is deliberately a projection of the pool's YAML rather than the pool
// struct itself: this package is imported by the API layer and by the
// frontend's mirror, and neither should have to know about AMI families, user
// data or volumes to ask what a pool costs.
type EC2PoolConfig struct {
	// InstanceTypes is the pool's type list in priority order. The first entry
	// with a known price is the costing basis -- an ASG with a mixed-instances
	// policy may launch any of them, so a single figure is an estimate by
	// construction, and taking the first priced entry keeps it deterministic.
	InstanceTypes []string `json:"instanceTypes"`

	// InstanceCount is the number of INSTANCES the pool runs. 0 is valid and
	// costs nothing: a pool scaled to zero is off.
	InstanceCount int `json:"instanceCount"`

	// CapacityType is on_demand | spot | spot_with_base. An unknown value is
	// treated as on_demand, the most expensive reading, so a typo never
	// under-reports cost.
	CapacityType string `json:"capacityType"`

	// OnDemandBase counts INSTANCES, not tasks: the on-demand base capacity
	// held before the spot allocation applies to anything above it. It is read
	// only when CapacityType is spot_with_base, matching
	// modules/workloads/ec2_capacity.tf's local.pool_on_demand_base.
	OnDemandBase int `json:"onDemandBase"`
}

// S3Config holds S3 configuration
type S3Config struct {
	StorageGB      float64 `json:"storageGb"`
	RequestsPerDay int     `json:"requestsPerDay"`
}

// EnvironmentCost represents the total cost breakdown for an environment
type EnvironmentCost struct {
	Region       string             `json:"region"`
	TotalMonthly float64            `json:"totalMonthly"`
	Services     map[string]float64 `json:"services"` // service name -> monthly cost
	LastUpdated  time.Time          `json:"lastUpdated"`
}

// CacheMetrics tracks cache performance
type CacheMetrics struct {
	Hits        int64     `json:"hits"`
	Misses      int64     `json:"misses"`
	LastRefresh time.Time `json:"lastRefresh"`
	Errors      int64     `json:"errors"`
}

// Constants for calculations
const (
	HoursPerMonth = 730 // Standard hours per month for cost calculations
)
