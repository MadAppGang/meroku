package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v2"
)

// Test fixtures for different schema versions
var (
	v1YAMLFixture = `project: testproject
env: dev
region: us-east-1
state_bucket: test-bucket
state_file: state.tfstate
workload:
  backend_image_port: 8080
  bucket_public: false
domain:
  enabled: true
  domain_name: test.com
postgres:
  enabled: true
  dbname: testdb
  username: admin
  engine_version: "14"
cognito:
  enabled: false
ses:
  enabled: false
`

	v2YAMLFixture = `project: testproject
env: dev
region: us-east-1
state_bucket: test-bucket
state_file: state.tfstate
workload:
  backend_image_port: 8080
  bucket_public: false
domain:
  enabled: true
  domain_name: test.com
postgres:
  enabled: true
  dbname: testdb
  username: admin
  engine_version: "14"
  aurora: true
  min_capacity: 0.5
  max_capacity: 1.0
cognito:
  enabled: false
ses:
  enabled: false
alb:
  enabled: false
`

	v3YAMLFixture = `project: testproject
env: dev
region: us-east-1
state_bucket: test-bucket
state_file: state.tfstate
workload:
  backend_image_port: 8080
  bucket_public: false
domain:
  enabled: true
  domain_name: test.com
  zone_id: Z123456
  root_zone_id: Z789012
  is_dns_root: false
postgres:
  enabled: true
  dbname: testdb
  username: admin
  engine_version: "14"
  aurora: true
  min_capacity: 0.5
  max_capacity: 1.0
cognito:
  enabled: false
ses:
  enabled: false
alb:
  enabled: false
`

	v4YAMLFixture = `project: testproject
env: dev
region: us-east-1
state_bucket: test-bucket
state_file: state.tfstate
workload:
  backend_image_port: 8080
  bucket_public: false
  backend_desired_count: 2
  backend_autoscaling_enabled: true
  backend_autoscaling_min_capacity: 1
  backend_autoscaling_max_capacity: 10
  backend_cpu: "512"
  backend_memory: "1024"
domain:
  enabled: true
  domain_name: test.com
  zone_id: Z123456
  root_zone_id: Z789012
  is_dns_root: false
postgres:
  enabled: true
  dbname: testdb
  username: admin
  engine_version: "14"
  aurora: true
  min_capacity: 0.5
  max_capacity: 1.0
cognito:
  enabled: false
ses:
  enabled: false
alb:
  enabled: false
`

	v5YAMLFixture = `project: testproject
env: dev
region: us-east-1
account_id: "123456789012"
aws_profile: "default"
state_bucket: test-bucket
state_file: state.tfstate
workload:
  backend_image_port: 8080
  bucket_public: false
  backend_desired_count: 2
  backend_autoscaling_enabled: true
  backend_autoscaling_min_capacity: 1
  backend_autoscaling_max_capacity: 10
  backend_cpu: "512"
  backend_memory: "1024"
domain:
  enabled: true
  domain_name: test.com
  zone_id: Z123456
  root_zone_id: Z789012
  is_dns_root: false
postgres:
  enabled: true
  dbname: testdb
  username: admin
  engine_version: "14"
  aurora: true
  min_capacity: 0.5
  max_capacity: 1.0
cognito:
  enabled: false
ses:
  enabled: false
alb:
  enabled: false
schema_version: 5
`
)

func TestDetectSchemaVersion(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		expected int
	}{
		{
			name:     "v1 - no version indicators",
			yaml:     v1YAMLFixture,
			expected: 1,
		},
		{
			name:     "v2 - has aurora",
			yaml:     v2YAMLFixture,
			expected: 2,
		},
		{
			name:     "v3 - has zone_id",
			yaml:     v3YAMLFixture,
			expected: 3,
		},
		{
			name:     "v4 - has backend_desired_count",
			yaml:     v4YAMLFixture,
			expected: 4,
		},
		{
			name:     "v5 - has account_id",
			yaml:     v5YAMLFixture,
			expected: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data map[string]interface{}
			err := yaml.Unmarshal([]byte(tt.yaml), &data)
			if err != nil {
				t.Fatalf("Failed to unmarshal YAML: %v", err)
			}

			version := detectSchemaVersion(data)
			if version != tt.expected {
				t.Errorf("Expected version %d, got %d", tt.expected, version)
			}
		})
	}
}

