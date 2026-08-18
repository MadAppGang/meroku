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

	// Numbered, because registrar forms take one nameserver per field and the
	// number is the key that copies that line. Brightest thing on the panel:
	// these are the values being copied out.
	idxStyle := lipgloss.NewStyle().Foreground(mutedColor).PaddingLeft(6)
	nsStyle := lipgloss.NewStyle().Foreground(adapt("#2563eb", "#60a5fa")).Bold(true)
	for i, ns := range nameservers {
		b.WriteString(idxStyle.Render(fmt.Sprintf("%d ", i+1)) + nsStyle.Render(ns) + "\n")
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

	// Truncate to the row budget. Letting the detail wrap pushes the overflow to
	// column 0 — outside the panel border — which breaks every row below it.
	//   cursor(2) + name(18) + space + badge(statusW) + space
	detailBudget := width - 2 - 18 - 1 - statusW - 1
	if detailBudget < 12 {
		detailBudget = 12
	}
	if lipgloss.Width(detail) > detailBudget {
		detail = truncateToWidth(detail, detailBudget-1) + "…"
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

// renderResolverList shows one resolver per line with its own state.
//
// A compact grid of dots fits on one line but says almost nothing: the shapes
// are small, the colours are the only signal, and while a check runs nothing
// moves. One row each gives every resolver a spinner while it is being asked and
// a badge once it answers, so the screen shows both what is happening and what
// has already been established.
//
// A resolver that has already answered yes keeps its badge through later checks.
// It is not being re-asked in any meaningful sense — the delegation does not
// un-resolve — and blinking it back to "checking" would suggest doubt about
// something already settled.
//
// A stale resolver keeps its badge for the same reason, in the other direction.
// It is not lagging, it is looking at a delegation that is not ours, and that
// does not resolve itself between two polls ten seconds apart. Animating it as
// "checking" would imply progress that is not being made.
func renderResolverList(results map[string]dohVerdict, order []string, checking bool, anim, width int) string {
	nameW := 0
	for _, r := range order {
		if n := lipgloss.Width(r); n > nameW {
			nameW = n
		}
	}
	if nameW > width-16 {
		nameW = max(8, width-16)
	}

	var b strings.Builder
	for i, r := range order {
		var icon, badgeCell string
		switch {
		case results[r] == dohResolved:
			icon = lipgloss.NewStyle().Foreground(successColor).Render("✓")
			badgeCell = badge("RESOLVED", successColor)
		case results[r] == dohStale:
			icon = lipgloss.NewStyle().Foreground(dangerColor).Render("✗")
			badgeCell = badge("STALE", dangerColor)
		case checking:
			icon = lipgloss.NewStyle().Foreground(accentColor).Render(spinnerFrame(anim + i*2))
			badgeCell = lipgloss.NewStyle().Foreground(accentColor).Render("checking…")
		default:
			icon = lipgloss.NewStyle().Foreground(mutedColor).Render("·")
			badgeCell = lipgloss.NewStyle().Foreground(mutedColor).Render("not yet")
		}

		name := lipgloss.NewStyle().Foreground(fgColor).Width(nameW + 2).
			Render(truncateToWidth(r, nameW))

		b.WriteString(icon + " " + name + badgeCell)
		if i < len(order)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// staleDelegationNote explains the red badges.
//
// The settle window exists to keep the certificate request away from resolvers
// holding a stale *negative* answer, which clears in minutes. This is not that.
// A stale resolver is answering from a previous incarnation of the zone, and
// since Route53 issues a fresh nameserver set to every hosted zone it created,
// the old set no longer answers for the name at all. That cache lives as long as
// the child zone's own NS TTL — two days by default — so the honest thing to say
// is that it will not clear inside this deploy, not that we are waiting for it.
func staleDelegationNote(stale int) string {
	subject := fmt.Sprintf("%d resolvers are", stale)
	if stale == 1 {
		subject = "One resolver is"
	}
	return subject + " answering from an earlier version of this zone rather " +
		"than the record just written. That clears when their cached copy of the " +
		"old nameservers expires, which can take up to two days — waiting here " +
		"will not change it."
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

// renderDNSFooter renders contextual key hints, dimmed as chrome, packed onto
// as many lines as the width needs.
//
// The manual screen offers six bindings, which is 114 columns on one line — so
// a fixed single-line footer overflowed the frame on any terminal under that.
// Hints are never dropped: a key the operator cannot see is a key that does not
// exist to them.
func renderDNSFooter(hints []string, width int) string {
	style := lipgloss.NewStyle().Foreground(mutedColor)
	const indent = "  "
	const sep = "   "

	var lines []string
	cur := ""
	for _, h := range hints {
		candidate := h
		if cur != "" {
			candidate = cur + sep + h
		}
		if lipgloss.Width(indent+candidate) > width && cur != "" {
			lines = append(lines, style.Render(indent+cur))
			cur = h
			continue
		}
		cur = candidate
	}
	if cur != "" {
		// A single hint wider than the terminal still has to be cut, or it would
		// wrap at column 0 and break the frame below it.
		if lipgloss.Width(indent+cur) > width {
			cur = truncateToWidth(cur, width-len(indent))
		}
		lines = append(lines, style.Render(indent+cur))
	}
	return strings.Join(lines, "\n")
}
