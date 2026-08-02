package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Shared visual language for the DNS setup screen.
//
// Design rules, applied consistently:
//   - colour is semantic, never decorative: green = verified/ok, amber = waiting
//     or needs a human, red = failed, blue = in progress, gray = not started.
//   - state is a badge (dark ink on a saturated background), not coloured prose.
//   - progress is a gradient meter, not a number.
//   - chrome recedes (gray borders and labels), data and the active step are bright.

// badge renders a status chip: dark text on a saturated background.
// Lip Gloss v1 uses its own TerminalColor interface; image/color is the v2 model.
func badge(text string, bg lipgloss.TerminalColor) string {
	return lipgloss.NewStyle().
		Background(bg).
		Foreground(lipgloss.Color("#0a0a0a")).
		Bold(true).
		Padding(0, 1).
		Render(text)
}

// badgeFixed is badge() padded to a fixed width, so a column of differently
// worded statuses still lines its following text up. Ragged badges make a list
// look broken even when every row is correct.
func badgeFixed(text string, bg lipgloss.TerminalColor, width int) string {
	return lipgloss.NewStyle().
		Background(bg).
		Foreground(lipgloss.Color("#0a0a0a")).
		Bold(true).
		Width(width).
		Align(lipgloss.Center).
		Render(text)
}

// meterRow lays out a gradient meter with a right-hand label inside a total
// width, sizing the bar so the label always fits on the same line.
//
// Sizing the bar independently of its label is how the label ends up wrapping
// onto its own line and breaking the panel's height.
func meterRow(total int, ratio float64, from, to, label string) string {
	labelW := lipgloss.Width(label)
	barW := total - labelW - 2
	if barW < 8 {
		barW = 8
	}
	return gradientMeter(barW, ratio, from, to) + "  " +
		lipgloss.NewStyle().Foreground(dimColor).Render(label)
}

// hexToRGB parses "#rrggbb".
func hexToRGB(hex string) (int, int, int) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 255, 255, 255
	}
	var r, g, b int
	fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	return r, g, b
}

// gradientMeter draws a filled bar whose colour blends from `from` to `to`
// across the filled portion, one colour per cell.
//
// Lip Gloss v1 has no Blend1D, so the interpolation is done here. Blending per
// cell (rather than colouring the whole bar one shade) is what makes the meter
// read as a continuous quantity instead of a chunky block.
func gradientMeter(width int, ratio float64, from, to string) string {
	if width < 1 {
		return ""
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}

	filled := int(ratio * float64(width))
	r1, g1, b1 := hexToRGB(from)
	r2, g2, b2 := hexToRGB(to)

	var b strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			t := 0.0
			if width > 1 {
				t = float64(i) / float64(width-1)
			}
			c := fmt.Sprintf("#%02x%02x%02x",
				int(float64(r1)+(float64(r2)-float64(r1))*t),
				int(float64(g1)+(float64(g2)-float64(g1))*t),
				int(float64(b1)+(float64(b2)-float64(b1))*t))
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render("█"))
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(borderColor).Render("░"))
		}
	}
	return b.String()
}

// dnsStep identifies a stage of the DNS setup flow.
type dnsStep int

const (
	stepCreateZone dnsStep = iota
	stepShowNameservers
	stepFindParent
	stepWriteRecord
	stepPropagate
	stepDone
)

var dnsStepLabels = map[dnsStep]string{
	stepCreateZone:      "Create zone",
	stepShowNameservers: "Nameservers",
	stepFindParent:      "Find parent",
	stepWriteRecord:     "Delegate",
	stepPropagate:       "Propagate",
	stepDone:            "Done",
}

var dnsStepOrder = []dnsStep{
	stepCreateZone, stepShowNameservers, stepFindParent, stepWriteRecord, stepPropagate,
}

// stepState is how a step is rendered in the rail.
type stepState int

const (
	stepPending stepState = iota
	stepActive
	stepOK
	stepFailed
	stepSkipped
)

// renderStepRail draws the horizontal progress rail across the top.
//
// This is the piece that makes a multi-stage flow legible: at a glance you can
// see where you are, what already succeeded, and what is left — which the plain
// text version could not convey at all.
func renderStepRail(current dnsStep, states map[dnsStep]stepState, width int) string {
	var parts []string

	for i, s := range dnsStepOrder {
		state := states[s]
		if s == current && state == stepPending {
			state = stepActive
		}

		label := dnsStepLabels[s]
		var chip string
		switch state {
		case stepOK:
			chip = badge("✓ "+label, successColor)
		case stepActive:
			chip = badge("▸ "+label, accentColor)
		case stepFailed:
			chip = badge("✗ "+label, dangerColor)
		case stepSkipped:
			chip = lipgloss.NewStyle().Foreground(mutedColor).Render("· " + label)
		default:
			chip = lipgloss.NewStyle().Foreground(mutedColor).Render("  " + label)
		}
		parts = append(parts, chip)

		if i < len(dnsStepOrder)-1 {
			sep := lipgloss.NewStyle().Foreground(borderColor).Render(" ── ")
			parts = append(parts, sep)
		}
	}

	rail := lipgloss.JoinHorizontal(lipgloss.Center, parts...)
	if lipgloss.Width(rail) > width-4 {
		// Narrow terminal: fall back to "step N of M — label".
		idx := 1
		for i, s := range dnsStepOrder {
			if s == current {
				idx = i + 1
			}
		}
		rail = lipgloss.NewStyle().Foreground(dimColor).
			Render(fmt.Sprintf("Step %d of %d — %s", idx, len(dnsStepOrder), dnsStepLabels[current]))
	}
	return rail
}

