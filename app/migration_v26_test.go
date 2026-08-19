package main

import (
	"testing"

	"gopkg.in/yaml.v2"
)

// Schema v26 adds a runtime selector to the backend and to every service, so an
// ECS unit can be placed on an EC2 capacity pool instead of on Fargate.
//
// The migration's whole job is to make that choice visible without making it.
// It writes runtime: fargate — the value that renders exactly what the file
// rendered before — and writes nothing else. Every test below is a statement
// about one half of that: the stamp lands, or something that must not be
// touched was not touched.
//
// All fixtures are synthetic. No account ID, region or name here comes from a
// real environment.

// v26AnyKeyed builds the document shape yaml.v2 actually produces: nested
// mappings decoded as map[interface{}]interface{}.
func v26AnyKeyed(serviceNames ...string) map[string]interface{} {
	services := make([]interface{}, 0, len(serviceNames))
	for _, name := range serviceNames {
		services = append(services, map[interface{}]interface{}{
			"name":           name,
			"container_port": 8080,
		})
	}
	return map[string]interface{}{
		"project":  "testproject",
		"env":      "dev",
		"workload": map[interface{}]interface{}{"backend_image_port": 8080},
		"services": services,
	}
}

// v26StringKeyed builds the same document with string-keyed nested mappings —
// the shape a hand-written test or convertToJSONCompatible produces. A
// migration that handles only one of the two shapes passes half its tests and
// silently no-ops on the other half, so both go through every assertion.
func v26StringKeyed(serviceNames ...string) map[string]interface{} {
	services := make([]interface{}, 0, len(serviceNames))
	for _, name := range serviceNames {
		services = append(services, map[string]interface{}{
			"name":           name,
			"container_port": 8080,
		})
	}
	return map[string]interface{}{
		"project":  "testproject",
		"env":      "dev",
		"workload": map[string]interface{}{"backend_image_port": 8080},
		"services": services,
	}
}

// v26Value reads a key out of a nested mapping in whichever shape it has.
func v26Value(t *testing.T, mapping interface{}, key string) (interface{}, bool) {
	t.Helper()
	switch m := mapping.(type) {
	case map[interface{}]interface{}:
		value, exists := m[key]
		return value, exists
	case map[string]interface{}:
		value, exists := m[key]
		return value, exists
	default:
		t.Fatalf("not a mapping: %#v", mapping)
		return nil, false
	}
}

func v26Workload(t *testing.T, doc map[string]interface{}) interface{} {
	t.Helper()
	workload, exists := doc["workload"]
	if !exists {
		t.Fatal("document has no workload section")
	}
	return workload
}

func v26Service(t *testing.T, doc map[string]interface{}, index int) interface{} {
	t.Helper()
	services, ok := doc["services"].([]interface{})
	if !ok {
		t.Fatal("services is not a list")
	}
	if index >= len(services) {
		t.Fatalf("services has %d element(s), wanted index %d", len(services), index)
	}
	return services[index]
}

