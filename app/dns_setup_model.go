package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The DNS setup screen: one Bubble Tea program covering the whole two-phase
// bootstrap — create the zone, show the delegation record, find the parent zone,
// write the NS record, wait for propagation.
//
// This replaces a stretch of the deploy that printed raw terraform output and a
// bare prompt into the scrollback, so the session dropped out of its full-screen
// interface partway through. All I/O happens in commands; the model only holds
// state and the view only renders it.

// ---------------------------------------------------------------- messages ---

type dnsZoneProgressMsg struct{ line string }

type dnsZoneCreatedMsg struct {
	zoneID      string
	nameservers []string
	err         error
}

type dnsCandidatesMsg struct {
	candidates []parentZoneCandidate
	// reason is set when we cannot offer automatic delegation at all.
	reason string
	err    error
}

type dnsDelegatedMsg struct{ err error }

type dnsPropagationMsg struct {
	results map[string]bool
	ok      bool
}

type dnsTickMsg time.Time

// ------------------------------------------------------------------- keys ---

type dnsKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Enter  key.Binding
	Manual key.Binding
	Copy   key.Binding
	Retry  key.Binding
	Skip   key.Binding
	Quit   key.Binding
}

var dnsKeys = dnsKeyMap{
	Up:     key.NewBinding(key.WithKeys("up", "k")),
	Down:   key.NewBinding(key.WithKeys("down", "j")),
	Enter:  key.NewBinding(key.WithKeys("enter")),
	Manual: key.NewBinding(key.WithKeys("m")),
	Copy:   key.NewBinding(key.WithKeys("c")),
	Retry:  key.NewBinding(key.WithKeys("r")),
	Skip:   key.NewBinding(key.WithKeys("s")),
	Quit:   key.NewBinding(key.WithKeys("ctrl+c", "q")),
}

// ------------------------------------------------------------------ model ---

type dnsSetupModel struct {
	width, height int
	env           Env

	zone   string
	parent string

	step   dnsStep
	states map[dnsStep]stepState

	startTime time.Time
	elapsed   time.Duration

	// create zone
	zoneID      string
	nameservers []string
	logLines    []string

	// find parent / choose
	candidates []parentZoneCandidate
	cursor     int
	choosing   bool

	// propagate
	resolvers       []string
	resolverResults map[string]bool

	// outcome
	err          error
	manualReason string
	quitting     bool

	// Delegated reports whether delegation is verified, so the caller knows
	// whether phase 2 can run.
	Delegated bool

	keys dnsKeyMap
}

// newDNSSetupModel builds the screen for an environment whose zone is missing
// (bootstrap) or undelegated (blocked).
func newDNSSetupModel(e Env, res dnsPreflightResult) *dnsSetupModel {
	m := &dnsSetupModel{
		env:             e,
		zone:            res.ZoneName,
		parent:          res.ParentDomain,
		zoneID:          res.ZoneID,
		nameservers:     res.ZoneNameservers,
		states:          map[dnsStep]stepState{},
		startTime:       time.Now(),
		resolvers:       []string{"8.8.8.8", "1.1.1.1", "9.9.9.9", "208.67.222.222"},
		resolverResults: map[string]bool{},
		keys:            dnsKeys,
		width:           100,
		height:          30,
	}

	// If the zone already exists we skip straight to finding the parent.
	if res.ZoneID != "" && len(res.ZoneNameservers) > 0 {
		m.step = stepFindParent
		m.states[stepCreateZone] = stepSkipped
		m.states[stepShowNameservers] = stepOK
	} else {
		m.step = stepCreateZone
	}
	return m
}

func (m *dnsSetupModel) Init() tea.Cmd {
	if m.step == stepCreateZone {
		return tea.Batch(m.tick(), m.createZoneCmd())
	}
	return tea.Batch(m.tick(), m.findParentCmd())
}

func (m *dnsSetupModel) tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return dnsTickMsg(t) })
}

// --------------------------------------------------------------- commands ---

// createZoneCmd runs phase 1: a targeted apply that creates only the hosted zone.
func (m *dnsSetupModel) createZoneCmd() tea.Cmd {
	env := m.env
	return func() tea.Msg {
		cmd := exec.Command("terraform", "apply",
			"-no-color", "-auto-approve", "-target="+zoneTargetAddress)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return dnsZoneCreatedMsg{err: err}
		}
		cmd.Stderr = cmd.Stdout
		if err := cmd.Start(); err != nil {
			return dnsZoneCreatedMsg{err: err}
		}

		var tail []string
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line != "" {
				tail = append(tail, line)
			}
		}
		if err := cmd.Wait(); err != nil {
			return dnsZoneCreatedMsg{err: fmt.Errorf("%w\n%s", err, strings.Join(lastN(tail, 12), "\n"))}
		}

		// Read the zone back from AWS rather than trusting the exit code:
		// terraform does not fail when -target matches nothing.
		res, err := checkDNSPreflight(context.Background(), env)
		if err != nil {
			return dnsZoneCreatedMsg{err: err}
		}
		if res.ZoneID == "" {
			return dnsZoneCreatedMsg{err: fmt.Errorf(
				"apply reported success but zone %s does not exist — the -target %q may no longer match",
				res.ZoneName, zoneTargetAddress)}
		}
		return dnsZoneCreatedMsg{zoneID: res.ZoneID, nameservers: res.ZoneNameservers}
	}
}

