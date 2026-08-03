package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The domain-adoption screens, reached from the manual fallback when the current
// DNS host cannot delegate a subdomain at all.
//
// Five steps, each gated on the operator: choose where the apex zone lives,
// discover what the domain serves today, review and confirm the copy, check the
// copy against the live zone, then switch nameservers at the registrar. Nothing
// is visible to the internet until that last step, so everything before it can
// be undone by deleting the new zone.

type adoptPhase int

const (
	adoptOff adoptPhase = iota
	adoptPickProfile
	adoptDiscovering
	adoptReview
	adoptCopying
	adoptVerify
	adoptWaitNS
)

type adoptState struct {
	phase      adoptPhase
	profiles   []string
	profileIdx int

	snapshot zoneSnapshot
	summary  adoptionSummary
	diffs    []resolutionDiff

	nsCheckIn int
	checking  bool
	err       error
}

func (a adoptState) profile() string {
	if a.profileIdx < len(a.profiles) {
		return a.profiles[a.profileIdx]
	}
	return ""
}

// ---------------------------------------------------------------- messages ---

type adoptDiscoveredMsg struct {
	snap zoneSnapshot
	err  error
}

type adoptCopiedMsg struct {
	summary adoptionSummary
	err     error
}

type adoptVerifiedMsg struct{ diffs []resolutionDiff }

type adoptNSLiveMsg struct{ live bool }

// ---------------------------------------------------------------- commands ---

// beginAdoption loads the local profiles, preselecting the one this environment
// deploys with — the apex zone almost always belongs in the same account as
// everything else the environment owns, and it is the one profile we already
// know works.
func (m *dnsSetupModel) beginAdoption() {
	profiles, err := getLocalAWSProfiles()
	if err != nil || len(profiles) == 0 {
		m.adopt.err = fmt.Errorf("no local AWS profiles found")
		m.adopt.phase = adoptPickProfile
		return
	}

	m.adopt.profiles = profiles
	m.adopt.profileIdx = 0
	for i, p := range profiles {
		if p == m.env.AWSProfile {
			m.adopt.profileIdx = i
			break
		}
	}
	m.adopt.phase = adoptPickProfile
}

func (m *dnsSetupModel) adoptDiscoverCmd() tea.Cmd {
	domain := m.parent
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		ns, err := queryNameservers(domain)
		if err != nil || len(ns) == 0 {
			return adoptDiscoveredMsg{err: fmt.Errorf("could not resolve the nameservers for %s", domain)}
		}
		snap, err := discoverZoneRecords(ctx, domain, ns)
		return adoptDiscoveredMsg{snap: snap, err: err}
	}
}

func (m *dnsSetupModel) adoptCopyCmd() tea.Cmd {
	profile := m.adopt.profile()
	domain := m.parent
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		summary, err := adoptDomainIntoRoute53(ctx, profile, domain)
		return adoptCopiedMsg{summary: summary, err: err}
	}
}

// adoptVerifyCmd compares every copied record between the old host and the new
// zone. This runs before the nameserver switch is even shown, because it is the
// only chance to see what would break while breaking nothing.
func (m *dnsSetupModel) adoptVerifyCmd() tea.Cmd {
	domain := m.parent
	records := recordsToCopy(m.adopt.summary.Snapshot)
	newNS := m.adopt.summary.Zone.Nameservers
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		oldNS, err := queryNameservers(domain)
		if err != nil || len(oldNS) == 0 {
			return adoptVerifiedMsg{}
		}
		return adoptVerifiedMsg{diffs: compareZones(ctx, records, oldNS, newNS)}
	}
}

// adoptCheckApexCmd asks whether the registrar change has taken effect yet.
func (m *dnsSetupModel) adoptCheckApexCmd() tea.Cmd {
	domain := m.parent
	expected := m.adopt.summary.Zone.Nameservers
	return func() tea.Msg {
		observed, err := queryNameservers(domain)
		if err != nil {
			return adoptNSLiveMsg{live: false}
		}
		return adoptNSLiveMsg{live: nameserverSetsMatch(expected, observed)}
	}
}

// ------------------------------------------------------------------ update ---