func TestMigrateToV2(t *testing.T) {
	var data map[string]interface{}
	err := yaml.Unmarshal([]byte(v1YAMLFixture), &data)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	// Apply v2 migration
	err = migrateToV2(data)
	if err != nil {
		t.Fatalf("Migration to v2 failed: %v", err)
	}

	// Check postgres fields
	postgres, ok := data["postgres"].(map[interface{}]interface{})
	if !ok {
		t.Fatal("postgres field is not a map")
	}

	if aurora, ok := postgres["aurora"].(bool); !ok || aurora != false {
		t.Errorf("Expected aurora=false, got %v", postgres["aurora"])
	}

	if minCap, ok := postgres["min_capacity"].(float64); !ok || minCap != 0.5 {
		t.Errorf("Expected min_capacity=0.5, got %v", postgres["min_capacity"])
	}

	if maxCap, ok := postgres["max_capacity"].(float64); !ok || maxCap != 1.0 {
		t.Errorf("Expected max_capacity=1.0, got %v", postgres["max_capacity"])
	}

	// Check ALB field
	alb, ok := data["alb"].(map[string]interface{})
	if !ok {
		t.Fatal("alb field is not a map")
	}

	if enabled, ok := alb["enabled"].(bool); !ok || enabled != false {
		t.Errorf("Expected alb.enabled=false, got %v", alb["enabled"])
	}
}

func TestMigrateToV3(t *testing.T) {
	var data map[string]interface{}
	err := yaml.Unmarshal([]byte(v2YAMLFixture), &data)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	// Apply v3 migration
	err = migrateToV3(data)
	if err != nil {
		t.Fatalf("Migration to v3 failed: %v", err)
	}

	// Check domain fields
	domain, ok := data["domain"].(map[interface{}]interface{})
	if !ok {
		t.Fatal("domain field is not a map")
	}

	requiredFields := []string{"zone_id", "root_zone_id", "root_account_id",
		"is_dns_root", "dns_root_account_id", "delegation_role_arn",
		"api_domain_prefix", "add_env_domain_prefix"}

	for _, field := range requiredFields {
		if _, exists := domain[field]; !exists {
			t.Errorf("Expected field %s to exist in domain", field)
		}
	}

	// Verify is_dns_root is false by default
	if isDNSRoot, ok := domain["is_dns_root"].(bool); !ok || isDNSRoot != false {
		t.Errorf("Expected is_dns_root=false, got %v", domain["is_dns_root"])
	}
}

func TestMigrateToV4(t *testing.T) {
	var data map[string]interface{}
	err := yaml.Unmarshal([]byte(v3YAMLFixture), &data)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	// Apply v4 migration
	err = migrateToV4(data)
	if err != nil {
		t.Fatalf("Migration to v4 failed: %v", err)
	}

	// Check workload fields
	workload, ok := data["workload"].(map[interface{}]interface{})
	if !ok {
		t.Fatal("workload field is not a map")
	}

	tests := []struct {
		field    string
		expected interface{}
	}{
		{"backend_desired_count", 1},
		{"backend_autoscaling_enabled", false},
		{"backend_autoscaling_min_capacity", 1},
		{"backend_autoscaling_max_capacity", 4},
		{"backend_cpu", "256"},
		{"backend_memory", "512"},
		{"backend_alb_domain_name", ""},
	}

	for _, tt := range tests {
		value, exists := workload[tt.field]
		if !exists {
			t.Errorf("Expected field %s to exist in workload", tt.field)
			continue
		}

		switch expected := tt.expected.(type) {
		case int:
			if v, ok := value.(int); !ok || v != expected {
				t.Errorf("Expected %s=%d, got %v", tt.field, expected, value)
			}
		case bool:
			if v, ok := value.(bool); !ok || v != expected {
				t.Errorf("Expected %s=%v, got %v", tt.field, expected, value)
			}
		case string:
			if v, ok := value.(string); !ok || v != expected {
				t.Errorf("Expected %s=%s, got %v", tt.field, expected, value)
			}
		}
	}
}

