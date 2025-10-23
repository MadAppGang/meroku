package main

import (
	"context"

	"madappgang.com/meroku/awsutil"
	pricingpkg "madappgang.com/meroku/pricing"
)

// AppContext holds application-wide state and dependencies
// This replaces global variables and enables dependency injection for better testing
type AppContext struct {
	// Context for cancellation and timeouts
	Context context.Context

	// Configuration state
	SelectedEnvironment string
	SelectedAWSProfile  string
	SelectedAWSRegion   string

	// Services
	PricingService *pricingpkg.Service
	AWSClientFactory *awsutil.ClientFactory

	// Version information
	Version string
}

// NewAppContext creates a new application context with the given base context
func NewAppContext(ctx context.Context) *AppContext {
	return &AppContext{
		Context: ctx,
	}
}

// WithEnvironment sets the selected environment
func (a *AppContext) WithEnvironment(env string) *AppContext {
	a.SelectedEnvironment = env
	return a
}

// WithAWSProfile sets the selected AWS profile
func (a *AppContext) WithAWSProfile(profile string) *AppContext {
	a.SelectedAWSProfile = profile
	return a
}

// WithRegion sets the selected AWS region
func (a *AppContext) WithRegion(region string) *AppContext {
	a.SelectedAWSRegion = region
	return a
}

// WithPricingService sets the pricing service
func (a *AppContext) WithPricingService(svc *pricingpkg.Service) *AppContext {
	a.PricingService = svc
	return a
}

// WithAWSClientFactory sets the AWS client factory
func (a *AppContext) WithAWSClientFactory(factory *awsutil.ClientFactory) *AppContext {
	a.AWSClientFactory = factory
	return a
}

// WithVersion sets the version string
func (a *AppContext) WithVersion(version string) *AppContext {
	a.Version = version
	return a
}

// GetEnvironment returns the selected environment
func (a *AppContext) GetEnvironment() string {
	return a.SelectedEnvironment
}

// GetAWSProfile returns the selected AWS profile
func (a *AppContext) GetAWSProfile() string {
	return a.SelectedAWSProfile
}

// GetRegion returns the selected AWS region
func (a *AppContext) GetRegion() string {
	return a.SelectedAWSRegion
}
