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

	// Storage pricing
	Storage StoragePricing `json:"storage"` // EBS, gp3
	S3      S3Pricing      `json:"s3"`      // S3 storage and requests

	// Networking pricing
	ALB        ALBPricing        `json:"alb"`
	APIGateway APIGatewayPricing `json:"apiGateway"`
	NATGateway NATGatewayPricing `json:"natGateway"`

	// Other services
	CloudWatch CloudWatchPricing `json:"cloudWatch"`
	Route53    Route53Pricing    `json:"route53"`
	Cognito    CognitoPricing    `json:"cognito"`
	SES        SESPricing        `json:"ses"`
	EventBridge EventBridgePricing `json:"eventBridge"`
	ECR        ECRPricing         `json:"ecr"`
}

// AuroraPricing holds Aurora Serverless v2 pricing
type AuroraPricing struct {
	ACUHourly       float64 `json:"acuHourly"`       // $/ACU/hour (e.g., 0.12)
	StorageGBMonth  float64 `json:"storageGbMonth"`  // $/GB/month
	IORequestsPerM  float64 `json:"ioRequestsPerM"`  // $/million I/O requests
}

// FargatePricing holds ECS Fargate pricing
type FargatePricing struct {
	VCPUHourly     float64 `json:"vcpuHourly"`     // $/vCPU/hour (e.g., 0.04048)
	MemoryGBHourly float64 `json:"memoryGbHourly"` // $/GB/hour (e.g., 0.004445)
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
	HostedZonePerMonth   float64 `json:"hostedZonePerMonth"`   // $/zone/month (e.g., 0.50)
	QueriesPerMillion    float64 `json:"queriesPerMillion"`    // $/million queries (e.g., 0.40)
}

// CognitoPricing holds Cognito pricing
type CognitoPricing struct {
	MAUPrice     float64 `json:"mauPrice"`     // $/MAU (e.g., 0.0055)
	FreeMAUs     int     `json:"freeMAUs"`     // Free tier MAUs (e.g., 50000)
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
