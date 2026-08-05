package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/aymerick/raymond"
)

// Helpers that emit HCL or JSON must never be HTML-escaped by Handlebars.
//
// The regression: `array`, `mmap`, `envArray` and `envToEnvArray` returned a
// plain string, so a double-stache call site turned every " into &quot;. Three
// call sites in env/main.hbs used {{array ...}} rather than {{{array ...}}} —
// including cognito.dashboard_callback_urls — which produced
//
//	dashboard_callback_urls = [&quot;https://jwt.io&quot;]
//
// That is not valid HCL, so `terraform init` failed for any project with cognito
// enabled. Returning raymond.SafeString makes both stache styles emit raw text.
func TestArrayHelper_IsNotHTMLEscaped(t *testing.T) {
	registerCustomHelpers()

	ctx := map[string]interface{}{
		"urls": []interface{}{"https://jwt.io", "https://example.com/cb"},
	}

	for _, tpl := range []string{`{{array urls}}`, `{{{array urls}}}`} {
		out, err := raymond.Render(tpl, ctx)
		if err != nil {
			t.Fatalf("render %s: %v", tpl, err)
		}
		if strings.Contains(out, "&quot;") {
			t.Errorf("%s produced HTML-escaped quotes, which is invalid HCL: %s", tpl, out)
		}
		if !strings.Contains(out, `"https://jwt.io"`) {
			t.Errorf("%s should emit real quotes, got %s", tpl, out)
		}
	}
}

func TestEnvArrayHelper_IsNotHTMLEscaped(t *testing.T) {
	registerCustomHelpers()

	ctx := map[string]interface{}{
		"vars": map[string]interface{}{"API_URL": "https://api.example.com"},
	}

	out, err := raymond.Render(`{{envArray vars}}`, ctx)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(out, "&quot;") {
		t.Errorf("envArray produced HTML-escaped quotes: %s", out)
	}
}

// Generation must be reproducible. Go randomises map iteration, so helpers and
// {{#each}} blocks that walk maps used to emit a different order every run —
// meaning `meroku generate` produced a diff even when nothing changed, burying
// real changes in noise.
func TestSortedPairs_IsDeterministicAndSorted(t *testing.T) {
	registerCustomHelpers()

	ctx := map[string]interface{}{
		"vars": map[string]interface{}{
			"REACT_APP_ENV":                  "production",
			"REACT_APP_API_URL":              "https://api.example.com",
			"REACT_APP_COGNITO_REGION":       "ap-southeast-2",
			"REACT_APP_COGNITO_CLIENT_ID":    "abc",
			"REACT_APP_COGNITO_USER_POOL_ID": "def",
		},
	}
	const tpl = `{{#each (sortedPairs vars)}}{{key}}={{value}};{{/each}}`

	first, err := raymond.Render(tpl, ctx)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// Repeat enough times that random map order would almost certainly differ.
	for i := 0; i < 50; i++ {
		out, err := raymond.Render(tpl, ctx)
		if err != nil {
			t.Fatalf("render %d: %v", i, err)
		}
		if out != first {
			t.Fatalf("output changed between renders:\n  run 0: %s\n  run %d: %s", first, i, out)
		}
	}

	want := "REACT_APP_API_URL=https://api.example.com;" +
		"REACT_APP_COGNITO_CLIENT_ID=abc;" +
		"REACT_APP_COGNITO_REGION=ap-southeast-2;" +
		"REACT_APP_COGNITO_USER_POOL_ID=def;" +
		"REACT_APP_ENV=production;"
	if first != want {
		t.Errorf("expected key-sorted output\n  want: %s\n  got:  %s", want, first)
	}
}

// YAML unmarshalling yields map[interface{}]interface{}; that shape must sort too.
func TestSortedPairs_HandlesYAMLMapShape(t *testing.T) {
	registerCustomHelpers()

	ctx := map[string]interface{}{
		"vars": map[interface{}]interface{}{"b": "2", "a": "1", "c": "3"},
	}

	out, err := raymond.Render(`{{#each (sortedPairs vars)}}{{key}}{{/each}}`, ctx)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if out != "abc" {
		t.Errorf("expected sorted keys \"abc\", got %q", out)
	}
}

// A non-map input must not panic — templates call this on optional config.
func TestSortedPairs_NonMapReturnsNothing(t *testing.T) {
	registerCustomHelpers()

	for _, tpl := range []string{
		`{{#each (sortedPairs missing)}}x{{/each}}`,
		`{{#each (sortedPairs aString)}}x{{/each}}`,
	} {
		out, err := raymond.Render(tpl, map[string]interface{}{"aString": "not a map"})
		if err != nil {
			t.Fatalf("render %s: %v", tpl, err)
		}
		if out != "" {
			t.Errorf("%s should render nothing, got %q", tpl, out)
		}
	}
}

