package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"gopkg.in/yaml.v2"
)

// S3BucketInfo represents information about an S3 bucket
type S3BucketInfo struct {
	Name           string        `json:"name"`
	Type           string        `json:"type"` // "static" or "configured"
	PublicAccess   bool          `json:"publicAccess"`
	Versioning     string        `json:"versioning"`
	CORSRules      []CORSRule    `json:"corsRules,omitempty"`
	ConsoleURL     string        `json:"consoleUrl"`
	Region         string        `json:"region"`
	CreationDate   *string       `json:"creationDate,omitempty"`
}

// CORSRule represents S3 CORS configuration
type CORSRule struct {
	AllowedHeaders []string `json:"allowedHeaders"`
	AllowedMethods []string `json:"allowedMethods"`
	AllowedOrigins []string `json:"allowedOrigins"`
	ExposeHeaders  []string `json:"exposeHeaders"`
	MaxAgeSeconds  int32    `json:"maxAgeSeconds"`
}

// BucketConfig represents bucket configuration from YAML
type BucketConfig struct {
	Name       string       `yaml:"name"`
	Public     bool         `yaml:"public"`
	Versioning *bool        `yaml:"versioning"`
	CORSRules  []CORSConfig `yaml:"cors_rules"`
}

// CORSConfig represents CORS configuration from YAML
type CORSConfig struct {
	AllowedHeaders []string `yaml:"allowed_headers"`
	AllowedMethods []string `yaml:"allowed_methods"`
	AllowedOrigins []string `yaml:"allowed_origins"`
	ExposeHeaders  []string `yaml:"expose_headers"`
	MaxAgeSeconds  int32    `yaml:"max_age_seconds"`
}

