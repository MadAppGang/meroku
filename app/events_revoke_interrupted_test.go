package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The revocation debt of the enhanced bus survives an apply that stops half
// way, under Terraform's real graph, with the fakes of
// events_subscriber_chain_order_test.go. An interruption is a targeted apply
// that changes only the RAM principal associations, optionally followed by
// removing the old sweep from state: what Terraform leaves when it is killed
// after it destroyed the old sweep and the association, before it created the
// new sweep.
func TestEventBusRevokeSurvivesInterruptedApplies(t *testing.T) {
	if testing.Short() {
		t.Skip("runs terraform applies with one-second operations")
	}
	terraform, err := exec.LookPath("terraform")
	if err != nil {
		t.Skip("terraform is not on PATH")
	}
	fakes, err := filepath.Abs(chainFakeDir)
	if err != nil {
		t.Fatal(err)
	}
	h := &chainHarness{t: t, terraform: terraform, fakes: fakes, work: t.TempDir(), root: newEventsRenderRoot(t)}
	h.state = emptyEventsState()
	stubEventsState(t, func() eventsAwsccState { return h.state })

	services := `
  - name: orders
    container_port: 3000
    events:
      subscribes:
        - { name: s, detail_type: s.x }
`
	shared := chainStepYAML(chainEnhancedShared, services)
	publishOnly := chainStepYAML(chainEnhancedPublishOnly, services)
	const principals = "-target=module.event_bus_enhanced.terraform_data.principal"
	const revokeLost = "start revoke granted= lost=account:000000000001"

	// interrupted renders yamlBody and applies only the RAM associations.
	interrupted := func(yamlBody string) {
		t.Helper()
		mainTf, _ := h.root.mustRender(t, yamlBody)
		h.apply(h.fakeMain(mainTf), principals)
	}
	noRevoke := func(step string, log chainLog) {
		t.Helper()
		if i := log.index("start revoke"); i >= 0 {
			t.Errorf("%s: a settled bus ran the sweep again: %s", step, log[i])
		}
	}

	log, _ := h.step(shared)
	log.mustHave(t, "share", "start revoke granted=account:000000000001 lost=")

	// 1. Removing the subscribe principal stops after the association and the
	// old sweep are gone. The record still names the principal.
	interrupted(publishOnly)
	h.run("state", "rm", "module.event_bus_enhanced.terraform_data.revoke[0]")
	h.state = h.readState()
	if h.state.Revoke != nil || containsString(h.state.RamPrincipals, "subscribe/account/000000000001") || h.state.RevokeRecord == nil {
		t.Fatalf("interruption 1 did not leave only the record: %+v", h.state)
	}
	log, mainTf := h.step(publishOnly)
	assertContains(t, "main.tf after interruption 1", mainTf, `"lost":["account:000000000001"]`, "# meroku:events lost=account:000000000001\n")
	log.mustHave(t, "after interruption 1", revokeLost)
	log, _ = h.step(publishOnly)
	noRevoke("settled after interruption 1", log)

	// 2. The principal is shared again; the apply stops after it is
	// associated, before its sweep. The YAML is then reverted: granted and
	// lost read as the last round's, but the principal held a grant again.
	interrupted(shared)
	if !containsString(h.state.RamPrincipals, "subscribe/account/000000000001") {
		t.Fatalf("interruption 2 did not associate the principal: %+v", h.state)
	}
	log, _ = h.step(publishOnly)
	log.mustHave(t, "rollback after interruption 2", revokeLost)
	log.mustBefore(t, "rollback after interruption 2", "end principal-delete subscribe/account/000000000001", "start revoke")
	log, _ = h.step(publishOnly)
	noRevoke("settled after interruption 2", log)

	if t.Failed() {
		t.Logf("state: %s", strings.Join(h.state.RamPrincipals, ","))
	}
}