// updateAdopt handles the adoption sub-flow. Returns handled=false when the
// message is not ours, so the main Update can carry on.
func (m *dnsSetupModel) updateAdopt(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case adoptDiscoveredMsg:
		if msg.err != nil {
			m.adopt.err = msg.err
			m.adopt.phase = adoptReview
			return nil, true
		}
		m.adopt.snapshot = msg.snap
		m.adopt.phase = adoptReview
		return nil, true

	case adoptCopiedMsg:
		if msg.err != nil {
			m.adopt.err = msg.err
			m.adopt.phase = adoptReview
			return nil, true
		}
		m.adopt.summary = msg.summary
		m.adopt.phase = adoptVerify
		return m.adoptVerifyCmd(), true

	case adoptVerifiedMsg:
		m.adopt.diffs = msg.diffs
		return nil, true

	case adoptNSLiveMsg:
		m.adopt.checking = false
		if !msg.live {
			m.adopt.nsCheckIn = secondsBetweenDNSChecks
			return nil, true
		}
		// The apex is ours now, so the parent zone is one we control and the
		// subdomain delegation is an ordinary write. Hand back to the normal
		// path rather than duplicating it.
		m.adopt.phase = adoptOff
		m.manualReason = ""
		m.states[stepFindParent] = stepOK
		m.step = stepWriteRecord
		return m.delegateCmd(parentZoneCandidate{
			Profile:       m.adopt.profile(),
			ZoneID:        m.adopt.summary.Zone.ZoneID,
			Authoritative: true,
		}), true

	case dnsTickMsg:
		if m.adopt.phase == adoptWaitNS && !m.adopt.checking {
			if m.adopt.nsCheckIn > 0 {
				m.adopt.nsCheckIn--
			}
			if m.adopt.nsCheckIn == 0 {
				m.adopt.checking = true
				return m.adoptCheckApexCmd(), false // let the tick continue too
			}
		}
		return nil, false
	}
	return nil, false
}

// handleAdoptKey handles keys while the adoption flow is on screen.
func (m *dnsSetupModel) handleAdoptKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.adopt.phase == adoptOff {
		return nil, false
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		m.quitting = true
		return tea.Quit, true

	// Esc steps back rather than out: the copy is reversible right up until the
	// nameservers change, and an operator who wants a second look should not
	// have to restart the deploy to get one.
	case key.Matches(msg, m.keys.Continue):
		switch m.adopt.phase {
		case adoptPickProfile:
			m.adopt.phase = adoptOff
		case adoptReview:
			m.adopt.phase = adoptPickProfile
			m.adopt.err = nil
		default:
			m.adopt.phase = adoptOff
		}
		return nil, true

	case key.Matches(msg, m.keys.Up):
		if m.adopt.phase == adoptPickProfile && m.adopt.profileIdx > 0 {
			m.adopt.profileIdx--
		}
		return nil, true

	case key.Matches(msg, m.keys.Down):
		if m.adopt.phase == adoptPickProfile && m.adopt.profileIdx < len(m.adopt.profiles)-1 {
			m.adopt.profileIdx++
		}
		return nil, true

	case key.Matches(msg, m.keys.Enter):
		switch m.adopt.phase {
		case adoptPickProfile:
			if m.adopt.profile() == "" {
				return nil, true
			}
			m.adopt.phase = adoptDiscovering
			return m.adoptDiscoverCmd(), true

		case adoptReview:
			if m.adopt.err != nil {
				return nil, true
			}
			m.adopt.phase = adoptCopying
			return m.adoptCopyCmd(), true

		case adoptVerify:
			m.adopt.phase = adoptWaitNS
			m.adopt.nsCheckIn = secondsBetweenDNSChecks
			return nil, true
		}
		return nil, true

	case key.Matches(msg, m.keys.Copy):
		if m.adopt.phase == adoptWaitNS && len(m.adopt.summary.Zone.Nameservers) > 0 {
			m.copyToClipboard(strings.Join(m.adopt.summary.Zone.Nameservers, "\n"),
				"copied the new nameservers")
		}
		return nil, true

	case key.Matches(msg, m.keys.CopyOne):
		if m.adopt.phase == adoptWaitNS {
			ns := m.adopt.summary.Zone.Nameservers
			if n := int(msg.String()[0] - '0'); n >= 1 && n <= len(ns) {
				m.copyToClipboard(ns[n-1], "copied "+ns[n-1])
			}
		}
		return nil, true
	}
	return nil, true
}

// -------------------------------------------------------------------- view ---

func (m *dnsSetupModel) renderAdopt(width int) string {
	inner := width - 4
	switch m.adopt.phase {
	case adoptPickProfile:
		return m.renderAdoptPickProfile(width)
	case adoptDiscovering:
		return boxStyle.Width(inner).Render(
			titleStyle.Render("Reading the current "+m.parent+" zone") + "\n" +
				lipgloss.NewStyle().Foreground(dimColor).Render(
					"asking the nameservers that serve it today what they hold") + "\n\n" +
				meterRow(inner-4, pulse(m.elapsed), "#3b82f6", "#10b981", "scanning"))
	case adoptReview:
		return m.renderAdoptReview(width)
	case adoptCopying:
		return boxStyle.Width(inner).Render(
			titleStyle.Render("Creating the zone and copying records") + "\n\n" +
				meterRow(inner-4, pulse(m.elapsed), "#3b82f6", "#10b981", "copying"))
	case adoptVerify:
		return m.renderAdoptVerify(width)
	case adoptWaitNS:
		return m.renderAdoptWaitNS(width)
	}
	return ""
}

