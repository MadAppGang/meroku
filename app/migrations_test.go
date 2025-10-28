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

func TestCurrentSchemaVersion_V11(t *testing.T) {
	// Verify that CurrentSchemaVersion is updated to 11
	if CurrentSchemaVersion != 11 {
		t.Errorf("Expected CurrentSchemaVersion to be 11, got %d", CurrentSchemaVersion)
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
