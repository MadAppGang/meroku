package main

import (
	"strings"
	"testing"

	"github.com/aymerick/raymond"
)

// Five sites in env/main.hbs that rendered without error and produced the wrong
// Terraform. They are grouped because they are one defect wearing five hats:
// raymond treats an unresolvable name as the empty string, so a typo, a missing
// helper and a mis-scoped path are all indistinguishable from a field the user
// chose to leave unset. Generation reported success for every one of them.
//
// Each subtest asserts the value that now reaches Terraform, and records what
// used to be emitted instead. TestMainHBSLint stops new instances appearing;
// these stop these five regressing.

// defectFixtureOverlay is a config that reaches all five sites at once. Every
// block involved is optional and gated, so the shipped project/dev.yaml renders
// none of them.
func defectFixtureOverlay(t *testing.T) map[string]interface{} {
	t.Helper()

	workload := fixtureWorkload(t)
	workload["backend_container_command"] = []interface{}{"npm", "start"}

	return map[string]interface{}{
		"workload": workload,
		"event_processor_tasks": []interface{}{
			map[string]interface{}{
				"name":              "reconcile",
				"container_command": []interface{}{"npm", "run", "cron"},
				"detail_types":      []interface{}{"order.created"},
				"sources":           []interface{}{"api"},
				"rule_name":         "reconcile_rule",
			},
		},
		"cloudfront_distributions": []interface{}{
			map[string]interface{}{
				"name":    "cdn",
				"enabled": true,
				// Two aliases: the second is what has to reach the certificate.
				"domain_aliases": []interface{}{"a.example.com", "b.example.com"},
				"logging": map[string]interface{}{
					"enabled":     true,
					"bucket_name": "logs-bucket",
				},
			},
		},
		"extensions": map[string]interface{}{
			"sns_topics": []interface{}{
				map[string]interface{}{
					"name": "orders",
					"webhooks": []interface{}{
						map[string]interface{}{
							"path":          "/hook",
							"filter_policy": map[string]interface{}{"event": []interface{}{"created"}},
						},
					},
				},
			},
		},
	}
}

func TestGenerationDefects(t *testing.T) {
	rendered := renderMainHBS(t, defectFixtureOverlay(t))

	// Whatever else is asserted below, the file has to parse. Every one of these
	// defects produced valid HCL, which is exactly why none of them were caught.
	if err := validateGeneratedHCL("env/dev/main.tf", rendered); err != nil {
		t.Fatalf("the fixture does not generate valid terraform:\n%v", err)
	}

	t.Run("event processor container_command renders as a list", func(t *testing.T) {
		// Was `{{{ container_command }}}`, a raw stache over a []string, which
		// raymond renders by concatenating the elements: `npmruncron`. That is a
		// valid HCL identifier, so the file parsed and Terraform then reported a
		// reference to an undeclared resource.
		//
		// The sibling site on scheduled tasks was fixed when the field became a
		// list; this one was not, and has been broken since it was written.
		assertRenders(t, rendered, `container_command = ["npm","run","cron"]`)
		assertDoesNotRender(t, rendered, "npmruncron")
	})

	t.Run("backend_container_command reads the workload scope", func(t *testing.T) {
		// The guard read workload.backend_container_command and the body read the
		// bare name. There is no {{#with}} anywhere in the template, so the body
		// resolved against the config root, found nothing, and the array helper
		// turned nil into `[]` — a valid empty list(string).
		//
		// Terraform accepted it and the container ran its image's own CMD, so the
		// only symptom was a command that never took effect.
		assertRenders(t, rendered, `backend_container_command = ["npm","start"]`)
		assertDoesNotRender(t, rendered, "backend_container_command = []")
	})

	t.Run("SNS filter policies render their JSON", func(t *testing.T) {
		// {{{json filter_policy}}} called a helper that was never registered.
		// raymond resolves an unknown helper name as a path, finds nothing and
		// renders empty, so this was `jsonencode()` — well-formed HCL that fails
		// at terraform validate on argument count.
		assertRenders(t, rendered, `filter_policy = jsonencode({"event":["created"]})`)
		assertDoesNotRender(t, rendered, "jsonencode()")
	})

	t.Run("a certificate covers every domain alias", func(t *testing.T) {
		// The worst of the five. {{#if (gt (len domain_aliases) 1)}} called an
		// unregistered helper, so the subexpression rendered nothing, the {{#if}}
		// saw a falsy value, and subject_alternative_names was never emitted.
		//
		// The generated Terraform was valid and applied cleanly. The certificate
		// simply covered the first alias only, and every other alias failed TLS
		// at request time.
		assertRenders(t, rendered, "subject_alternative_names")
		assertRenders(t, rendered, `"b.example.com"`)
	})

	t.Run("cloudfront logging falls back to a per-distribution prefix", func(t *testing.T) {
		// Was {{default logging.prefix (concat ...)}}, and concat was never
		// registered either — raymond has no variadic helper support, so it never
		// could have been registered with that call's six arguments. With the
		// fallback rendering as nothing, every distribution wrote its logs to the
		// bucket root instead of under project/env/name/.
		assertRenders(t, rendered, `logging_prefix          = "instagram/dev/cdn/"`)
	})
}

