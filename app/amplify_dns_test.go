package main

import (
	"strings"
	"testing"
)

// amplifyModuleBlock returns the body of `module "amplify" { ... }`.
//
// The block contains nested `{ }` in amplify_apps, so it is delimited by a
// closing brace in column zero rather than the first one found.
func amplifyModuleBlock(t *testing.T, rendered string) string {
	t.Helper()

	start := strings.Index(rendered, `module "amplify" {`)
	if start < 0 {
		t.Fatal(`module "amplify" not found in the rendered template`)
	}
	rest := rendered[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatal(`module "amplify" block is unterminated`)
	}
	return rest[:end]
}

// manage_dns_records is a three-state field, and the three states must stay
// distinguishable all the way to HCL.
//
// Amplify writes its own Route53 records when the zone is in the same account,
// so the records modules/amplify can create are for the cross-account case
// only. The module defaults to false. An absent key therefore has to render as
// nothing at all rather than as `false` — they mean the same today, and would
// stop meaning the same the moment that default changes.
//
// This is why the template uses `{{#if (exists ...)}}` and the model field is a
// *bool: a plain {{#if}} cannot tell explicit-false from missing, and a
// non-pointer bool would have flattened the distinction before the template
// ever ran.
func TestAmplifyManageDNSRecordsRendersAllThreeStates(t *testing.T) {
	tests := []struct {
		name    string
		overlay map[string]interface{}
		want    string // "" means the argument must be absent
	}{
		{
			name:    "true renders true",
			overlay: map[string]interface{}{"manage_dns_records": true},
			want:    "manage_dns_records = true",
		},
		{
			name:    "explicit false still renders",
			overlay: map[string]interface{}{"manage_dns_records": false},
			want:    "manage_dns_records = false",
		},
		{
			name:    "absent renders nothing",
			overlay: map[string]interface{}{},
			want:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			block := amplifyModuleBlock(t, renderMainHBS(t, tc.overlay))

			if tc.want == "" {
				if strings.Contains(block, "manage_dns_records") {
					t.Errorf("manage_dns_records should be absent when unset, got:\n%s", block)
				}
				return
			}
			if !strings.Contains(block, tc.want) {
				t.Errorf("want %q in the amplify block, got:\n%s", tc.want, block)
			}
		})
	}
}
