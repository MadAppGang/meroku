package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `meroku generate` used to look for <env>.yaml at the working-directory root
// and nowhere else, while every other reader went through loadEnvWithMigration,
// which searches four locations — `project/<env>.yaml` among them.
//
// project/ is not a hypothetical layout: it is the one this repository ships
// (project/dev.yaml, project/prod.yaml) and the one CLAUDE.md documents. So a
// project following the documentation deployed fine, opened fine in the TUI and
// the web UI, and failed `meroku generate` with "environment file 'dev.yaml'
// not found" — a message naming a path that was never supposed to exist.

func TestResolveEnvFilePath(t *testing.T) {
	tests := []struct {
		name  string
		write []string // paths to create, relative to the temp root
		want  string
	}{
		{
			name:  "the working-directory root",
			write: []string{"dev.yaml"},
			want:  "dev.yaml",
		},
		{
			name:  "project/, the layout this repo itself uses",
			write: []string{filepath.Join("project", "dev.yaml")},
			want:  filepath.Join("project", "dev.yaml"),
		},
		{
			// The order is the loader's, not a preference of this function's:
			// generate must resolve to the same file every other reader does,
			// or the two disagree about what was generated from what.
			name:  "the root wins when both exist, as it does for the loader",
			write: []string{"dev.yaml", filepath.Join("project", "dev.yaml")},
			want:  "dev.yaml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			chdir(t, root)

			for _, rel := range tc.write {
				full := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(full, []byte("env: dev\n"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
			}

			got, err := resolveEnvFilePath("dev")
			if err != nil {
				t.Fatalf("resolveEnvFilePath: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveEnvFilePath = %q, want %q", got, tc.want)
			}
		})
	}
}

// A directory named dev.yaml is not a config, and taking it for one would only
// move the failure to the read.
func TestResolveEnvFilePathSkipsADirectory(t *testing.T) {
	root := t.TempDir()
	chdir(t, root)

	if err := os.MkdirAll(filepath.Join(root, "dev.yaml"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	realPath := filepath.Join(root, "project", "dev.yaml")
	if err := os.MkdirAll(filepath.Dir(realPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(realPath, []byte("env: dev\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := resolveEnvFilePath("dev")
	if err != nil {
		t.Fatalf("resolveEnvFilePath: %v", err)
	}
	if want := filepath.Join("project", "dev.yaml"); got != want {
		t.Errorf("resolveEnvFilePath = %q, want %q", got, want)
	}
}

// "not found" about a file the user is looking straight at is the least useful
// thing this could say, so the message lists everywhere it looked.
func TestResolveEnvFilePathNamesEveryPathTried(t *testing.T) {
	chdir(t, t.TempDir())

	_, err := resolveEnvFilePath("dev")
	if err == nil {
		t.Fatal("expected an error when no config exists anywhere")
	}
	for _, want := range []string{
		"dev.yaml",
		filepath.Join("project", "dev.yaml"),
		filepath.Join("..", "..", "project", "dev.yaml"),
		filepath.Join("..", "dev.yaml"),
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %q: %v", want, err)
		}
	}
}

// The bug end to end: the documented layout, and nothing at the root.
func TestGenerateFindsAConfigUnderProject(t *testing.T) {
	root := t.TempDir()
	chdir(t, root)

	projectDir := filepath.Join(root, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	writeStaleConfig(t, projectDir, "dev", CurrentSchemaVersion)

	templateDir := filepath.Join(root, "infrastructure", "env")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatalf("mkdir template dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "main.hbs"), []byte("# {{env}}\n"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "dev.yaml")); err == nil {
		t.Fatal("the fixture put a config at the root; this test is about the layout without one")
	}

	out := captureOutput(t, func() {
		if err := generateEnvironmentFiles("dev"); err != nil {
			t.Errorf("generateEnvironmentFiles: %v", err)
		}
	})

	generated, err := os.ReadFile(filepath.Join(root, "env", "dev", "main.tf"))
	if err != nil {
		t.Fatalf("generation wrote no main.tf: %v (output: %s)", err, out)
	}
	if !strings.Contains(string(generated), "dev") {
		t.Errorf("main.tf was rendered from something other than the config: %q", generated)
	}
}