func (m *dnsSetupModel) renderAdoptPickProfile(width int) string {
	inner := width - 4
	var b strings.Builder
	b.WriteString(titleStyle.Render("Move "+m.parent+" to Route53") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render(wordWrap(
		"Your DNS host cannot delegate a subdomain, but every registrar can "+
			"change a domain's nameservers. Hosting the whole zone in Route53 makes "+
			"the delegation ours to write.", inner-6)) + "\n\n")

	b.WriteString(lipgloss.NewStyle().Foreground(mutedColor).
		Render("Which AWS account should hold the zone?") + "\n\n")

	for i, p := range m.adopt.profiles {
		cursor := lipgloss.NewStyle().Foreground(borderColor).Render("│ ")
		name := lipgloss.NewStyle().Foreground(fgColor).Render(p)
		if i == m.adopt.profileIdx {
			cursor = lipgloss.NewStyle().Foreground(primaryColor).Bold(true).Render("▸ ")
			name = lipgloss.NewStyle().Foreground(fgColor).Bold(true).Render(p)
		}
		note := ""
		if p == m.env.AWSProfile {
			note = lipgloss.NewStyle().Foreground(mutedColor).
				Render("   this environment deploys here")
		}
		b.WriteString(cursor + name + note + "\n")
	}

	if m.adopt.err != nil {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(dangerColor).
			Render(truncateToWidth(m.adopt.err.Error(), inner-6)))
	}
	return boxStyle.Width(inner).Render(strings.TrimRight(b.String(), "\n"))
}

func (m *dnsSetupModel) renderAdoptReview(width int) string {
	inner := width - 4
	if m.adopt.err != nil {
		return boxStyle.Width(inner).Render(
			badge("FAILED", dangerColor) + "  " +
				lipgloss.NewStyle().Foreground(fgColor).Render("could not read the current zone") + "\n\n" +
				lipgloss.NewStyle().Foreground(dimColor).
					Render(wordWrap(m.adopt.err.Error(), inner-6)))
	}

	snap := m.adopt.snapshot
	records := recordsToCopy(snap)

	// Counts as chips, not a sentence — the numbers are what the operator checks
	// against their provider's own record list.
	confidence := badge("PARTIAL", warningColor)
	if snap.Complete {
		confidence = badge("COMPLETE", successColor)
	}
	head := confidence + "  " +
		statChip("records", fmt.Sprintf("%d", len(records)), fgColor) + "   " +
		statChip("probed", fmt.Sprintf("%d", snap.NamesProbed), dimColor) + "   " +
		lipgloss.NewStyle().Foreground(mutedColor).Render(snap.Method)

	var b strings.Builder
	b.WriteString(head + "\n\n")

	if w := snap.Warning(); w != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(warningColor).
			Render("⚠ "+wordWrap(w, inner-6)) + "\n\n")
	}
	if snap.WildcardHint != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(mutedColor).
			Render(wordWrap(snap.WildcardHint, inner-6)) + "\n\n")
	}

	// Column header, so the table reads as a table.
	b.WriteString(lipgloss.NewStyle().Foreground(mutedColor).Bold(true).Render(
		lipgloss.NewStyle().Width(30).Render("NAME")+
			lipgloss.NewStyle().Width(8).Render("TYPE")+"VALUE") + "\n")

	shown := records
	const maxRows = 10
	if len(shown) > maxRows {
		shown = shown[:maxRows]
	}
	for _, r := range shown {
		name := lipgloss.NewStyle().Foreground(fgColor).Width(30).
			Render(truncateToWidth(r.Name, 29))
		val := truncateToWidth(strings.Join(r.Values, ", "), max(12, inner-46))
		b.WriteString(name + recordTypeBadge(r.Type) + " " +
			lipgloss.NewStyle().Foreground(dimColor).Render(val) + "\n")
	}
	if len(records) > maxRows {
		b.WriteString(lipgloss.NewStyle().Foreground(mutedColor).
			Render(fmt.Sprintf("… %d more", len(records)-maxRows)) + "\n")
	}

	b.WriteString("\n" + renderActions([]action{
		{key: "enter", title: "CREATE ZONE IN " + strings.ToUpper(m.adopt.profile()),
			tone: successColor, detail: "copies these records; nothing resolves differently yet"},
	}, inner-4))

	return panel("review "+snap.Domain, b.String(), width, accentColor)
}

