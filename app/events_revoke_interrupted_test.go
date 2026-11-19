package main

import (
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