// The happy path: one apply, over both decoded shapes, stamping the backend and
// every service.
func TestMigrateToV26(t *testing.T) {
	shapes := []struct {
		name string
		doc  map[string]interface{}
	}{
		{"yaml.v2 shape (map[interface{}]interface{})", v26AnyKeyed("api", "worker")},
		{"string-keyed shape (map[string]interface{})", v26StringKeyed("api", "worker")},
	}

	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			doc := shape.doc

			if err := migrateToV26(doc); err != nil {
				t.Fatalf("migrateToV26: %v", err)
			}

			runtime, exists := v26Value(t, v26Workload(t, doc), "backend_runtime")
			if !exists {
				t.Fatal("workload.backend_runtime was not written — the migration no-opped on this shape")
			}
			if runtime != "fargate" {
				t.Errorf("workload.backend_runtime = %#v, want \"fargate\"", runtime)
			}

			for index, name := range []string{"api", "worker"} {
				service := v26Service(t, doc, index)
				runtime, exists := v26Value(t, service, "runtime")
				if !exists {
					t.Fatalf("services[%d] (%s) has no runtime — the migration no-opped on this shape", index, name)
				}
				if runtime != "fargate" {
					t.Errorf("services[%d] (%s) runtime = %#v, want \"fargate\"", index, name, runtime)
				}
			}
		})
	}

	// compute_pool is the pool a unit is placed on. A fargate unit is not
	// placed on a pool, and inventing a name here would name a pool that does
	// not exist and fail generation with an index error.
	t.Run("never writes compute_pool", func(t *testing.T) {
		doc := v26AnyKeyed("api")
		if err := migrateToV26(doc); err != nil {
			t.Fatalf("migrateToV26: %v", err)
		}
		if _, exists := v26Value(t, v26Workload(t, doc), "backend_compute_pool"); exists {
			t.Error("migration wrote workload.backend_compute_pool")
		}
		if _, exists := v26Value(t, v26Service(t, doc, 0), "compute_pool"); exists {
			t.Error("migration wrote services[0].compute_pool")
		}
	})

	// The zero-diff contract. A compute block, even an empty one, is a change
	// to a file the user never edited. And this schema has no vpc: map at all —
	// use_default_vpc and vpc_cidr are flat top-level keys — so a migration that
	// invented one would be introducing a second convention by accident.
	t.Run("creates neither a compute block nor a vpc map", func(t *testing.T) {
		doc := v26AnyKeyed("api")
		if err := migrateToV26(doc); err != nil {
			t.Fatalf("migrateToV26: %v", err)
		}
		if _, exists := doc["compute"]; exists {
			t.Errorf("migration invented a compute block: %#v", doc["compute"])
		}
		if _, exists := doc["vpc"]; exists {
			t.Errorf("migration invented a vpc block: %#v", doc["vpc"])
		}
	})

	// applyMigrations owns schema_version and writes it once after the whole
	// chain. A migration that set it itself would stop every later migration
	// in the same run from being applied.
	t.Run("does not set schema_version", func(t *testing.T) {
		doc := v26AnyKeyed("api")
		if err := migrateToV26(doc); err != nil {
			t.Fatalf("migrateToV26: %v", err)
		}
		if _, exists := doc["schema_version"]; exists {
			t.Errorf("migration set schema_version to %#v; applyMigrations owns that", doc["schema_version"])
		}
	})
}

// Nothing here is an error. A truncated or hand-written config still has to
// load, so an odd shape is skipped and the rest of the file is migrated.
func TestMigrateToV26_SkipsOddShapes(t *testing.T) {
	t.Run("no workload section", func(t *testing.T) {
		doc := map[string]interface{}{"services": []interface{}{}}
		if err := migrateToV26(doc); err != nil {
			t.Fatalf("migrateToV26 should skip, not fail: %v", err)
		}
		if _, exists := doc["workload"]; exists {
			t.Error("migration invented a workload section")
		}
	})

	t.Run("no services key", func(t *testing.T) {
		doc := map[string]interface{}{"workload": map[interface{}]interface{}{}}
		if err := migrateToV26(doc); err != nil {
			t.Fatalf("migrateToV26 should skip, not fail: %v", err)
		}
		if _, exists := doc["services"]; exists {
			t.Error("migration invented a services key")
		}
	})

	t.Run("services is not a list", func(t *testing.T) {
		doc := map[string]interface{}{"services": "nonsense"}
		if err := migrateToV26(doc); err != nil {
			t.Fatalf("migrateToV26 should skip, not fail: %v", err)
		}
		if doc["services"] != "nonsense" {
			t.Errorf("migration rewrote a malformed services key to %#v", doc["services"])
		}
	})

	t.Run("a service element is not a mapping", func(t *testing.T) {
		doc := map[string]interface{}{
			"workload": map[interface{}]interface{}{},
			"services": []interface{}{
				"nonsense",
				map[interface{}]interface{}{"name": "api"},
			},
		}
		if err := migrateToV26(doc); err != nil {
			t.Fatalf("migrateToV26 should skip, not fail: %v", err)
		}
		if got := v26Service(t, doc, 0); got != "nonsense" {
			t.Errorf("migration rewrote a malformed service element to %#v", got)
		}
		// The valid neighbour is still migrated.
		if runtime, _ := v26Value(t, v26Service(t, doc, 1), "runtime"); runtime != "fargate" {
			t.Errorf("a malformed element stopped the rest of the list: services[1] runtime = %#v", runtime)
		}
	})

	t.Run("empty services list", func(t *testing.T) {
		doc := map[string]interface{}{
			"workload": map[interface{}]interface{}{},
			"services": []interface{}{},
		}
		if err := migrateToV26(doc); err != nil {
			t.Fatalf("migrateToV26: %v", err)
		}
		services, ok := doc["services"].([]interface{})
		if !ok || len(services) != 0 {
			t.Errorf("services = %#v, want an untouched empty list", doc["services"])
		}
	})
}

