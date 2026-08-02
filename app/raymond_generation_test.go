package main

import (
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