// findParentCmd resolves the parent domain and scans local AWS profiles for it.
func (m *dnsSetupModel) findParentCmd() tea.Cmd {
	parent := m.parent
	return func() tea.Msg {
		if parent == "" {
			return dnsCandidatesMsg{reason: "this is a root domain — delegate it at your registrar"}
		}

		publicNS, err := queryNameservers(parent)
		if err != nil || len(publicNS) == 0 {
			return dnsCandidatesMsg{reason: fmt.Sprintf(
				"could not resolve nameservers for %s, so no candidate can be verified", parent)}
		}
		if !looksLikeRoute53(publicNS) {
			return dnsCandidatesMsg{reason: fmt.Sprintf(
				"%s is not hosted on Route53 (%s)", parent,
				strings.Join(normalizeNameservers(publicNS)[:1], ", "))}
		}

		profiles, err := getLocalAWSProfiles()
		if err != nil || len(profiles) == 0 {
			return dnsCandidatesMsg{reason: "no local AWS profiles found"}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		var usable []parentZoneCandidate
		for _, c := range scanProfilesForParentZone(ctx, profiles, parent, publicNS) {
			if c.ZoneID != "" || c.Err != nil {
				usable = append(usable, c)
			}
		}
		if len(usable) == 0 {
			return dnsCandidatesMsg{reason: fmt.Sprintf(
				"no local AWS profile holds a %s zone", parent)}
		}
		return dnsCandidatesMsg{candidates: usable}
	}
}

// delegateCmd writes the NS record into the chosen parent zone.
func (m *dnsSetupModel) delegateCmd(c parentZoneCandidate) tea.Cmd {
	req := delegationRequest{
		ParentProfile: c.Profile,
		ParentZoneID:  c.ZoneID,
		Subdomain:     m.zone,
		Nameservers:   m.nameservers,
	}
	env := m.env
	parent := m.parent
	zoneID := m.zoneID
	ns := m.nameservers
	return func() tea.Msg {
		if err := applyDelegation(req); err != nil {
			return dnsDelegatedMsg{err: err}
		}
		// Persistence is a convenience; a failure here does not undo the write.
		_ = recordDelegation(parent, req.Subdomain, env.AccountID, zoneID, ns)
		return dnsDelegatedMsg{}
	}
}

// pollPropagationCmd checks each public resolver individually.
//
// Per-resolver results are what make partial propagation visible; a single
// "waiting" spinner hides the fact that it is already live in half the world.
func (m *dnsSetupModel) pollPropagationCmd() tea.Cmd {
	zone := m.zone
	expected := m.nameservers
	servers := m.resolvers
	return func() tea.Msg {
		results := map[string]bool{}
		matched := 0
		for _, s := range servers {
			ok := resolverSeesDelegation(s, zone, expected)
			results[s] = ok
			if ok {
				matched++
			}
		}
		// A majority is enough: ACM only needs the record visible to its own
		// resolvers, and waiting for every last one stalls on slow caches.
		return dnsPropagationMsg{results: results, ok: matched >= 2}
	}
}

// resolverSeesDelegation asks one resolver whether the zone is delegated to us.
func resolverSeesDelegation(server, domain string, expected []string) bool {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 4 * time.Second}
			return d.DialContext(ctx, network, net.JoinHostPort(server, "53"))
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	found, err := r.LookupNS(ctx, domain)
	if err != nil || len(found) == 0 {
		return false
	}
	got := make([]string, 0, len(found))
	for _, ns := range found {
		got = append(got, ns.Host)
	}
	return nameserverSetsMatch(expected, got)
}

