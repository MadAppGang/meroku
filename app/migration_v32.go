package main

import "fmt"

// migrateToV32 writes the Neon defaults into an existing `neon:` block:
// owner_role neondb_owner, create: false on every database, and in create
// mode project.pg_version 17. An env with no `neon:` block is not touched;
// Neon is opt-in.
//
// Every value is written only where the key is absent, so a user's choice is
// kept and a second run changes nothing. The version is not set here.
// applyMigrations owns schema_version.
func migrateToV32(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v32: Neon Postgres defaults (opt-in)")

	neon, ok := asYAMLMapping(data["neon"])
	if !ok {
		fmt.Println("    ℹ️  No neon block; nothing to do")
		return nil
	}

	if neon.setIfAbsent("owner_role", neonDefaultOwnerRole) {
		fmt.Printf("    ✓ neon.owner_role set to %s\n", neonDefaultOwnerRole)
	}

	if raw, present := neon.lookup("databases"); present {
		if list, ok := raw.([]interface{}); ok {
			for _, item := range list {
				if db, ok := asYAMLMapping(item); ok && db.setIfAbsent("create", false) {
					fmt.Printf("    ✓ neon.databases '%s': create set to false (an existing database)\n", db.stringValue("name"))
				}
			}
		}
	}

	if neon.stringValue("mode") == NeonModeCreate {
		if raw, present := neon.lookup("project"); present {
			if project, ok := asYAMLMapping(raw); ok && project.setIfAbsent("pg_version", neonDefaultPGVersion) {
				fmt.Printf("    ✓ neon.project.pg_version set to %d\n", neonDefaultPGVersion)
			}
		}
	}
	return nil
}

// checkSchemaNotNewer refuses a file written by a newer meroku. Loading it
// would drop the keys this binary does not know on the next save, and
// migrating it would stamp an older version over it.
func checkSchemaNotNewer(file string, version int) error {
	if version <= CurrentSchemaVersion {
		return nil
	}
	return fmt.Errorf("%s was written by a newer meroku (schema v%d, this binary understands v%d). Update meroku; this file was not changed",
		file, version, CurrentSchemaVersion)
}
