package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// Migrations run in two kinds of caller: `meroku migrate` and the automatic
// file migration, which pass os.Stdout, and the read-only loaders behind the
// web API and the TUI warning gate, which pass io.Discard. A migration that
// prints straight to stdout ignores the writer it is given and leaks into the
// web server's status screen and above the TUI gate. These tests keep every
// migration writing only to its writer.

// migrationSourceFiles are the files that define the migrations themselves and
// the helpers they call; method calls are resolved against these files only.
func migrationSourceFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("migration*.go")
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, "env_policy.go")
	var out []string
	for _, f := range files {
		if !strings.HasSuffix(f, "_test.go") {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

// parsedPackage is every non-test file of package main, parsed once.
type parsedPackage struct {
	fset  *token.FileSet
	files map[string]*ast.File
	// funcs maps a package-level function name to its declaration.
	funcs map[string]*ast.FuncDecl
	// methods maps a method name to its declarations in migrationSourceFiles.
	methods map[string][]*ast.FuncDecl
	// fileOf maps a declaration to the file it is in.
	fileOf map[*ast.FuncDecl]*ast.File
}

func parsePackageForMigrations(t *testing.T) *parsedPackage {
	t.Helper()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	migrationFiles := map[string]bool{}
	for _, f := range migrationSourceFiles(t) {
		migrationFiles[f] = true
	}
	p := &parsedPackage{
		fset:    token.NewFileSet(),
		files:   map[string]*ast.File{},
		funcs:   map[string]*ast.FuncDecl{},
		methods: map[string][]*ast.FuncDecl{},
		fileOf:  map[*ast.FuncDecl]*ast.File{},
	}
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(p.fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		p.files[name] = file
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			p.fileOf[fn] = file
			if fn.Recv == nil {
				p.funcs[fn.Name.Name] = fn
			} else if migrationFiles[name] {
				p.methods[fn.Name.Name] = append(p.methods[fn.Name.Name], fn)
			}
		}
	}
	return p
}

// migrationApplyRoots returns the function names AllMigrations uses as Apply,
// read from the AllMigrations literal in migrations.go.
func migrationApplyRoots(t *testing.T, p *parsedPackage) []string {
	t.Helper()
	file := p.files["migrations.go"]
	if file == nil {
		t.Fatal("migrations.go not parsed")
	}
	var roots []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0].Name != "AllMigrations" {
			return true
		}
		for _, v := range spec.Values {
			ast.Inspect(v, func(n ast.Node) bool {
				kv, ok := n.(*ast.KeyValueExpr)
				if !ok {
					return true
				}
				if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Apply" {
					if fn, ok := kv.Value.(*ast.Ident); ok {
						roots = append(roots, fn.Name)
					} else {
						t.Errorf("%s: Apply is not a named function; the guard cannot follow it",
							p.fset.Position(kv.Value.Pos()))
					}
				}
				return true
			})
		}
		return false
	})
	if len(roots) == 0 {
		t.Fatal("found no Apply functions in AllMigrations")
	}
	return roots
}

// importName returns the name a file uses for an imported package path, or ""
// when the file does not import it.
func importName(file *ast.File, path string) string {
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != path {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return filepath.Base(path)
	}
	return ""
}

// directOutput lists every place fn writes to the process's stdout or stderr
// without going through a writer it was given.
func directOutput(p *parsedPackage, fn *ast.FuncDecl) []string {
	file := p.fileOf[fn]
	fmtName := importName(file, "fmt")
	osName := importName(file, "os")
	var found []string
	ast.Inspect(fn, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if id, ok := x.Fun.(*ast.Ident); ok && (id.Name == "print" || id.Name == "println") {
				found = append(found, p.fset.Position(x.Pos()).String()+": "+id.Name)
			}
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
				if pkg, ok := sel.X.(*ast.Ident); ok && fmtName != "" && pkg.Name == fmtName &&
					strings.HasPrefix(sel.Sel.Name, "Print") {
					found = append(found, p.fset.Position(x.Pos()).String()+": fmt."+sel.Sel.Name)
				}
			}
		case *ast.SelectorExpr:
			if pkg, ok := x.X.(*ast.Ident); ok && osName != "" && pkg.Name == osName &&
				(x.Sel.Name == "Stdout" || x.Sel.Name == "Stderr") {
				found = append(found, p.fset.Position(x.Pos()).String()+": os."+x.Sel.Name)
			}
		}
		return true
	})
	return found
}

// callees lists the declarations fn calls or refers to: package-level
// functions by name, and methods declared in the migration files by name.
func callees(p *parsedPackage, fn *ast.FuncDecl) []*ast.FuncDecl {
	var out []*ast.FuncDecl
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			out = append(out, p.methods[x.Sel.Name]...)
			// Do not treat the selected name as a package-level function.
			ast.Inspect(x.X, func(m ast.Node) bool {
				if id, ok := m.(*ast.Ident); ok {
					if decl := p.funcs[id.Name]; decl != nil {
						out = append(out, decl)
					}
				}
				return true
			})
			return false
		case *ast.Ident:
			if decl := p.funcs[x.Name]; decl != nil {
				out = append(out, decl)
			}
		}
		return true
	})
	return out
}

