package main

import (
	"reflect"
	"testing"
)

// scheduled_tasks[].container_command was a scalar string that the template
// rendered raw. It is now a list(string) end to end, so existing configs have
// to be converted on load.
//
// The values on disk are not uniform. Because the raw render put the scalar
// into HCL unquoted, the only way to get a valid list was to type the HCL
// yourself, so real configs contain a mix of bare commands and hand-written
// JSON arrays. Both have to come out the other side meaning the same thing.

func v25Doc(tasks ...map[interface{}]interface{}) map[string]interface{} {
	list := make([]interface{}, 0, len(tasks))
	for _, t := range tasks {
		list = append(list, t)
	}
	return map[string]interface{}{"scheduled_tasks": list}
}

func v25Command(t *testing.T, doc map[string]interface{}, index int) interface{} {
	t.Helper()
	tasks, ok := doc["scheduled_tasks"].([]interface{})
	if !ok {
		t.Fatal("scheduled_tasks is not a list")
	}
	task, ok := tasks[index].(map[interface{}]interface{})
	if !ok {
		t.Fatalf("scheduled_tasks[%d] is not a mapping", index)
	}
	return task["container_command"]
}

func TestMigrateToV25ConvertsScalarCommands(t *testing.T) {
	tests := []struct {
		name  string
		given interface{}
		want  interface{}
	}{
		{
			name:  "bare command becomes one argument",
			given: "cleanup",
			want:  []interface{}{"cleanup"},
		},
		{
			name:  "hand-written JSON array is decoded into arguments",
			given: `["npm","run","cron"]`,
			want:  []interface{}{"npm", "run", "cron"},
		},
		{
			name:  "JSON array with spaces after commas still decodes",
			given: `["npm", "run", "cron"]`,
			want:  []interface{}{"npm", "run", "cron"},
		},
		{
			// Splitting on whitespace would look right here and corrupt any
			// command with a quoted argument containing a space, so a
			// space-separated scalar is deliberately kept whole.
			name:  "space-separated scalar is kept as one argument",
			given: "npm run cron",
			want:  []interface{}{"npm run cron"},
		},
		{
			name:  "empty string becomes an empty list",
			given: "",
			want:  []interface{}{},
		},
		{
			name:  "bracketed but undecodable text is kept whole",
			given: "[not json at all",
			want:  []interface{}{"[not json at all"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := v25Doc(map[interface{}]interface{}{
				"name":              "cleanup",
				"container_command": tc.given,
			})

			if err := migrateToV25(doc); err != nil {
				t.Fatalf("migrateToV25: %v", err)
			}

			got := v25Command(t, doc, 0)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

// Running twice must not change anything the first run produced. Migrations
// re-run whenever a file's recorded version is behind, and a converted list
// must not be re-wrapped into a list containing a list.
func TestMigrateToV25IsIdempotent(t *testing.T) {
	doc := v25Doc(map[interface{}]interface{}{
		"name":              "cleanup",
		"container_command": `["npm","run","cron"]`,
	})

	if err := migrateToV25(doc); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first := v25Command(t, doc, 0)

	if err := migrateToV25(doc); err != nil {
		t.Fatalf("second run: %v", err)
	}
	second := v25Command(t, doc, 0)

	if !reflect.DeepEqual(first, second) {
		t.Errorf("second run changed the value: %#v -> %#v", first, second)
	}
}

func TestMigrateToV25LeavesOtherShapesAlone(t *testing.T) {
	t.Run("missing container_command", func(t *testing.T) {
		doc := v25Doc(map[interface{}]interface{}{"name": "cleanup"})
		if err := migrateToV25(doc); err != nil {
			t.Fatalf("migrateToV25: %v", err)
		}
		if got := v25Command(t, doc, 0); got != nil {
			t.Errorf("a task without a command gained one: %#v", got)
		}
	})

	t.Run("no scheduled_tasks key", func(t *testing.T) {
		doc := map[string]interface{}{}
		if err := migrateToV25(doc); err != nil {
			t.Fatalf("migrateToV25: %v", err)
		}
		if _, exists := doc["scheduled_tasks"]; exists {
			t.Error("migration invented a scheduled_tasks key")
		}
	})

	t.Run("scheduled_tasks is not a list", func(t *testing.T) {
		doc := map[string]interface{}{"scheduled_tasks": "nonsense"}
		if err := migrateToV25(doc); err != nil {
			t.Fatalf("migrateToV25 should skip, not fail: %v", err)
		}
	})

	t.Run("unexpected command type is left for the decoder", func(t *testing.T) {
		doc := v25Doc(map[interface{}]interface{}{
			"name":              "cleanup",
			"container_command": 42,
		})
		if err := migrateToV25(doc); err != nil {
			t.Fatalf("migrateToV25: %v", err)
		}
		if got := v25Command(t, doc, 0); got != 42 {
			t.Errorf("got %#v, want the original 42 left untouched", got)
		}
	})
}

// The version constant and the registered migration list have to agree, or a
// file is stamped with a version whose migration never ran.
//
// This asserted CurrentSchemaVersion == 25 outright. That is a pin, not an
// invariant: it fails on the next migration anyone writes, and the only possible
// fix is to edit the number, so it can never catch a real defect — it just costs
// whoever adds v26 a confusing red test. The same assertion was removed from the
// v24 test for the same reason and came back with v25, so it is spelt out here.
//
// The properties actually worth holding are below.
func TestMigrationChainIsWellFormed(t *testing.T) {
	if len(AllMigrations) == 0 {
		t.Fatal("no migrations are registered")
	}

	t.Run("v25 is still reachable", func(t *testing.T) {
		for _, migration := range AllMigrations {
			if migration.Version == 25 {
				return
			}
		}
		t.Error("migration v25 is no longer registered, so configs written before it can never be brought forward")
	})

	t.Run("the constant names the head of the chain", func(t *testing.T) {
		last := AllMigrations[len(AllMigrations)-1]
		if last.Version != CurrentSchemaVersion {
			t.Errorf("last registered migration is v%d, but CurrentSchemaVersion is %d",
				last.Version, CurrentSchemaVersion)
		}
	})

	// A duplicate version silently shadows one of the two migrations: the runner
	// compares against the file's stored version, so whichever is reached second
	// looks like it has already been applied. This repository has had that
	// collision before, when v21 was claimed twice.
	t.Run("versions ascend without gaps or repeats", func(t *testing.T) {
		for i := 1; i < len(AllMigrations); i++ {
			previous, current := AllMigrations[i-1].Version, AllMigrations[i].Version
			if current != previous+1 {
				t.Errorf("migration %d follows %d; versions must ascend by one", current, previous)
			}
		}
	})

	t.Run("every migration has a description and an implementation", func(t *testing.T) {
		for _, migration := range AllMigrations {
			if migration.Description == "" {
				t.Errorf("migration v%d has no description; it is printed to the user as it runs", migration.Version)
			}
			if migration.Apply == nil {
				t.Errorf("migration v%d has no Apply function", migration.Version)
			}
		}
	})
}
