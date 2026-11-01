package main

import "fmt"

// Network access values for services[].network_access (schema v30).
//
// The Terraform module (modules/workloads/variables.tf) validates the same four
// strings; NetworkAccessValues is the Go half, used by the generate-time check
// and by the web UI's choices.
const (
	NetworkAccessPublic   = "public"    // only through the project's front door (ALB, else API Gateway). The default.
	NetworkAccessInternal = "internal"  // only from inside the VPC
	NetworkAccessFullOpen = "full_open" // front door AND the container port open to the internet (pre-v30)
	NetworkAccessNone     = "none"      // no inbound connections
)

var NetworkAccessValues = []string{
	NetworkAccessPublic,
	NetworkAccessInternal,
	NetworkAccessFullOpen,
	NetworkAccessNone,
}

// migrateToV30 pins every existing service to network_access: full_open.
//
// Before v30 every service's security group opened its container port to
// 0.0.0.0/0 and ::/0, and every service had an API Gateway route as well as
// its ALB wiring. full_open is exactly that. The template's default for an
// absent field is public (front door only), which is the safer choice for a
// NEW service but would narrow every existing one on its next apply and break
// anything that reaches a task's IP directly. Stamping full_open keeps an
// untouched environment at 0 to add, 0 to change, 0 to destroy, and puts the
// choice in the YAML where the owner can see it and tighten it.
//
// A service that already names a network_access — hand-written, or set in the
// web UI before the file was migrated — keeps it.
//
// The version is not set here. applyMigrations owns schema_version.
func migrateToV30(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v30: Pinning existing services to network_access: full_open")

	servicesRaw, exists := data["services"]
	if !exists || servicesRaw == nil {
		fmt.Println("    ℹ️  No services to migrate")
		return nil
	}

	services, ok := servicesRaw.([]interface{})
	if !ok {
		fmt.Println("    ⚠️  services is not an array, skipping")
		return nil
	}

	stamped := 0
	for _, serviceRaw := range services {
		service, ok := asYAMLMapping(serviceRaw)
		if !ok {
			continue
		}
		if !service.setIfAbsent("network_access", NetworkAccessFullOpen) {
			continue
		}
		stamped++
		if name := service.stringValue("name"); name != "" {
			fmt.Printf("    ✓ service '%s': Set network_access to full_open (current behaviour, no infrastructure change)\n", name)
		}
	}

	switch {
	case len(services) == 0:
		fmt.Println("    ℹ️  No services to migrate")
	case stamped == 0:
		fmt.Println("    ℹ️  Every service already has a network_access")
	default:
		fmt.Printf("    ✓ Pinned %d service(s) to full_open. Set network_access: public to stop exposing the container port directly.\n", stamped)
	}
	return nil
}
