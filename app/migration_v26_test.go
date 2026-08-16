package main

import (
	"reflect"
	"testing"
)

// workload.backend_container_command is a list(string) in Terraform and was a
// scalar string in the model. v26 converts it, exactly as v25 did for scheduled
// tasks.
//
// The two fields failed differently, which is why they were fixed apart. The
// scheduled task's scalar reached main.tf unquoted through a raw stache, so
// people hand-wrote JSON arrays to make the output parse, and both forms exist
// on disk. This one was read from the wrong scope, so it rendered `[]` no matter
// what was written — nothing errored, and the container just ran its image's own
// CMD. Configs therefore contain whatever people tried before giving up.

func v26Doc(command interface{}) map[string]interface{} {
	workload := map[interface{}]interface{}{"backend_image_port": 8080}
	if command != nil {
		workload["backend_container_command"] = command
	}
	return map[string]interface{}{"workload": workload}
}

func v26Command(t *testing.T, doc map[string]interface{}) (interface{}, bool) {
	t.Helper()

	workload, ok := doc["workload"].(map[interface{}]interface{})
	if !ok {
		t.Fatalf("workload is not a mapping: %#v", doc["workload"])
	}
	value, present := workload["backend_container_command"]
	return value, present
}

func TestMigrateToV26ConvertsScalarCommands(t *testing.T) {
	tests := []struct {
		name  string
		given interface{}
		want  interface{}
	}{
		{
			name:  "bare command becomes one argument",
			given: "server",
			want:  []interface{}{"server"},
		},
		{
			name:  "hand-written JSON array is decoded into arguments",
			given: `["npm","start"]`,
			want:  []interface{}{"npm", "start"},
		},
		{
			// Splitting on whitespace would look right here and corrupt any
			// command with a quoted argument containing a space, so the whole
			// string is kept as one argument. Same rule as v25.
			name:  "a spaced command stays one argument",
			given: "npm start",
			want:  []interface{}{"npm start"},
		},
		{
			name:  "an already-converted list is left alone",
			given: []interface{}{"npm", "start"},
			want:  []interface{}{"npm", "start"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := v26Doc(test.given)
			if err := migrateToV26(doc); err != nil {
				t.Fatalf("migrateToV26: %v", err)
			}

			got, present := v26Command(t, doc)
			if !present {
				t.Fatal("backend_container_command was dropped")
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("got %#v, want %#v", got, test.want)
			}
		})
	}
}

// The shipped default was an empty string, and it meant "unset". Writing []
// would behave the same — the template guard treats both as falsy — but leaves a
// line in the user's YAML that says nothing.
func TestMigrateToV26DropsAnEmptyCommand(t *testing.T) {
	doc := v26Doc("")
	if err := migrateToV26(doc); err != nil {
		t.Fatalf("migrateToV26: %v", err)
	}

	if got, _ := v26Command(t, doc); got != nil {
		t.Errorf("got %#v, want the key cleared", got)
	}
}

func TestMigrateToV26LeavesUnrelatedConfigsAlone(t *testing.T) {
	t.Run("no workload section", func(t *testing.T) {
		doc := map[string]interface{}{"project": "demo"}
		if err := migrateToV26(doc); err != nil {
			t.Fatalf("migrateToV26: %v", err)
		}
		if _, present := doc["workload"]; present {
			t.Error("migration invented a workload section")
		}
	})

	t.Run("no command set", func(t *testing.T) {
		doc := v26Doc(nil)
		if err := migrateToV26(doc); err != nil {
			t.Fatalf("migrateToV26: %v", err)
		}
		if _, present := v26Command(t, doc); present {
			t.Error("migration invented a backend_container_command")
		}
	})

	// Generation reads YAML into map[string]interface{} via
	// convertToJSONCompatible, while a freshly decoded file is
	// map[interface{}]interface{}. Both reach this migration.
	t.Run("a string-keyed workload", func(t *testing.T) {
		doc := map[string]interface{}{
			"workload": map[string]interface{}{"backend_container_command": "server"},
		}
		if err := migrateToV26(doc); err != nil {
			t.Fatalf("migrateToV26: %v", err)
		}

		workload := doc["workload"].(map[string]interface{})
		want := []interface{}{"server"}
		if got := workload["backend_container_command"]; !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	// A value of an unexpected type is left for the Env decoder to reject with a
	// message naming the field, rather than mangled here.
	t.Run("an unexpected type", func(t *testing.T) {
		doc := v26Doc(42)
		if err := migrateToV26(doc); err != nil {
			t.Fatalf("migrateToV26: %v", err)
		}
		if got, _ := v26Command(t, doc); got != 42 {
			t.Errorf("got %#v, want the original 42 left untouched", got)
		}
	})
}

// Running it twice must be a no-op, so a re-run and an already-converted file
// behave the same.
func TestMigrateToV26IsIdempotent(t *testing.T) {
	doc := v26Doc("npm start")
	for i := range 2 {
		if err := migrateToV26(doc); err != nil {
			t.Fatalf("migrateToV26 run %d: %v", i+1, err)
		}
	}

	want := []interface{}{"npm start"}
	if got, _ := v26Command(t, doc); !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}