// renderNameserverPanel shows the records that must exist in the parent zone.
//
// These are the actual delegation set read from Route53 — never a guess. The
// web UI used to display a fabricated `_acme-challenge` record here, which is a
// Let's Encrypt name ACM never creates.
func renderNameserverPanel(zone, parent string, nameservers []string, width int) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Delegation record") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(dimColor).
		Render(fmt.Sprintf("add to the %s zone", parent)) + "\n\n")

	label := lipgloss.NewStyle().Foreground(mutedColor).Width(8)
	value := lipgloss.NewStyle().Foreground(fgColor)

	b.WriteString(label.Render("Name") + value.Render(zone) + "\n")
	b.WriteString(label.Render("Type") + value.Render("NS") + "\n")
	b.WriteString(label.Render("TTL") + value.Render("300") + "\n")
	b.WriteString(label.Render("Value") + "\n")

	nsStyle := lipgloss.NewStyle().Foreground(accentColor).PaddingLeft(8)
	for _, ns := range nameservers {
		b.WriteString(nsStyle.Render(ns) + "\n")
	}

	return boxStyle.Width(width).Render(strings.TrimRight(b.String(), "\n"))
}

// profileCandidateLine renders one parent-zone candidate as a selectable row.
//
// Only a candidate whose nameservers match public DNS is actionable: writing
// into a same-named zone in an unrelated account would silently do nothing, so
// those are shown but cannot be chosen.
func profileCandidateLine(c parentZoneCandidate, selected bool, width int) string {
	// Fixed width so the detail column aligns across rows.
	const statusW = 10
	var status string
	switch {
	case c.Err != nil:
		status = badgeFixed("ERROR", dangerColor, statusW)
	case c.Authoritative:
		status = badgeFixed("VERIFIED", successColor, statusW)
	default:
		status = badgeFixed("MISMATCH", warningColor, statusW)
	}

	name := lipgloss.NewStyle().Foreground(fgColor).Bold(selected).Width(18).Render(c.Profile)

	detail := ""
	switch {
	case c.Err != nil:
		detail = shortError(c.Err)
	case c.Authoritative:
		detail = fmt.Sprintf("matches public DNS · account %s", c.AccountID)
	default:
		detail = "does not serve this domain — cannot delegate here"
	}
	detailStyle := lipgloss.NewStyle().Foreground(mutedColor)
	if selected {
		detailStyle = detailStyle.Foreground(dimColor)
	}

	// Selection is shown with a cursor and a bright left bar rather than a
	// background fill. Lip Gloss resets the background inside nested styles, so a
	// row containing badges renders as a half-highlighted block — visually broken.
	cursor := lipgloss.NewStyle().Foreground(borderColor).Render("│ ")
	if selected {
		cursor = lipgloss.NewStyle().Foreground(primaryColor).Bold(true).Render("▸ ")
	}

	return cursor + name + " " + status + " " + detailStyle.Render(detail)
}

// renderResolverGrid shows per-resolver propagation state.
//
// Delegation does not appear everywhere at once, so a single "waiting" spinner
// hides the fact that it is partially live. One badge per resolver makes the
// rollout visible.
func renderResolverGrid(results map[string]bool, order []string) string {
	var cells []string
	for _, r := range order {
		ok, checked := results[r]
		switch {
		case !checked:
			cells = append(cells, lipgloss.NewStyle().Foreground(mutedColor).Render("○ "+r))
		case ok:
			cells = append(cells, lipgloss.NewStyle().Foreground(successColor).Render("● "+r))
		default:
			cells = append(cells, lipgloss.NewStyle().Foreground(warningColor).Render("◐ "+r))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(cells, "   "))
}

// renderDNSHeader is the title bar: what we are doing and to which zone.
func renderDNSHeader(zone string, elapsed time.Duration, width int) string {
	left := lipgloss.NewStyle().Bold(true).Render("🌐 DNS setup") +
		lipgloss.NewStyle().Foreground(dimColor).Render("  "+zone)

	right := lipgloss.NewStyle().Foreground(mutedColor).
		Render(fmt.Sprintf("%02d:%02d", int(elapsed.Minutes()), int(elapsed.Seconds())%60))

	gap := width - lipgloss.Width(left) - lipgloss.Width(right) - 4
	if gap < 1 {
		gap = 1
	}
	return headerStyle.Width(width).Render(left + strings.Repeat(" ", gap) + right)
}

// renderDNSFooter renders contextual key hints, dimmed as chrome.
func renderDNSFooter(hints []string) string {
	return lipgloss.NewStyle().Foreground(mutedColor).Render("  " + strings.Join(hints, "   "))
}
