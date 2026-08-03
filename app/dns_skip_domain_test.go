package main

import (
	"strings"
	"testing"
)

// Flipping domain.enabled must not touch the six other `enabled:` keys in a
// meroku env file. A naive string replace disables whichever block comes first
// — in the generated layout that is domain, so the bug would hide until someone
// reordered the file.
func TestSetDomainEnabledFalse_OnlyTouchesDomainBlock(t *testing.T) {
	doc := `schema_version: 20
project: coretechx
domain:
  enabled: true
  create_domain_zone: true
  domain_name: coretechx.dev
postgres:
  enabled: true
  dbname: app
cognito:
  enabled: true
ses:
  enabled: true
`
	got, ok := setDomainEnabledFalse(doc)
	if !ok {
		t.Fatal("expected domain.enabled to be found")
	}

	if !strings.Contains(got, "domain:\n  enabled: false\n") {
		t.Errorf("domain.enabled was not disabled:\n%s", got)
	}
	if strings.Count(got, "enabled: false") != 1 {
		t.Errorf("exactly one key should have changed, got:\n%s", got)
	}
	for _, block := range []string{"postgres", "cognito", "ses"} {
		idx := strings.Index(got, block+":\n  enabled: true")
		if idx < 0 {
			t.Errorf("%s.enabled should still be true:\n%s", block, got)
		}
	}
}

// Everything outside the one line must survive byte for byte — this file is the
// only thing in a project that cannot be regenerated.
func TestSetDomainEnabledFalse_PreservesEverythingElse(t *testing.T) {
	doc := `project: coretechx

# a comment meroku's struct does not model
custom_field: kept
domain:
  enabled: true
  domain_name: coretechx.dev
trailing: value
`
	got, _ := setDomainEnabledFalse(doc)

	for _, must := range []string{
		"# a comment meroku's struct does not model",
		"custom_field: kept",
		"domain_name: coretechx.dev",
		"trailing: value",
	} {
		if !strings.Contains(got, must) {
			t.Errorf("lost %q from the document:\n%s", must, got)
		}
	}
	if strings.Count(got, "\n") != strings.Count(doc, "\n") {
		t.Error("line count changed; the edit should be one line in place")
	}
}

// Indentation styles vary; the replacement must keep whatever was there.
func TestSetDomainEnabledFalse_PreservesIndentation(t *testing.T) {
	doc := "domain:\n    enabled: true\n"
	got, ok := setDomainEnabledFalse(doc)
	if !ok {
		t.Fatal("expected a match")
	}
	if !strings.Contains(got, "    enabled: false") {
		t.Errorf("indentation not preserved: %q", got)
	}
}

func TestSetDomainEnabledFalse_AlreadyDisabledIsNotAnError(t *testing.T) {
	doc := "domain:\n  enabled: false\n"
	got, ok := setDomainEnabledFalse(doc)
	if !ok {
		t.Error("already-disabled should report success, not failure")
	}
	if got != doc {
		t.Error("nothing should have changed")
	}
}

// A file with no domain block must report failure rather than silently doing
// nothing, so the caller can tell the operator to edit it by hand.
func TestSetDomainEnabledFalse_ReportsMissingBlock(t *testing.T) {
	if _, ok := setDomainEnabledFalse("project: x\npostgres:\n  enabled: true\n"); ok {
		t.Error("expected failure when there is no domain block")
	}
}

// A nested `enabled` deeper inside the domain block must not be mistaken for
// domain.enabled itself.
func TestSetDomainEnabledFalse_TakesTheFirstEnabledInBlock(t *testing.T) {
	doc := "domain:\n  enabled: true\n  sub:\n    enabled: true\n"
	got, _ := setDomainEnabledFalse(doc)
	if !strings.HasPrefix(got, "domain:\n  enabled: false\n") {
		t.Errorf("wrong key changed:\n%s", got)
	}
	if !strings.Contains(got, "    enabled: true") {
		t.Errorf("the nested key should be untouched:\n%s", got)
	}
}
