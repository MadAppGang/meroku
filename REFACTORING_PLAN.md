# Meroku GoLang Application - Comprehensive Refactoring Plan

**Date:** 2025-10-23
**Status:** Ready for Implementation
**Estimated Effort:** High (significant refactoring)

## Executive Summary

The Meroku application is a mature, feature-rich infrastructure management tool with excellent functionality. However, the codebase has accumulated technical debt that impacts maintainability, testability, and extensibility. This plan addresses these issues systematically.

## Code Analysis Summary

### Strengths
- ✅ Clear separation of concerns (TUI, API, business logic)
- ✅ Comprehensive AWS integration (15+ services)
- ✅ Advanced TUI capabilities with Bubble Tea
- ✅ Robust schema migration system
- ✅ Good error wrapping with context

### Critical Issues Identified

| Priority | Issue | Impact | Files Affected |
|----------|-------|--------|----------------|
| 🔴 HIGH | Global state management | Testing, concurrency, maintainability | 5 files |
| 🔴 HIGH | Repetitive API handler code | Maintenance, bugs | 17 API files |
| 🔴 HIGH | No dependency injection | Testing, flexibility | Entire app |
| 🟡 MEDIUM | File naming typo (`terrafrom.go`) | Confusion, professionalism | 1 file |
| 🟡 MEDIUM | Direct fmt.Printf usage (538 occurrences) | Logging, debugging | 32 files |
| 🟡 MEDIUM | Environment variable mutation | Side effects, testing | 14 locations |
| 🟡 MEDIUM | Large files (174KB, 111KB) | Navigation, maintainability | 2 files |
| 🟢 LOW | Missing interfaces for AWS clients | Testing, mocking | Multiple files |
| 🟢 LOW | Inconsistent error handling | Error reporting | Throughout |

---

## Detailed Refactoring Roadmap

### Phase 1: Critical Fixes (Quick Wins) ⚡

#### 1.1 Fix File Naming Typo
**Files:** `app/terrafrom.go` → `app/terraform.go`

```bash
# Simple file rename
git mv app/terrafrom.go app/terraform.go
```

**Impact:** Improves professionalism, fixes confusion
**Effort:** 5 minutes
**Risk:** Low (compile-time error if missed)

---

### Phase 2: Foundation Refactoring 🏗️

#### 2.1 Introduce Application Context Structure

**Problem:** Global variables scattered throughout:
```go
var selectedAWSProfile string
var selectedEnvironment string
var selectedAWSRegion string
var globalPricingService *pricingpkg.Service
```

**Solution:** Create centralized application context

**New File:** `app/context.go`
```go
package main

import (
	"context"
	pricingpkg "madappgang.com/meroku/pricing"
)

// AppContext holds application-wide state and dependencies
type AppContext struct {
	// Configuration
	SelectedEnvironment string
	SelectedAWSProfile  string
	SelectedAWSRegion   string

	// Services
	PricingService *pricingpkg.Service

	// Context for cancellation
	Context context.Context
}

// NewAppContext creates a new application context
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
```

**Migration Strategy:**
1. Create `AppContext` struct
2. Pass context through function parameters instead of globals
3. Gradually migrate functions one by one
4. Remove global variables last

**Files to Update:**
- `main.go` - Initialize AppContext
- `api.go` - Pass context to handlers
- `main_menu.go` - Accept context parameter
- All API handler files (17 files)

**Effort:** 4-6 hours
**Risk:** Medium (requires careful migration)

---

#### 2.2 Create HTTP Utilities Package

**Problem:** Repetitive HTTP handler boilerplate in 17 API files:
```go
// Repeated in EVERY handler
if r.Method != http.MethodGet {
    http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    return
}

w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(response)
```

**Solution:** Extract common utilities

**New File:** `app/httputil/response.go`
```go
package httputil

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error  string `json:"error"`
	Code   string `json:"code,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// RespondJSON sends a JSON response
func RespondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// RespondError sends a JSON error response
func RespondError(w http.ResponseWriter, status int, message string) {
	RespondJSON(w, status, ErrorResponse{Error: message})
}

// RespondErrorWithDetail sends a JSON error response with details
func RespondErrorWithDetail(w http.ResponseWriter, status int, message string, detail string) {
	RespondJSON(w, status, ErrorResponse{
		Error:  message,
		Detail: detail,
	})
}