// mmap sorts its keys too (it is the map-shaped sibling of sortedPairs).
func TestMmapHelper_IsSorted(t *testing.T) {
	registerCustomHelpers()

	ctx := map[string]interface{}{
		"m": map[string]interface{}{"zebra": "1", "alpha": "2", "middle": "3"},
	}

	out, err := raymond.Render(`{{mmap m}}`, ctx)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	alpha := strings.Index(out, "alpha")
	middle := strings.Index(out, "middle")
	zebra := strings.Index(out, "zebra")
	if alpha < 0 || middle < 0 || zebra < 0 {
		t.Fatalf("expected all keys present, got %s", out)
	}
	if !(alpha < middle && middle < zebra) {
		t.Errorf("expected keys in sorted order, got %s", out)
	}
}

// An absent optional array is an empty array.
//
// The regression: the array helper panicked on nil. env/main.hbs calls
// {{array ...}} at twenty sites — efs, buckets, services, ses.test_emails,
// cognito.dashboard_callback_urls and the rest — and every one of them reads an
// optional key. Any config omitting a single one of them crashed generation with
// a Go stack trace instead of producing output.
//
// It is a stack trace rather than an error message because raymond's errRecover
// only converts a panic carrying an `error`; a panic carrying a string is
// re-panicked and unwinds all the way out to the user.
func TestArrayHelper_NilRendersEmptyList(t *testing.T) {
	registerCustomHelpers()

	// Every shape a missing or empty list arrives in.
	for name, ctx := range map[string]map[string]interface{}{
		"key absent from the config": {},
		"key present but null":       {"items": nil},
		"empty YAML sequence":        {"items": []interface{}{}},
		"nil []interface{}":          {"items": []interface{}(nil)},
		"nil []string":               {"items": []string(nil)},
		"nil []map":                  {"items": []map[string]interface{}(nil)},
	} {
		for _, tpl := range []string{`{{array items}}`, `{{{array items}}}`} {
			out, err := raymond.Render(tpl, ctx)
			if err != nil {
				t.Errorf("%s via %s: %v", name, tpl, err)
				continue
			}
			if out != "[]" {
				t.Errorf("%s via %s: got %q, want %q", name, tpl, out, "[]")
			}
		}
	}
}

// `null` is not a valid HCL list. A typed nil slice marshals to it, so that
// shape has to be caught before json.Marshal sees it.
func TestArrayHelper_NeverEmitsNull(t *testing.T) {
	registerCustomHelpers()

	out, err := raymond.Render(`{{array items}}`, map[string]interface{}{"items": []string(nil)})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(out, "null") {
		t.Errorf("got %q — terraform rejects null where it wants list(string)", out)
	}
}

// A populated list still renders as before.
func TestArrayHelper_RendersValues(t *testing.T) {
	registerCustomHelpers()

	out, err := raymond.Render(`{{array items}}`, map[string]interface{}{
		"items": []interface{}{"email", "phone_number"},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if out != `["email","phone_number"]` {
		t.Errorf("got %q", out)
	}
}

// A wrong type is a real configuration error — `buckets: my-bucket` instead of a
// list is a mistake worth stopping for. But it must surface as an error raymond
// can return, so the caller prints a message; panicking with a string reaches
// the user as a stack trace, which is never the right way to report bad YAML.
func TestArrayHelper_WrongTypeIsAnErrorNotAPanic(t *testing.T) {
	registerCustomHelpers()

	for name, value := range map[string]interface{}{
		"string": "https://jwt.io",
		"int":    3,
		"bool":   true,
		"map":    map[string]interface{}{"a": "b"},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("helper panicked out of raymond instead of returning an error: %v", r)
				}
			}()

			out, err := raymond.Render(`{{array items}}`, map[string]interface{}{"items": value})
			if err == nil {
				t.Fatalf("a %s where a list belongs must fail, got %q", name, out)
			}
			if !strings.Contains(err.Error(), "expected a list") {
				t.Errorf("message should say what was wrong, got: %v", err)
			}
		})
	}
}

// The message must name the offending value, so the user can find it in a file
// they wrote rather than a Go type they did not.
func TestArrayHelper_WrongTypeNamesTheValue(t *testing.T) {
	registerCustomHelpers()

	_, err := raymond.Render(`{{array items}}`, map[string]interface{}{"items": "https://jwt.io"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), `"https://jwt.io"`) {
		t.Errorf("message must quote the offending value, got: %v", err)
	}
}

// raymond gives a helper no access to the expression it was called with, and
// main.hbs has twenty {{array ...}} sites, so "expected a list, got string"
// alone leaves the user hunting. describeArrayHelperError fills in the key, and
// runs only after a render has already failed so it can never cause one.
func TestDescribeArrayHelperError_NamesTheKey(t *testing.T) {
	registerCustomHelpers()

	const tpl = `{{array cognito.dashboard_callback_urls}}`
	envMap := map[string]interface{}{
		"cognito": map[string]interface{}{
			"enabled":                 true,
			"dashboard_callback_urls": "https://jwt.io",
		},
	}

	_, err := raymond.Render(tpl, envMap)
	if err == nil {
		t.Fatal("expected the render to fail")
	}

	described := describeArrayHelperError(err, tpl, envMap)
	if !strings.Contains(described.Error(), "cognito.dashboard_callback_urls") {
		t.Errorf("expected the offending key to be named, got: %v", described)
	}
	if !strings.Contains(described.Error(), `"https://jwt.io"`) {
		t.Errorf("expected the offending value to be quoted, got: %v", described)
	}
}

