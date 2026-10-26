package main

import (
	"regexp"
	"strings"
	"testing"
)

// A project deployed before Schema v17 holds its SES resources at unkeyed
// addresses. env/main.hbs moves them to the for_each key the module now uses,
// and that key must be the exact domain the module receives — a move to any
// other key is a destroy of the live identity and a create of a new one.
//
// Each case renders the real template through the real pre-processing and
// checks three things: the module's domain, that every moved block targets that
// same domain, and that every address a pre-v17 state can hold has a move.

// sesLegacyAddresses is every unkeyed address the ses module has ever had
// (git log -- modules/ses/main.tf, before c586f0a), with the key it moves to.
var sesLegacyAddresses = map[string]string{
	"aws_ses_domain_identity.domain":                          "%s",
	"aws_ses_domain_dkim.dkim":                                "%s",
	"aws_route53_record.domain_amazonses_verification_record": "%s",
	"aws_route53_record.domain_amazonses_dkim_record[0]":      "%s-dkim-0",
	"aws_route53_record.domain_amazonses_dkim_record[1]":      "%s-dkim-1",
	"aws_route53_record.domain_amazonses_dkim_record[2]":      "%s-dkim-2",
	"aws_route53_record.spf":                                  "%s",
	"aws_ses_domain_mail_from.mail_from[0]":                   "%s",
	"aws_route53_record.mail_from_mx[0]":                      "%s",
	"aws_route53_record.mail_from_spf[0]":                     "%s",
	"aws_route53_record.dmarc":                                "%s",
}

var sesMovedBlock = regexp.MustCompile(`moved \{\n  from = module\.ses\.(\S+)\n  to   = module\.ses\.\S+\["([^"]+)"\]\n\}`)

func TestSESLegacyMovesFollowTheModuleDomain(t *testing.T) {
	cases := []struct {
		name    string
		overlay map[string]interface{}
		want    string
	}{
		{
			name: "domains list uses its first entry",
			overlay: map[string]interface{}{"ses": map[string]interface{}{
				"enabled": true,
				"domains": []interface{}{
					map[string]interface{}{"domain": "passflow.dev"},
					map[string]interface{}{"domain": "other.example.com"},
				},
			}},
			want: "passflow.dev",
		},
		{
			name: "legacy domain_name",
			overlay: map[string]interface{}{"ses": map[string]interface{}{
				"enabled": true, "domain_name": "mail.example.com",
			}},
			want: "mail.example.com",
		},
		{
			// migrateToV17 skips an empty domain_name, so this shape survives
			// to the current schema and still takes the legacy branch.
			name: "derived name outside prod",
			overlay: map[string]interface{}{
				"env":    "dev",
				"domain": map[string]interface{}{"enabled": false, "domain_name": "example.com"},
				"ses":    map[string]interface{}{"enabled": true, "domain_name": nil},
			},
			want: "mail.dev.example.com",
		},
		{
			name: "derived name in prod",
			overlay: map[string]interface{}{
				"env":    "prod",
				"domain": map[string]interface{}{"enabled": false, "domain_name": "example.com"},
				"ses":    map[string]interface{}{"enabled": true},
			},
			want: "mail.example.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rendered := renderMainHBS(t, tc.overlay)

			if _, hasList := tc.overlay["ses"].(map[string]interface{})["domains"]; !hasList {
				wantLine := `domain = "` + tc.want + `"`
				if !strings.Contains(rendered, wantLine) {
					t.Errorf("module \"ses\" did not receive %s", wantLine)
				}
			}

			moved := map[string]string{}
			for _, m := range sesMovedBlock.FindAllStringSubmatch(rendered, -1) {
				moved[m[1]] = m[2]
			}
			for from, keyFmt := range sesLegacyAddresses {
				wantKey := strings.ReplaceAll(keyFmt, "%s", tc.want)
				got, ok := moved[from]
				if !ok {
					t.Errorf("no moved block for module.ses.%s", from)
					continue
				}
				if got != wantKey {
					t.Errorf("module.ses.%s moves to key %q, want %q", from, got, wantKey)
				}
			}
			if len(moved) != len(sesLegacyAddresses) {
				t.Errorf("rendered %d ses moved blocks, want %d", len(moved), len(sesLegacyAddresses))
			}
		})
	}
}

func TestSESLegacyMovesAbsentWhenSESDisabled(t *testing.T) {
	rendered := renderMainHBS(t, map[string]interface{}{
		"ses": map[string]interface{}{"enabled": false, "domain_name": "mail.example.com"},
	})
	if strings.Contains(rendered, "module.ses.") {
		t.Error("moved blocks rendered for a disabled ses module; they would point at a module that does not exist")
	}
}