func TestMigrationsDoNotWriteToStdoutDirectly(t *testing.T) {
	p := parsePackageForMigrations(t)
	roots := migrationApplyRoots(t, p)

	// path records how each reachable function was reached, for the message.
	path := map[*ast.FuncDecl]string{}
	var queue []*ast.FuncDecl
	for _, name := range roots {
		decl := p.funcs[name]
		if decl == nil {
			t.Fatalf("Apply function %s is not declared in package main", name)
		}
		if _, seen := path[decl]; !seen {
			path[decl] = name
			queue = append(queue, decl)
		}
	}
	for len(queue) > 0 {
		fn := queue[0]
		queue = queue[1:]
		for _, callee := range callees(p, fn) {
			if _, seen := path[callee]; seen {
				continue
			}
			path[callee] = path[fn] + " -> " + callee.Name.Name
			queue = append(queue, callee)
		}
	}

	var violations []string
	for fn, via := range path {
		for _, v := range directOutput(p, fn) {
			violations = append(violations, v+" (reached as "+via+")")
		}
	}
	sort.Strings(violations)
	if len(violations) > 0 {
		t.Fatalf("migrations must write only to the io.Writer passed to Apply "+
			"(fmt.Fprint*(out, ...)), never to stdout or stderr directly:\n  %s",
			strings.Join(violations, "\n  "))
	}

	// Every file that defines migration code must have been reached from at
	// least one root, or the roots are not what this test thinks they are.
	reachedFiles := map[string]bool{}
	for fn := range path {
		reachedFiles[p.fset.Position(fn.Pos()).Filename] = true
	}
	for _, f := range migrationSourceFiles(t) {
		if !reachedFiles[f] {
			t.Errorf("no function in %s is reachable from AllMigrations; the guard has lost track of it", f)
		}
	}
}

// oldSchemaYAML crosses every migration that has optional output: a workload
// (v26 backend), named services (v26, v30), and a neon block with a database
// (v32). Synthetic values only.
const oldSchemaYAML = `schema_version: 1
project: myapp
env: dev
region: us-east-1
account_id: "000000000000"
workload:
  backend_image_port: 8080
services:
  - name: api
    container_port: 8080
  - name: worker
    container_port: 9090
neon:
  databases:
    - name: app
`

// captureStdoutStderr runs fn with os.Stdout and os.Stderr redirected to pipes
// and returns what was written to each.
func captureStdoutStderr(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	capture := func(target **os.File) (restore func() string) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("creating pipe: %v", err)
		}
		original := *target
		*target = w
		done := make(chan string, 1)
		go func() {
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)
			done <- buf.String()
		}()
		return func() string {
			*target = original
			w.Close()
			out := <-done
			r.Close()
			return out
		}
	}
	restoreOut := capture(&os.Stdout)
	restoreErr := capture(&os.Stderr)
	defer func() {
		stdout, stderr = restoreOut(), restoreErr()
	}()
	fn()
	return
}

// Not parallel: it swaps os.Stdout and os.Stderr.
func TestReadOnlyLoadersPrintNothingForOldSchema(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("dev.yaml", []byte(oldSchemaYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	loaders := []struct {
		name string
		run  func() error
	}{
		{"loadEnvMapReadOnly", func() error {
			_, envMap, err := loadEnvMapReadOnly("dev")
			if err == nil && envMap["schema_version"] != CurrentSchemaVersion {
				t.Errorf("loadEnvMapReadOnly did not migrate: schema_version = %v", envMap["schema_version"])
			}
			return err
		}},
		{"loadEnvReadOnly", func() error {
			_, err := loadEnvReadOnly("dev")
			return err
		}},
		{"collectEnvConfigWarnings", func() error {
			collectEnvConfigWarnings("dev", warningInputs{})
			return nil
		}},
	}
	for _, loader := range loaders {
		var err error
		stdout, stderr := captureStdoutStderr(t, func() { err = loader.run() })
		if err != nil {
			t.Fatalf("%s: %v", loader.name, err)
		}
		if stdout != "" {
			t.Errorf("%s wrote to stdout for a v1 YAML:\n%s", loader.name, stdout)
		}
		if stderr != "" {
			t.Errorf("%s wrote to stderr for a v1 YAML:\n%s", loader.name, stderr)
		}
	}

	after, err := os.ReadFile("dev.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != oldSchemaYAML {
		t.Errorf("a read-only loader rewrote dev.yaml:\n%s", after)
	}
}

// The migration text still exists; it goes to the writer the caller passes.
func TestApplyMigrationsWritesToGivenWriter(t *testing.T) {
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(oldSchemaYAML), &data); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	var err error
	stdout, _ := captureStdoutStderr(t, func() {
		err = applyMigrations(data, 1, &buf)
	})
	if err != nil {
		t.Fatalf("applyMigrations: %v", err)
	}
	if stdout != "" {
		t.Errorf("applyMigrations wrote to stdout instead of its writer:\n%s", stdout)
	}
	for _, want := range []string{
		"Successfully migrated",
		"Migrating to v26",
		"service 'api': Set runtime to fargate",
		"service 'worker': Set network_access to full_open",
		"Migrating to v31",
		"neon.databases 'app': create set to false",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("migration output lacks %q:\n%s", want, buf.String())
		}
	}
}