// MethodGuard ensures only specified methods are allowed
func MethodGuard(allowedMethods ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			for _, method := range allowedMethods {
				if r.Method == method {
					next(w, r)
					return
				}
			}
			RespondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}
}

// WithCORS adds CORS headers to responses
func WithCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Requested-With")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "3600")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next(w, r)
	}
}
```

**New File:** `app/httputil/request.go`
```go
package httputil

import (
	"encoding/json"
	"io"
	"net/http"
)

// DecodeJSON decodes JSON request body into target struct
func DecodeJSON(r *http.Request, target interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

// ReadBody reads the entire request body
func ReadBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

// QueryParam gets a query parameter with a default value
func QueryParam(r *http.Request, key string, defaultValue string) string {
	value := r.URL.Query().Get(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// RequiredQueryParam gets a required query parameter
func RequiredQueryParam(w http.ResponseWriter, r *http.Request, key string) (string, bool) {
	value := r.URL.Query().Get(key)
	if value == "" {
		RespondError(w, http.StatusBadRequest, key+" parameter is required")
		return "", false
	}
	return value, true
}
```

**Usage Example (Before & After):**

**Before:**
```go
func getEnvironmentConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	envName := r.URL.Query().Get("name")
	if envName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name parameter is required"})
		return
	}

	// ... business logic ...

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ConfigResponse{Content: string(content)})
}
```

**After:**
```go
func getEnvironmentConfig(w http.ResponseWriter, r *http.Request) {
	envName, ok := httputil.RequiredQueryParam(w, r, "name")
	if !ok {
		return
	}

	// ... business logic ...

	httputil.RespondJSON(w, http.StatusOK, ConfigResponse{Content: string(content)})
}
```

**Files to Update:**
- All 17 `api_*.go` files
- `api.go` main file

**Effort:** 3-4 hours
**Impact:** Reduces duplication by ~300 lines

---

#### 2.3 Create AWS Client Factory

**Problem:** Repetitive AWS client initialization in every API handler:
```go
ctx := context.Background()
cfg, err := config.LoadDefaultConfig(ctx)
if err != nil {
	// error handling
}
ecsClient := ecs.NewFromConfig(cfg)
```

**Solution:** Centralized AWS client factory

**New File:** `app/awsutil/client_factory.go`
```go
package awsutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// ClientFactory creates AWS service clients with caching
type ClientFactory struct {
	cfg    aws.Config
	mu     sync.RWMutex

	// Cached clients
	ecsClient    *ecs.Client
	rdsClient    *rds.Client
	ec2Client    *ec2.Client
	s3Client     *s3.Client
	ssmClient    *ssm.Client
	stsClient    *sts.Client
	route53Client *route53.Client
	ecrClient    *ecr.Client
}

// NewClientFactory creates a new AWS client factory
func NewClientFactory(ctx context.Context, profile, region string) (*ClientFactory, error) {
	var cfg aws.Config
	var err error

	if profile != "" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithSharedConfigProfile(profile),
		)
	} else {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
		)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &ClientFactory{cfg: cfg}, nil
}

// ECS returns an ECS client (creates if not cached)
func (f *ClientFactory) ECS() *ecs.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ecsClient == nil {
		f.ecsClient = ecs.NewFromConfig(f.cfg)
	}
	return f.ecsClient
}

// RDS returns an RDS client
func (f *ClientFactory) RDS() *rds.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.rdsClient == nil {
		f.rdsClient = rds.NewFromConfig(f.cfg)
	}
	return f.rdsClient
}

// EC2 returns an EC2 client
func (f *ClientFactory) EC2() *ec2.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ec2Client == nil {
		f.ec2Client = ec2.NewFromConfig(f.cfg)
	}
	return f.ec2Client
}

// S3 returns an S3 client
func (f *ClientFactory) S3() *s3.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.s3Client == nil {
		f.s3Client = s3.NewFromConfig(f.cfg)
	}
	return f.s3Client
}

// SSM returns an SSM client
func (f *ClientFactory) SSM() *ssm.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ssmClient == nil {
		f.ssmClient = ssm.NewFromConfig(f.cfg)
	}
	return f.ssmClient
}

// STS returns an STS client
func (f *ClientFactory) STS() *sts.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.stsClient == nil {
		f.stsClient = sts.NewFromConfig(f.cfg)
	}
	return f.stsClient
}

