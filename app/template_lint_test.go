package main

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"

	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// Static checks over env/main.hbs.
//
// Every defect this file guards against rendered successfully. raymond has no
// notion of a value being the wrong shape for the hole it is going into: an
// unknown path is the empty string, an unregistered helper is an unknown path,
// and a slice printed without a helper is its elements run together. All three
// produce output, so generation reports success, and the mistake surfaces
// somewhere else entirely — at terraform init, at apply, or not at all.
//
// validateGeneratedHCL catches the subset that stops the output parsing. These
// checks cover the rest, where the generated Terraform is perfectly well-formed
// and simply wrong.

// mustacheToken is one {{...}} in the template.
type mustacheToken struct {
	line int
	// raw is a triple-stache, {{{...}}}, which skips HTML escaping.
	raw bool
	// body is the text between the braces, trimmed.
	body string
	// eachDepth is how many {{#each}} blocks enclose this token. Custom block
	// helpers all render through options.Fn(), so #each is the only construct in
	// this template that changes the context a path resolves against.
	eachDepth int
}

func TestMainHBSLint(t *testing.T) {
	template := readMainHBS(t)
	tokens := scanMustaches(t, template)
	helpers := registeredHelperNames(t)
	rootKeys := fixtureRootKeys(t)

	t.Run("every triple-stache is a helper call", func(t *testing.T) {
		// A bare {{{ x }}} hands the value to raymond's Str(), which concatenates
		// a slice's elements with nothing between them: ["npm","run","cron"]
		// renders as `npmruncron`. That is a valid HCL identifier, so it parses,
		// and Terraform then reports a reference to an undeclared resource.
		//
		// Scalars survive it, which is why these sites sit unnoticed until the
		// field they read becomes a list.
		for _, token := range tokens {
			if !token.raw || isBlockToken(token.body) {
				continue
			}
			invoked, _ := classifyAtoms(bodyAtoms(token.body), helpers)
			if len(invoked) == 0 {
				t.Errorf("env/main.hbs:%d: {{{%s}}} renders a value directly.\n"+
					"    A list rendered this way concatenates into one bare word.\n"+
					"    Use a helper that emits HCL: {{{array %s}}}",
					token.line, token.body, token.body)
			}
		}
	})

	t.Run("every helper invoked is registered", func(t *testing.T) {
		// raymond resolves an unknown helper name as a path. It finds nothing and
		// renders the empty string, so `jsonencode({{{json x}}})` became
		// `jsonencode()` — well-formed HCL that fails at terraform validate on
		// argument count, and only for configs that set the field.
		for _, token := range tokens {
			invoked, _ := classifyAtoms(bodyAtoms(token.body), helpers)
			for _, name := range invoked {
				if !helpers[name] {
					t.Errorf("env/main.hbs:%d: {{%s}} calls helper %q, which is not registered in raymond.go.\n"+
						"    An unregistered helper renders as nothing, silently.",
						token.line, token.body, name)
				}
			}
		}
	})

	t.Run("bare paths outside an each resolve against the config root", func(t *testing.T) {
		// The template has no {{#with}}, so outside an {{#each}} the context is
		// always the whole config. An undotted path there must therefore be a
		// top-level key.
		//
		// backend_container_command was not. Its guard read
		// workload.backend_container_command and its body read the bare name, so
		// the helper got nil and rendered [] — a valid empty list(string), which
		// Terraform accepted and which meant the user's command never applied.
		// Nothing anywhere reported a problem.
		for _, token := range tokens {
			if token.eachDepth > 0 {
				continue
			}
			_, paths := classifyAtoms(bodyAtoms(token.body), helpers)
			for _, path := range paths {
				root, _, _ := strings.Cut(path, ".")
				if root == "" || rootKeys[root] {
					continue
				}
				t.Errorf("env/main.hbs:%d: {{%s}} reads %q, which is not a top-level config key.\n"+
					"    Outside an {{#each}} the context is the config root, so this resolves to nothing.\n"+
					"    Did you mean workload.%s?",
					token.line, token.body, path, path)
			}
		}
	})
}

