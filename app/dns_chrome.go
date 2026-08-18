package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// App chrome for the DNS screens: header bar, titled panels, action menus, key
// legend, and per-type record badges.
//
// The design targets are lazygit and posting — a persistent frame you navigate
// rather than a page of prose. Three rules carry most of the weight:
//
//   - state is a badge, never a sentence;
//   - the chrome recedes in gray so the data can be the only bright thing;
//   - every key that does something is visible in the legend at all times, so
//     the screen teaches itself and there is nothing to memorise.

// ------------------------------------------------------------------ header ---

// renderAppHeader draws the top bar: product mark, section, subject, and the
// right-aligned context that answers "which account am I about to change?".
//
// The account is on screen permanently rather than only in a confirmation,
// because deploying to the wrong account is the mistake worth designing against
// and it is invisible once a confirmation has scrolled away.
func renderAppHeader(section, subject, context, right string, width int) string {
	mark := lipgloss.NewStyle().
		Background(primaryColor).Foreground(lipgloss.Color("#0a0a0a")).
		Bold(true).Padding(0, 1).Render("MEROKU")

	sec := lipgloss.NewStyle().
		Background(adapt("#f3f4f6", "#1f2937")).Foreground(fgColor).
		Bold(true).Padding(0, 1).Render(section)

	subj := lipgloss.NewStyle().Foreground(fgColor).Bold(true).Render("  " + subject)

	left := mark + sec + subj

	var rightParts []string
	if context != "" {
		rightParts = append(rightParts, lipgloss.NewStyle().Foreground(mutedColor).Render(context))
	}
	if right != "" {
		rightParts = append(rightParts, lipgloss.NewStyle().
			Background(adapt("#f3f4f6", "#1f2937")).Foreground(dimColor).
			Padding(0, 1).Render(right))
	}
	rightSide := strings.Join(rightParts, "  ")

	gap := width - lipgloss.Width(left) - lipgloss.Width(rightSide)
	if gap < 1 {
		// Drop the context before the timer: which account you are in matters
		// less than nothing at all, but the subject matters more than both.
		rightSide = right
		gap = width - lipgloss.Width(left) - lipgloss.Width(rightSide)
		if gap < 1 {
			gap = 1
		}
	}
	return left + strings.Repeat(" ", gap) + rightSide
}

// ------------------------------------------------------------------ panels ---