// listBuckets returns information about S3 buckets for the project
func listBuckets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get environment from query parameter
	envName := r.URL.Query().Get("env")
	if envName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env parameter is required"})
		return
	}

	// Load environment config
	filename := fmt.Sprintf("%s.yaml", envName)
	content, err := os.ReadFile(filename)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to load environment config"})
		return
	}

	var envConfig Env
	if err := yaml.Unmarshal(content, &envConfig); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to parse environment config"})
		return
	}

	// Load AWS configuration
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Failed to load AWS config: %v", err)})
		return
	}

	// Create S3 client
	s3Client := s3.NewFromConfig(cfg)

	buckets := []S3BucketInfo{}

	// Add the static backend bucket
	backendBucketName := fmt.Sprintf("%s-backend-%s", envConfig.Project, envName)
	if envConfig.Workload.BucketPostfix != "" {
		backendBucketName += envConfig.Workload.BucketPostfix
	}

	// Get backend bucket info
	backendBucket := S3BucketInfo{
		Name:         backendBucketName,
		Type:         "static",
		PublicAccess: envConfig.Workload.BucketPublic,
		Versioning:   "Enabled", // Default for backend bucket
		ConsoleURL:   fmt.Sprintf("https://s3.console.aws.amazon.com/s3/buckets/%s?region=%s", backendBucketName, cfg.Region),
		Region:       cfg.Region,
	}

	// Check if backend bucket exists and get its info
	if bucketInfo := getBucketInfo(ctx, s3Client, backendBucketName); bucketInfo != nil {
		backendBucket.Versioning = bucketInfo.Versioning
		if bucketInfo.CORSRules != nil {
			backendBucket.CORSRules = bucketInfo.CORSRules
		}
	}

	buckets = append(buckets, backendBucket)

	// Add configured buckets
	for _, bucketCfg := range envConfig.Buckets {
		bucketName := fmt.Sprintf("%s-%s-%s", envConfig.Project, bucketCfg.Name, envName)
		
		bucket := S3BucketInfo{
			Name:         bucketName,
			Type:         "configured",
			PublicAccess: bucketCfg.Public,
			Versioning:   "Enabled", // Default
			ConsoleURL:   fmt.Sprintf("https://s3.console.aws.amazon.com/s3/buckets/%s?region=%s", bucketName, cfg.Region),
			Region:       cfg.Region,
		}

		// Handle versioning configuration
		if bucketCfg.Versioning != nil && !*bucketCfg.Versioning {
			bucket.Versioning = "Disabled"
		}

		// Convert CORS rules
		if len(bucketCfg.CORSRules) > 0 {
			for _, rule := range bucketCfg.CORSRules {
				corsRule := CORSRule{
					AllowedHeaders: rule.AllowedHeaders,
					AllowedMethods: rule.AllowedMethods,
					AllowedOrigins: rule.AllowedOrigins,
					ExposeHeaders:  rule.ExposeHeaders,
					MaxAgeSeconds:  rule.MaxAgeSeconds,
				}
				// Set defaults if not specified
				if len(corsRule.AllowedHeaders) == 0 {
					corsRule.AllowedHeaders = []string{"*"}
				}
				if len(corsRule.AllowedMethods) == 0 {
					corsRule.AllowedMethods = []string{"GET", "PUT", "POST", "DELETE", "HEAD"}
				}
				if len(corsRule.AllowedOrigins) == 0 {
					corsRule.AllowedOrigins = []string{"*"}
				}
				if len(corsRule.ExposeHeaders) == 0 {
					corsRule.ExposeHeaders = []string{"ETag"}
				}
				if corsRule.MaxAgeSeconds == 0 {
					corsRule.MaxAgeSeconds = 3600
				}
				bucket.CORSRules = append(bucket.CORSRules, corsRule)
			}
		}

		// Check if bucket exists and get its actual info
		if bucketInfo := getBucketInfo(ctx, s3Client, bucketName); bucketInfo != nil {
			bucket.Versioning = bucketInfo.Versioning
			if bucketInfo.CreationDate != nil {
				bucket.CreationDate = bucketInfo.CreationDate
			}
			// Override with actual CORS if bucket exists
			if bucketInfo.CORSRules != nil {
				bucket.CORSRules = bucketInfo.CORSRules
			}
		}

		buckets = append(buckets, bucket)
	}

	// Also check for any other buckets that match the project pattern
	listOutput, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err == nil && listOutput != nil {
		projectPrefix := fmt.Sprintf("%s-", envConfig.Project)
		envSuffix := fmt.Sprintf("-%s", envName)
		
		for _, bucket := range listOutput.Buckets {
			if bucket.Name != nil && strings.HasPrefix(*bucket.Name, projectPrefix) && strings.HasSuffix(*bucket.Name, envSuffix) {
				// Check if we already have this bucket
				found := false
				for _, b := range buckets {
					if b.Name == *bucket.Name {
						found = true
						break
					}
				}
				
				if !found {
					// This is an additional bucket not in config
					additionalBucket := S3BucketInfo{
						Name:         *bucket.Name,
						Type:         "configured",
						PublicAccess: false, // Default to private
						Versioning:   "Unknown",
						ConsoleURL:   fmt.Sprintf("https://s3.console.aws.amazon.com/s3/buckets/%s?region=%s", *bucket.Name, cfg.Region),
						Region:       cfg.Region,
					}
					
					if bucket.CreationDate != nil {
						creationDate := bucket.CreationDate.Format("2006-01-02T15:04:05Z")
						additionalBucket.CreationDate = &creationDate
					}
					
					// Get detailed info
					if bucketInfo := getBucketInfo(ctx, s3Client, *bucket.Name); bucketInfo != nil {
						additionalBucket.Versioning = bucketInfo.Versioning
						additionalBucket.PublicAccess = bucketInfo.PublicAccess
						if bucketInfo.CORSRules != nil {
							additionalBucket.CORSRules = bucketInfo.CORSRules
						}
					}
					
					buckets = append(buckets, additionalBucket)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(buckets)
}

// BucketDetailedInfo holds detailed information about a bucket
type BucketDetailedInfo struct {
	Versioning   string
	PublicAccess bool
	CORSRules    []CORSRule
	CreationDate *string
}

// getBucketInfo retrieves detailed information about a specific bucket
func getBucketInfo(ctx context.Context, client *s3.Client, bucketName string) *BucketDetailedInfo {
	info := &BucketDetailedInfo{
		Versioning: "Unknown",
	}

	// Check if bucket exists by getting its location
	_, err := client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{
		Bucket: &bucketName,
	})
	if err != nil {
		// Bucket doesn't exist
		return nil
	}

	// Get versioning status
	versioningOutput, err := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{
		Bucket: &bucketName,
	})
	if err == nil && versioningOutput != nil {
		if versioningOutput.Status == s3types.BucketVersioningStatusEnabled {
			info.Versioning = "Enabled"
		} else if versioningOutput.Status == s3types.BucketVersioningStatusSuspended {
			info.Versioning = "Suspended"
		} else {
			info.Versioning = "Disabled"
		}
	}

	// Check public access block
	pabOutput, err := client.GetPublicAccessBlock(ctx, &s3.GetPublicAccessBlockInput{
		Bucket: &bucketName,
	})
	if err == nil && pabOutput != nil && pabOutput.PublicAccessBlockConfiguration != nil {
		// If all public access is blocked, it's private
		cfg := pabOutput.PublicAccessBlockConfiguration
		info.PublicAccess = !(cfg.BlockPublicAcls != nil && *cfg.BlockPublicAcls &&
			cfg.BlockPublicPolicy != nil && *cfg.BlockPublicPolicy &&
			cfg.IgnorePublicAcls != nil && *cfg.IgnorePublicAcls &&
			cfg.RestrictPublicBuckets != nil && *cfg.RestrictPublicBuckets)
	}

	// Get CORS configuration
	corsOutput, err := client.GetBucketCors(ctx, &s3.GetBucketCorsInput{
		Bucket: &bucketName,
	})
	if err == nil && corsOutput != nil && len(corsOutput.CORSRules) > 0 {
		for _, rule := range corsOutput.CORSRules {
			corsRule := CORSRule{}
			
			if rule.AllowedHeaders != nil {
				corsRule.AllowedHeaders = rule.AllowedHeaders
			}
			if rule.AllowedMethods != nil {
				corsRule.AllowedMethods = rule.AllowedMethods
			}
			if rule.AllowedOrigins != nil {
				corsRule.AllowedOrigins = rule.AllowedOrigins
			}
			if rule.ExposeHeaders != nil {
				corsRule.ExposeHeaders = rule.ExposeHeaders
			}
			if rule.MaxAgeSeconds != nil {
				corsRule.MaxAgeSeconds = *rule.MaxAgeSeconds
			}
			
			info.CORSRules = append(info.CORSRules, corsRule)
		}
	}

	return info
}