func TestMigrateToV5(t *testing.T) {
	var data map[string]interface{}
	err := yaml.Unmarshal([]byte(v4YAMLFixture), &data)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	// Apply v5 migration
	err = migrateToV5(data)
	if err != nil {
		t.Fatalf("Migration to v5 failed: %v", err)
	}

	// Check account_id
	if accountID, ok := data["account_id"].(string); !ok || accountID != "" {
		t.Errorf("Expected account_id='', got %v", data["account_id"])
	}

	// Check aws_profile
	if awsProfile, ok := data["aws_profile"].(string); !ok || awsProfile != "" {
		t.Errorf("Expected aws_profile='', got %v", data["aws_profile"])
	}
}

func TestApplyMigrationsChain(t *testing.T) {
	// Start with v1 and migrate to current
	var data map[string]interface{}
	err := yaml.Unmarshal([]byte(v1YAMLFixture), &data)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	// Detect initial version
	version := detectSchemaVersion(data)
	if version != 1 {
		t.Errorf("Expected initial version 1, got %d", version)
	}

	// Apply all migrations
	err = applyMigrations(data, version)
	if err != nil {
		t.Fatalf("Migration chain failed: %v", err)
	}

	// Verify schema_version is set
	if schemaVer, ok := data["schema_version"].(int); !ok || schemaVer != CurrentSchemaVersion {
		t.Errorf("Expected schema_version=%d, got %v", CurrentSchemaVersion, data["schema_version"])
	}

	// Verify all v2 fields exist
	postgres := data["postgres"].(map[interface{}]interface{})
	if _, exists := postgres["aurora"]; !exists {
		t.Error("Expected aurora field to exist after migration")
	}

	// Verify all v3 fields exist
	domain := data["domain"].(map[interface{}]interface{})
	if _, exists := domain["zone_id"]; !exists {
		t.Error("Expected zone_id field to exist after migration")
	}

	// Verify all v4 fields exist
	workload := data["workload"].(map[interface{}]interface{})
	if _, exists := workload["backend_desired_count"]; !exists {
		t.Error("Expected backend_desired_count field to exist after migration")
	}

	// Verify all v5 fields exist
	if _, exists := data["account_id"]; !exists {
		t.Error("Expected account_id field to exist after migration")
	}
}

func TestMigrationIdempotency(t *testing.T) {
	// Apply migrations multiple times and verify result is the same
	var data1 map[string]interface{}
	err := yaml.Unmarshal([]byte(v1YAMLFixture), &data1)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	// First migration
	err = applyMigrations(data1, 1)
	if err != nil {
		t.Fatalf("First migration failed: %v", err)
	}

	// Serialize to YAML
	yaml1, err := yaml.Marshal(data1)
	if err != nil {
		t.Fatalf("Failed to marshal YAML: %v", err)
	}

	// Second migration (should be no-op)
	var data2 map[string]interface{}
	err = yaml.Unmarshal(yaml1, &data2)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	version := detectSchemaVersion(data2)
	err = applyMigrations(data2, version)
	if err != nil {
		t.Fatalf("Second migration failed: %v", err)
	}

	yaml2, err := yaml.Marshal(data2)
	if err != nil {
		t.Fatalf("Failed to marshal YAML: %v", err)
	}

	// Compare results
	if string(yaml1) != string(yaml2) {
		t.Error("Migration is not idempotent - results differ")
	}
}

