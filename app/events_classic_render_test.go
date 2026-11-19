package main

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The classic event-bus render, pinned byte for byte. Each fixture in
// testdata/events_classic/<name>.yaml is a whole synthetic env YAML (env dev,
// account 000000000000). It is generated through applyTemplate, the function
// `meroku generate` runs, in a throwaway project root, and the env/dev/main.tf
// and env/dev/_bridge.tf it writes must equal <name>.main.tf.golden and
// <name>.bridge.tf.golden exactly. The goldens do not end in .tf, so
// terraform fmt leaves them alone: they hold the generator's output byte for
// byte, formatted or not.
//
// A fixture may carry custom Terraform in <name>.custom/: every *.tf there is
// copied to custom/terraform/dev/ and so appended to main.tf, as in a real
// project.
//
// Rewrite the goldens with:
//
//	go test -run TestClassicEventsRenderUnchanged -update ./...
//
// and review the diff: a changed golden means a classic environment's plan
// changes.

const eventsClassicDir = "testdata/events_classic"

func eventsClassicFixtures(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(eventsClassicDir, "*.yaml"))
	if err != nil {
		t.Fatalf("listing fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatalf("no fixtures in %s", eventsClassicDir)
	}
	names := make([]string, 0, len(paths))
	for _, p := range paths {
		names = append(names, strings.TrimSuffix(filepath.Base(p), ".yaml"))
	}
	sort.Strings(names)
	return names
}

// renderEventsClassicFixture generates one fixture and returns the main.tf and
// _bridge.tf it produced.
func renderEventsClassicFixture(t *testing.T, name string) (mainTf, bridgeTf string) {
	t.Helper()

	srcDir, err := filepath.Abs(eventsClassicDir)
	if err != nil {
		t.Fatal(err)
	}
	template, err := os.ReadFile(filepath.Join("..", "env", "main.hbs"))
	if err != nil {
		t.Fatalf("reading env/main.hbs: %v", err)
	}
	yamlBody, err := os.ReadFile(filepath.Join(srcDir, name+".yaml"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	root := t.TempDir()
	writeFile := func(rel string, data []byte) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(filepath.Join("infrastructure", "env", "main.hbs"), template)
	writeFile("dev.yaml", yamlBody)
	if err := os.MkdirAll(filepath.Join(root, "env", "dev"), 0o755); err != nil {
		t.Fatal(err)
	}

	customFiles, _ := filepath.Glob(filepath.Join(srcDir, name+".custom", "*.tf"))
	for _, f := range customFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(filepath.Join("custom", "terraform", "dev", filepath.Base(f)), data)
	}

	chdir(t, root)
	registerCustomHelpers()

	var genErr error
	out := captureOutput(t, func() { genErr = applyTemplate("dev") })
	if genErr != nil {
		t.Fatalf("generate refused the fixture: %v\n%s", genErr, out)
	}

	main, err := os.ReadFile(filepath.Join(root, "env", "dev", "main.tf"))
	if err != nil {
		t.Fatalf("reading generated main.tf: %v", err)
	}
	bridge, err := os.ReadFile(filepath.Join(root, "env", "dev", "_bridge.tf"))
	if err != nil {
		t.Fatalf("reading generated _bridge.tf: %v", err)
	}
	return string(main), string(bridge)
}

func TestClassicEventsRenderUnchanged(t *testing.T) {
	for _, name := range eventsClassicFixtures(t) {
		t.Run(name, func(t *testing.T) {
			// Absolute before rendering, which moves into a temp root.
			dir, err := filepath.Abs(eventsClassicDir)
			if err != nil {
				t.Fatal(err)
			}
			mainTf, bridgeTf := renderEventsClassicFixture(t, name)

			goldens := []struct{ path, got string }{
				{filepath.Join(dir, name+".main.tf.golden"), mainTf},
				{filepath.Join(dir, name+".bridge.tf.golden"), bridgeTf},
			}
			for _, g := range goldens {
				path := g.path
				if *updateGolden {
					if err := os.WriteFile(path, []byte(g.got), 0o644); err != nil {
						t.Fatal(err)
					}
					continue
				}
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("reading %s (run with -update to create it): %v", path, err)
				}
				if g.got != string(want) {
					t.Errorf("%s differs from the golden:\n%s", filepath.Base(path), eventsClassicFirstDiff(string(want), g.got))
				}
			}
		})
	}
}

// eventsClassicFirstDiff names the first line where want and got part, with a little
// context, so a one-line change in a long main.tf is readable.
func eventsClassicFirstDiff(want, got string) string {
	wl := strings.Split(want, "\n")
	gl := strings.Split(got, "\n")
	n := len(wl)
	if len(gl) < n {
		n = len(gl)
	}
	i := 0
	for i < n && wl[i] == gl[i] {
		i++
	}
	from := i - 3
	if from < 0 {
		from = 0
	}
	ctx := func(lines []string) string {
		to := i + 4
		if to > len(lines) {
			to = len(lines)
		}
		if from >= to {
			return "(end of file)"
		}
		return strings.Join(lines[from:to], "\n")
	}
	return "first difference at line " + strconv.Itoa(i+1) +
		" (want " + strconv.Itoa(len(wl)) + " lines, got " + strconv.Itoa(len(gl)) + ")\n--- want ---\n" +
		ctx(wl) + "\n--- got ---\n" + ctx(gl)
}