func readMainHBS(t *testing.T) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "env", "main.hbs"))
	if err != nil {
		t.Fatalf("reading env/main.hbs: %v", err)
	}
	return string(raw)
}

// registeredHelperNames reads the helper names out of raymond.go, so that
// registering a helper is all it takes to make the template allowed to call it.
// A hand-maintained list here would be one more thing to forget.
var registerHelperCall = regexp.MustCompile(`RegisterHelper\("([A-Za-z0-9_]+)"`)

// raymondBuiltins are the block and value helpers raymond provides itself.
// `else` is not a helper but appears in helper position of an inverse section.
var raymondBuiltins = []string{"if", "unless", "each", "with", "log", "lookup", "else"}

func registeredHelperNames(t *testing.T) map[string]bool {
	t.Helper()

	source, err := os.ReadFile("raymond.go")
	if err != nil {
		t.Fatalf("reading raymond.go: %v", err)
	}

	names := map[string]bool{}
	for _, name := range raymondBuiltins {
		names[name] = true
	}
	for _, match := range registerHelperCall.FindAllStringSubmatch(string(source), -1) {
		names[match[1]] = true
	}
	if len(names) == len(raymondBuiltins) {
		t.Fatal("found no RegisterHelper calls in raymond.go; the lint would pass vacuously")
	}
	return names
}

// fixtureRootKeys is the set of top-level names a path may legitimately start
// with: the Env struct's own yaml tags, the keys of the shipped config, and what
// applyTemplate injects.
//
// Both config sources are needed. The struct is the schema, but generation reads
// the YAML into a plain map and never goes through it, so a key can be real and
// working without a field — `efs` is one today. The shipped config covers those,
// and the struct covers optional keys no fixture happens to set.
func fixtureRootKeys(t *testing.T) map[string]bool {
	t.Helper()

	keys := map[string]bool{}

	envType := reflect.TypeOf(Env{})
	for i := range envType.NumField() {
		name, _, _ := strings.Cut(envType.Field(i).Tag.Get("yaml"), ",")
		if name != "" && name != "-" {
			keys[name] = true
		}
	}

	raw, err := os.ReadFile(filepath.Join("..", "project", "dev.yaml"))
	if err != nil {
		t.Fatalf("reading project/dev.yaml: %v", err)
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing project/dev.yaml: %v", err)
	}
	for key := range doc {
		keys[key] = true
	}

	// Set by applyTemplate rather than by the config file.
	for _, injected := range []string{"modules", "custom_modules", "has_custom_pre", "has_custom_post"} {
		keys[injected] = true
	}
	return keys
}

// scanMustaches walks the template and returns every {{...}}, tracking how many
// {{#each}} blocks enclose it.
//
// This is a scanner rather than a regexp because a triple-stache and a
// double-stache differ only in a brace that also terminates the other, and
// getting that wrong silently reclassifies exactly the tokens under test.
func scanMustaches(t *testing.T, template string) []mustacheToken {
	t.Helper()

	var tokens []mustacheToken
	eachDepth := 0
	line := 1

	for i := 0; i < len(template); {
		if template[i] == '\n' {
			line++
			i++
			continue
		}
		if !strings.HasPrefix(template[i:], "{{") {
			i++
			continue
		}

		raw := strings.HasPrefix(template[i:], "{{{")
		open, close := "{{", "}}"
		if raw {
			open, close = "{{{", "}}}"
		}

		rest := template[i+len(open):]
		end := strings.Index(rest, close)
		if end < 0 {
			t.Fatalf("env/main.hbs:%d: unterminated %s", line, open)
		}
		body := rest[:end]

		// Comments can contain anything, including braces and stray helper-looking
		// words, so they are skipped rather than parsed.
		if !strings.HasPrefix(strings.TrimSpace(body), "!") {
			trimmed := strings.TrimSpace(body)
			switch {
			case strings.HasPrefix(trimmed, "#each"):
				tokens = append(tokens, mustacheToken{line: line, raw: raw, body: trimmed, eachDepth: eachDepth})
				eachDepth++
			case strings.HasPrefix(trimmed, "/each"):
				eachDepth--
				tokens = append(tokens, mustacheToken{line: line, raw: raw, body: trimmed, eachDepth: eachDepth})
			default:
				tokens = append(tokens, mustacheToken{line: line, raw: raw, body: trimmed, eachDepth: eachDepth})
			}
		}

		line += strings.Count(body, "\n")
		i += len(open) + end + len(close)
	}

	if eachDepth != 0 {
		t.Fatalf("env/main.hbs: %d unclosed {{#each}} block(s)", eachDepth)
	}
	return tokens
}