// An explicit prefix must still win over the generated fallback.
func TestCloudFrontLoggingPrefixOverride(t *testing.T) {
	overlay := defectFixtureOverlay(t)
	distributions := overlay["cloudfront_distributions"].([]interface{})
	distribution := distributions[0].(map[string]interface{})
	distribution["logging"].(map[string]interface{})["prefix"] = "custom/prefix/"

	rendered := renderMainHBS(t, overlay)

	assertRenders(t, rendered, `logging_prefix          = "custom/prefix/"`)
	assertDoesNotRender(t, rendered, `logging_prefix          = "instagram/dev/cdn/"`)
}

// gt has to be numeric, not lexicographic. The compare helper it sits beside
// takes its operands as strings, where "10" < "9".
func TestGtIsNumeric(t *testing.T) {
	registerCustomHelpers()

	cases := []struct {
		name string
		a, b interface{}
		want bool
	}{
		{"double digits beat single digits", 10, 9, true},
		{"equal is not greater", 2, 2, false},
		{"smaller is not greater", 1, 2, false},
		{"floats compare numerically", 2.5, 2, true},
		{"numeric strings are coerced", "10", "9", true},
		{"a non-number is not greater than anything", "many", 1, false},
		{"nothing is not greater than anything", nil, 0, false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := renderGt(t, testCase.a, testCase.b)
			if got != testCase.want {
				t.Errorf("gt(%#v, %#v) = %v, want %v", testCase.a, testCase.b, got, testCase.want)
			}
		})
	}
}

// renderGt exercises the helper the way the template calls it — as a
// subexpression feeding {{#if}} — rather than calling the Go function directly,
// so that raymond's own argument coercion is part of what is under test.
func renderGt(t *testing.T, a, b interface{}) bool {
	t.Helper()

	out, err := raymond.Render(`{{#if (gt a b)}}yes{{else}}no{{/if}}`,
		map[string]interface{}{"a": a, "b": b})
	if err != nil {
		t.Fatalf("rendering gt(%#v, %#v): %v", a, b, err)
	}
	return out == "yes"
}

func assertRenders(t *testing.T, rendered, want string) {
	t.Helper()
	if !strings.Contains(rendered, want) {
		t.Errorf("rendered output is missing:\n    %s", want)
	}
}

func assertDoesNotRender(t *testing.T, rendered, unwanted string) {
	t.Helper()
	if strings.Contains(rendered, unwanted) {
		t.Errorf("rendered output still contains the old broken form:\n    %s", unwanted)
	}
}
