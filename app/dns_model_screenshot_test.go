package main

import (
	"errors"
	"os"
	"testing"
	"time"
)

// Renders the real dnsSetupModel.View() at each state, so what is reviewed is
// the actual screen rather than a mock-up of it.
//
//	MEROKU_TUI_SHOTS=1 go test -run TestRenderDNSModel ./app
func TestRenderDNSModel(t *testing.T) {
	if os.Getenv("MEROKU_TUI_SHOTS") != "1" {
		t.Skip("set MEROKU_TUI_SHOTS=1 to render DNS model screenshots")
	}
	lipglossForceTrueColor()

	base := func() *dnsSetupModel {
		m := newDNSSetupModel(
			Env{Env: "dev", AccountID: "285253872242"},
			dnsPreflightResult{ZoneName: "dev.coretechx.dev", ParentDomain: "coretechx.dev"},
		)
		m.width, m.height = 120, 32
		m.nameservers = []string{
			"ns-1930.awsdns-49.co.uk",
			"ns-1050.awsdns-03.org",
			"ns-678.awsdns-20.net",
			"ns-247.awsdns-30.com",
		}
		m.zoneID = "Z0580793YTBKHE7ID6NJ"
		return m
	}

	candidates := []parentZoneCandidate{
		{Profile: "mag", AccountID: "891880437329", ZoneID: "Z039", Authoritative: true},
		{Profile: "alpha", AccountID: "111122223333", ZoneID: "Z777"},
		{Profile: "timeroo", Err: errors.New("ExpiredToken: the security token included in the request is expired")},
	}

	shots := map[string]func() *dnsSetupModel{
		"model-1-create": func() *dnsSetupModel {
			m := base()
			m.step = stepCreateZone
			m.nameservers = nil
			m.zoneID = ""
			m.elapsed = 14 * time.Second
			return m
		},
		"model-2-find": func() *dnsSetupModel {
			m := base()
			m.step = stepFindParent
			m.states[stepCreateZone] = stepOK
			m.states[stepShowNameservers] = stepOK
			m.elapsed = 42 * time.Second
			return m
		},
		"model-3-choose": func() *dnsSetupModel {
			m := base()
			m.step = stepWriteRecord
			m.states[stepCreateZone] = stepOK
			m.states[stepShowNameservers] = stepOK
			m.states[stepFindParent] = stepOK
			m.candidates = candidates
			m.choosing = true
			m.elapsed = 58 * time.Second
			return m
		},
		"model-4-propagate": func() *dnsSetupModel {
			m := base()
			m.step = stepPropagate
			for _, s := range []dnsStep{stepCreateZone, stepShowNameservers, stepFindParent, stepWriteRecord} {
				m.states[s] = stepOK
			}
			m.resolverResults = map[string]bool{"8.8.8.8": true, "1.1.1.1": true, "9.9.9.9": false}
			m.elapsed = 96 * time.Second
			return m
		},
		"model-5-done": func() *dnsSetupModel {
			m := base()
			m.step = stepDone
			for _, s := range dnsStepOrder {
				m.states[s] = stepOK
			}
			m.Delegated = true
			m.elapsed = 131 * time.Second
			return m
		},
		"model-6-manual": func() *dnsSetupModel {
			m := base()
			m.step = stepFindParent
			m.states[stepCreateZone] = stepOK
			m.states[stepShowNameservers] = stepOK
			m.states[stepFindParent] = stepFailed
			m.states[stepWriteRecord] = stepSkipped
			m.manualReason = "coretechx.dev is not hosted on Route53 (ada.ns.cloudflare.com)"
			m.elapsed = 51 * time.Second
			return m
		},
		"model-7-narrow": func() *dnsSetupModel {
			m := base()
			m.width, m.height = 80, 24
			m.step = stepWriteRecord
			m.states[stepCreateZone] = stepOK
			m.states[stepShowNameservers] = stepOK
			m.states[stepFindParent] = stepOK
			m.candidates = candidates
			m.choosing = true
			m.elapsed = 58 * time.Second
			return m
		},
	}

	for name, build := range shots {
		m := build()
		path := "/tmp/meroku-tui-" + name + ".ansi"
		if err := os.WriteFile(path, []byte(m.View()), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("wrote %s", path)
	}
}