func (m *dnsSetupModel) renderAdoptVerify(width int) string {
	inner := width - 4
	var b strings.Builder
	sum := m.adopt.summary

	b.WriteString(badge("COPIED", successColor) + "  " +
		lipgloss.NewStyle().Foreground(fgColor).Render(
			fmt.Sprintf("%d records into zone %s", len(sum.Written), sum.Zone.ZoneID)) + "\n\n")

	if len(sum.Failures) > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(dangerColor).Render(
			fmt.Sprintf("%d could not be written — add these by hand:", len(sum.Failures))) + "\n")
		for i, f := range sum.Failures {
			if i >= 4 {
				break
			}
			b.WriteString(lipgloss.NewStyle().Foreground(mutedColor).Render(
				truncateToWidth("  "+f.Record.Name+" "+f.Record.Type+" — "+shortError(f.Err), inner-6)) + "\n")
		}
		b.WriteString("\n")
	}

	if m.adopt.diffs == nil {
		b.WriteString(meterRow(inner-4, pulse(m.elapsed), "#3b82f6", "#10b981", "comparing"))
		return boxStyle.Width(inner).Render(b.String())
	}

	bad := countMismatches(m.adopt.diffs)
	if bad == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(successColor).Render(
			"✓ every copied record resolves identically from the new zone") + "\n\n")
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(warningColor).Render(
			fmt.Sprintf("⚠ %d record(s) differ between the old host and the new zone —", bad)) + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(warningColor).Render(
			"  these would change the moment you switch nameservers:") + "\n")
		shown := 0
		for _, d := range m.adopt.diffs {
			if d.Name == "" || d.Match || shown >= 5 {
				continue
			}
			shown++
			b.WriteString(lipgloss.NewStyle().Foreground(mutedColor).Render(truncateToWidth(
				fmt.Sprintf("  %s %s: was %v, now %v", d.Name, d.Type, d.Old, d.New), inner-6)) + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render(wordWrap(
		"[Enter] shows the nameservers to set at your registrar. Until you do that, "+
			"nothing has changed — deleting the new zone undoes all of this.", inner-6)))

	return boxStyle.Width(inner).Render(b.String())
}

func (m *dnsSetupModel) renderAdoptWaitNS(width int) string {
	inner := width - 4
	ns := m.adopt.summary.Zone.Nameservers

	var b strings.Builder
	b.WriteString(titleStyle.Render("Set these nameservers at your registrar") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render(
		"for "+m.parent+" — replace the existing ones entirely") + "\n\n")

	idx := lipgloss.NewStyle().Foreground(mutedColor).PaddingLeft(2)
	val := lipgloss.NewStyle().Foreground(lipgloss.Color("#60a5fa")).Bold(true)
	for i, n := range ns {
		b.WriteString(idx.Render(fmt.Sprintf("%d ", i+1)) + val.Render(n) + "\n")
	}

	b.WriteString("\n" + lipgloss.NewStyle().Foreground(warningColor).Render(wordWrap(
		"This is the step that moves the domain. Mail and anything else on "+m.parent+
			" starts being answered by the new zone as it propagates, which can take "+
			"up to 48 hours depending on the old TTL.", inner-6)) + "\n\n")

	if m.adopt.checking {
		b.WriteString(lipgloss.NewStyle().Foreground(accentColor).Render("⟳ checking…"))
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(mutedColor).Render(fmt.Sprintf(
			"next check in %ds — the subdomain is delegated automatically once this lands",
			m.adopt.nsCheckIn)))
	}

	return boxStyle.Width(inner).Render(b.String())
}

// adoptFooterHints returns the key hints for the current adoption phase.
func (m *dnsSetupModel) adoptFooterHints() []keyHint {
	switch m.adopt.phase {
	case adoptPickProfile:
		return []keyHint{
			{"↑↓", "choose account"}, {"enter", "read the zone"},
			{"esc", "back"}, {"^C", "abort deploy"}}
	case adoptReview:
		if m.adopt.err != nil {
			return []keyHint{{"esc", "back"}, {"^C", "abort deploy"}}
		}
		return []keyHint{
			{"enter", "create zone and copy"}, {"esc", "back"}, {"^C", "abort deploy"}}
	case adoptVerify:
		return []keyHint{
			{"enter", "show nameservers"}, {"esc", "stop here"}, {"^C", "abort deploy"}}
	case adoptWaitNS:
		return []keyHint{
			{"c", "copy all"}, {"1-4", "copy one"},
			{"esc", "stop waiting"}, {"^C", "abort deploy"}}
	}
	return []keyHint{{"^C", "abort deploy"}}
}