// Two interrupted applies that leave no record of a principal: the grant
// associates account 1 in RAM and stops before its sweep and record, account
// 1 creates a subscriber, and the rollback disassociates it and stops before
// its sweep. RAM, the sweep and the record then all read as the last settled
// round, and none names account 1. The subscriber listing still shows its
// subscriber, so the next apply sweeps it, and once it is revoked the bus is
// settled again with an empty plan.
func TestEventBusRevokeFindsSubscribersNoRecordNames(t *testing.T) {
	if testing.Short() {
		t.Skip("runs terraform applies with one-second operations")
	}
	terraform, err := exec.LookPath("terraform")
	if err != nil {
		t.Skip("terraform is not on PATH")
	}
	fakes, err := filepath.Abs(chainFakeDir)
	if err != nil {
		t.Fatal(err)
	}
	h := &chainHarness{t: t, terraform: terraform, fakes: fakes, work: t.TempDir(), root: newEventsRenderRoot(t)}
	h.state = emptyEventsState()
	stubEventsState(t, func() eventsAwsccState { return h.state })
	var subscribers []eventBusSubscriber
	stubEventBusSubscribers(t, func() ([]eventBusSubscriber, error) { return subscribers, nil })

	services := `
  - name: orders
    container_port: 3000
    events:
      subscribes:
        - { name: s, detail_type: s.x }
`
	shared := chainStepYAML(chainEnhancedShared, services)
	publishOnly := chainStepYAML(chainEnhancedPublishOnly, services)
	const principals = "-target=module.event_bus_enhanced.terraform_data.principal"
	const subscribe1 = "subscribe/account/000000000001"

	// The fake sweep only logs. The real script revokes every subscriber of
	// an account no keep entry names, which here is every other account's.
	sweep := func(log chainLog) {
		if log.index("start revoke") < 0 {
			return
		}
		for i := range subscribers {
			subscribers[i].Revoked = true
		}
	}
	settledRound := func(step string) {
		t.Helper()
		s := h.state
		if s.Revoke == nil || s.RevokeRecord == nil || s.Revoke.Round != s.RevokeRecord.Round ||
			len(s.Revoke.Granted)+len(s.RevokeRecord.Granted) != 0 || containsString(s.RamPrincipals, subscribe1) {
			t.Fatalf("%s: want the sweep and record of one round, neither naming account 1, and no RAM association: %+v %+v %v",
				step, s.Revoke, s.RevokeRecord, s.RamPrincipals)
		}
	}

	log, _ := h.step(publishOnly)
	log.mustHave(t, "first deploy", "start revoke granted= lost=")
	log, _ = h.step(publishOnly)
	if log.index("start revoke") >= 0 {
		t.Fatalf("a settled bus ran the sweep again:\n%s", strings.Join(log, "\n"))
	}
	settledRound("settled")
	round := h.state.RevokeRecord.Round

	// 1. The grant stops after the RAM association, before the sweep.
	mainTf, _ := h.root.mustRender(t, shared)
	h.apply(h.fakeMain(mainTf), principals)
	if !containsString(h.state.RamPrincipals, subscribe1) {
		t.Fatalf("interruption 1 did not associate account 1: %v", h.state.RamPrincipals)
	}
	subscribers = append(subscribers, eventBusSubscriber{
		ARN: "arn:aws:events:ap-southeast-2:000000000001:subscriber/feed/0001", Account: "000000000001",
	})

	// 2. The rollback stops after the RAM disassociation, before the sweep.
	mainTf, _ = h.root.mustRender(t, publishOnly)
	h.apply(h.fakeMain(mainTf), principals)
	settledRound("after interruption 2")
	if h.state.RevokeRecord.Round != round {
		t.Fatalf("interruption 2 moved the record from round %d: %+v", round, h.state.RevokeRecord)
	}

	// 3. The next deploy sweeps the subscriber no record names.
	log, mainTf = h.step(publishOnly)
	assertContains(t, "main.tf after interruption 2", mainTf,
		`"lost":["account:000000000001"]`, "# meroku:events lost=account:000000000001\n")
	log.mustHave(t, "after interruption 2", "start revoke granted= lost=account:000000000001")
	sweep(log)

	// 4. Settled: no sweep, and an empty plan.
	log, mainTf = h.step(publishOnly)
	if i := log.index("start revoke"); i >= 0 {
		t.Errorf("a settled bus ran the sweep again: %s", log[i])
	}
	h.planEmpty(h.fakeMain(mainTf))
}

// planEmpty asserts that terraform plans no change over main.tf.
func (h *chainHarness) planEmpty(mainTf string) {
	h.t.Helper()
	if err := os.WriteFile(filepath.Join(h.work, "main.tf"), []byte(mainTf), 0o644); err != nil {
		h.t.Fatal(err)
	}
	cmd := exec.Command(h.terraform, "plan", "-detailed-exitcode", "-input=false", "-no-color")
	cmd.Dir = h.work
	cmd.Env = append(os.Environ(), "TF_IN_AUTOMATION=1", "CHECKPOINT_DISABLE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		h.t.Errorf("the plan after settling is not empty (%v):\n%s", err, out)
	}
}
