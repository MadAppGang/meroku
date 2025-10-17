package pricing

import (
	"log"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/pricing"
)

// FALLBACK_PRICING_DATE tracks when fallback prices were last updated
// Update this date when refreshing prices (see docs/PRICING_MAINTENANCE.md)
const FALLBACK_PRICING_DATE = "2025-01-15"

// AWSPricingClient wraps AWS Pricing API with fallback to hardcoded prices
// Ensures pricing data is always available
//
// Current Strategy: Use hardcoded fallback prices (updated quarterly)
// Future: Can implement real-time AWS Pricing API integration if needed
type AWSPricingClient struct {
	client         *pricing.Client
	fallbackRates  *PriceRates
	loggedFallback sync.Once // Only log fallback message once
}

// NewAWSPricingClient creates a new AWS Pricing API client
// client: AWS SDK pricing client (configured for us-east-1)
func NewAWSPricingClient(client *pricing.Client) *AWSPricingClient {
	return &AWSPricingClient{
		client:        client,
		fallbackRates: getHardcodedFallbackRates(),
	}
}

// FetchRates fetches pricing data for the given region
// Currently returns hardcoded fallback prices (intentional design choice)
// Always returns valid pricing data (never fails)
func (c *AWSPricingClient) FetchRates(region string) (*PriceRates, error) {
	// Log once on first use to inform about pricing source
	c.loggedFallback.Do(func() {
		log.Printf("[Pricing] Using fallback pricing (last updated: %s)", FALLBACK_PRICING_DATE)
		log.Printf("[Pricing] Prices are accurate as of January 2025 and updated quarterly")
	})

	// Return fallback prices
	// Note: AWS Pricing API integration is deferred for future implementation
	// Rationale: AWS pricing changes infrequently (~1-2 times per year)
	// Quarterly manual updates are sufficient and more reliable
	fallback := c.getFallbackForRegion(region)
	fallback.Source = "fallback"
	fallback.LastUpdate = time.Now()

	return fallback, nil
}

// fetchRatesOnce would implement real-time AWS Pricing API calls
// Kept as placeholder for future implementation
//
// Implementation notes:
// - AWS Pricing API is only available in us-east-1 and ap-south-1
// - Requires complex JSON parsing for each service
// - Different services have different API structures
// - Estimated implementation time: 20-30 hours
//
// Example implementation:
//   input := &pricing.GetProductsInput{
//       ServiceCode: aws.String("AmazonRDS"),
//       Filters: []types.Filter{
//           {Type: types.FilterTypeTermMatch, Field: aws.String("location"), Value: aws.String(region)},
//       },
//   }
//   result, err := c.client.GetProducts(context.Background(), input)
//   // Parse complex JSON response...
func (c *AWSPricingClient) fetchRatesOnce(region string) (*PriceRates, error) {
	// Not implemented - using fallback prices intentionally
	return nil, ErrNotImplemented
}

// getFallbackForRegion returns hardcoded fallback prices for a region
// These prices are based on us-east-1 pricing as of January 2025
func (c *AWSPricingClient) getFallbackForRegion(region string) *PriceRates {
	// Clone the fallback rates
	rates := *c.fallbackRates
	rates.Region = region
	return &rates
}

// getHardcodedFallbackRates returns hardcoded AWS pricing
//
// Pricing Source: AWS Public Pricing (us-east-1)
// Last Updated: January 15, 2025
// Next Review: April 15, 2025 (quarterly)
//
// Maintenance: Update prices quarterly using docs/PRICING_MAINTENANCE.md
// These prices are used intentionally as the primary pricing source
func getHardcodedFallbackRates() *PriceRates {
	return &PriceRates{
		Region:      "us-east-1",
		Source:      "fallback",
		PricingDate: FALLBACK_PRICING_DATE,

		// RDS Instance Pricing (hourly rates, Single-AZ)
		RDS: map[string]float64{
			// T4g instances (ARM-based, cheapest)
			"db.t4g.micro":   0.016,
			"db.t4g.small":   0.032,
			"db.t4g.medium":  0.065,
			"db.t4g.large":   0.129,

			// T3 instances (x86-based)
			"db.t3.micro":    0.018,
			"db.t3.small":    0.036,
			"db.t3.medium":   0.073,

			// M6i instances (general purpose)
			"db.m6i.large":   0.178,
			"db.m6i.xlarge":  0.356,
			"db.m6i.2xlarge": 0.712,

			// M5 instances (general purpose, previous gen)
			"db.m5.large":    0.192,
			"db.m5.xlarge":   0.384,

			// R6i instances (memory optimized)
			"db.r6i.large":   0.240,
			"db.r6i.xlarge":  0.480,

			// R5 instances (memory optimized, previous gen)
			"db.r5.large":    0.260,
			"db.r5.xlarge":   0.520,
		},

		// Aurora Serverless v2 Pricing
		Aurora: AuroraPricing{
			ACUHourly:      0.12,   // $/ACU/hour
			StorageGBMonth: 0.10,   // $/GB/month
			IORequestsPerM: 0.20,   // $/million I/O requests
		},

		// Fargate Pricing
		Fargate: FargatePricing{
			VCPUHourly:     0.04048,  // $/vCPU/hour
			MemoryGBHourly: 0.004445, // $/GB/hour
		},

		// Storage Pricing
		Storage: StoragePricing{
			GP3PerGBMonth: 0.115, // $/GB/month (gp3)
			GP2PerGBMonth: 0.10,  // $/GB/month (gp2)
		},

		// S3 Pricing
		S3: S3Pricing{
			StandardPerGBMonth: 0.023,  // $/GB/month (standard)
			RequestsPer1000:    0.0004, // $/1000 PUT/POST requests
		},

		// ALB Pricing
		ALB: ALBPricing{
			HourlyPrice: 0.0225, // $/hour
			LCUPrice:    0.008,  // $/LCU/hour
		},

		// API Gateway Pricing
		APIGateway: APIGatewayPricing{
			RequestsPerMillion: 3.50, // $/million requests (REST API)
		},

		// NAT Gateway Pricing
		NATGateway: NATGatewayPricing{
			HourlyPrice:    0.045, // $/hour
			DataPerGBMonth: 0.045, // $/GB processed
		},

		// CloudWatch Pricing
		CloudWatch: CloudWatchPricing{
			LogsIngestionPerGB: 0.50,  // $/GB ingested
			MetricsPerMetric:   0.30,  // $/metric/month (custom metrics)
		},

		// Route53 Pricing
		Route53: Route53Pricing{
			HostedZonePerMonth: 0.50, // $/hosted zone/month
			QueriesPerMillion:  0.40, // $/million queries
		},

		// Cognito Pricing
		Cognito: CognitoPricing{
			MAUPrice: 0.0055,  // $/MAU (after free tier)
			FreeMAUs: 50000,   // First 50k MAUs free
		},

		// SES Pricing
		SES: SESPricing{
			Per1000Emails: 0.10, // $/1000 emails
		},

		// EventBridge Pricing
		EventBridge: EventBridgePricing{
			EventsPerMillion: 1.00, // $/million events
		},

		// ECR Pricing
		ECR: ECRPricing{
			StoragePerGBMonth: 0.10, // $/GB/month
		},
	}
}

// Custom errors
var (
	ErrNotImplemented = &PricingError{"AWS Pricing API integration not yet implemented"}
)

// PricingError represents a pricing-related error
type PricingError struct {
	Message string
}

func (e *PricingError) Error() string {
	return e.Message
}
