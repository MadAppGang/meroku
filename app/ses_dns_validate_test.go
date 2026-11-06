package main

import (
	"strings"
	"testing"
)

func sesValidateOne(d map[string]interface{}) []sesDNSIssue {
	return validateSESDomainDNS(map[string]interface{}{"enabled": true}, d)
}

func sesHasCode(issues []sesDNSIssue, field, code string) bool {
	for _, i := range issues {
		if i.Field == field && i.Code == code {
			return true
		}
	}
	return false
}

func TestValidateSESDomainDNS_AcceptsValidConfig(t *testing.T) {
	issues := sesValidateOne(map[string]interface{}{
		"domain":          "example.com",
		"spf_record":      "v=spf1 include:_spf.google.com include:amazonses.com ~all",
		"spf_ttl":         0,
		"apex_txt_values": []interface{}{"google-site-verification=fake-token-do-not-use", `part1" "part2`},
		"txt_records": []interface{}{
			map[string]interface{}{"name": "google._domainkey", "ttl": 3600, "values": []interface{}{"v=DKIM1; k=rsa; p=" + strings.Repeat("fake", 100)}},
			map[string]interface{}{"name": "_acme-challenge.app", "values": []interface{}{"x"}},
			// MAIL FROM is on with the default subdomain; "mail" is free.
			map[string]interface{}{"name": "mail", "values": []interface{}{"x"}},
		},
	})
	if len(issues) != 0 {
		t.Errorf("valid config refused: %+v", issues)
	}
}

func TestValidateSESDomainDNS_EmptySPFRecordMeansDefault(t *testing.T) {
	if issues := sesValidateOne(map[string]interface{}{"domain": "example.com", "spf_record": ""}); len(issues) != 0 {
		t.Errorf("an empty spf_record is not rendered, so it must not be refused: %+v", issues)
	}
}

func TestValidateSESDomainDNS_Refusals(t *testing.T) {
	rec := func(name string, values ...interface{}) map[string]interface{} {
		return map[string]interface{}{"domain": "example.com", "txt_records": []interface{}{
			map[string]interface{}{"name": name, "values": values},
		}}
	}
	cases := []struct {
		name  string
		d     map[string]interface{}
		field string
		code  string
	}{
		{"spf not spf", map[string]interface{}{"domain": "example.com", "spf_record": "google-site-verification=x"}, "spf_record", "not_spf"},
		{"spf upper case", map[string]interface{}{"domain": "example.com", "spf_record": "V=SPF1 ~all"}, "spf_record", "not_spf"},
		{"spf quote", map[string]interface{}{"domain": "example.com", "spf_record": `v=spf1 "a" ~all`}, "spf_record", "stray_quote"},
		{"fractional ttl", map[string]interface{}{"domain": "example.com", "spf_ttl": 300.5}, "spf_ttl", "bad_ttl"},
		{"negative ttl", map[string]interface{}{"domain": "example.com", "spf_ttl": -1}, "spf_ttl", "bad_ttl"},
		{"spf in apex list", map[string]interface{}{"domain": "example.com", "apex_txt_values": []interface{}{"v=spf1 -all"}}, "apex_txt_values[0]", "spf_in_list"},
		{"empty apex value", map[string]interface{}{"domain": "example.com", "apex_txt_values": []interface{}{`" "`}}, "apex_txt_values[0]", "empty_value"},
		{"non printable", map[string]interface{}{"domain": "example.com", "apex_txt_values": []interface{}{"café"}}, "apex_txt_values[0]", "non_printable"},
		{"split inside escape", map[string]interface{}{"domain": "example.com", "apex_txt_values": []interface{}{strings.Repeat("a", 253) + `\344` + "b"}}, "apex_txt_values[0]", "split_inside_escape"},
		{"empty name", rec("", "x"), "txt_records[0].name", "apex_name"},
		{"at name", rec("@", "x"), "txt_records[0].name", "apex_name"},
		{"absolute name", rec("google._domainkey.example.com", "x"), "txt_records[0].name", "absolute_name"},
		{"upper-case name", rec("Google._domainkey", "x"), "txt_records[0].name", "bad_name"},
		{"trailing dot", rec("google._domainkey.", "x"), "txt_records[0].name", "bad_name"},
		{"wildcard", rec("*.app", "x"), "txt_records[0].name", "bad_name"},
		{"amazonses", rec("_amazonses", "x"), "txt_records[0].name", "reserved_name"},
		{"dmarc", rec("_dmarc", "v=DMARC1; p=none"), "txt_records[0].name", "reserved_name"},
		{"mail from", rec("bounce", "x"), "txt_records[0].name", "reserved_name"},
		{"no values", rec("a"), "txt_records[0].values", "no_values"},
		{"spf value", rec("a", "v=spf1 -all"), "txt_records[0].values[0]", "spf_in_list"},
		{"quote in value", rec("a", `say "hi"`), "txt_records[0].values[0]", "stray_quote"},
		{"duplicate name", map[string]interface{}{"domain": "example.com", "txt_records": []interface{}{
			map[string]interface{}{"name": "a", "values": []interface{}{"x"}},
			map[string]interface{}{"name": "a", "values": []interface{}{"y"}},
		}}, "txt_records[1].name", "duplicate_name"},
		{"bad record ttl", map[string]interface{}{"domain": "example.com", "txt_records": []interface{}{
			map[string]interface{}{"name": "a", "ttl": 1.5, "values": []interface{}{"x"}},
		}}, "txt_records[0].ttl", "bad_ttl"},
		{"too long", rec(strings.Repeat("a.", 124)+"a", "x"), "txt_records[0].name", "name_too_long"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := sesValidateOne(tc.d)
			if !sesHasCode(issues, tc.field, tc.code) {
				t.Fatalf("want %s on %s, got %+v", tc.code, tc.field, issues)
			}
			for _, i := range issues {
				if strings.TrimSpace(i.Hint) == "" || strings.TrimSpace(i.Message) == "" {
					t.Errorf("issue %s has no message or hint", i.Code)
				}
			}
		})
	}
}