// Route53 returns a Route53 client
func (f *ClientFactory) Route53() *route53.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.route53Client == nil {
		f.route53Client = route53.NewFromConfig(f.cfg)
	}
	return f.route53Client
}

// ECR returns an ECR client
func (f *ClientFactory) ECR() *ecr.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ecrClient == nil {
		f.ecrClient = ecr.NewFromConfig(f.cfg)
	}
	return f.ecrClient
}

// Config returns the underlying AWS config
func (f *ClientFactory) Config() aws.Config {
	return f.cfg
}
```

**Benefits:**
- ✅ Single AWS config loading point
- ✅ Client reuse (performance)
- ✅ Thread-safe with mutex
- ✅ Consistent error handling
- ✅ Easy to mock for testing

**Effort:** 2-3 hours
**Impact:** Simplifies AWS client usage across entire app

---

#### 2.4 Extract Logging Utility

**Problem:** 538 direct `fmt.Printf` and `fmt.Println` calls scattered across 32 files

**Solution:** Structured logging utility

**New File:** `app/logger/logger.go`
```go
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

// Level represents log level
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

// Logger provides structured logging
type Logger struct {
	level  Level
	output io.Writer
}

var defaultLogger = &Logger{
	level:  INFO,
	output: os.Stdout,
}

// New creates a new logger
func New(level Level, output io.Writer) *Logger {
	return &Logger{level: level, output: output}
}

// Default returns the default logger
func Default() *Logger {
	return defaultLogger
}

// SetLevel sets the log level
func (l *Logger) SetLevel(level Level) {
	l.level = level
}

// log formats and writes a log message
func (l *Logger) log(level Level, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	var prefix string
	switch level {
	case DEBUG:
		prefix = "🔍 DEBUG"
	case INFO:
		prefix = "ℹ️  INFO "
	case WARN:
		prefix = "⚠️  WARN "
	case ERROR:
		prefix = "❌ ERROR"
	}

	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, args...)

	log.SetOutput(l.output)
	log.SetFlags(0) // We handle our own formatting
	log.Printf("[%s] %s: %s\n", timestamp, prefix, message)
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

// Success logs a success message (special info with emoji)
func (l *Logger) Success(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.Info("✅ %s", message)
}

// Package-level convenience functions
func Debug(format string, args ...interface{}) {
	defaultLogger.Debug(format, args...)
}

func Info(format string, args ...interface{}) {
	defaultLogger.Info(format, args...)
}

func Warn(format string, args ...interface{}) {
	defaultLogger.Warn(format, args...)
}

func Error(format string, args ...interface{}) {
	defaultLogger.Error(format, args...)
}

func Success(format string, args ...interface{}) {
	defaultLogger.Success(format, args...)
}
```

**Migration Strategy:**
1. Create logger package
2. Replace direct fmt.Printf calls gradually
3. Add structured context (env, profile, etc.)
4. Keep fmt.Printf for TUI output only

**Before:**
```go
fmt.Printf("✅ Terraform initialized successfully.\n")
fmt.Printf("Error: %v\n", err)
```

**After:**
```go
logger.Success("Terraform initialized successfully")
logger.Error("Terraform initialization failed: %v", err)
```

**Effort:** 6-8 hours (gradual migration)
**Impact:** Better debugging, structured logs, easier testing

---

### Phase 3: Clean Code Improvements 🧹

#### 3.1 Environment Variable Management

**Problem:** Direct `os.Setenv` calls in 14 locations with side effects

**Solution:** Centralized environment manager

**New File:** `app/envutil/manager.go`
```go
package envutil

import (
	"fmt"
	"os"
)

// Manager handles environment variable management
type Manager struct {
	original map[string]string
}

// NewManager creates a new environment manager
func NewManager() *Manager {
	return &Manager{
		original: make(map[string]string),
	}
}

// Set sets an environment variable and tracks the original value
func (m *Manager) Set(key, value string) error {
	// Save original value if not already saved
	if _, exists := m.original[key]; !exists {
		m.original[key] = os.Getenv(key)
	}

	if err := os.Setenv(key, value); err != nil {
		return fmt.Errorf("failed to set %s: %w", key, err)
	}

	return nil
}

// Get gets an environment variable
func (m *Manager) Get(key string) string {
	return os.Getenv(key)
}

