package main

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Visual QA for the DNS setup screen. Writes colour ANSI frames for conversion
// to PNG. Gated so ordinary `go test` stays quiet.
//
//	MEROKU_TUI_SHOTS=1 go test -run TestRenderDNSScreens ./app
func TestRenderDNSScreens(t *testing.T) {
	if os.Getenv("MEROKU_TUI_SHOTS") != "1" {
		t.Skip("set MEROKU_TUI_SHOTS=1 to render DNS screenshots")
	}
	lipglossForceTrueColor()

	const w = 120
	zone, parent := "dev.coretechx.dev", "coretechx.dev"
	ns := []string{
		"ns-1930.awsdns-49.co.uk",
		"ns-1050.awsdns-03.org",
		"ns-678.awsdns-20.net",
		"ns-247.awsdns-30.com",
	}

	candidates := []parentZoneCandidate{
		{Profile: "mag", AccountID: "891880437329", ZoneID: "Z039", Authoritative: true,
			Nameservers: []string{"ns-523.awsdns-01.net"}},
		{Profile: "alpha", AccountID: "111122223333", ZoneID: "Z777", Authoritative: false},
		{Profile: "timeroo", Err: errors.New("ExpiredToken: the security token included in the request is expired")},
	}

	frames := map[string]func() string{
		// 1. Creating the zone.
		"dns-1-create": func() string {
			states := map[dnsStep]stepState{}
			var b strings.Builder
			b.WriteString(renderDNSHeader(zone, 12*time.Second, w) + "\n\n")
			b.WriteString(renderStepRail(stepCreateZone, states, w) + "\n\n")
			b.WriteString(boxStyle.Width(w-4).Render(
				titleStyle.Render("Creating hosted zone")+"\n"+
					lipgloss.NewStyle().Foreground(dimColor).Render(
						"the zone must exist before its nameservers can be delegated")+"\n\n"+
					meterRow(w-8, 0.55, "#3b82f6", "#10b981", "12s")) + "\n\n")
			b.WriteString(renderDNSFooter([]string{"[Ctrl+C] cancel"}, w))
			return b.String()
		},

		// 2. Zone created; delegation record shown; scanning profiles.
		"dns-2-nameservers": func() string {
			states := map[dnsStep]stepState{stepCreateZone: stepOK, stepShowNameservers: stepOK}
			var b strings.Builder
			b.WriteString(renderDNSHeader(zone, 48*time.Second, w) + "\n\n")
			b.WriteString(renderStepRail(stepFindParent, states, w) + "\n\n")
			b.WriteString(renderNameserverPanel(zone, parent, ns, w-4) + "\n\n")
			b.WriteString(boxStyle.Width(w-4).Render(
				titleStyle.Render("Looking for the "+parent+" zone")+"\n"+
					lipgloss.NewStyle().Foreground(dimColor).Render(
						"scanning 12 local AWS profiles — a match is proved against public DNS")+"\n\n"+
					meterRow(w-8, 0.75, "#3b82f6", "#10b981", "9/12 profiles")) + "\n\n")
			b.WriteString(renderDNSFooter([]string{"[c] copy nameservers", "[Ctrl+C] cancel"}, w))
			return b.String()
		},

		// 3. Choose which profile owns the parent zone.
		"dns-3-choose": func() string {
			states := map[dnsStep]stepState{
				stepCreateZone: stepOK, stepShowNameservers: stepOK, stepFindParent: stepOK,
			}
			var b strings.Builder
			b.WriteString(renderDNSHeader(zone, 61*time.Second, w) + "\n\n")
			b.WriteString(renderStepRail(stepWriteRecord, states, w) + "\n\n")

			var rows strings.Builder
			rows.WriteString(titleStyle.Render("Which AWS profile manages "+parent+"?") + "\n")
			rows.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render(
				"only a profile whose nameservers match public DNS can be delegated to") + "\n\n")
			for i, c := range candidates {
				rows.WriteString(profileCandidateLine(c, i == 0, w-8) + "\n")
			}
			rows.WriteString("\n" + lipgloss.NewStyle().Foreground(mutedColor).
				Render("  I'll add the record myself   ·   Cancel"))
			b.WriteString(boxStyle.Width(w-4).Render(rows.String()) + "\n\n")
			b.WriteString(renderDNSFooter([]string{"[↑↓] select", "[Enter] delegate", "[m] do it manually", "[Ctrl+C] cancel"}, w))
			return b.String()
		},

		// 4. Record written; waiting for propagation.
		"dns-4-propagate": func() string {
			states := map[dnsStep]stepState{
				stepCreateZone: stepOK, stepShowNameservers: stepOK,
				stepFindParent: stepOK, stepWriteRecord: stepOK,
			}
			resolvers := []string{"8.8.8.8", "1.1.1.1", "9.9.9.9", "208.67.222.222"}
			results := map[string]dohVerdict{"8.8.8.8": dohResolved, "1.1.1.1": dohResolved, "9.9.9.9": dohNotYet}

			var b strings.Builder
			b.WriteString(renderDNSHeader(zone, 95*time.Second, w) + "\n\n")
			b.WriteString(renderStepRail(stepPropagate, states, w) + "\n\n")
			b.WriteString(boxStyle.Width(w-4).Render(
				titleStyle.Render("Waiting for delegation to appear")+"\n"+
					lipgloss.NewStyle().Foreground(dimColor).Render(
						"NS record written to zone Z039 in account 891880437329")+"\n\n"+
					renderResolverList(results, resolvers, false, 0, w-10)+"\n\n"+
					meterRow(w-8, 0.5, "#f59e0b", "#10b981", "2/4 resolvers")) + "\n\n")
			b.WriteString(renderDNSFooter([]string{"[s] stop waiting (record is saved)", "[Ctrl+C] cancel"}, w))
			return b.String()
		},

		// 5. Verified — continuing to the full deploy.
		"dns-5-done": func() string {
			states := map[dnsStep]stepState{
				stepCreateZone: stepOK, stepShowNameservers: stepOK, stepFindParent: stepOK,
				stepWriteRecord: stepOK, stepPropagate: stepOK,
			}
			var b strings.Builder
			b.WriteString(renderDNSHeader(zone, 132*time.Second, w) + "\n\n")
			b.WriteString(renderStepRail(stepDone, states, w) + "\n\n")
			b.WriteString(boxStyle.Width(w-4).Render(
				badge("DELEGATED", successColor)+"  "+
					lipgloss.NewStyle().Foreground(fgColor).Render(zone+" resolves to this account")+"\n\n"+
					lipgloss.NewStyle().Foreground(dimColor).Render(
						"Certificate validation can now succeed. Continuing with the full deploy.")) + "\n\n")
			b.WriteString(renderDNSFooter([]string{"[Enter] continue to phase 2"}, w))
			return b.String()
		},

		// 6. Fallback: parent is not on Route53 / not ours.
		"dns-6-manual": func() string {
			states := map[dnsStep]stepState{
				stepCreateZone: stepOK, stepShowNameservers: stepOK,
				stepFindParent: stepFailed, stepWriteRecord: stepSkipped,
			}
			var b strings.Builder
			b.WriteString(renderDNSHeader(zone, 55*time.Second, w) + "\n\n")
			b.WriteString(renderStepRail(stepFindParent, states, w) + "\n\n")
			b.WriteString(boxStyle.Width(w-4).Render(
				badge("MANUAL", warningColor)+"  "+
					lipgloss.NewStyle().Foreground(fgColor).Render("meroku cannot write this record for you")+"\n\n"+
					lipgloss.NewStyle().Foreground(dimColor).Render(
						"Why: coretechx.dev is not hosted on Route53 (nameservers: ada.ns.cloudflare.com)")) + "\n\n")
			b.WriteString(renderNameserverPanel(zone, parent, ns, w-4) + "\n\n")
			b.WriteString(renderDNSFooter([]string{"[c] copy", "[r] re-check", "[Ctrl+C] cancel"}, w))
			return b.String()
		},
	}

	for name, render := range frames {
		path := "/tmp/meroku-tui-" + name + ".ansi"
		if err := os.WriteFile(path, []byte(render()), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("wrote %s", path)
	}
}
