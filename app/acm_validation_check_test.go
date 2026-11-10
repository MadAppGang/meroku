package main

import (
	"errors"
	"strings"
	"testing"
)

func pendingValidation() acmValidation {
	return acmValidation{
		Domain: "example.work", Status: "PENDING_VALIDATION",
		RecordName:  "_abc.example.work.",
		RecordValue: "_def.acm-validations.aws.",
	}
}

func fakeDNS(t *testing.T, delegated []string, delegErr error, cname string, found bool, cnameErr error) {
	t.Helper()
	prevD, prevC, prevR := delegationLookup, authoritativeCNAME, recordCheck
	t.Cleanup(func() { delegationLookup, authoritativeCNAME, recordCheck = prevD, prevC, prevR })
	recordCheck = func(string, string, string) map[string]dohVerdict {
		return map[string]dohVerdict{"Google": dohNotYet, "Cloudflare": dohStale}
	}
	delegationLookup = func(string) ([]string, error) { return delegated, delegErr }
	authoritativeCNAME = func(string, []string) (string, bool, error) { return cname, found, cnameErr }
}

func TestJudgeValidation(t *testing.T) {
	cases := []struct {
		name      string
		v         acmValidation
		delegated []string
		delegErr  error
		cname     string
		found     bool
		cnameErr  error
		want      certVerdict
	}{
		{name: "issued", v: acmValidation{Domain: "example.work", Status: "SUCCESS"}, want: certIssued},
		{name: "failed", v: acmValidation{Domain: "example.work", Status: "FAILED", FailureReason: "NO_AVAILABLE_CONTACTS"}, want: certFailed},
		{name: "not delegated", v: pendingValidation(), want: certNotDelegated},
		{name: "intercepted", v: pendingValidation(), delegErr: errPort53Intercepted, want: certUnchecked},
		{name: "record missing", v: pendingValidation(), delegated: testZoneNS, want: certRecordMissing},
		{name: "record wrong", v: pendingValidation(), delegated: testZoneNS, cname: "_other.acm-validations.aws", found: true, want: certRecordWrong},
		{name: "correct, ACM yet to look", v: pendingValidation(), delegated: testZoneNS, cname: "_DEF.acm-validations.aws", found: true, want: certWaitingOnACM},
		{name: "nameservers silent", v: pendingValidation(), delegated: testZoneNS, cnameErr: errors.New("timeout"), want: certNotDelegated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeDNS(t, tc.delegated, tc.delegErr, tc.cname, tc.found, tc.cnameErr)
			got := judgeValidation(tc.v, "example.work")
			if got.Verdict != tc.want {
				t.Errorf("verdict = %v (%s), want %v", got.Verdict, got.Detail, tc.want)
			}
		})
	}
}

func TestJudgeValidationOutsideTheZone(t *testing.T) {
	v := pendingValidation()
	v.RecordName = "_abc.other.com."
	if got := judgeValidation(v, "example.work"); got.Verdict != certOutsideZone {
		t.Errorf("verdict = %v, want outside zone", got.Verdict)
	}
}

func TestSummaryLeadsWithTheWorstVerdict(t *testing.T) {
	checks := []certCheck{
		{Verdict: certIssued, Detail: "ok"},
		{Verdict: certRecordMissing, Detail: "record missing"},
	}
	// checkCertificateValidation sorts; mimic it here.
	if verdictSeverity(certFailed) <= verdictSeverity(certRecordMissing) ||
		verdictSeverity(certRecordMissing) <= verdictSeverity(certWaitingOnACM) {
		t.Fatal("severity order is wrong")
	}
	checks[0], checks[1] = checks[1], checks[0]
	got := summarizeCertChecks(checks)
	if !strings.Contains(got, "1/2 validated") || !strings.Contains(got, "record missing") {
		t.Errorf("summary = %q", got)
	}
	if got := summarizeCertChecks([]certCheck{{Verdict: certIssued}}); got != "ACM: all 1 validated" {
		t.Errorf("summary = %q", got)
	}
}

func TestInZone(t *testing.T) {
	for name, want := range map[string]bool{
		"example.work": true, "*.example.work": true, "_x.example.work.": true,
		"example.works": false, "other.com": false,
	} {
		if got := inZone(name, "example.work"); got != want {
			t.Errorf("inZone(%q) = %v", name, got)
		}
	}
}

// Terraform's progress hook rewrites the status every few seconds; the ACM
// note must survive it, and a newer note must replace the old one.
func TestWithNoteSurvivesProgressUpdates(t *testing.T) {
	s := withNote("In progress (40.0s)", "ACM: 0/1 validated · waiting")
	s = withNote(s, "ACM: all 1 validated")
	if s != "In progress (40.0s) · ACM: all 1 validated" {
		t.Errorf("status = %q", s)
	}
}

// On a network that intercepts port 53 the validation record is read through
// the public resolvers over HTTPS instead.
func TestJudgeValidationThroughResolvers(t *testing.T) {
	fakeDNS(t, nil, errPort53Intercepted, "", false, nil)
	recordCheck = func(name, rrtype, want string) map[string]dohVerdict {
		if name != "_abc.example.work." || rrtype != "CNAME" || want != "_def.acm-validations.aws." {
			t.Errorf("asked for %s %s = %s", name, rrtype, want)
		}
		return map[string]dohVerdict{"Google": dohResolved, "Cloudflare": dohResolved, "AdGuard": dohResolved, "NextDNS": dohStale}
	}
	got := judgeValidation(pendingValidation(), "example.work")
	if got.Verdict != certWaitingOnACM || !strings.Contains(got.Detail, "3 of 4") {
		t.Errorf("verdict = %v (%s)", got.Verdict, got.Detail)
	}

	recordCheck = func(string, string, string) map[string]dohVerdict {
		return map[string]dohVerdict{"Google": dohStale, "Cloudflare": dohStale}
	}
	if got := judgeValidation(pendingValidation(), "example.work"); got.Verdict != certNotDelegated {
		t.Errorf("all resolvers failing: verdict = %v", got.Verdict)
	}
}
