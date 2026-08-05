package main

import (
	"os"
	"path/filepath"
	"strings"
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

	requiredFields := []string{"root_zone_id", "root_account_id",
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
	if _, exists := domain["root_zone_id"]; !exists {
		t.Error("Expected root_zone_id field to exist after migration")
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
		if _, exists := domain["root_zone_id"]; !exists {
			t.Error("Expected root_zone_id field after migration")
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

func TestMigrateV8ToV9_Services(t *testing.T) {
	// Test migration with services
	data := map[string]interface{}{
		"services": []interface{}{
			map[interface{}]interface{}{
				"name":         "api",
				"docker_image": "api:latest",
				"port":         8080,
			},
			map[interface{}]interface{}{
				"name":         "worker",
				"docker_image": "worker:latest",
			},
		},
	}

	err := migrateV8ToV9(data)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify services have ecr_config added
	services := data["services"].([]interface{})
	if len(services) != 2 {
		t.Fatalf("Expected 2 services, got %d", len(services))
	}

	for i, svcRaw := range services {
		svc := svcRaw.(map[interface{}]interface{})
		ecrConfig, exists := svc["ecr_config"]
		if !exists {
			t.Errorf("Service %d: ecr_config not added", i)
			continue
		}

		ecrConfigMap := ecrConfig.(map[string]interface{})
		mode, exists := ecrConfigMap["mode"]
		if !exists {
			t.Errorf("Service %d: ecr_config.mode not set", i)
			continue
		}

		if mode != "create_ecr" {
			t.Errorf("Service %d: expected mode='create_ecr', got '%s'", i, mode)
		}
	}
}

func TestMigrateV8ToV9_EventProcessorTasks(t *testing.T) {
	// Test migration with event processor tasks
	data := map[string]interface{}{
		"event_processor_tasks": []interface{}{
			map[interface{}]interface{}{
				"name":           "processor",
				"docker_image":   "processor:latest",
				"event_bus_name": "default",
				"event_pattern":  "{}",
			},
		},
	}

	err := migrateV8ToV9(data)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify tasks have ecr_config added
	tasks := data["event_processor_tasks"].([]interface{})
	task := tasks[0].(map[interface{}]interface{})
	ecrConfig, exists := task["ecr_config"]
	if !exists {
		t.Fatal("ecr_config not added to event processor task")
	}

	ecrConfigMap := ecrConfig.(map[string]interface{})
	if ecrConfigMap["mode"] != "create_ecr" {
		t.Errorf("Expected mode='create_ecr', got '%s'", ecrConfigMap["mode"])
	}
}

func TestMigrateV8ToV9_ScheduledTasks(t *testing.T) {
	// Test migration with scheduled tasks
	data := map[string]interface{}{
		"scheduled_tasks": []interface{}{
			map[interface{}]interface{}{
				"name":         "daily-sync",
				"docker_image": "sync:latest",
				"schedule":     "cron(0 2 * * ? *)",
			},
		},
	}

	err := migrateV8ToV9(data)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify tasks have ecr_config added
	tasks := data["scheduled_tasks"].([]interface{})
	task := tasks[0].(map[interface{}]interface{})
	ecrConfig, exists := task["ecr_config"]
	if !exists {
		t.Fatal("ecr_config not added to scheduled task")
	}

	ecrConfigMap := ecrConfig.(map[string]interface{})
	if ecrConfigMap["mode"] != "create_ecr" {
		t.Errorf("Expected mode='create_ecr', got '%s'", ecrConfigMap["mode"])
	}
}

func TestMigrateV8ToV9_AllTypes(t *testing.T) {
	// Test migration with all types
	data := map[string]interface{}{
		"services": []interface{}{
			map[interface{}]interface{}{
				"name": "api",
			},
		},
		"event_processor_tasks": []interface{}{
			map[interface{}]interface{}{
				"name": "processor",
			},
		},
		"scheduled_tasks": []interface{}{
			map[interface{}]interface{}{
				"name": "daily",
			},
		},
	}

	err := migrateV8ToV9(data)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify all types have ecr_config
	services := data["services"].([]interface{})
	svc := services[0].(map[interface{}]interface{})
	if _, exists := svc["ecr_config"]; !exists {
		t.Error("ecr_config not added to service")
	}

	eventTasks := data["event_processor_tasks"].([]interface{})
	eventTask := eventTasks[0].(map[interface{}]interface{})
	if _, exists := eventTask["ecr_config"]; !exists {
		t.Error("ecr_config not added to event processor task")
	}

	scheduledTasks := data["scheduled_tasks"].([]interface{})
	scheduledTask := scheduledTasks[0].(map[interface{}]interface{})
	if _, exists := scheduledTask["ecr_config"]; !exists {
		t.Error("ecr_config not added to scheduled task")
	}
}

func TestMigrateV8ToV9_ExistingConfig(t *testing.T) {
	// Test that existing ecr_config is not overwritten
	data := map[string]interface{}{
		"services": []interface{}{
			map[interface{}]interface{}{
				"name": "api",
				"ecr_config": map[interface{}]interface{}{
					"mode":           "manual_repo",
					"repository_uri": "123456789012.dkr.ecr.us-east-1.amazonaws.com/custom",
				},
			},
			map[interface{}]interface{}{
				"name": "worker",
			},
		},
	}

	err := migrateV8ToV9(data)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	services := data["services"].([]interface{})

	// First service should keep its existing config
	svc1 := services[0].(map[interface{}]interface{})
	ecrConfig1 := svc1["ecr_config"].(map[interface{}]interface{})
	if mode, ok := ecrConfig1["mode"]; !ok || mode != "manual_repo" {
		t.Error("Existing ecr_config was modified")
	}

	// Second service should get default config
	svc2 := services[1].(map[interface{}]interface{})
	ecrConfig2 := svc2["ecr_config"].(map[string]interface{})
	if ecrConfig2["mode"] != "create_ecr" {
		t.Error("Default ecr_config not added to service without config")
	}
}

func TestMigrateV8ToV9_EmptyArrays(t *testing.T) {
	// Test migration with empty arrays (should not fail)
	data := map[string]interface{}{
		"services":              []interface{}{},
		"event_processor_tasks": []interface{}{},
		"scheduled_tasks":       []interface{}{},
	}

	err := migrateV8ToV9(data)
	if err != nil {
		t.Fatalf("Migration failed on empty arrays: %v", err)
	}
}

func TestMigrateV8ToV9_MissingArrays(t *testing.T) {
	// Test migration with missing arrays (should not fail)
	data := map[string]interface{}{
		"project": "test",
		"env":     "dev",
	}

	err := migrateV8ToV9(data)
	if err != nil {
		t.Fatalf("Migration failed on missing arrays: %v", err)
	}
}

func TestMigrateV8ToV9_InvalidData(t *testing.T) {
	// Test migration with invalid data types (should handle gracefully)
	data := map[string]interface{}{
		"services": "not-an-array", // Invalid type
	}

	err := migrateV8ToV9(data)
	if err != nil {
		t.Fatalf("Migration should handle invalid data gracefully: %v", err)
	}
}

func TestCurrentSchemaVersion(t *testing.T) {
	// Verify that CurrentSchemaVersion matches the number of migrations + 1
	expectedVersion := len(AllMigrations) + 1
	if CurrentSchemaVersion != expectedVersion {
		t.Errorf("Expected CurrentSchemaVersion to be %d (len(AllMigrations)+1), got %d", expectedVersion, CurrentSchemaVersion)
	}
}

func TestMigrationChain_IncludesV10(t *testing.T) {
	// Verify that v10 migration is in the chain
	found := false
	for _, migration := range AllMigrations {
		if migration.Version == 10 {
			found = true
			if migration.Description != "Add per-service ECR configuration" {
				t.Errorf("Wrong description for v10 migration: %s", migration.Description)
			}
			break
		}
	}

	if !found {
		t.Error("v10 migration not found in AllMigrations")
	}
}

func TestMigrationChain_IncludesV11(t *testing.T) {
	// Verify that v11 migration is in the chain
	found := false
	for _, migration := range AllMigrations {
		if migration.Version == 11 {
			found = true
			if migration.Description != "Ensure host_port matches container_port for services (awsvpc compatibility)" {
				t.Errorf("Wrong description for v11 migration: %s", migration.Description)
			}
			break
		}
	}

	if !found {
		t.Error("v11 migration not found in AllMigrations")
	}
}

func TestMigrateToV11_MissingHostPort(t *testing.T) {
	// Test migration with services missing host_port
	data := map[string]interface{}{
		"services": []interface{}{
			map[interface{}]interface{}{
				"name":           "test1",
				"container_port": 8080,
				// host_port is missing
			},
			map[interface{}]interface{}{
				"name":           "test2",
				"container_port": 3000,
				// host_port is missing
			},
		},
	}

	err := migrateToV11(data)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify host_port was added
	services := data["services"].([]interface{})
	service1 := services[0].(map[interface{}]interface{})
	if service1["host_port"] != 8080 {
		t.Errorf("Expected host_port to be 8080, got %v", service1["host_port"])
	}

	service2 := services[1].(map[interface{}]interface{})
	if service2["host_port"] != 3000 {
		t.Errorf("Expected host_port to be 3000, got %v", service2["host_port"])
	}
}

func TestMigrateToV11_MismatchedHostPort(t *testing.T) {
	// Test migration with services having mismatched host_port
	data := map[string]interface{}{
		"services": []interface{}{
			map[interface{}]interface{}{
				"name":           "test1",
				"container_port": 8080,
				"host_port":      3000, // Mismatched!
			},
		},
	}

	err := migrateToV11(data)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify host_port was corrected
	services := data["services"].([]interface{})
	service1 := services[0].(map[interface{}]interface{})
	if service1["host_port"] != 8080 {
		t.Errorf("Expected host_port to be corrected to 8080, got %v", service1["host_port"])
	}
}

func TestMigrateToV11_AlreadyMatching(t *testing.T) {
	// Test migration with services that already have matching ports
	data := map[string]interface{}{
		"services": []interface{}{
			map[interface{}]interface{}{
				"name":           "test1",
				"container_port": 8080,
				"host_port":      8080, // Already matching
			},
		},
	}

	err := migrateToV11(data)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify host_port remained unchanged
	services := data["services"].([]interface{})
	service1 := services[0].(map[interface{}]interface{})
	if service1["host_port"] != 8080 {
		t.Errorf("Expected host_port to remain 8080, got %v", service1["host_port"])
	}
}

func TestMigrateToV11_NoServices(t *testing.T) {
	// Test migration with no services array
	data := map[string]interface{}{
		"project": "test",
		"env":     "dev",
	}

	err := migrateToV11(data)
	if err != nil {
		t.Fatalf("Migration should handle missing services gracefully: %v", err)
	}
}

func TestMigrateToV11_EmptyServices(t *testing.T) {
	// Test migration with empty services array
	data := map[string]interface{}{
		"services": []interface{}{},
	}

	err := migrateToV11(data)
	if err != nil {
		t.Fatalf("Migration should handle empty services array: %v", err)
	}
}

func TestMigrateToV11_NoContainerPort(t *testing.T) {
	// Test migration with service missing container_port (should skip)
	data := map[string]interface{}{
		"services": []interface{}{
			map[interface{}]interface{}{
				"name": "test1",
				// container_port is missing
			},
		},
	}

	err := migrateToV11(data)
	if err != nil {
		t.Fatalf("Migration should handle missing container_port gracefully: %v", err)
	}

	// Verify host_port was not added (no container_port to match)
	services := data["services"].([]interface{})
	service1 := services[0].(map[interface{}]interface{})
	if _, exists := service1["host_port"]; exists {
		t.Errorf("host_port should not be added when container_port is missing")
	}
}

func TestMigrationChain_IncludesV21(t *testing.T) {
	found := false
	for _, migration := range AllMigrations {
		if migration.Version == 21 {
			found = true
			if migration.Description != "Add AppSync Lambda authorizer configuration (jwks_uri, jwt_issuer, jwt_audience)" {
				t.Errorf("Wrong description for v21 migration: %s", migration.Description)
			}
			break
		}
	}

	if !found {
		t.Error("v21 migration not found in AllMigrations")
	}
}

// The whole point of v21 is that the authorizer's trust anchor must be a
// deliberate choice. A migration that filled in a working-looking JWKS URL would
// recreate the authentication bypass it exists to close, so the field is added
// empty and validation refuses to render until a human sets it.
func TestMigrateToV21_AddsEmptyJWKSURI(t *testing.T) {
	data := map[string]interface{}{
		"pubsub_appsync": map[interface{}]interface{}{
			"enabled":     true,
			"schema":      true,
			"auth_lambda": true,
			"resolvers":   true,
		},
	}

	if err := migrateToV21(data); err != nil {
		t.Fatalf("migrateToV21: %v", err)
	}

	appsync := data["pubsub_appsync"].(map[interface{}]interface{})
	for _, field := range []string{"jwks_uri", "jwt_issuer", "jwt_audience"} {
		value, exists := appsync[field]
		if !exists {
			t.Fatalf("%s should have been added", field)
		}
		if value != "" {
			t.Errorf("%s must be empty — the migration must never invent an identity provider, got %q", field, value)
		}
	}

	// Untouched keys must survive.
	if appsync["auth_lambda"] != true || appsync["schema"] != true {
		t.Errorf("existing pubsub_appsync values were modified: %v", appsync)
	}
}

func TestMigrateToV21_PreservesConfiguredJWKSURI(t *testing.T) {
	data := map[string]interface{}{
		"pubsub_appsync": map[interface{}]interface{}{
			"enabled":     true,
			"auth_lambda": true,
			"jwks_uri":    "https://idp.example.com/.well-known/jwks.json",
			"jwt_issuer":  "https://idp.example.com/",
		},
	}

	if err := migrateToV21(data); err != nil {
		t.Fatalf("migrateToV21: %v", err)
	}

	appsync := data["pubsub_appsync"].(map[interface{}]interface{})
	if appsync["jwks_uri"] != "https://idp.example.com/.well-known/jwks.json" {
		t.Errorf("configured jwks_uri was overwritten: %v", appsync["jwks_uri"])
	}
	if appsync["jwt_issuer"] != "https://idp.example.com/" {
		t.Errorf("configured jwt_issuer was overwritten: %v", appsync["jwt_issuer"])
	}
	if appsync["jwt_audience"] != "" {
		t.Errorf("jwt_audience should have been added empty, got %v", appsync["jwt_audience"])
	}
}

func TestMigrateToV21_NoAppSyncSection(t *testing.T) {
	data := map[string]interface{}{"project": "test"}

	if err := migrateToV21(data); err != nil {
		t.Fatalf("migration should be a no-op without pubsub_appsync: %v", err)
	}
	if _, exists := data["pubsub_appsync"]; exists {
		t.Error("migration must not create a pubsub_appsync section")
	}
}

func TestMigrateToV21_InvalidShape(t *testing.T) {
	data := map[string]interface{}{"pubsub_appsync": "not a map"}

	if err := migrateToV21(data); err != nil {
		t.Fatalf("migration should handle a malformed section gracefully: %v", err)
	}
}

func TestMigrateToV21_Idempotent(t *testing.T) {
	data := map[string]interface{}{
		"pubsub_appsync": map[interface{}]interface{}{
			"enabled":  true,
			"jwks_uri": "https://idp.example.com/.well-known/jwks.json",
		},
	}

	for i := 0; i < 3; i++ {
		if err := migrateToV21(data); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	appsync := data["pubsub_appsync"].(map[interface{}]interface{})
	if appsync["jwks_uri"] != "https://idp.example.com/.well-known/jwks.json" {
		t.Errorf("repeated runs changed jwks_uri: %v", appsync["jwks_uri"])
	}
	if len(appsync) != 4 {
		t.Errorf("expected exactly enabled + 3 authorizer keys, got %v", appsync)
	}
}

// ---------------------------------------------------------------- v22

func TestMigrationChain_IncludesV22(t *testing.T) {
	found := false
	for _, migration := range AllMigrations {
		if migration.Version == 22 {
			found = true
			if migration.Description != "Add auto_deploy to backend, services and scheduled_tasks (CI/CD auto-deploy policy)" {
				t.Errorf("Wrong description for v22 migration: %s", migration.Description)
			}
			break
		}
	}

	if !found {
		t.Error("v22 migration not found in AllMigrations")
	}
}

// v22Fixture is one config with a backend, two services and two scheduled
// tasks, none of which declares auto_deploy.
func v22Fixture(env string) map[string]interface{} {
	return map[string]interface{}{
		"env": env,
		"workload": map[interface{}]interface{}{
			"backend_health_endpoint": "/health",
		},
		"services": []interface{}{
			map[interface{}]interface{}{"name": "api"},
			map[interface{}]interface{}{"name": "payment-worker"},
		},
		"scheduled_tasks": []interface{}{
			map[interface{}]interface{}{"name": "cleanup", "schedule": "rate(1 day)"},
			map[interface{}]interface{}{"name": "archive", "schedule": "rate(7 days)"},
		},
	}
}

func v22AutoDeployOf(t *testing.T, data map[string]interface{}, key, name string) interface{} {
	t.Helper()
	for _, raw := range data[key].([]interface{}) {
		item := raw.(map[interface{}]interface{})
		if item["name"] == name {
			return item["auto_deploy"]
		}
	}
	t.Fatalf("%s %q not found", key, name)
	return nil
}

// The environment decides the default: pushing an image is a development
// gesture, redeploying production is not.
func TestMigrateToV22_DevDefaultsToTrue(t *testing.T) {
	data := v22Fixture("dev")

	if err := migrateToV22(data); err != nil {
		t.Fatalf("migrateToV22: %v", err)
	}

	workload := data["workload"].(map[interface{}]interface{})
	if workload["backend_auto_deploy"] != true {
		t.Errorf("backend_auto_deploy = %v, want true in dev", workload["backend_auto_deploy"])
	}
	for _, name := range []string{"api", "payment-worker"} {
		if got := v22AutoDeployOf(t, data, "services", name); got != true {
			t.Errorf("service %s auto_deploy = %v, want true in dev", name, got)
		}
	}
	for _, name := range []string{"cleanup", "archive"} {
		if got := v22AutoDeployOf(t, data, "scheduled_tasks", name); got != true {
			t.Errorf("scheduled task %s auto_deploy = %v, want true in dev", name, got)
		}
	}
}

// Only production opts out. "prod" and "production" are both accepted spellings;
// matching one and not the other would leave a production environment
// auto-deploying, which is the single case the policy exists to prevent.
func TestMigrateToV22_ProductionDefaultsToFalse(t *testing.T) {
	for _, env := range []string{"prod", "production"} {
		data := v22Fixture(env)

		if err := migrateToV22(data); err != nil {
			t.Fatalf("env %s: %v", env, err)
		}

		workload := data["workload"].(map[interface{}]interface{})
		if workload["backend_auto_deploy"] != false {
			t.Errorf("env %s: backend_auto_deploy = %v, want false", env, workload["backend_auto_deploy"])
		}
		if got := v22AutoDeployOf(t, data, "services", "api"); got != false {
			t.Errorf("env %s: service api auto_deploy = %v, want false", env, got)
		}
		if got := v22AutoDeployOf(t, data, "scheduled_tasks", "cleanup"); got != false {
			t.Errorf("env %s: task cleanup auto_deploy = %v, want false", env, got)
		}
	}
}

// Everything that is not production keeps auto-deploying. Staging and qa exist
// to be deployed to, and an earlier draft of this migration disabled them too —
// which would have silently removed the fast feedback those environments are
// for, on nothing more than a schema bump.
func TestMigrateToV22_NonProductionKeepsAutoDeploy(t *testing.T) {
	for _, env := range []string{"staging", "qa", "uat", "preview"} {
		data := v22Fixture(env)

		if err := migrateToV22(data); err != nil {
			t.Fatalf("env %s: %v", env, err)
		}

		workload := data["workload"].(map[interface{}]interface{})
		if workload["backend_auto_deploy"] != true {
			t.Errorf("env %s: backend_auto_deploy = %v, want true", env, workload["backend_auto_deploy"])
		}
		if got := v22AutoDeployOf(t, data, "services", "api"); got != true {
			t.Errorf("env %s: service api auto_deploy = %v, want true", env, got)
		}
		if got := v22AutoDeployOf(t, data, "scheduled_tasks", "cleanup"); got != true {
			t.Errorf("env %s: task cleanup auto_deploy = %v, want true", env, got)
		}
	}
}

// The value has to be written explicitly, not left implicit: CLAUDE.md's rule
// for core policy booleans is that the YAML states them, with the migration as
// the single source of the default. A prod file where the key is simply absent
// reads as "nobody has decided", and absent means true everywhere downstream.
func TestMigrateToV22_WritesTheKeyExplicitly(t *testing.T) {
	data := v22Fixture("prod")

	if err := migrateToV22(data); err != nil {
		t.Fatalf("migrateToV22: %v", err)
	}

	workload := data["workload"].(map[interface{}]interface{})
	if _, ok := workload["backend_auto_deploy"]; !ok {
		t.Error("backend_auto_deploy key is absent; it must be written, not implied")
	}
	for _, raw := range data["services"].([]interface{}) {
		service := raw.(map[interface{}]interface{})
		if _, ok := service["auto_deploy"]; !ok {
			t.Errorf("service %v has no auto_deploy key", service["name"])
		}
	}
	for _, raw := range data["scheduled_tasks"].([]interface{}) {
		task := raw.(map[interface{}]interface{})
		if _, ok := task["auto_deploy"]; !ok {
			t.Errorf("scheduled task %v has no auto_deploy key", task["name"])
		}
	}
}

// A value someone has already chosen is never overwritten, in either direction.
// This is what makes "opt prod back in" survive the next migration run.
func TestMigrateToV22_PreservesExistingValues(t *testing.T) {
	data := v22Fixture("prod")
	data["workload"].(map[interface{}]interface{})["backend_auto_deploy"] = true
	data["services"].([]interface{})[0].(map[interface{}]interface{})["auto_deploy"] = true
	data["scheduled_tasks"].([]interface{})[0].(map[interface{}]interface{})["auto_deploy"] = true

	if err := migrateToV22(data); err != nil {
		t.Fatalf("migrateToV22: %v", err)
	}

	if got := data["workload"].(map[interface{}]interface{})["backend_auto_deploy"]; got != true {
		t.Errorf("backend_auto_deploy = %v, want the configured true to survive", got)
	}
	if got := v22AutoDeployOf(t, data, "services", "api"); got != true {
		t.Errorf("service api auto_deploy = %v, want the configured true to survive", got)
	}
	if got := v22AutoDeployOf(t, data, "scheduled_tasks", "cleanup"); got != true {
		t.Errorf("task cleanup auto_deploy = %v, want the configured true to survive", got)
	}
	// And the untouched siblings still got the environment default.
	if got := v22AutoDeployOf(t, data, "services", "payment-worker"); got != false {
		t.Errorf("service payment-worker auto_deploy = %v, want false", got)
	}
}

// A dev config that was explicitly turned off stays off — the same guarantee,
// in the direction where getting it wrong deploys something rather than not
// deploying it.
func TestMigrateToV22_PreservesExplicitFalseInDev(t *testing.T) {
	data := v22Fixture("dev")
	data["services"].([]interface{})[1].(map[interface{}]interface{})["auto_deploy"] = false

	if err := migrateToV22(data); err != nil {
		t.Fatalf("migrateToV22: %v", err)
	}

	if got := v22AutoDeployOf(t, data, "services", "payment-worker"); got != false {
		t.Errorf("payment-worker auto_deploy = %v, want the configured false to survive", got)
	}
	if got := v22AutoDeployOf(t, data, "services", "api"); got != true {
		t.Errorf("api auto_deploy = %v, want the dev default true", got)
	}
}

func TestMigrateToV22_Idempotent(t *testing.T) {
	data := v22Fixture("prod")

	for i := 0; i < 3; i++ {
		if err := migrateToV22(data); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	if got := v22AutoDeployOf(t, data, "services", "api"); got != false {
		t.Errorf("repeated runs changed api auto_deploy: %v", got)
	}
	service := data["services"].([]interface{})[0].(map[interface{}]interface{})
	if len(service) != 2 {
		t.Errorf("expected exactly name + auto_deploy, got %v", service)
	}
}

// auto_deploy is orthogonal to enabled: one says whether the thing exists in
// AWS, the other whether CI may redeploy it. Neither may be inferred from the
// other.
func TestMigrateToV22_DoesNotTouchEnabled(t *testing.T) {
	data := v22Fixture("dev")
	data["services"].([]interface{})[0].(map[interface{}]interface{})["enabled"] = false

	if err := migrateToV22(data); err != nil {
		t.Fatalf("migrateToV22: %v", err)
	}

	service := data["services"].([]interface{})[0].(map[interface{}]interface{})
	if service["enabled"] != false {
		t.Errorf("enabled = %v, want the configured false untouched", service["enabled"])
	}
	if service["auto_deploy"] != true {
		t.Errorf("auto_deploy = %v: a disabled service still gets the environment default", service["auto_deploy"])
	}
}

// event_processor_tasks are deliberately left alone: they appear in none of the
// CI Lambda's maps, so an auto_deploy there would be configuration nothing reads.
func TestMigrateToV22_SkipsEventProcessorTasks(t *testing.T) {
	data := v22Fixture("dev")
	data["event_processor_tasks"] = []interface{}{
		map[interface{}]interface{}{"name": "on-upload"},
	}

	if err := migrateToV22(data); err != nil {
		t.Fatalf("migrateToV22: %v", err)
	}

	task := data["event_processor_tasks"].([]interface{})[0].(map[interface{}]interface{})
	if _, ok := task["auto_deploy"]; ok {
		t.Error("event_processor_tasks must not gain auto_deploy: nothing reads it")
	}
}

func TestMigrateToV22_HandlesMissingSections(t *testing.T) {
	for name, data := range map[string]map[string]interface{}{
		"empty":             {"env": "dev"},
		"no workload":       {"env": "prod", "services": []interface{}{}},
		"nil services":      {"env": "dev", "services": nil},
		"services not list": {"env": "dev", "services": "oops"},
	} {
		if err := migrateToV22(data); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// A missing env key is not production, so it follows the same rule as every
// other non-production environment.
//
// This test previously asserted the opposite, back when the rule was
// "dev is true, everything else false" and an unreadable env therefore landed on
// the conservative side by accident. Under "only production is false" that
// accident inverts: treating an unknown env as production would disable a
// staging pipeline over a missing key. Neither default is safe in the dark,
// which is why migrateToV22 prints a warning for this case rather than picking
// silently — the warning is the actual mitigation, not the boolean.
func TestMigrateToV22_MissingEnvIsNotProduction(t *testing.T) {
	data := map[string]interface{}{
		"services": []interface{}{map[interface{}]interface{}{"name": "api"}},
	}

	if err := migrateToV22(data); err != nil {
		t.Fatalf("migrateToV22: %v", err)
	}

	if got := v22AutoDeployOf(t, data, "services", "api"); got != true {
		t.Errorf("auto_deploy = %v, want true when env is unknown (unknown is not production)", got)
	}
}

// ---------------------------------------------------------------- v23

func TestMigrationChain_IncludesV23(t *testing.T) {
	found := false
	for _, migration := range AllMigrations {
		if migration.Version == 23 {
			found = true
			if migration.Description != "Add AppSync auth_mode and explicit api_key_enabled" {
				t.Errorf("Wrong description for v23 migration: %s", migration.Description)
			}
			break
		}
	}

	if !found {
		t.Error("v23 migration not found in AllMigrations")
	}
}

func v23AppSync(t *testing.T, data map[string]interface{}) map[interface{}]interface{} {
	t.Helper()
	appsync, ok := data["pubsub_appsync"].(map[interface{}]interface{})
	if !ok {
		t.Fatalf("pubsub_appsync missing or wrong shape: %#v", data["pubsub_appsync"])
	}
	return appsync
}

// The headline decision of this migration. An environment with AppSync enabled
// has an API key deployed right now, because the module created one for every
// deployment. Setting api_key_enabled to false here would destroy that key on
// the next apply and take down whatever is using it, so the existing credential
// is preserved and the migration says loudly what it is.
func TestMigrateToV23_EnabledAppSyncKeepsItsAPIKey(t *testing.T) {
	data := map[string]interface{}{
		"env": "prod",
		"pubsub_appsync": map[interface{}]interface{}{
			"enabled":     true,
			"schema":      true,
			"auth_lambda": true,
			"resolvers":   true,
			"jwks_uri":    "https://idp.example.com/.well-known/jwks.json",
		},
	}

	if err := migrateToV23(data); err != nil {
		t.Fatalf("migrateToV23: %v", err)
	}

	appsync := v23AppSync(t, data)
	if appsync["api_key_enabled"] != true {
		t.Errorf("api_key_enabled = %v: an enabled environment must keep the key it already has, or the next apply breaks its clients", appsync["api_key_enabled"])
	}
	if appsync["auth_mode"] != AppSyncAuthLambda {
		t.Errorf("auth_mode = %v, want %q: that is what the module deploys today", appsync["auth_mode"], AppSyncAuthLambda)
	}

	// Nothing else may move.
	if appsync["jwks_uri"] != "https://idp.example.com/.well-known/jwks.json" {
		t.Errorf("jwks_uri was modified: %v", appsync["jwks_uri"])
	}
	if appsync["auth_lambda"] != true || appsync["schema"] != true || appsync["resolvers"] != true {
		t.Errorf("existing pubsub_appsync values were modified: %v", appsync)
	}
}

// Where AppSync is off there is no key in AWS and nothing to preserve, so the
// safe default applies with no caveat.
func TestMigrateToV23_DisabledAppSyncGetsNoAPIKey(t *testing.T) {
	data := map[string]interface{}{
		"env": "dev",
		"pubsub_appsync": map[interface{}]interface{}{
			"enabled": false,
		},
	}

	if err := migrateToV23(data); err != nil {
		t.Fatalf("migrateToV23: %v", err)
	}

	appsync := v23AppSync(t, data)
	if appsync["api_key_enabled"] != false {
		t.Errorf("api_key_enabled = %v, want false: nothing exists to preserve", appsync["api_key_enabled"])
	}
	if appsync["auth_mode"] != AppSyncAuthLambda {
		t.Errorf("auth_mode = %v, want %q", appsync["auth_mode"], AppSyncAuthLambda)
	}
}

// auth_lambda only ever chose whose authorizer source was packaged, never
// whether a Lambda authorizer was used — authentication_type was hardcoded to
// AWS_LAMBDA either way. So both settings infer the same mode, and asserting it
// for auth_lambda: false is the part that would silently break if someone later
// "fixed" the inference to key off that flag.
func TestMigrateToV23_InfersLambdaModeRegardlessOfAuthLambdaFlag(t *testing.T) {
	for _, authLambda := range []bool{true, false} {
		data := map[string]interface{}{
			"pubsub_appsync": map[interface{}]interface{}{
				"enabled":     true,
				"auth_lambda": authLambda,
			},
		}

		if err := migrateToV23(data); err != nil {
			t.Fatalf("auth_lambda=%v: %v", authLambda, err)
		}

		appsync := v23AppSync(t, data)
		if appsync["auth_mode"] != AppSyncAuthLambda {
			t.Errorf("auth_lambda=%v: auth_mode = %v, want %q — anything else changes what the next apply deploys",
				authLambda, appsync["auth_mode"], AppSyncAuthLambda)
		}
	}
}

// A project that has already chosen a mode, or already turned the key off, must
// not have that choice reverted by re-running migrations.
func TestMigrateToV23_NeverOverwritesExistingValues(t *testing.T) {
	data := map[string]interface{}{
		"pubsub_appsync": map[interface{}]interface{}{
			"enabled":         true,
			"auth_mode":       AppSyncAuthCognito,
			"api_key_enabled": false,
		},
	}

	if err := migrateToV23(data); err != nil {
		t.Fatalf("migrateToV23: %v", err)
	}

	appsync := v23AppSync(t, data)
	if appsync["auth_mode"] != AppSyncAuthCognito {
		t.Errorf("auth_mode = %v, want the configured %q untouched", appsync["auth_mode"], AppSyncAuthCognito)
	}
	if appsync["api_key_enabled"] != false {
		t.Errorf("api_key_enabled = %v: a project that already turned the key off must not have it turned back on", appsync["api_key_enabled"])
	}
}

func TestMigrateToV23_Idempotent(t *testing.T) {
	data := map[string]interface{}{
		"pubsub_appsync": map[interface{}]interface{}{
			"enabled": true,
		},
	}

	for i := 0; i < 3; i++ {
		if err := migrateToV23(data); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	appsync := v23AppSync(t, data)
	if appsync["auth_mode"] != AppSyncAuthLambda || appsync["api_key_enabled"] != true {
		t.Errorf("repeated runs changed the result: %v", appsync)
	}
	if len(appsync) != 3 {
		t.Errorf("expected exactly enabled + auth_mode + api_key_enabled, got %v", appsync)
	}
}

// Mode-specific optional settings are the `default` helper's job, not a
// migration's (CLAUDE.md). Writing oidc_issuer into every lambda-mode config
// would be noise nothing reads.
func TestMigrateToV23_DoesNotAddOIDCFields(t *testing.T) {
	data := map[string]interface{}{
		"pubsub_appsync": map[interface{}]interface{}{"enabled": true},
	}

	if err := migrateToV23(data); err != nil {
		t.Fatalf("migrateToV23: %v", err)
	}

	appsync := v23AppSync(t, data)
	for _, field := range []string{"oidc_issuer", "oidc_client_id"} {
		if _, exists := appsync[field]; exists {
			t.Errorf("%s should not be written by the migration; it is only meaningful in oidc mode", field)
		}
	}
}

func TestMigrateToV23_HandlesMissingAndMalformedSections(t *testing.T) {
	for name, data := range map[string]map[string]interface{}{
		"no appsync section": {"project": "test"},
		"nil appsync":        {"pubsub_appsync": nil},
		"appsync not a map":  {"pubsub_appsync": "oops"},
	} {
		if err := migrateToV23(data); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	// And it must not invent a section that was not there.
	data := map[string]interface{}{"project": "test"}
	if err := migrateToV23(data); err != nil {
		t.Fatal(err)
	}
	if _, exists := data["pubsub_appsync"]; exists {
		t.Error("migration must not create a pubsub_appsync section")
	}
}

// A brand new project is the case where "default off" applies without
// qualification: there is no deployed key to preserve.
func TestNewEnvironmentDefaultsAPIKeyOff(t *testing.T) {
	env := createEnv("testproject", "dev")

	if env.AppSyncPubSub.APIKeyEnabled {
		t.Error("a new environment must not be given an API key: it bypasses auth_mode entirely")
	}
	if env.AppSyncPubSub.AuthMode != AppSyncAuthLambda {
		t.Errorf("AuthMode = %q, want %q so a new project and an un-migrated one behave identically",
			env.AppSyncPubSub.AuthMode, AppSyncAuthLambda)
	}
}

// The serialized form is what the next `meroku` run reads back, so assert on the
// YAML rather than only on the struct.
func TestNewEnvironmentSerializesAPIKeyOff(t *testing.T) {
	env := createEnv("testproject", "dev")

	out, err := yaml.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if !strings.Contains(string(out), "api_key_enabled: false") {
		t.Errorf("new env YAML must state api_key_enabled: false explicitly, got:\n%s", out)
	}
	if !strings.Contains(string(out), "auth_mode: lambda") {
		t.Errorf("new env YAML must state auth_mode, got:\n%s", out)
	}
}

// ---------------------------------------------------------------- v24

func TestMigrationChain_IncludesV24(t *testing.T) {
	found := false
	for _, migration := range AllMigrations {
		if migration.Version == 24 {
			found = true
			if migration.Description != "Repair the misspelt cognito.dashboard_callback_ur_ls key" {
				t.Errorf("Wrong description for v24 migration: %s", migration.Description)
			}
			break
		}
	}

	if !found {
		t.Error("v24 migration not found in AllMigrations")
	}

	if CurrentSchemaVersion != 24 {
		t.Errorf("CurrentSchemaVersion = %d, want 24", CurrentSchemaVersion)
	}
}

func v24Cognito(t *testing.T, data map[string]interface{}) map[interface{}]interface{} {
	t.Helper()
	cognito, ok := data["cognito"].(map[interface{}]interface{})
	if !ok {
		t.Fatalf("cognito missing or wrong shape: %#v", data["cognito"])
	}
	return cognito
}

// The headline case. A config written by any earlier meroku carries the misspelt
// key, and env/main.hbs reads the correct one — so the configured URLs were
// invisible to generation. The migration has to move the value across, not just
// add the right key.
func TestMigrateToV24_MovesValueOntoCorrectKey(t *testing.T) {
	data := map[string]interface{}{
		"cognito": map[interface{}]interface{}{
			"enabled":                  true,
			"dashboard_callback_ur_ls": []interface{}{"https://admin.example.com/callback"},
		},
	}

	if err := migrateToV24(data); err != nil {
		t.Fatalf("migrateToV24: %v", err)
	}

	cognito := v24Cognito(t, data)
	if _, stillThere := cognito["dashboard_callback_ur_ls"]; stillThere {
		t.Error("the misspelt key must be removed, or the file keeps teaching it to the next reader")
	}

	urls, ok := cognito["dashboard_callback_urls"].([]interface{})
	if !ok {
		t.Fatalf("dashboard_callback_urls missing or wrong shape: %#v", cognito["dashboard_callback_urls"])
	}
	if len(urls) != 1 || urls[0] != "https://admin.example.com/callback" {
		t.Errorf("dashboard_callback_urls = %#v, want the value carried over from the misspelt key", urls)
	}
}

// Migrations run on every load, and a repair that only works once is a repair
// that corrupts on the second pass.
func TestMigrateToV24_IsIdempotent(t *testing.T) {
	data := map[string]interface{}{
		"cognito": map[interface{}]interface{}{
			"dashboard_callback_ur_ls": []interface{}{"https://jwt.io"},
		},
	}

	for i := 0; i < 3; i++ {
		if err := migrateToV24(data); err != nil {
			t.Fatalf("migrateToV24 run %d: %v", i, err)
		}
	}

	cognito := v24Cognito(t, data)
	if _, stillThere := cognito["dashboard_callback_ur_ls"]; stillThere {
		t.Error("misspelt key reappeared")
	}
	urls, ok := cognito["dashboard_callback_urls"].([]interface{})
	if !ok || len(urls) != 1 || urls[0] != "https://jwt.io" {
		t.Errorf("dashboard_callback_urls = %#v, want the value unchanged across repeated runs", cognito["dashboard_callback_urls"])
	}
}

// The shape actually found on disk: the round-trip through the mistagged struct
// wrote an empty list, so most real configs carry the misspelt key holding [].
// Nothing is lost, but the key must still go.
func TestMigrateToV24_DropsEmptyMisspeltKey(t *testing.T) {
	data := map[string]interface{}{
		"cognito": map[interface{}]interface{}{
			"enabled":                  false,
			"dashboard_callback_ur_ls": []interface{}{},
		},
	}

	if err := migrateToV24(data); err != nil {
		t.Fatalf("migrateToV24: %v", err)
	}

	cognito := v24Cognito(t, data)
	if _, stillThere := cognito["dashboard_callback_ur_ls"]; stillThere {
		t.Error("the misspelt key must be removed even when it holds nothing")
	}
	if urls, ok := cognito["dashboard_callback_urls"].([]interface{}); !ok || len(urls) != 0 {
		t.Errorf("dashboard_callback_urls = %#v, want an empty list", cognito["dashboard_callback_urls"])
	}
}

// A hand-edited file can hold both keys. The correctly spelled one is what the
// template reads and therefore what is deployed today, so changing it would
// change live behaviour — the migration must not.
func TestMigrateToV24_PrefersTheDeployedValueWhenBothExist(t *testing.T) {
	data := map[string]interface{}{
		"cognito": map[interface{}]interface{}{
			"dashboard_callback_urls":  []interface{}{"https://live.example.com/cb"},
			"dashboard_callback_ur_ls": []interface{}{"https://stale.example.com/cb"},
		},
	}

	if err := migrateToV24(data); err != nil {
		t.Fatalf("migrateToV24: %v", err)
	}

	cognito := v24Cognito(t, data)
	if _, stillThere := cognito["dashboard_callback_ur_ls"]; stillThere {
		t.Error("the misspelt key must be removed")
	}
	urls, _ := cognito["dashboard_callback_urls"].([]interface{})
	if len(urls) != 1 || urls[0] != "https://live.example.com/cb" {
		t.Errorf("dashboard_callback_urls = %#v, want the value that is deployed today left alone", urls)
	}
}

// An empty correct key alongside a populated misspelt one is the shape produced
// by loading a hand-written config and saving it back: the load dropped the
// URLs, the save wrote [] under the right name and the originals under the
// wrong one. The real URLs must win.
func TestMigrateToV24_FillsAnEmptyCorrectKey(t *testing.T) {
	data := map[string]interface{}{
		"cognito": map[interface{}]interface{}{
			"dashboard_callback_urls":  []interface{}{},
			"dashboard_callback_ur_ls": []interface{}{"https://jwt.io"},
		},
	}

	if err := migrateToV24(data); err != nil {
		t.Fatalf("migrateToV24: %v", err)
	}

	urls, _ := v24Cognito(t, data)["dashboard_callback_urls"].([]interface{})
	if len(urls) != 1 || urls[0] != "https://jwt.io" {
		t.Errorf("dashboard_callback_urls = %#v, want the populated value", urls)
	}
}

// Migrations run against every config, most of which have nothing to repair.
func TestMigrateToV24_ToleratesMissingOrOddCognito(t *testing.T) {
	for name, data := range map[string]map[string]interface{}{
		"no cognito key":    {"env": "dev"},
		"nil cognito":       {"cognito": nil},
		"cognito not a map": {"cognito": "oops"},
		"already correct": {"cognito": map[interface{}]interface{}{
			"dashboard_callback_urls": []interface{}{"https://jwt.io"},
		}},
	} {
		if err := migrateToV24(data); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// The tag on Cognito.DashboardCallbackURLs is the source of the whole defect: it
// is what read the wrong key and what wrote it back. A repair migration is no
// use if the next save re-introduces the misspelling.
func TestCognitoStructRoundTripsTheCorrectKey(t *testing.T) {
	var env Env
	const in = "cognito:\n  enabled: true\n  dashboard_callback_urls:\n    - https://jwt.io\n"

	if err := yaml.Unmarshal([]byte(in), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(env.Cognito.DashboardCallbackURLs) != 1 || env.Cognito.DashboardCallbackURLs[0] != "https://jwt.io" {
		t.Fatalf("loading dropped the configured URLs: %#v", env.Cognito.DashboardCallbackURLs)
	}

	out, err := yaml.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(out), "dashboard_callback_ur_ls") {
		t.Errorf("saving re-introduced the misspelt key:\n%s", out)
	}
	if !strings.Contains(string(out), "dashboard_callback_urls") {
		t.Errorf("saving must write dashboard_callback_urls, got:\n%s", out)
	}
}