func TestMigrateYAMLFileIntegration(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "migration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write v1 fixture to file
	testFile := filepath.Join(tmpDir, "test.yaml")
	err = os.WriteFile(testFile, []byte(v1YAMLFixture), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Run migration
	err = MigrateYAMLFile(testFile)
	if err != nil {
		t.Fatalf("MigrateYAMLFile failed: %v", err)
	}

	// Verify backup was created in backup/ directory
	backupDir := filepath.Join(filepath.Dir(testFile), "backup")
	backupPattern := filepath.Join(backupDir, filepath.Base(testFile)+".backup_*")
	backupFiles, err := filepath.Glob(backupPattern)
	if err != nil {
		t.Fatalf("Failed to glob backup files: %v", err)
	}
	if len(backupFiles) != 1 {
		t.Errorf("Expected 1 backup file in %s, found %d", backupDir, len(backupFiles))
	}

	// Read migrated file
	migratedData, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read migrated file: %v", err)
	}

	// Parse and verify
	var data map[string]interface{}
	err = yaml.Unmarshal(migratedData, &data)
	if err != nil {
		t.Fatalf("Failed to unmarshal migrated YAML: %v", err)
	}

	// Verify version
	if schemaVer, ok := data["schema_version"].(int); !ok || schemaVer != CurrentSchemaVersion {
		t.Errorf("Expected schema_version=%d, got %v", CurrentSchemaVersion, data["schema_version"])
	}

	// Verify key fields from each migration
	if postgres, ok := data["postgres"].(map[interface{}]interface{}); ok {
		if _, exists := postgres["aurora"]; !exists {
			t.Error("Expected aurora field after migration")
		}
	} else {
		t.Error("postgres field is missing or not a map")
	}

	if domain, ok := data["domain"].(map[interface{}]interface{}); ok {
		if _, exists := domain["zone_id"]; !exists {
			t.Error("Expected zone_id field after migration")
		}
	} else {
		t.Error("domain field is missing or not a map")
	}

	if workload, ok := data["workload"].(map[interface{}]interface{}); ok {
		if _, exists := workload["backend_desired_count"]; !exists {
			t.Error("Expected backend_desired_count field after migration")
		}
	} else {
		t.Error("workload field is missing or not a map")
	}

	if _, exists := data["account_id"]; !exists {
		t.Error("Expected account_id field after migration")
	}
}

func TestMigrationPreservesExistingValues(t *testing.T) {
	// Create YAML with custom values
	customYAML := `project: myproject
env: production
region: eu-west-1
state_bucket: my-custom-bucket
postgres:
  enabled: true
  dbname: customdb
  username: customuser
  engine_version: "15"
  public_access: false
domain:
  enabled: true
  domain_name: example.com
  create_domain_zone: true
workload:
  backend_image_port: 3000
  bucket_public: true
`

	var data map[string]interface{}
	err := yaml.Unmarshal([]byte(customYAML), &data)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	// Store original values
	originalProject := data["project"]
	originalEnv := data["env"]
	originalRegion := data["region"]
	postgres := data["postgres"].(map[interface{}]interface{})
	originalDBName := postgres["dbname"]
	originalUsername := postgres["username"]

	// Apply migrations
	err = applyMigrations(data, 1)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify original values are preserved
	if data["project"] != originalProject {
		t.Errorf("project value changed: expected %v, got %v", originalProject, data["project"])
	}
	if data["env"] != originalEnv {
		t.Errorf("env value changed: expected %v, got %v", originalEnv, data["env"])
	}
	if data["region"] != originalRegion {
		t.Errorf("region value changed: expected %v, got %v", originalRegion, data["region"])
	}

	postgres = data["postgres"].(map[interface{}]interface{})
	if postgres["dbname"] != originalDBName {
		t.Errorf("dbname value changed: expected %v, got %v", originalDBName, postgres["dbname"])
	}
	if postgres["username"] != originalUsername {
		t.Errorf("username value changed: expected %v, got %v", originalUsername, postgres["username"])
	}
}