func isBlockToken(body string) bool {
	return strings.HasPrefix(body, "#") || strings.HasPrefix(body, "/")
}

// atomize splits a mustache body into atoms: identifiers, literals, and the
// parentheses that delimit subexpressions. A quoted string is a single atom even
// when it contains spaces or parentheses.
//
// The first version of this used a regexp anchored on `(?:^|\()` and was wrong
// in a way worth recording: matching the opening parenthesis consumes it, so in
// `{{#if (exists x)}}` the match for `if` swallowed the `(` and `exists` was
// never seen in helper position at all. Go's RE2 has no lookahead to express
// "preceded by" without consuming, so the scan is explicit instead.
func atomize(body string) []string {
	var atoms []string
	var current strings.Builder

	flush := func() {
		if current.Len() > 0 {
			atoms = append(atoms, current.String())
			current.Reset()
		}
	}

	var quote rune
	for _, r := range body {
		switch {
		case quote != 0:
			current.WriteRune(r)
			if r == quote {
				quote = 0
				flush()
			}
		case r == '"' || r == '\'':
			flush()
			quote = r
			current.WriteRune(r)
		case r == '(' || r == ')':
			flush()
			atoms = append(atoms, string(r))
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return atoms
}

// classifyAtoms splits a mustache body into the helpers it invokes and the
// config paths it reads.
//
// An atom is in helper position when it opens the body or follows a `(`, and is
// an invocation when something other than a closing parenthesis follows it.
// {{else}} and a no-argument helper are in helper position with nothing after
// them, which is why the registered set is consulted as well.
func classifyAtoms(atoms []string, helpers map[string]bool) (invoked, paths []string) {
	for i, atom := range atoms {
		if atom == "(" || atom == ")" {
			continue
		}

		helperPos := i == 0 || atoms[i-1] == "("
		hasArgument := i+1 < len(atoms) && atoms[i+1] != ")"

		switch {
		case helperPos && hasArgument:
			invoked = append(invoked, atom)
		case helperPos && helpers[atom]:
			// {{else}}, or a helper called with no arguments.
		case isTemplateLiteral(atom):
		default:
			paths = append(paths, atom)
		}
	}
	return invoked, paths
}

// isTemplateLiteral reports whether an atom is something other than a path into
// the config.
func isTemplateLiteral(atom string) bool {
	switch {
	// @root, @index, @key and friends. @root.x is an explicit escape back to the
	// config root and is correct wherever it appears, so it needs no scope check.
	case strings.HasPrefix(atom, "@"):
		return true
	// ../ walks to the parent context, which this lint does not model.
	case strings.HasPrefix(atom, "../"):
		return true
	case strings.HasPrefix(atom, `"`), strings.HasPrefix(atom, `'`):
		return true
	case atom == "true", atom == "false", atom == "this", atom == ".":
		return true
	// Hash arguments (key=value).
	case strings.Contains(atom, "="):
		return true
	}
	_, err := strconv.ParseFloat(atom, 64)
	return err == nil
}

// bodyAtoms returns the atoms of a body, with the block marker removed. A
// closing tag reads nothing and contributes nothing.
func bodyAtoms(body string) []string {
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "/") {
		return nil
	}
	return atomize(strings.TrimPrefix(trimmed, "#"))
}

// TestMainHBSLint reads the real template, so if the classifier below quietly
// misreads everything the lint passes and proves nothing. These are the bodies
// of the five defects it was written for, plus the constructs that must not be
// mistaken for them.
func TestLintClassifiesMustacheBodies(t *testing.T) {
	helpers := map[string]bool{
		"if": true, "each": true, "unless": true, "else": true,
		"array": true, "json": true, "gt": true, "len": true,
		"compare": true, "default": true, "exists": true,
	}

	tests := []struct {
		name        string
		body        string
		wantInvoked []string
		wantPaths   []string
	}{
		// The five defects.
		{
			name:      "a bare value renders directly",
			body:      " container_command ",
			wantPaths: []string{"container_command"},
		},
		{
			name:        "an unregistered helper is still in helper position",
			body:        "concat @root.project \"/\" name",
			wantInvoked: []string{"concat"},
			wantPaths:   []string{"name"},
		},
		{
			name:        "a mis-scoped argument is a path",
			body:        "array backend_container_command",
			wantInvoked: []string{"array"},
			wantPaths:   []string{"backend_container_command"},
		},
		{
			name:        "nested subexpressions are all seen",
			body:        "#if (gt (len domain_aliases) 1)",
			wantInvoked: []string{"if", "gt", "len"},
			wantPaths:   []string{"domain_aliases"},
		},
		{
			name:        "a helper inside a default is seen",
			body:        `default logging.prefix (concat @root.project "/")`,
			wantInvoked: []string{"default", "concat"},
			wantPaths:   []string{"logging.prefix"},
		},

		// Constructs that must not be flagged.
		{
			name: "else is a helper with no arguments",
			body: "else",
		},
		{
			name:        "an each names its collection",
			body:        "#each extensions.sns_topics",
			wantInvoked: []string{"each"},
			wantPaths:   []string{"extensions.sns_topics"},
		},
		{
			name: "a closing tag reads nothing",
			body: "/each",
		},
		{
			name: "@root is an explicit escape to the config root",
			body: "@root.project",
		},
		{
			name: "../ walks to the parent context",
			body: "../name",
		},
		{
			name:      "a plain dotted path is a path",
			body:      "workload.setup_fcnsns",
			wantPaths: []string{"workload.setup_fcnsns"},
		},
		{
			name:        "compare takes a quoted operator and a number",
			body:        `#compare (len efs) ">" 0`,
			wantInvoked: []string{"compare", "len"},
			wantPaths:   []string{"efs"},
		},
		{
			name:        "a quoted string containing spaces is one atom",
			body:        `default title "a b c"`,
			wantInvoked: []string{"default"},
			wantPaths:   []string{"title"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invoked, paths := classifyAtoms(bodyAtoms(test.body), helpers)
			assertSameStrings(t, "helpers", invoked, test.wantInvoked)
			assertSameStrings(t, "paths", paths, test.wantPaths)
		})
	}
}

func TestLintScannerTracksRawnessAndEachDepth(t *testing.T) {
	const template = `
{{project}}
{{{array buckets}}}
{{#each services}}
  {{name}}
  {{#each ports}}{{this}}{{/each}}
{{/each}}
{{region}}
{{!-- {{#each never}} a comment is not a block --}}
`
	tokens := scanMustaches(t, template)

	byBody := map[string]mustacheToken{}
	for _, token := range tokens {
		byBody[token.body] = token
	}

	if token, ok := byBody["project"]; !ok || token.raw {
		t.Errorf("{{project}}: got %+v, want a non-raw token", token)
	}
	if token, ok := byBody["array buckets"]; !ok || !token.raw {
		t.Errorf("{{{array buckets}}}: got %+v, want a raw token", token)
	}
	if token := byBody["name"]; token.eachDepth != 1 {
		t.Errorf("{{name}} inside one each: eachDepth = %d, want 1", token.eachDepth)
	}
	if token := byBody["this"]; token.eachDepth != 2 {
		t.Errorf("{{this}} inside two eaches: eachDepth = %d, want 2", token.eachDepth)
	}
	// The each blocks have to balance back out, or every token after them is
	// classified against the wrong scope.
	if token := byBody["region"]; token.eachDepth != 0 {
		t.Errorf("{{region}} after the each closed: eachDepth = %d, want 0", token.eachDepth)
	}
	for _, token := range tokens {
		if strings.Contains(token.body, "never") {
			t.Errorf("a comment was scanned as a token: %+v", token)
		}
	}
}

func assertSameStrings(t *testing.T, what string, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Errorf("%s: got %v, want %v", what, got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s: got %v, want %v", what, got, want)
			return
		}
	}
}