func TestValidateSESDomainDNS_MailFromSubdomainFreeWhenOff(t *testing.T) {
	issues := validateSESDomainDNS(map[string]interface{}{"enable_mail_from": false}, map[string]interface{}{
		"domain":      "example.com",
		"txt_records": []interface{}{map[string]interface{}{"name": "bounce", "values": []interface{}{"x"}}},
	})
	if len(issues) != 0 {
		t.Errorf("with MAIL FROM off its subdomain is free: %+v", issues)
	}
}

func TestPreprocessRefusesInvalidSESRecords(t *testing.T) {
	envMap := map[string]interface{}{
		"env": "dev",
		"ses": map[string]interface{}{
			"enabled": true,
			"domains": []interface{}{map[string]interface{}{
				"domain":      "example.com",
				"txt_records": []interface{}{map[string]interface{}{"name": "_dmarc", "values": []interface{}{"x"}}},
			}},
		},
	}
	err := preprocessEnvMap(envMap)
	if err == nil {
		t.Fatal("generate must refuse a txt_records name meroku writes itself")
	}
	if !strings.Contains(err.Error(), "ses.domains example.com: txt_records[0].name") || !strings.Contains(err.Error(), "Pick another name") {
		t.Errorf("error must name the field and say what to do: %v", err)
	}
}

func TestPreprocessIgnoresDisabledSES(t *testing.T) {
	envMap := map[string]interface{}{
		"env": "dev",
		"ses": map[string]interface{}{
			"enabled": false,
			"domains": []interface{}{map[string]interface{}{
				"domain":      "example.com",
				"txt_records": []interface{}{map[string]interface{}{"name": "_dmarc", "values": []interface{}{"x"}}},
			}},
		},
	}
	if err := preprocessEnvMap(envMap); err != nil {
		t.Errorf("SES off renders nothing, so nothing is refused: %v", err)
	}
}
