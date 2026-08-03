package main

import (
	"context"
	"strings"
	"testing"
)

// The message for a missing delegation has to name the trap, because the
// operator's own dig will contradict it.
//
// A resolver holding a cached delegation from a previous DNS provider answers
// with exactly the expected nameservers, so "there is no NS record" reads as
// wrong until you know why. sploty.app moved from Hover to Route53 without its
// dev.sploty.app NS record; resolvers served the stale delegation for hours
// while the parent zone held five records and none of them was that NS.
func TestDescribeDelegationCheck_ExplainsTheCacheTrap(t *testing.T) {
	msg := describeDelegationCheck(delegationCheck{}, "dev.sploty.app", "sploty.app")

	for _, want := range []string{"no NS record", "cache", "expires"} {
		if !strings.Contains(msg, want) {
			t.Errorf("a missing delegation should mention %q:\n%s", want, msg)
		}
	}
}

// A delegation pointing somewhere else is a different problem from none at all,
// and needs a different action — update, not add.
func TestDescribeDelegationCheck_DistinguishesWrongFromMissing(t *testing.T) {
	wrong := describeDelegationCheck(delegationCheck{
		Present:  true,
		Matches:  false,
		Observed: []string{"ns1.hover.com.", "ns2.hover.com."},
	}, "dev.sploty.app", "sploty.app")

	if !strings.Contains(wrong, "ns1.hover.com") {
		t.Errorf("it should show what the parent actually delegates to:\n%s", wrong)
	}
	if !strings.Contains(wrong, "updating, not adding") {
		t.Errorf("a wrong delegation needs a different action from a missing one:\n%s", wrong)
	}
}

// A correct delegation has nothing to report.
func TestDescribeDelegationCheck_SilentWhenCorrect(t *testing.T) {
	if got := describeDelegationCheck(
		delegationCheck{Present: true, Matches: true}, "dev.x.com", "x.com"); got != "" {
		t.Errorf("a correct delegation should produce no message, got %q", got)
	}
}

// The write path must not report success on an unverified change.
//
// Route53 accepting a change batch is not evidence the zone now delegates what
// we asked for, and every other check in this flow asks a resolver — which can
// answer from a cache left by a previous provider, and did.
func TestApplyDelegation_FailsWhenTheZoneDoesNotConfirm(t *testing.T) {
	orig := delegationWriter
	origVerify := delegationVerifier
	defer func() { delegationWriter = orig; delegationVerifier = origVerify }()

	// The write "succeeds" but the zone does not contain the record afterwards.
	delegationWriter = func(profile, zoneID, subdomain string, ns []string) error {
		return nil
	}
	delegationVerifier = func(_ context.Context, _, _, _ string, _ []string) (delegationCheck, error) {
		return delegationCheck{}, nil
	}

	err := applyDelegation(delegationRequest{
		ParentProfile: "no-such-profile-for-tests",
		ParentZoneID:  "ZDOESNOTEXIST",
		Subdomain:     "dev.example.com",
		Nameservers:   []string{"ns-1.example.net"},
	})
	if err == nil {
		t.Fatal("an unverifiable write must not be reported as success")
	}
	if !strings.Contains(err.Error(), "could not be verified") &&
		!strings.Contains(err.Error(), "no NS record") {
		t.Errorf("the error should say the write was not confirmed, got: %v", err)
	}
}
