package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// eventsRenderRoot is a throwaway project root that env YAMLs are generated
// in, through applyTemplate (what `meroku generate` runs), with the repo's
// env/main.hbs. Renders in one root share env/dev/main.tf, so a render sees
// the markers of the one before it, as a real project does.
type eventsRenderRoot struct {
	dir string
}

func newEventsRenderRoot(t *testing.T) *eventsRenderRoot {
	t.Helper()
	template, err := os.ReadFile(filepath.Join("..", "env", "main.hbs"))
	if err != nil {
		t.Fatalf("reading env/main.hbs: %v", err)
	}
	root := &eventsRenderRoot{dir: t.TempDir()}
	root.write(t, filepath.Join("infrastructure", "env", "main.hbs"), template)
	if err := os.MkdirAll(filepath.Join(root.dir, "env", "dev"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func (r *eventsRenderRoot) write(t *testing.T, rel string, data []byte) {
	t.Helper()
	path := filepath.Join(r.dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// stubEventsState makes the state probe answer with state for the rest of
// the test. The probe sets Needed itself, so a stub returns what was read:
// eventsAwsccState{} is a state that was not read.
func stubEventsState(t *testing.T, state func() eventsAwsccState) {
	t.Helper()
	saved := eventsAwsccStateFor
	t.Cleanup(func() { eventsAwsccStateFor = saved })
	eventsAwsccStateFor = func(context.Context, string, Env, bool) eventsAwsccState { return state() }
}

// render generates dev.yaml from yamlBody and returns the main.tf and
// _bridge.tf written, the printed output, and generate's error.
func (r *eventsRenderRoot) render(t *testing.T, yamlBody string) (mainTf, bridgeTf, output string, genErr error) {
	t.Helper()
	r.write(t, "dev.yaml", []byte(yamlBody))
	chdir(t, r.dir)
	registerCustomHelpers()
	output = captureOutput(t, func() { genErr = applyTemplate("dev") })
	if genErr != nil {
		return "", "", output, genErr
	}
	main, err := os.ReadFile(filepath.Join(r.dir, "env", "dev", "main.tf"))
	if err != nil {
		t.Fatalf("reading generated main.tf: %v", err)
	}
	bridge, err := os.ReadFile(filepath.Join(r.dir, "env", "dev", "_bridge.tf"))
	if err != nil {
		t.Fatalf("reading generated _bridge.tf: %v", err)
	}
	return string(main), string(bridge), output, nil
}

// mustRender is render that fails the test when generate refuses.
func (r *eventsRenderRoot) mustRender(t *testing.T, yamlBody string) (mainTf, bridgeTf string) {
	t.Helper()
	mainTf, bridgeTf, output, err := r.render(t, yamlBody)
	if err != nil {
		t.Fatalf("generate refused the YAML: %v\n%s", err, output)
	}
	return mainTf, bridgeTf
}

// hclTopBlocks returns the top-level blocks of a rendered main.tf whose
// header line starts with prefix (such as `module "event_subscriber_`), in
// file order, each from its header to the closing brace in column 0.
func hclTopBlocks(mainTf, prefix string) []string {
	var blocks []string
	lines := strings.Split(mainTf, "\n")
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], prefix) {
			continue
		}
		j := i
		for j < len(lines) && lines[j] != "}" {
			j++
		}
		blocks = append(blocks, strings.Join(lines[i:j+1], "\n"))
		i = j
	}
	return blocks
}

// hclModuleName is the label of a `module "<name>" {` block.
func hclModuleName(block string) string {
	header, _, _ := strings.Cut(block, "\n")
	name := strings.TrimPrefix(header, `module "`)
	name, _, _ = strings.Cut(name, `"`)
	return name
}