// Restore restores all environment variables to their original values
func (m *Manager) Restore() {
	for key, value := range m.original {
		if value == "" {
			os.Unsetenv(key)
		} else {
			os.Setenv(key, value)
		}
	}
}

// SetAWSProfile sets AWS profile-related environment variables
func (m *Manager) SetAWSProfile(profile string) error {
	return m.Set("AWS_PROFILE", profile)
}

// SetAWSRegion sets AWS region-related environment variables
func (m *Manager) SetAWSRegion(region string) error {
	if err := m.Set("AWS_REGION", region); err != nil {
		return err
	}
	return m.Set("AWS_DEFAULT_REGION", region)
}

// SetAWSConfig sets both profile and region
func (m *Manager) SetAWSConfig(profile, region string) error {
	if err := m.SetAWSProfile(profile); err != nil {
		return err
	}
	if region != "" {
		return m.SetAWSRegion(region)
	}
	return nil
}
```

**Benefits:**
- ✅ Tracks original values
- ✅ Easy rollback/restore
- ✅ Centralized management
- ✅ Better testing support

**Effort:** 2 hours
**Impact:** Safer environment variable management

---

#### 3.2 Extract Common Environment Loading Logic

**Problem:** Repetitive environment loading code across multiple files

**Solution:** Environment service layer

**New File:** `app/services/environment_service.go`
```go
package services

import (
	"fmt"
	"os"
	"gopkg.in/yaml.v2"
)

// EnvironmentService handles environment configuration operations
type EnvironmentService struct {
	basePath string
}

// NewEnvironmentService creates a new environment service
func NewEnvironmentService(basePath string) *EnvironmentService {
	return &EnvironmentService{basePath: basePath}
}

// Load loads an environment configuration by name
func (s *EnvironmentService) Load(name string) (*Env, error) {
	filename := fmt.Sprintf("%s/%s.yaml", s.basePath, name)

	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read environment file: %w", err)
	}

	var env Env
	if err := yaml.Unmarshal(content, &env); err != nil {
		return nil, fmt.Errorf("failed to parse environment config: %w", err)
	}

	// Apply migrations if needed
	if err := s.migrate(&env); err != nil {
		return nil, fmt.Errorf("failed to migrate environment: %w", err)
	}

	return &env, nil
}

// Save saves an environment configuration
func (s *EnvironmentService) Save(env *Env, name string) error {
	filename := fmt.Sprintf("%s/%s.yaml", s.basePath, name)

	data, err := yaml.Marshal(env)
	if err != nil {
		return fmt.Errorf("failed to marshal environment: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write environment file: %w", err)
	}

	return nil
}

// List lists all available environments
func (s *EnvironmentService) List() ([]string, error) {
	files, err := os.ReadDir(s.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	var envs []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".yaml") {
			name := strings.TrimSuffix(file.Name(), ".yaml")
			envs = append(envs, name)
		}
	}

	return envs, nil
}

// migrate applies schema migrations to an environment
func (s *EnvironmentService) migrate(env *Env) error {
	// Migration logic here
	return nil
}
```

**Effort:** 3 hours
**Impact:** Reduces duplication, improves testability

---

### Phase 4: Advanced Improvements 🚀

#### 4.1 Add Interfaces for Testing

**New File:** `app/interfaces/aws.go`
```go
package interfaces

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	// ... other AWS services
)