// Migrations re-run whenever a file's recorded version is behind, and the same
// document can pass through the chain more than once in a session. A second run
// must change nothing the first run produced.
func TestMigrateToV26_IsIdempotent(t *testing.T) {
	doc := v26AnyKeyed("api", "worker")

	if err := migrateToV26(doc); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal after first run: %v", err)
	}

	if err := migrateToV26(doc); err != nil {
		t.Fatalf("second run: %v", err)
	}
	second, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal after second run: %v", err)
	}

	if string(first) != string(second) {
		t.Errorf("second run changed the document:\n--- after first ---\n%s\n--- after second ---\n%s", first, second)
	}
}

// A value the user set is a decision. The migration fills gaps; it does not
// have opinions about what is already there.
func TestMigrateToV26_PreservesExistingValues(t *testing.T) {
	doc := map[string]interface{}{
		"workload": map[interface{}]interface{}{
			"backend_runtime":      "ec2",
			"backend_compute_pool": "general",
		},
		"services": []interface{}{
			map[interface{}]interface{}{
				"name":         "api",
				"runtime":      "ec2",
				"compute_pool": "general",
			},
			map[interface{}]interface{}{
				"name": "worker",
			},
		},
	}

	if err := migrateToV26(doc); err != nil {
		t.Fatalf("migrateToV26: %v", err)
	}

	workload := v26Workload(t, doc)
	if runtime, _ := v26Value(t, workload, "backend_runtime"); runtime != "ec2" {
		t.Errorf("workload.backend_runtime = %#v, want the user's \"ec2\"", runtime)
	}
	if pool, _ := v26Value(t, workload, "backend_compute_pool"); pool != "general" {
		t.Errorf("workload.backend_compute_pool = %#v, want the user's \"general\"", pool)
	}

	api := v26Service(t, doc, 0)
	if runtime, _ := v26Value(t, api, "runtime"); runtime != "ec2" {
		t.Errorf("services[0] runtime = %#v, want the user's \"ec2\"", runtime)
	}
	if pool, _ := v26Value(t, api, "compute_pool"); pool != "general" {
		t.Errorf("services[0] compute_pool = %#v, want the user's \"general\"", pool)
	}

	// The untouched neighbour is still stamped: preserving one value is not a
	// reason to skip the rest of the list.
	if runtime, _ := v26Value(t, v26Service(t, doc, 1), "runtime"); runtime != "fargate" {
		t.Errorf("services[1] runtime = %#v, want \"fargate\"", runtime)
	}

	// An explicitly empty value is still a value. Presence, not truthiness, is
	// what guards the write.
	t.Run("an explicit empty string is not overwritten", func(t *testing.T) {
		doc := map[string]interface{}{
			"workload": map[interface{}]interface{}{"backend_runtime": ""},
			"services": []interface{}{map[interface{}]interface{}{"name": "api", "runtime": ""}},
		}
		if err := migrateToV26(doc); err != nil {
			t.Fatalf("migrateToV26: %v", err)
		}
		if runtime, _ := v26Value(t, v26Workload(t, doc), "backend_runtime"); runtime != "" {
			t.Errorf("workload.backend_runtime = %#v, want the explicit empty string kept", runtime)
		}
		if runtime, _ := v26Value(t, v26Service(t, doc, 0), "runtime"); runtime != "" {
			t.Errorf("services[0] runtime = %#v, want the explicit empty string kept", runtime)
		}
	})
}

