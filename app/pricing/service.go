package pricing

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
)

// Service is the main pricing service orchestrator
// Provides centralized access to AWS pricing data and calculations
// Thread-safe for concurrent use
type Service struct {
	cache  *PriceCache
	client *AWSPricingClient
}

// NewService creates a new pricing service with background refresh
// Initializes AWS Pricing API client, cache, and background refresh goroutine
//
// @param ctx - Context for lifecycle management
// @param regions - List of AWS regions to cache pricing for
// @return Initialized pricing service
func NewService(ctx context.Context, regions []string) (*Service, error) {
	// Initialize AWS Pricing API client
	// Note: AWS Pricing API is only available in us-east-1 and ap-south-1
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		// Continue with fallback prices only (no warning needed - fallback is intentional)
	}

	// Create AWS client with retry and fallback
	awsClient := NewAWSPricingClient(pricing.NewFromConfig(cfg))

	// Create cache with 24-hour TTL
	cache := NewPriceCache(awsClient, 24*time.Hour)

	// Create service
	service := &Service{
		cache:  cache,
		client: awsClient,
	}

	// Pre-warm cache for all regions (silently)
	for _, region := range regions {
		cache.Get(region) // Ignore errors - fallback always works
	}

	// Start background refresh (every 12 hours)
	cache.StartBackgroundRefresh(ctx, regions, 12*time.Hour)

	return service, nil
}

// GetRates returns cached pricing rates for a region
// Automatically refreshes if cache is stale
//
// @param region - AWS region (e.g., "us-east-1")
// @return Pricing rates for the region
func (s *Service) GetRates(region string) (*PriceRates, error) {
	return s.cache.Get(region)
}

// GetCacheMetrics returns current cache performance metrics
// Useful for monitoring and debugging
func (s *Service) GetCacheMetrics() CacheMetrics {
	return s.cache.GetMetrics()
}

// CalculateRDS calculates RDS pricing for given configuration
// Uses cached pricing rates
func (s *Service) CalculateRDS(region string, config RDSConfig) (float64, error) {
	rates, err := s.GetRates(region)
	if err != nil {
		return 0, fmt.Errorf("failed to get rates: %w", err)
	}
	return CalculateRDSPrice(config, rates), nil
}

// CalculateAurora calculates Aurora Serverless v2 pricing
// Uses cached pricing rates
func (s *Service) CalculateAurora(region string, config AuroraConfig) (float64, error) {
	rates, err := s.GetRates(region)
	if err != nil {
		return 0, fmt.Errorf("failed to get rates: %w", err)
	}
	return CalculateAuroraPrice(config, rates), nil
}

// CalculateECS calculates ECS Fargate pricing
// Uses cached pricing rates
func (s *Service) CalculateECS(region string, config ECSConfig) (float64, error) {
	rates, err := s.GetRates(region)
	if err != nil {
		return 0, fmt.Errorf("failed to get rates: %w", err)
	}
	return CalculateECSPrice(config, rates), nil
}

// CalculateS3 calculates S3 pricing
// Uses cached pricing rates
func (s *Service) CalculateS3(region string, config S3Config) (float64, error) {
	rates, err := s.GetRates(region)
	if err != nil {
		return 0, fmt.Errorf("failed to get rates: %w", err)
	}
	return CalculateS3Price(config, rates), nil
}

// RefreshRegion forces a refresh of pricing data for a specific region
// Useful when you know pricing has changed
func (s *Service) RefreshRegion(region string) error {
	s.cache.Invalidate(region)
	_, err := s.cache.Get(region)
	return err
}

// ClearCache clears all cached pricing data
// Useful for testing or forcing a full refresh
func (s *Service) ClearCache() {
	s.cache.Clear()
}