// ECSClient interface for ECS operations
type ECSClient interface {
	DescribeClusters(ctx context.Context, params *ecs.DescribeClustersInput, optFns ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error)
	ListServices(ctx context.Context, params *ecs.ListServicesInput, optFns ...func(*ecs.Options)) (*ecs.ListServicesOutput, error)
	DescribeServices(ctx context.Context, params *ecs.DescribeServicesInput, optFns ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
}

// RDSClient interface for RDS operations
type RDSClient interface {
	DescribeDBInstances(ctx context.Context, params *rds.DescribeDBInstancesInput, optFns ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
	DescribeDBClusters(ctx context.Context, params *rds.DescribeDBClustersInput, optFns ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error)
}

// ... other interfaces
```

**Benefits:**
- ✅ Enables mocking for unit tests
- ✅ Decouples from AWS SDK implementation
- ✅ Facilitates testing without AWS credentials

---

#### 4.2 Split Large Files

**Files to Split:**

1. **`terraform_plan_modern_tui.go` (174KB)** → Split into:
   - `terraform_plan_model.go` - Data structures
   - `terraform_plan_view.go` - View rendering
   - `terraform_plan_update.go` - Update logic
   - `terraform_plan_tree.go` - Tree building logic
   - `terraform_plan_styles.go` - Styles and formatting

2. **`dns_setup_tui.go` (111KB)** → Split into:
   - `dns_setup_model.go` - Model and state
   - `dns_setup_view.go` - View rendering
   - `dns_setup_update.go` - Update logic
   - `dns_setup_wizard.go` - Wizard steps
   - `dns_setup_validation.go` - Validation logic

**Effort:** 4-6 hours
**Impact:** Easier navigation, better organization

---

### Phase 5: Testing Infrastructure 🧪

#### 5.1 Add Unit Tests

**New Files:**
- `app/httputil/response_test.go`
- `app/awsutil/client_factory_test.go`
- `app/services/environment_service_test.go`

**Coverage Goals:**
- httputil: 90%+
- awsutil: 80%+
- services: 85%+
- business logic: 70%+

---

## Implementation Priority

### Week 1: Foundation
1. ✅ Fix file naming (`terrafrom.go`)
2. ✅ Create `httputil` package
3. ✅ Create `logger` package
4. ✅ Create `AppContext` struct

### Week 2: Refactoring
5. ✅ Create `awsutil.ClientFactory`
6. ✅ Create `envutil.Manager`
7. ✅ Create `services.EnvironmentService`
8. ✅ Migrate API handlers to use new utilities

### Week 3: Clean-up
9. ✅ Replace fmt.Printf with logger
10. ✅ Add interfaces for AWS clients
11. ✅ Split large TUI files

### Week 4: Testing
12. ✅ Write unit tests
13. ✅ Integration tests
14. ✅ Document refactored code

---

## Metrics & Success Criteria

| Metric | Before | Target | Measurement |
|--------|--------|--------|-------------|
| Lines of Code (LoC) | ~31,276 | -10% (save ~3,000 LoC) | Removed duplication |
| Global Variables | 12 | 0 | Dependency injection |
| fmt.Printf calls | 538 | < 50 (TUI only) | Structured logging |
| Test Coverage | ~5% | 60%+ | Unit + integration tests |
| Largest File | 174 KB | < 50 KB | Split large files |
| API Handler Duplication | High | Low | Utility functions |
| AWS Client Creation | Scattered | Centralized | ClientFactory |

---

## Risks & Mitigation

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Breaking changes | High | Medium | Gradual migration, extensive testing |
| Regression bugs | High | Medium | Comprehensive test suite |
| Time overrun | Medium | High | Prioritize critical issues first |
| Team resistance | Low | Low | Document benefits clearly |

---

## Rollout Strategy

### Phase A: Non-Breaking Changes (Week 1)
- Add new utility packages
- Fix typos and naming
- No changes to existing function signatures

### Phase B: API Migration (Week 2)
- Migrate API handlers one file at a time
- Keep old code alongside new during transition
- Test each migration thoroughly

### Phase C: Global State Removal (Week 3)
- Introduce AppContext gradually
- Update function signatures
- Remove globals last

### Phase D: Validation (Week 4)
- Run full integration tests
- Performance testing
- Code review and documentation

---

## Long-Term Recommendations

1. **CI/CD Integration**
   - Add linters (golangci-lint)
   - Enforce test coverage thresholds
   - Automate code quality checks

2. **Documentation**
   - Add GoDoc comments to all public functions
   - Create architecture decision records (ADRs)
   - Document API contracts

3. **Performance**
   - Add benchmarks for critical paths
   - Profile memory usage
   - Optimize hot paths

4. **Security**
   - Static analysis (gosec)
   - Dependency scanning
   - Secrets detection

---

## Conclusion

This refactoring plan addresses the accumulated technical debt in the Meroku application systematically. By following this plan, we will achieve:

- ✅ **Better Testability**: Through dependency injection and interfaces
- ✅ **Improved Maintainability**: Through reduced duplication and better organization
- ✅ **Enhanced Extensibility**: Through cleaner abstractions and service layers
- ✅ **Increased Quality**: Through structured logging and error handling
- ✅ **Professional Codebase**: Through consistent patterns and best practices

The estimated total effort is **80-120 hours** spread over 4 weeks with 1-2 developers.

**Next Step:** Review and approve this plan before implementation begins.