// The zero-diff stamp, asserted as one property over a whole environment rather
// than field by field: after migrating a config that has services and a
// workload but no compute configuration, every ECS unit in the file says
// fargate, and the file has gained nothing else. That is what makes an existing
// environment render byte-identical Terraform and plan 0 to add, 0 to change,
// 0 to destroy.
func TestMigrateToV26_StampsFargateEverywhereForZeroDiff(t *testing.T) {
	const before = `
project: testproject
env: dev
region: us-east-1
account_id: "000000000000"
workload:
  backend_image_port: 8080
  backend_cpu: "256"
  backend_memory: "512"
services:
  - name: api
    container_port: 8080
  - name: worker
    container_port: 9090
  - name: reports
    container_port: 7070
scheduled_tasks:
  - name: cleanup
`

	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(before), &doc); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	if err := migrateToV26(doc); err != nil {
		t.Fatalf("migrateToV26: %v", err)
	}

	// Every ECS unit carries the runtime that renders today's Terraform.
	if runtime, _ := v26Value(t, v26Workload(t, doc), "backend_runtime"); runtime != "fargate" {
		t.Errorf("workload.backend_runtime = %#v, want \"fargate\"", runtime)
	}
	services, ok := doc["services"].([]interface{})
	if !ok {
		t.Fatal("services is not a list")
	}
	if len(services) != 3 {
		t.Fatalf("fixture lost services: got %d, want 3", len(services))
	}
	for index := range services {
		service := v26Service(t, doc, index)
		name, _ := v26Value(t, service, "name")
		runtime, exists := v26Value(t, service, "runtime")
		if !exists {
			t.Errorf("services[%d] (%v) has no runtime", index, name)
			continue
		}
		if runtime != "fargate" {
			t.Errorf("services[%d] (%v) runtime = %#v, want \"fargate\"", index, name, runtime)
		}
	}

	// And nothing else was added. Any new top-level key beyond the fixture's is
	// a diff on a file the user never edited.
	wantTopLevel := map[string]bool{
		"project": true, "env": true, "region": true, "account_id": true,
		"workload": true, "services": true, "scheduled_tasks": true,
	}
	for key := range doc {
		if !wantTopLevel[key] {
			t.Errorf("migration added top-level key %q = %#v", key, doc[key])
		}
	}

	// Sibling collections are not the migration's business. scheduled_tasks
	// have no runtime — they are not long-lived ECS services — and stamping one
	// would be a field nothing reads.
	tasks, ok := doc["scheduled_tasks"].([]interface{})
	if !ok || len(tasks) != 1 {
		t.Fatalf("scheduled_tasks = %#v, want the fixture's single task", doc["scheduled_tasks"])
	}
	if _, exists := v26Value(t, tasks[0], "runtime"); exists {
		t.Error("migration stamped a runtime on a scheduled task")
	}
}

// The version constant, the history comment and the registered migration list
// have to agree, or a file migrates to a version whose migration never ran.
func TestV26IsRegisteredAtTheCurrentVersion(t *testing.T) {
	if CurrentSchemaVersion != 26 {
		t.Fatalf("CurrentSchemaVersion = %d, want 26", CurrentSchemaVersion)
	}

	var v26 *Migration
	for index := range AllMigrations {
		if AllMigrations[index].Version == 26 {
			v26 = &AllMigrations[index]
			break
		}
	}
	if v26 == nil {
		t.Fatal("AllMigrations has no entry for version 26")
	}
	if v26.Apply == nil {
		t.Error("the v26 entry has no Apply function")
	}

	last := AllMigrations[len(AllMigrations)-1]
	if last.Version != CurrentSchemaVersion {
		t.Errorf("last registered migration is v%d, but CurrentSchemaVersion is %d",
			last.Version, CurrentSchemaVersion)
	}
}