// panel draws a titled box. The title sits in the border, btop-style, so it
// costs no interior line.
func panel(title, body string, width int, accent lipgloss.TerminalColor) string {
	if accent == nil {
		accent = borderColor
	}
	head := lipgloss.NewStyle().Foreground(accent).Bold(true).Render(strings.ToUpper(title))

	inner := width - 4
	if inner < 10 {
		inner = 10
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(inner).
		Render(head + "\n" + body)
}

// kvRow renders a dim label against a bright value — the standard way to show a
// fact without spending a sentence on it.
func kvRow(label, value string, labelWidth int) string {
	return lipgloss.NewStyle().Foreground(mutedColor).Width(labelWidth).Render(label) +
		lipgloss.NewStyle().Foreground(fgColor).Render(value)
}

// ------------------------------------------------------------------ badges ---

// recordTypeColors gives every DNS record type a fixed hue, so a zone listing is
// scannable by shape rather than by reading each row.
//
// The groupings are semantic: address records blue, mail amber (the ones whose
// loss hurts most), delegation green, text purple.
var recordTypeColors = map[string]lipgloss.TerminalColor{
	"A":     adapt("#2563eb", "#3b82f6"),
	"AAAA":  adapt("#4f46e5", "#6366f1"),
	"CNAME": adapt("#0891b2", "#06b6d4"),
	"MX":    adapt("#b45309", "#f59e0b"),
	"TXT":   adapt("#7e22ce", "#a855f7"),
	"SPF":   adapt("#7e22ce", "#a855f7"),
	"NS":    adapt("#059669", "#10b981"),
	"SOA":   lipgloss.Color("#6b7280"),
	"SRV":   adapt("#db2777", "#ec4899"),
	"CAA":   adapt("#0f766e", "#14b8a6"),
}

func recordTypeBadge(t string) string {
	c, ok := recordTypeColors[strings.ToUpper(t)]
	if !ok {
		c = mutedColor
	}
	return badgeFixed(strings.ToUpper(t), c, 7)
}

// statChip is a compact labelled value: dim label, bright number, on a panel of
// its own. Used for counts that would otherwise be buried in a sentence.
func statChip(label, value string, tone lipgloss.TerminalColor) string {
	return lipgloss.NewStyle().Foreground(mutedColor).Render(label+" ") +
		lipgloss.NewStyle().Foreground(tone).Bold(true).Render(value)
}

// ----------------------------------------------------------------- actions ---

// keycap renders a pressable key.
//
// Keys are always neutral gray and status badges are always saturated colour.
// They used to share one treatment — dark ink on a bright fill — so a BLOCKED
// chip and a [t] key looked like the same kind of thing, and the screen
// suggested you could press BLOCKED. One visual, one meaning: gray means press
// me, colour means this is the state of something.
func keycap(k string) string {
	return lipgloss.NewStyle().
		Background(adapt("#e5e7eb", "#374151")).Foreground(fgColor).
		Bold(true).Padding(0, 1).Render(k)
}

// action is one choice in a menu: the key that triggers it, a short title, and
// one line of consequence.
type action struct {
	key    string
	title  string
	detail string
	// tone colours the title, not the key — the risk lives in what the option
	// does, not in the key you press to do it.
	tone lipgloss.TerminalColor
}

// renderActions draws a menu of choices as keycap + title + consequence.
//
// This replaces the paragraph that used to explain the same options in prose.
// The options were always there; they were just spelled out in sentences the
// operator had to parse to find the one key they needed.
func renderActions(actions []action, width int) string {
	var b strings.Builder
	for i, a := range actions {
		tone := a.tone
		if tone == nil {
			tone = fgColor
		}

		b.WriteString(keycap(a.key) + " " +
			lipgloss.NewStyle().Foreground(tone).Bold(true).Render(a.title) + "\n")

		if a.detail != "" {
			for _, line := range strings.Split(wordWrap(a.detail, width-8), "\n") {
				b.WriteString(lipgloss.NewStyle().Foreground(mutedColor).
					Render("     "+line) + "\n")
			}
		}
		if i < len(actions)-1 {
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// ------------------------------------------------------------------ legend ---

// keyHint is one entry in the bottom legend.
type keyHint struct {
	key   string
	label string
}

// renderKeyLegend draws the persistent shortcut bar: every key rendered as a cap
// with its meaning beside it, wrapped onto as many lines as the width needs.
//
// Nothing is ever dropped to make it fit. A key the operator cannot see does not
// exist to them, and a legend that silently truncates is worse than a long one
// because it teaches that the list is complete when it is not.
func renderKeyLegend(hints []keyHint, width int) string {
	capStyle := lipgloss.NewStyle().
		Background(adapt("#e5e7eb", "#374151")).Foreground(fgColor).
		Bold(true).Padding(0, 1)
	labelStyle := lipgloss.NewStyle().Foreground(mutedColor)

	rendered := make([]string, 0, len(hints))
	for _, h := range hints {
		rendered = append(rendered, capStyle.Render(h.key)+labelStyle.Render(" "+h.label))
	}

	const indent = " "
	const sep = "   "
	var lines []string
	cur := ""
	for _, r := range rendered {
		candidate := r
		if cur != "" {
			candidate = cur + sep + r
		}
		if lipgloss.Width(indent+candidate) > width && cur != "" {
			lines = append(lines, indent+cur)
			cur = r
			continue
		}
		cur = candidate
	}
	if cur != "" {
		lines = append(lines, indent+cur)
	}
	return strings.Join(lines, "\n")
}

// ------------------------------------------------------------- misc pieces ---

// spinnerFrames is a braille rotation — eight frames, one glyph, no width
// change between them so nothing beside it shifts.
var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

func spinnerFrame(phase int) string {
	return spinnerFrames[phase%len(spinnerFrames)]
}

// indeterminateRow renders work whose duration is genuinely unknown: a lit block
// sweeping back and forth along a dim track, with a spinner and a label.
//
// This exists because the alternative was a lie. The previous version drove a
// normal progress meter from elapsed time, easing toward 90% — so a bar that
// looked exactly like the determinate ones ("3/12 profiles") was in fact
// reporting nothing but how long it had been since it started. A terraform
// apply, a zone copy and a record comparison have no total to divide by, and a
// bar that fills anyway teaches the operator to distrust the ones that mean
// something. A sweeping block cannot be mistaken for a measurement.
func indeterminateRow(width, phase int, label string) string {
	labelText := spinnerFrame(phase) + " " + label
	trackW := width - lipgloss.Width(labelText) - 2
	if trackW < 8 {
		return lipgloss.NewStyle().Foreground(accentColor).Render(labelText)
	}

	const block = 8
	travel := trackW - block
	if travel < 1 {
		travel = 1
	}

	// Bounce rather than wrap: a block that reappears at the left after leaving
	// the right reads as a restart, which suggests a retry that is not happening.
	pos := phase % (2 * travel)
	if pos > travel {
		pos = 2*travel - pos
	}

	var b strings.Builder
	for i := 0; i < trackW; i++ {
		if i >= pos && i < pos+block {
			// Shade across the block so it has a leading edge and reads as moving
			// in a direction rather than blinking in place.
			t := float64(i-pos) / float64(block-1)
			r1, g1, b1 := hexToRGB("#3b82f6")
			r2, g2, b2 := hexToRGB("#10b981")
			c := fmt.Sprintf("#%02x%02x%02x",
				int(float64(r1)+(float64(r2)-float64(r1))*t),
				int(float64(g1)+(float64(g2)-float64(g1))*t),
				int(float64(b1)+(float64(b2)-float64(b1))*t))
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render("█"))
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(borderColor).Render("░"))
		}
	}

	return b.String() + "  " + lipgloss.NewStyle().Foreground(accentColor).Render(labelText)
}

// countdownRow shows a wait as a draining meter plus the seconds left, so the
// screen reads as "working" rather than "stuck".
func countdownRow(remaining, total int, label string, width int) string {
	if total <= 0 {
		total = 1
	}
	ratio := float64(total-remaining) / float64(total)
	return meterRow(width, ratio, "#3b82f6", "#10b981",
		fmt.Sprintf("%s %ds", label, remaining))
}

// columnWidth returns the width each of two side-by-side panels should be built
// at, and whether they will actually fit that way.
//
// The caller has to ask *before* rendering, not after: a panel built at half
// width and then stacked leaves half the terminal empty and truncates its own
// content to a width it no longer needs. The threshold is on the inner width a
// column would get — below about 46 columns a key/value panel starts wrapping
// its values, and stacked-and-full-width beats side-by-side-and-broken.
func columnWidth(total int) (int, bool) {
	const minColumn = 46
	half := total / 2
	if half < minColumn || total-half < minColumn {
		return total, false
	}
	return half, true
}

// joinColumns places two already-rendered columns side by side, padding the
// shorter one so they end level. Unequal columns read as a gap in the layout
// rather than as one panel simply having less to say.
func joinColumns(left, right string) string {
	lh, rh := lipgloss.Height(left), lipgloss.Height(right)
	if lh < rh {
		left += strings.Repeat("\n", rh-lh)
	} else if rh < lh {
		right += strings.Repeat("\n", lh-rh)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}
