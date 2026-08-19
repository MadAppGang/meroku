package main

import "fmt"

// migrateToV26 stamps an explicit runtime on the backend and on every service.
//
// Schema v26 introduces EC2 capacity pools. The two new selectors are
// workload.backend_runtime and services[].runtime, each fargate | ec2, with a
// compute_pool naming the pool an "ec2" unit is placed on.
//
// The stamp is not needed for correctness: an absent runtime and
// runtime: fargate render byte-identical Terraform, because the template's
// fargate branch is the one that emits today's literal launch_type = "FARGATE".
// It is needed for legibility. A user who opens the YAML has to be able to see
// that a runtime choice exists before they can make one, and the Compute tab's
// runtime control needs a value to display rather than inventing one on the
// user's behalf and then saving it back as though they had chosen it.
//
// Three things this migration deliberately does not do:
//
//   - It never writes compute_pool. An empty pool reference on a fargate unit
//     is correct; a populated one would name a pool that does not exist.
//   - It never creates a compute: block. No pools is the correct state for
//     every environment that predates v26, and an empty pools list written into
//     someone's file is a diff they did not ask for. This is also why there is
//     nothing here about assume_egress: that key lives on a pool, and this
//     migration creates no pools.
//   - It never adds a top-level vpc: key. This schema has none — use_default_vpc
//     and vpc_cidr are flat — and inventing one would be a second convention
//     for the same subject.
//
// Taken together those three make the zero-diff property hold: after migrating,
// an untouched environment plans 0 to add, 0 to change, 0 to destroy.
//
// The version is not set here. applyMigrations owns schema_version and writes
// it once, after the whole chain has run.
func migrateToV26(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v26: Stamping runtime: fargate on the backend and every service")

	migrateV26Backend(data)
	migrateV26Services(data)

	return nil
}

// migrateV26Backend stamps workload.backend_runtime.
func migrateV26Backend(data map[string]interface{}) {
	workload, ok := asYAMLMapping(data["workload"])
	if !ok {
		// A config with no workload section is minimal or hand-written. There
		// is nothing to annotate, and the template's default already renders
		// Fargate, so this is a skip rather than an error.
		fmt.Println("    ℹ️  No workload section; backend runtime left at its default (fargate)")
		return
	}

	if workload.setIfAbsent("backend_runtime", "fargate") {
		fmt.Println("    ✓ backend: Set runtime to fargate")
	} else {
		fmt.Println("    ℹ️  backend already has a runtime")
	}
}

// migrateV26Services stamps runtime on each element of services.
func migrateV26Services(data map[string]interface{}) {
	servicesRaw, exists := data["services"]
	if !exists || servicesRaw == nil {
		fmt.Println("    ℹ️  No services to migrate")
		return
	}

	services, ok := servicesRaw.([]interface{})
	if !ok {
		fmt.Println("    ⚠️  services is not an array, skipping")
		return
	}

	stamped := 0
	for _, serviceRaw := range services {
		service, ok := asYAMLMapping(serviceRaw)
		if !ok {
			continue
		}
		if !service.setIfAbsent("runtime", "fargate") {
			continue
		}
		stamped++
		if name := service.stringValue("name"); name != "" {
			fmt.Printf("    ✓ service '%s': Set runtime to fargate\n", name)
		}
	}

	switch {
	case len(services) == 0:
		fmt.Println("    ℹ️  No services to migrate")
	case stamped == 0:
		fmt.Println("    ℹ️  Every service already has a runtime")
	default:
		fmt.Printf("    ✓ Stamped runtime on %d service(s)\n", stamped)
	}
}

// yamlMapping is a decoded YAML mapping in either of the two shapes one can
// arrive in.
//
// yaml.v2 decodes a nested mapping as map[interface{}]interface{}, which is
// what every file on disk produces; a test that builds a document by hand
// naturally writes map[string]interface{}, and so does any code that has been
// through convertToJSONCompatible. A migration that asserts only the first
// shape passes its tests and then silently no-ops on real files — the failure
// mode is invisible, because nothing errors and the version still gets bumped.
// Handling both here once means no call site can get it wrong.
//
// Exactly one of the two fields is non-nil. Writes go to whichever it is, and
// because both are reference types the caller's document is mutated in place.
type yamlMapping struct {
	anyKeyed    map[interface{}]interface{}
	stringKeyed map[string]interface{}
}

// asYAMLMapping recognises a decoded YAML mapping in either shape. A value that
// is neither — nil, a scalar, a list — is not a mapping, and the caller skips
// it rather than failing the migration.
func asYAMLMapping(value interface{}) (yamlMapping, bool) {
	switch mapping := value.(type) {
	case map[interface{}]interface{}:
		return yamlMapping{anyKeyed: mapping}, true
	case map[string]interface{}:
		return yamlMapping{stringKeyed: mapping}, true
	default:
		return yamlMapping{}, false
	}
}

// lookup returns the value at key and whether the key is present. Presence, not
// truthiness: a key explicitly set to an empty string or to false is present,
// and a migration that overwrote it would be discarding a user's choice.
func (m yamlMapping) lookup(key string) (interface{}, bool) {
	if m.anyKeyed != nil {
		value, exists := m.anyKeyed[key]
		return value, exists
	}
	value, exists := m.stringKeyed[key]
	return value, exists
}

// setIfAbsent writes value at key only when the key is absent, and reports
// whether it wrote. This is the idempotency guarantee: a second run finds every
// key present and changes nothing.
func (m yamlMapping) setIfAbsent(key string, value interface{}) bool {
	if _, exists := m.lookup(key); exists {
		return false
	}
	if m.anyKeyed != nil {
		m.anyKeyed[key] = value
	} else {
		m.stringKeyed[key] = value
	}
	return true
}

// stringValue returns the value at key when it is a string, and "" otherwise.
// Used for progress output only, so a non-string simply prints nothing rather
// than being reported as a problem.
func (m yamlMapping) stringValue(key string) string {
	value, _ := m.lookup(key)
	text, _ := value.(string)
	return text
}