// A new project starts on Fargate with no compute block at all. An empty
// compute: {} in a generated file is a placeholder someone has to delete, and
// the template treats absence and empty identically anyway.
func TestCreateEnv_ComputeDefaults(t *testing.T) {
	env := createEnv("testproject", "dev")

	if env.Workload.BackendRuntime != "fargate" {
		t.Errorf("Workload.BackendRuntime = %q, want \"fargate\"", env.Workload.BackendRuntime)
	}
	if env.Workload.BackendComputePool != "" {
		t.Errorf("Workload.BackendComputePool = %q, want empty", env.Workload.BackendComputePool)
	}
	if len(env.Compute.Pools) != 0 {
		t.Errorf("Compute.Pools = %#v, want none", env.Compute.Pools)
	}

	encoded, err := yaml.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var roundTripped map[string]interface{}
	if err := yaml.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, exists := roundTripped["compute"]; exists {
		t.Errorf("a new project serialised a compute key:\n%s", encoded)
	}
}

// min_size: 0 is a real setting — it is how a pool scales to nothing when idle.
// The pool's numeric fields are pointers so that 0 survives a save as 0 rather
// than being dropped by omitempty and reloaded as absent.
func TestComputePool_ZeroIsDistinctFromAbsent(t *testing.T) {
	zero := 0
	pool := ComputePool{
		Name:          "general",
		InstanceTypes: []string{"m7i-flex.large"},
		MinSize:       &zero,
		OnDemandBase:  &zero,
	}

	encoded, err := yaml.Marshal(Compute{Pools: []ComputePool{pool}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Compute
	if err := yaml.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Pools) != 1 {
		t.Fatalf("pools = %#v, want one", decoded.Pools)
	}

	got := decoded.Pools[0]
	if got.MinSize == nil {
		t.Errorf("min_size: 0 was dropped on the round trip:\n%s", encoded)
	} else if *got.MinSize != 0 {
		t.Errorf("min_size = %d, want 0", *got.MinSize)
	}
	if got.OnDemandBase == nil {
		t.Errorf("on_demand_base: 0 was dropped on the round trip:\n%s", encoded)
	} else if *got.OnDemandBase != 0 {
		t.Errorf("on_demand_base = %d, want 0", *got.OnDemandBase)
	}
	// max_size was never set, and must stay absent rather than becoming 0.
	if got.MaxSize != nil {
		t.Errorf("max_size = %d, want absent", *got.MaxSize)
	}
}

// assume_egress is the operator's assertion that a pool's subnets can reach the
// internet, and it is the only thing that unlocks network_mode: awsvpc. It sits
// on the pool precisely so that it round-trips through a struct field. A key
// with no field would survive generation once and then be dropped by the next
// unrelated save, and the operator's next generate would fail with no edit to
// blame — so this test is the guard for that failure mode, not a tag check.
func TestComputePool_AssumeEgressRoundTrips(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name  string
		given *bool
	}{
		{"asserted true", &yes},
		{"explicitly false", &no},
		{"absent", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := yaml.Marshal(Compute{Pools: []ComputePool{{
				Name:          "general",
				InstanceTypes: []string{"m7i-flex.large"},
				NetworkMode:   "awsvpc",
				AssumeEgress:  tc.given,
			}}})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var decoded Compute
			if err := yaml.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(decoded.Pools) != 1 {
				t.Fatalf("pools = %#v, want one", decoded.Pools)
			}

			got := decoded.Pools[0].AssumeEgress
			switch {
			case tc.given == nil && got != nil:
				t.Errorf("absent assume_egress became %v:\n%s", *got, encoded)
			case tc.given != nil && got == nil:
				t.Errorf("assume_egress: %v was dropped on the round trip:\n%s", *tc.given, encoded)
			case tc.given != nil && *got != *tc.given:
				t.Errorf("assume_egress = %v, want %v", *got, *tc.given)
			}

			// network_mode is a plain string, and awsvpc must survive verbatim:
			// silently rewriting it to bridge would turn a refusal into a
			// working-but-wrong pool.
			if decoded.Pools[0].NetworkMode != "awsvpc" {
				t.Errorf("network_mode = %q, want \"awsvpc\"", decoded.Pools[0].NetworkMode)
			}
		})
	}
}