// The call sites inside {{#each}} blocks — actions, resources, detail_types —
// have paths relative to the loop, so matching on leaf key name is what makes
// them reachable at all.
func TestDescribeArrayHelperError_NamesKeysInsideLists(t *testing.T) {
	registerCustomHelpers()

	const tpl = `{{#each services}}{{array actions}}{{/each}}`
	envMap := map[string]interface{}{
		"services": []interface{}{
			map[string]interface{}{"name": "api", "actions": "s3:GetObject"},
		},
	}

	_, err := raymond.Render(tpl, envMap)
	if err == nil {
		t.Fatal("expected the render to fail")
	}

	described := describeArrayHelperError(err, tpl, envMap)
	if !strings.Contains(described.Error(), "services[0].actions") {
		t.Errorf("expected the indexed path to be named, got: %v", described)
	}
}

// Anything that is not an array helper failure passes through untouched.
func TestDescribeArrayHelperError_LeavesOtherErrorsAlone(t *testing.T) {
	original := errors.New("some other template problem")
	if got := describeArrayHelperError(original, "", nil); got != original {
		t.Errorf("got %v, want the original error unchanged", got)
	}
	if got := describeArrayHelperError(nil, "", nil); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// Many {{array ...}} sites sit inside an {{#if}} that a scalar never gets past —
// {{#if workload.backend_container_command}} guards one, so
// backend_container_command: "" reaches nothing and is working as intended.
// Matching on key name alone listed it anyway, sending the reader to a key that
// was not the problem. A candidate has to hold the value the helper choked on.
func TestDescribeArrayHelperError_SkipsKeysThatNeverReachedTheHelper(t *testing.T) {
	registerCustomHelpers()

	const tpl = `{{#if backend_container_command}}{{array backend_container_command}}{{/if}}` +
		`{{array cognito.dashboard_callback_urls}}`
	envMap := map[string]interface{}{
		"backend_container_command": "",
		"cognito": map[string]interface{}{
			"dashboard_callback_urls": "https://jwt.io",
		},
	}

	_, err := raymond.Render(tpl, envMap)
	if err == nil {
		t.Fatal("expected the render to fail")
	}

	described := describeArrayHelperError(err, tpl, envMap).Error()
	if !strings.Contains(described, "cognito.dashboard_callback_urls") {
		t.Errorf("the key that actually failed must be named, got: %s", described)
	}
	if strings.Contains(described, "backend_container_command") {
		t.Errorf("a guarded key that never reached the helper must not be blamed, got: %s", described)
	}
}

// The worked example should use the reader's own key, not a stand-in.
func TestDescribeArrayHelperError_ExampleUsesTheOffendingKey(t *testing.T) {
	registerCustomHelpers()

	const tpl = `{{array ses.test_emails}}`
	envMap := map[string]interface{}{
		"ses": map[string]interface{}{"test_emails": "ops@example.com"},
	}

	_, err := raymond.Render(tpl, envMap)
	if err == nil {
		t.Fatal("expected the render to fail")
	}

	described := describeArrayHelperError(err, tpl, envMap).Error()
	if !strings.Contains(described, "  test_emails:\n    - <value>") {
		t.Errorf("expected a worked example using test_emails, got: %s", described)
	}
}

// Every helper env/main.hbs can reach must survive a missing key.
//
// This is the guard for the whole class the array helper's nil panic belonged
// to. Templates call these on optional config constantly, and a helper that
// panics on nil takes the entire generate down with a Go stack trace — a config
// problem reported as a crash. A new helper that forgets its nil case fails
// here.
func TestAllHelpers_SurviveAMissingKey(t *testing.T) {
	registerCustomHelpers()

	for _, tpl := range []string{
		`{{array missing}}`,
		`{{envToEnvArray missing}}`,
		`{{#if (exists missing)}}x{{/if}}`,
		`{{#if (or missing missing)}}x{{/if}}`,
		`{{#if (eq missing missing)}}x{{/if}}`,
		`{{#if (isSubdomainOf missing missing)}}x{{/if}}`,
		`{{#compare missing "==" missing}}x{{/compare}}`,
		`{{len missing}}`,
		`{{default missing "fallback"}}`,
		`{{#notEmpty missing}}x{{/notEmpty}}`,
		`{{#notZero missing}}x{{/notZero}}`,
		`{{#isDefined missing}}x{{/isDefined}}`,
		`{{mmap missing}}`,
		`{{stringListMap missing}}`,
		`{{envArray missing}}`,
		`{{#each (sortedPairs missing)}}x{{/each}}`,
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s panicked on a missing key: %v", tpl, r)
				}
			}()
			if _, err := raymond.Render(tpl, map[string]interface{}{}); err != nil {
				t.Errorf("%s errored on a missing key: %v", tpl, err)
			}
		}()
	}
}