func lastN(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// ----------------------------------------------------------------- update ---

func (m *dnsSetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case dnsTickMsg:
		m.elapsed = time.Since(m.startTime)
		if m.quitting {
			return m, nil
		}
		return m, m.tick()

	case dnsZoneProgressMsg:
		m.logLines = lastN(append(m.logLines, msg.line), 6)
		return m, nil

	case dnsZoneCreatedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.states[stepCreateZone] = stepFailed
			m.quitting = true
			return m, tea.Quit
		}
		m.zoneID = msg.zoneID
		m.nameservers = msg.nameservers
		m.states[stepCreateZone] = stepOK
		m.states[stepShowNameservers] = stepOK
		m.step = stepFindParent
		return m, m.findParentCmd()

	case dnsCandidatesMsg:
		if msg.reason != "" || msg.err != nil {
			m.manualReason = msg.reason
			if msg.err != nil {
				m.manualReason = msg.err.Error()
			}
			m.states[stepFindParent] = stepFailed
			m.states[stepWriteRecord] = stepSkipped
			return m, nil
		}
		m.candidates = msg.candidates
		m.states[stepFindParent] = stepOK
		m.step = stepWriteRecord
		m.choosing = true
		// Start on the first candidate we are actually allowed to use.
		for i, c := range m.candidates {
			if c.Authoritative {
				m.cursor = i
				break
			}
		}
		return m, nil

	case dnsDelegatedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.manualReason = "the automatic write failed: " + shortError(msg.err)
			m.states[stepWriteRecord] = stepFailed
			return m, nil
		}
		m.states[stepWriteRecord] = stepOK
		m.step = stepPropagate
		return m, m.pollPropagationCmd()

	case dnsPropagationMsg:
		m.resolverResults = msg.results
		if msg.ok {
			m.states[stepPropagate] = stepOK
			m.step = stepDone
			m.Delegated = true
			return m, nil
		}
		// Keep polling until the operator stops us.
		return m, tea.Tick(10*time.Second, func(time.Time) tea.Msg {
			return dnsTickPollMsg{}
		})

	case dnsTickPollMsg:
		return m, m.pollPropagationCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// dnsTickPollMsg schedules the next propagation check.
type dnsTickPollMsg struct{}

func (m *dnsSetupModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.quitting = true
		return m, tea.Quit

	case key.Matches(msg, m.keys.Up):
		if m.choosing && m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case key.Matches(msg, m.keys.Down):
		if m.choosing && m.cursor < len(m.candidates)-1 {
			m.cursor++
		}
		return m, nil

	case key.Matches(msg, m.keys.Enter):
		if m.step == stepDone {
			m.quitting = true
			return m, tea.Quit
		}
		if m.choosing && m.cursor < len(m.candidates) {
			c := m.candidates[m.cursor]
			// Refuse non-authoritative zones: writing there changes nothing,
			// because that zone is not what the internet consults.
			if !c.Authoritative {
				return m, nil
			}
			m.choosing = false
			return m, m.delegateCmd(c)
		}
		return m, nil

	case key.Matches(msg, m.keys.Manual):
		if m.choosing {
			m.choosing = false
			m.manualReason = "you chose to add the record yourself"
			m.states[stepWriteRecord] = stepSkipped
		}
		return m, nil

	case key.Matches(msg, m.keys.Retry):
		if m.manualReason != "" {
			m.manualReason = ""
			m.err = nil
			m.states[stepFindParent] = stepPending
			m.step = stepFindParent
			return m, m.findParentCmd()
		}
		return m, nil

	case key.Matches(msg, m.keys.Skip):
		if m.step == stepPropagate {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil
	}
	return m, nil
}

// ------------------------------------------------------------------- view ---

func (m *dnsSetupModel) View() string {
	w := m.width
	if w < 60 {
		w = 60
	}
	inner := w - 4

	var b strings.Builder
	b.WriteString(renderDNSHeader(m.zone, m.elapsed, w) + "\n\n")
	b.WriteString(renderStepRail(m.step, m.states, w) + "\n\n")
	b.WriteString(m.renderBody(inner) + "\n\n")
	b.WriteString(renderDNSFooter(m.footerHints()))

	return lipgloss.Place(w, m.height, lipgloss.Left, lipgloss.Top, b.String())
}

func (m *dnsSetupModel) renderBody(inner int) string {
	switch {
	case m.err != nil && m.manualReason == "":
		return boxStyle.Width(inner).Render(
			badge("FAILED", dangerColor) + "  " +
				lipgloss.NewStyle().Foreground(fgColor).Render("DNS setup could not continue") + "\n\n" +
				lipgloss.NewStyle().Foreground(dimColor).Render(wordWrap(m.err.Error(), inner-4)))

	case m.manualReason != "":
		panel := boxStyle.Width(inner).Render(
			badge("MANUAL", warningColor) + "  " +
				lipgloss.NewStyle().Foreground(fgColor).Render("meroku cannot write this record for you") + "\n\n" +
				lipgloss.NewStyle().Foreground(dimColor).Render("Why: "+wordWrap(m.manualReason, inner-8)))
		if len(m.nameservers) > 0 {
			return panel + "\n\n" + renderNameserverPanel(m.zone, m.parent, m.nameservers, inner)
		}
		return panel

	case m.step == stepCreateZone:
		body := titleStyle.Render("Creating hosted zone") + "\n" +
			lipgloss.NewStyle().Foreground(dimColor).
				Render("the zone must exist before its nameservers can be delegated") + "\n\n" +
			meterRow(inner-4, pulse(m.elapsed), "#3b82f6", "#10b981",
				fmt.Sprintf("%ds", int(m.elapsed.Seconds())))
		return boxStyle.Width(inner).Render(body)

	case m.step == stepFindParent:
		body := titleStyle.Render("Looking for the "+m.parent+" zone") + "\n" +
			lipgloss.NewStyle().Foreground(dimColor).
				Render("scanning local AWS profiles — a match is proved against public DNS") + "\n\n" +
			meterRow(inner-4, pulse(m.elapsed), "#3b82f6", "#10b981", "scanning")
		return renderNameserverPanel(m.zone, m.parent, m.nameservers, inner) + "\n\n" +
			boxStyle.Width(inner).Render(body)

	case m.step == stepWriteRecord && m.choosing:
		var rows strings.Builder
		rows.WriteString(titleStyle.Render("Which AWS profile manages "+m.parent+"?") + "\n")
		rows.WriteString(lipgloss.NewStyle().Foreground(dimColor).
			Render("only a profile whose nameservers match public DNS can be delegated to") + "\n\n")
		for i, c := range m.candidates {
			rows.WriteString(profileCandidateLine(c, i == m.cursor, inner-4) + "\n")
		}
		return boxStyle.Width(inner).Render(strings.TrimRight(rows.String(), "\n"))

	case m.step == stepWriteRecord:
		return boxStyle.Width(inner).Render(
			titleStyle.Render("Writing delegation record") + "\n\n" +
				meterRow(inner-4, pulse(m.elapsed), "#3b82f6", "#10b981", "writing"))

	case m.step == stepPropagate:
		matched := 0
		for _, ok := range m.resolverResults {
			if ok {
				matched++
			}
		}
		ratio := float64(matched) / float64(len(m.resolvers))
		return boxStyle.Width(inner).Render(
			titleStyle.Render("Waiting for delegation to appear") + "\n" +
				lipgloss.NewStyle().Foreground(dimColor).
					Render(fmt.Sprintf("NS record written to zone %s", m.zoneID)) + "\n\n" +
				renderResolverGrid(m.resolverResults, m.resolvers) + "\n\n" +
				meterRow(inner-4, ratio, "#f59e0b", "#10b981",
					fmt.Sprintf("%d/%d resolvers", matched, len(m.resolvers))))

	case m.step == stepDone:
		return boxStyle.Width(inner).Render(
			badge("DELEGATED", successColor) + "  " +
				lipgloss.NewStyle().Foreground(fgColor).Render(m.zone+" resolves to this account") + "\n\n" +
				lipgloss.NewStyle().Foreground(dimColor).
					Render("Certificate validation can now succeed. Continuing with the full deploy."))
	}
	return ""
}

// pulse drives an indeterminate meter for work whose duration is unknown.
// It ramps to ~90% over a minute rather than pretending to be complete.
func pulse(elapsed time.Duration) float64 {
	s := elapsed.Seconds()
	return 0.9 * (1 - 1/(1+s/20))
}

func (m *dnsSetupModel) footerHints() []string {
	switch {
	case m.manualReason != "":
		return []string{"[r] re-check", "[Ctrl+C] continue without delegating"}
	case m.step == stepDone:
		return []string{"[Enter] continue to phase 2"}
	case m.choosing:
		return []string{"[↑↓] select", "[Enter] delegate", "[m] I'll do it myself", "[Ctrl+C] cancel"}
	case m.step == stepPropagate:
		return []string{"[s] stop waiting (record is saved)", "[Ctrl+C] cancel"}
	default:
		return []string{"[Ctrl+C] cancel"}
	}
}

// runDNSSetupTUI runs the screen and reports whether delegation is verified.
func runDNSSetupTUI(e Env, res dnsPreflightResult) (bool, error) {
	m := newDNSSetupModel(e, res)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return false, err
	}
	if fm, ok := final.(*dnsSetupModel); ok {
		return fm.Delegated, fm.err
	}
	return false, nil
}

// decodeTerraformLine is used when terraform is run with -json; kept separate so
// the streaming path stays testable.
func decodeTerraformLine(line string) (string, bool) {
	var msg struct {
		Message string `json:"@message"`
	}
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return "", false
	}
	return msg.Message, msg.Message != ""
}
