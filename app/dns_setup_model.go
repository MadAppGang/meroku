package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
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
	// cached names the profile that dns.yaml remembered, when it still verified
	// and so made a full scan unnecessary.
	cached string
	// reason is set when we cannot offer automatic delegation at all.
	reason string
	err    error
}

// dnsScanStartedMsg hands the model a live stream of profile probes.
type dnsScanStartedMsg struct {
	ch     <-chan parentZoneCandidate
	cancel context.CancelFunc
	total  int
	// note explains why a full scan is running when a shortcut was expected.
	note string
}

// dnsCandidateMsg is one profile's answer, delivered as soon as it lands.
type dnsCandidateMsg struct{ c parentZoneCandidate }

// dnsScanDoneMsg means every profile has reported, or the scan was cancelled.
type dnsScanDoneMsg struct{}

type dnsDelegatedMsg struct{ err error }

type dnsPropagationMsg struct {
	results map[string]bool
	ok      bool
}

type dnsTickMsg time.Time

// dnsAnimMsg drives the indeterminate loaders.
//
// It is separate from the one-second tick because a sweeping block needs to
// move several times a second to read as motion, while everything else on
// screen — elapsed time, countdowns — only changes once a second and would
// flicker if redrawn eight times as often for no reason.
type dnsAnimMsg time.Time

// ------------------------------------------------------------------- keys ---

type dnsKeyMap struct {
	Up         key.Binding
	Down       key.Binding
	Enter      key.Binding
	Manual     key.Binding
	Copy       key.Binding
	CopyOne    key.Binding
	Retry      key.Binding
	Skip       key.Binding
	SkipDomain key.Binding
	Adopt      key.Binding
	ScanAll    key.Binding
	Continue   key.Binding
	Quit       key.Binding
}

var dnsKeys = dnsKeyMap{
	Up:         key.NewBinding(key.WithKeys("up", "k")),
	Down:       key.NewBinding(key.WithKeys("down", "j")),
	Enter:      key.NewBinding(key.WithKeys("enter")),
	Manual:     key.NewBinding(key.WithKeys("m")),
	Copy:       key.NewBinding(key.WithKeys("c")),
	CopyOne:    key.NewBinding(key.WithKeys("1", "2", "3", "4", "5", "6")),
	Retry:      key.NewBinding(key.WithKeys("r")),
	Skip:       key.NewBinding(key.WithKeys("s")),
	SkipDomain: key.NewBinding(key.WithKeys("s")),
	Adopt:      key.NewBinding(key.WithKeys("t")),
	ScanAll:    key.NewBinding(key.WithKeys("a")),
	// Esc continues without delegating; Ctrl+C aborts. Those are opposite
	// intentions and used to share one binding, so the only way to proceed was
	// the key every other screen uses to cancel.
	Continue: key.NewBinding(key.WithKeys("esc")),
	Quit:     key.NewBinding(key.WithKeys("ctrl+c", "q")),
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
	// cursorPinned records that the operator has moved the cursor themselves.
	// Until then it tracks the best candidate as results stream in.
	cursorPinned bool

	// live profile scan
	scanCh     <-chan parentZoneCandidate
	scanCancel context.CancelFunc
	scanTotal  int
	scanned    int
	scanning   bool
	scanNote   string
	// cachedProfile is the profile dns.yaml remembered, when it verified and no
	// scan was needed.
	cachedProfile string
	// rescanAll forces a full scan even when a cached profile would do.
	rescanAll bool

	// propagate
	resolvers       []string
	resolverResults map[string]bool

	// manual fallback: poll public DNS on a timer, so a record added by hand at a
	// registrar is picked up without the operator having to press anything.
	nextCheckIn int
	checking    bool

	// domain adoption: moving the whole parent domain into Route53, for hosts
	// that cannot delegate a subdomain at all.
	adopt adoptState

	// anim advances the indeterminate loaders, several frames a second.
	anim int

	// transient "copied" confirmation
	copiedNote string
	copiedAt   time.Time

	// outcome
	err          error
	manualReason string
	quitting     bool

	// Delegated reports whether delegation is verified, so the caller knows
	// whether phase 2 can run.
	Delegated bool

	// SkipDomain means the operator chose to turn the custom domain off rather
	// than wait for delegation. The caller disables it in the env YAML and
	// regenerates, so the apply never reaches certificate validation at all.
	SkipDomain bool

	// ContinueAnyway means the operator accepted the risk and wants the apply to
	// run undelegated. It will stall on certificate validation.
	ContinueAnyway bool

	keys dnsKeyMap
}

// dnsSetupOutcome is what the deploy needs to know when the screen closes.
//
// Three distinct answers, and conflating any two of them produces a wrong
// deploy: Delegated proceeds normally, SkipDomain requires rewriting the config
// first, and ContinueAnyway proceeds into an apply that is expected to stall.
type dnsSetupOutcome struct {
	Delegated      bool
	SkipDomain     bool
	ContinueAnyway bool
}

// secondsBetweenDNSChecks is how often the manual screen re-checks public DNS.
//
// Registrar propagation is measured in minutes, so polling faster mostly
// produces a busier screen; 20s is frequent enough that the operator sees the
// change land while they are still watching.
const secondsBetweenDNSChecks = 20

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
		return tea.Batch(m.tick(), m.animTick(), m.createZoneCmd())
	}
	return tea.Batch(m.tick(), m.animTick(), m.findParentCmd())
}

func (m *dnsSetupModel) tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return dnsTickMsg(t) })
}

// animInterval is the frame time for indeterminate loaders. Fast enough that the
// block glides, slow enough that a remote terminal is not repainting constantly.
const animInterval = 120 * time.Millisecond

func (m *dnsSetupModel) animTick() tea.Cmd {
	return tea.Tick(animInterval, func(t time.Time) tea.Msg { return dnsAnimMsg(t) })
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

// findParentCmd resolves the parent domain, then either uses the profile
// dns.yaml remembered or starts a live scan of every local AWS profile.
func (m *dnsSetupModel) findParentCmd() tea.Cmd {
	// Copy what the closure needs: it runs on its own goroutine while Update
	// keeps mutating the model.
	parent := m.parent
	rescanAll := m.rescanAll

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

		ctx, cancel := context.WithCancel(context.Background())

		// Try the remembered profile before scanning: one API call instead of
		// one per profile. The memory is only a hint about where to look first —
		// the probe still has to pass the same public-DNS check as any other
		// candidate, so a stale entry costs a wasted call and nothing more.
		note := ""
		if !rescanAll {
			if profile, ok := cachedParentProfile(parent); ok {
				c := probeProfileForParentZone(ctx, profile, parent, publicNS)
				if c.Authoritative {
					cancel()
					return dnsCandidatesMsg{
						candidates: []parentZoneCandidate{c},
						cached:     profile,
					}
				}
				note = fmt.Sprintf(
					"%s was remembered for %s but no longer matches public DNS — scanning every profile",
					profile, parent)
			}
		}

		return dnsScanStartedMsg{
			ch:     scanProfilesForParentZoneStream(ctx, profiles, parent, publicNS),
			cancel: cancel,
			total:  len(profiles),
			note:   note,
		}
	}
}

// waitForCandidate blocks on the next profile result.
//
// This is the standard Bubble Tea bridge from a channel into the message loop:
// one command per receive, each scheduling the next. It keeps the model
// single-threaded while the probes run concurrently behind it.
func waitForCandidate(ch <-chan parentZoneCandidate) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return dnsScanDoneMsg{}
		}
		c, ok := <-ch
		if !ok {
			return dnsScanDoneMsg{}
		}
		return dnsCandidateMsg{c: c}
	}
}

// copyToClipboard puts text on the system clipboard and shows a brief note.
//
// A failure here is not worth interrupting the flow for — the nameservers are
// still on screen to be selected by hand — so it is reported in the same place
// the success would have been.
func (m *dnsSetupModel) copyToClipboard(text, note string) {
	if err := clipboard.WriteAll(text); err != nil {
		m.copiedNote = "could not reach the clipboard — select the text instead"
	} else {
		m.copiedNote = note
	}
	m.copiedAt = time.Now()
}

// stopScan abandons any in-flight probes.
//
// Called whenever the operator commits to a choice or leaves: without it the
// goroutines would keep running against a channel nobody drains.
func (m *dnsSetupModel) stopScan() {
	m.scanning = false
	if m.scanCancel != nil {
		m.scanCancel()
		m.scanCancel = nil
	}
}

// addCandidate inserts a streamed result and keeps the selection sensible.
func (m *dnsSetupModel) addCandidate(c parentZoneCandidate) {
	// The list is re-sorted on every arrival, so a cursor held as an index would
	// slide onto a different profile under the operator's finger. Track the
	// selection by identity instead.
	selected := ""
	if m.cursorPinned && m.cursor < len(m.candidates) {
		selected = m.candidates[m.cursor].Profile
	}

	m.candidates = append(m.candidates, c)
	sortCandidates(m.candidates)

	m.states[stepFindParent] = stepOK
	m.step = stepWriteRecord
	m.choosing = true

	if selected != "" {
		for i, x := range m.candidates {
			if x.Profile == selected {
				m.cursor = i
				return
			}
		}
	}

	// Until the operator moves it, the cursor follows the best candidate.
	m.cursor = 0
	for i, x := range m.candidates {
		if x.Authoritative {
			m.cursor = i
			break
		}
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
	record := delegationRecord{
		Subdomain:       m.zone,
		AccountID:       m.env.AccountID,
		ZoneID:          m.zoneID,
		Nameservers:     m.nameservers,
		ParentDomain:    m.parent,
		ParentProfile:   c.Profile,
		ParentZoneID:    c.ZoneID,
		ParentAccountID: c.AccountID,
	}
	return func() tea.Msg {
		if err := applyDelegation(req); err != nil {
			return dnsDelegatedMsg{err: err}
		}
		// Persistence is a convenience; a failure here does not undo the write.
		// It is what lets the next environment skip the profile scan.
		_ = recordDelegation(record)
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
		// Two independent resolvers agreeing is enough to proceed. That is half,
		// not a majority: resolvers cache negative answers for minutes after a
		// zone is created, so waiting for all four routinely stalls long after
		// the record is genuinely live at the authoritative servers — which is
		// all ACM actually needs. The count is reported rather than rounded up
		// to "fully propagated", because at this point it usually is not.
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
	// The adoption sub-flow owns its own messages and, while on screen, its own
	// keys. It returns handled=false for anything it does not claim so the main
	// switch below still sees ticks and window resizes.
	if cmd, handled := m.updateAdopt(msg); handled {
		return m, cmd
	} else if cmd != nil {
		return m, tea.Batch(cmd, m.tick())
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case dnsAnimMsg:
		if m.quitting {
			return m, nil
		}
		m.anim++
		return m, m.animTick()

	case dnsTickMsg:
		m.elapsed = time.Since(m.startTime)
		if m.quitting {
			return m, nil
		}

		// Clear the copy confirmation after a moment.
		if m.copiedNote != "" && time.Since(m.copiedAt) > 2*time.Second {
			m.copiedNote = ""
		}

		// While waiting on a human to add the record somewhere else, count down
		// and re-check by ourselves. Requiring a keypress to discover that the
		// record already landed is the difference between a screen you can leave
		// running and one you have to babysit.
		//
		// Not while the adoption flow is up: it runs its own countdown, and two
		// pollers on the same screen produced results the other did not expect —
		// the manual one completing the whole flow underneath a sub-screen that
		// then kept rendering as if nothing had happened.
		if m.manualReason != "" && m.adopt.phase == adoptOff && !m.checking {
			if m.nextCheckIn > 0 {
				m.nextCheckIn--
			}
			if m.nextCheckIn == 0 {
				m.checking = true
				return m, tea.Batch(m.tick(), m.pollPropagationCmd())
			}
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
			m.stopScan()
			m.manualReason = msg.reason
			if msg.err != nil {
				m.manualReason = msg.err.Error()
			}
			m.states[stepFindParent] = stepFailed
			m.states[stepWriteRecord] = stepSkipped
			m.nextCheckIn = secondsBetweenDNSChecks
			return m, nil
		}
		m.candidates = msg.candidates
		m.cachedProfile = msg.cached
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

	case dnsScanStartedMsg:
		m.scanCh = msg.ch
		m.scanCancel = msg.cancel
		m.scanTotal = msg.total
		m.scanned = 0
		m.scanning = true
		m.scanNote = msg.note
		return m, waitForCandidate(msg.ch)

	case dnsCandidateMsg:
		// Ignore anything that arrives after the operator has committed; the
		// picker must not reopen underneath a write that is already running.
		if !m.scanning {
			return m, nil
		}
		m.scanned++
		// Profiles with neither a zone nor an error are silent misses — listing
		// them would bury the two rows that matter under eight that do not.
		if msg.c.ZoneID != "" || msg.c.Err != nil {
			m.addCandidate(msg.c)
		}
		return m, waitForCandidate(m.scanCh)

	case dnsScanDoneMsg:
		m.scanning = false
		m.scanCancel = nil
		if m.step == stepFindParent && len(m.candidates) == 0 {
			m.manualReason = fmt.Sprintf("no local AWS profile holds a %s zone", m.parent)
			m.states[stepFindParent] = stepFailed
			m.states[stepWriteRecord] = stepSkipped
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
			// However the record got there — written by us, added by hand at a
			// registrar, or arriving while the adoption flow was still waiting on
			// the apex — the delegation is live and the screen's job is done. Any
			// sub-flow still on screen is now describing work that has completed.
			m.checking = false
			m.manualReason = ""
			m.adopt.phase = adoptOff
			m.adopt.checking = false
			m.states[stepFindParent] = stepOK
			m.states[stepWriteRecord] = stepOK
			m.states[stepPropagate] = stepOK
			m.step = stepDone
			m.Delegated = true
			return m, nil
		}

		// Manual mode owns its own cadence via the countdown, so it must not also
		// schedule a poll here or the two would compound.
		if m.manualReason != "" {
			m.checking = false
			m.nextCheckIn = secondsBetweenDNSChecks
			return m, nil
		}

		// Keep polling until the operator stops us.
		return m, tea.Tick(10*time.Second, func(time.Time) tea.Msg {
			return dnsTickPollMsg{}
		})

	case dnsTickPollMsg:
		return m, m.pollPropagationCmd()

	case tea.KeyMsg:
		if cmd, handled := m.handleAdoptKey(msg); handled {
			return m, cmd
		}
		return m.handleKey(msg)
	}
	return m, nil
}

// dnsTickPollMsg schedules the next propagation check.
type dnsTickPollMsg struct{}

func (m *dnsSetupModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.stopScan()
		m.quitting = true
		return m, tea.Quit

	case key.Matches(msg, m.keys.Copy):
		// Only meaningful where the nameservers are on screen to be copied.
		if len(m.nameservers) > 0 && (m.manualReason != "" || m.step == stepFindParent) {
			m.copyToClipboard(strings.Join(m.nameservers, "\n"),
				fmt.Sprintf("copied all %d nameservers", len(m.nameservers)))
		}
		return m, nil

	case key.Matches(msg, m.keys.CopyOne):
		// Registrar forms usually take one nameserver per field, so copying them
		// individually is the common case, not the exception.
		if len(m.nameservers) > 0 && (m.manualReason != "" || m.step == stepFindParent) {
			if n, err := strconv.Atoi(msg.String()); err == nil && n >= 1 && n <= len(m.nameservers) {
				ns := m.nameservers[n-1]
				m.copyToClipboard(ns, "copied "+ns)
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.Adopt) && m.manualReason != "":
		// The escape hatch for DNS hosts that cannot delegate a subdomain at all:
		// move the whole domain to Route53, where the delegation is ours to write.
		m.stopScan()
		m.beginAdoption()
		return m, nil

	case key.Matches(msg, m.keys.SkipDomain) && m.manualReason != "":
		// Turn the custom domain off rather than deploy into a certificate that
		// cannot validate. The caller rewrites the env YAML and regenerates.
		m.SkipDomain = true
		m.quitting = true
		m.stopScan()
		return m, tea.Quit

	case key.Matches(msg, m.keys.Continue) && m.manualReason != "":
		m.ContinueAnyway = true
		m.quitting = true
		m.stopScan()
		return m, tea.Quit

	case key.Matches(msg, m.keys.Up):
		if m.choosing && m.cursor > 0 {
			m.cursor--
			m.cursorPinned = true
		}
		return m, nil

	case key.Matches(msg, m.keys.Down):
		if m.choosing && m.cursor < len(m.candidates)-1 {
			m.cursor++
			m.cursorPinned = true
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
			m.stopScan()
			m.choosing = false
			return m, m.delegateCmd(c)
		}
		return m, nil

	case key.Matches(msg, m.keys.Manual):
		if m.choosing {
			m.stopScan()
			m.choosing = false
			m.manualReason = "you chose to add the record yourself"
			m.states[stepWriteRecord] = stepSkipped
		}
		return m, nil

	case key.Matches(msg, m.keys.ScanAll):
		// Escape hatch from the remembered profile: the operator may know the
		// domain moved, or simply want to see what else is out there.
		if m.choosing && m.cachedProfile != "" {
			m.stopScan()
			m.rescanAll = true
			m.cachedProfile = ""
			m.candidates = nil
			m.cursor = 0
			m.cursorPinned = false
			m.choosing = false
			m.step = stepFindParent
			m.states[stepFindParent] = stepPending
			return m, m.findParentCmd()
		}
		return m, nil

	case key.Matches(msg, m.keys.Retry):
		// Check public DNS now rather than waiting out the countdown. This polls
		// resolvers rather than rescanning profiles: in the manual case the record
		// is being added somewhere meroku cannot see, so the only useful question
		// is whether it has appeared on the internet yet.
		if m.manualReason != "" && !m.checking {
			m.checking = true
			m.nextCheckIn = secondsBetweenDNSChecks
			return m, m.pollPropagationCmd()
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

	// Which account this will change stays on screen for the whole flow. It is
	// the fact that makes a wrong run wrong, and a confirmation that has already
	// scrolled past protects nobody.
	context := m.env.AWSProfile
	if context != "" && m.env.Region != "" {
		context = context + " · " + m.env.Region
	}
	clock := fmt.Sprintf("%02d:%02d", int(m.elapsed.Minutes()), int(m.elapsed.Seconds())%60)

	var b strings.Builder
	b.WriteString(renderAppHeader("DNS", m.zone, context, clock, w) + "\n\n")
	b.WriteString(renderStepRail(m.step, m.states, w) + "\n\n")
	b.WriteString(m.renderBody(w) + "\n\n")
	b.WriteString(renderKeyLegend(m.footerHints(), w))

	return lipgloss.Place(w, m.height, lipgloss.Left, lipgloss.Top, b.String())
}

// renderBody dispatches to the screen for the current step. It takes the full
// terminal width; panels that want a single box derive their own inner width.
func (m *dnsSetupModel) renderBody(width int) string {
	inner := width - 4

	// Terminal states win over any sub-flow. Rendering the adoption screen ahead
	// of them once left a completed run showing "rechecking in 2s" under a rail
	// of green ticks, because the flow had finished by a route the sub-screen did
	// not know about.
	if m.step != stepDone && m.err == nil && m.adopt.phase != adoptOff {
		return m.renderAdopt(width)
	}
	switch {
	case m.err != nil && m.manualReason == "":
		return boxStyle.Width(inner).Render(
			badge("FAILED", dangerColor) + "  " +
				lipgloss.NewStyle().Foreground(fgColor).Render("DNS setup could not continue") + "\n\n" +
				lipgloss.NewStyle().Foreground(dimColor).Render(wordWrap(m.err.Error(), inner-4)))

	case m.manualReason != "":
		return m.renderManual(width)

	case m.step == stepCreateZone:
		body := titleStyle.Render("Creating hosted zone") + "\n" +
			lipgloss.NewStyle().Foreground(dimColor).
				Render("the zone must exist before its nameservers can be delegated") + "\n\n" +
			indeterminateRow(inner-4, m.anim,
				fmt.Sprintf("creating (%ds)", int(m.elapsed.Seconds())))
		return boxStyle.Width(inner).Render(body)

	case m.step == stepFindParent:
		body := titleStyle.Render("Looking for the "+m.parent+" zone") + "\n" +
			lipgloss.NewStyle().Foreground(dimColor).
				Render("scanning local AWS profiles — a match is proved against public DNS") + "\n\n" +
			m.scanMeter(inner-4)
		if m.scanNote != "" {
			body += "\n" + lipgloss.NewStyle().Foreground(warningColor).
				Render(truncateToWidth(m.scanNote, inner-6))
		}
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
		// The list is still filling in — show how much is left rather than
		// letting a short list look like the final answer.
		if m.scanning {
			rows.WriteString("\n" + m.scanMeter(inner-4) + "\n")
		}
		// Explain why only one profile is listed. The key hint itself lives in the
		// footer with every other binding, so it is not repeated here.
		if m.cachedProfile != "" {
			rows.WriteString("\n" + lipgloss.NewStyle().Foreground(mutedColor).Render(
				truncateToWidth(fmt.Sprintf(
					"%s remembered this profile for %s; re-verified against public DNS just now",
					DNSConfigFile, m.parent), inner-6)) + "\n")
		}
		return boxStyle.Width(inner).Render(strings.TrimRight(rows.String(), "\n"))

	case m.step == stepWriteRecord:
		return boxStyle.Width(inner).Render(
			titleStyle.Render("Writing delegation record") + "\n\n" +
				indeterminateRow(inner-4, m.anim, "writing the record"))

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
		// Say how many resolvers actually confirmed. We continue at two of four,
		// so an unqualified "resolves" would overstate what was observed —
		// typically two of them are still serving a cached negative answer.
		matched := 0
		for _, ok := range m.resolverResults {
			if ok {
				matched++
			}
		}
		detail := "Certificate validation can now succeed. Continuing with the full deploy."
		if matched < len(m.resolvers) {
			detail = fmt.Sprintf(
				"%d of %d resolvers see it so far; the rest are still serving cached answers.\n"+
					"That is enough for certificate validation. Continuing with the full deploy.",
				matched, len(m.resolvers))
		}
		return boxStyle.Width(inner).Render(
			badge("DELEGATED", successColor) + "  " +
				lipgloss.NewStyle().Foreground(fgColor).
					Render(fmt.Sprintf("%s now resolves to this account", m.zone)) + "\n\n" +
				lipgloss.NewStyle().Foreground(dimColor).Render(detail) + "\n\n" +
				renderResolverGrid(m.resolverResults, m.resolvers))
	}
	return ""
}

// renderRecheckLine shows the auto re-check countdown, or the copy confirmation
// when one is pending. The copy note takes the slot because it is transient and
// the operator has just asked for it.
func (m *dnsSetupModel) renderRecheckLine(width int) string {
	if m.copiedNote != "" {
		return lipgloss.NewStyle().Foreground(successColor).
			Render("  ✓ " + truncateToWidth(m.copiedNote, width-4))
	}

	if m.checking {
		return lipgloss.NewStyle().Foreground(accentColor).
			Render("  ⟳ checking public DNS…")
	}

	return lipgloss.NewStyle().Foreground(mutedColor).
		Render(fmt.Sprintf("  next check in %ds — it will continue on its own once the record resolves",
			m.nextCheckIn))
}

// scanMeter renders scan progress as a real fraction once the total is known.
//
// Streaming turns this from an indeterminate pulse into an honest ratio: we know
// exactly how many profiles there are and how many have answered, so the meter
// can say so instead of miming activity.
func (m *dnsSetupModel) scanMeter(width int) string {
	if m.scanTotal == 0 {
		return indeterminateRow(width, m.anim, "resolving")
	}
	ratio := float64(m.scanned) / float64(m.scanTotal)
	return meterRow(width, ratio, "#3b82f6", "#10b981",
		fmt.Sprintf("%d/%d profiles", m.scanned, m.scanTotal))
}

func (m *dnsSetupModel) footerHints() []keyHint {
	if m.adopt.phase != adoptOff {
		return m.adoptFooterHints()
	}

	switch {
	case m.manualReason != "":
		hints := []keyHint{}
		if len(m.nameservers) > 0 {
			hints = append(hints,
				keyHint{"c", "copy all"},
				keyHint{fmt.Sprintf("1-%d", len(m.nameservers)), "copy one"})
		}
		return append(hints,
			keyHint{"r", "check now"},
			keyHint{"t", "move to route53"},
			keyHint{"s", "deploy without domain"},
			keyHint{"esc", "continue anyway"},
			keyHint{"^C", "abort deploy"})

	case m.step == stepDone:
		return []keyHint{{"enter", "continue to phase 2"}}

	case m.choosing && m.cachedProfile != "":
		return []keyHint{
			{"enter", "delegate"}, {"a", "scan all profiles"},
			{"m", "do it myself"}, {"^C", "abort deploy"}}

	case m.choosing:
		return []keyHint{
			{"↑↓", "select"}, {"enter", "delegate"},
			{"m", "do it myself"}, {"^C", "abort deploy"}}

	case m.step == stepPropagate:
		return []keyHint{{"s", "stop waiting"}, {"^C", "abort deploy"}}

	default:
		return []keyHint{{"^C", "abort deploy"}}
	}
}

// runDNSSetupTUI runs the screen and reports what the operator decided.
func runDNSSetupTUI(e Env, res dnsPreflightResult) (dnsSetupOutcome, error) {
	m := newDNSSetupModel(e, res)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return dnsSetupOutcome{}, err
	}
	if fm, ok := final.(*dnsSetupModel); ok {
		return dnsSetupOutcome{
			Delegated:      fm.Delegated,
			SkipDomain:     fm.SkipDomain,
			ContinueAnyway: fm.ContinueAnyway,
		}, fm.err
	}
	return dnsSetupOutcome{}, nil
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

// renderManual is the fallback screen: the record on the left, why we cannot
// write it and what to do instead on the right.
//
// It replaced three paragraphs of prose. The options were always there — they
// were just spelled out in sentences the operator had to read through to find
// the one key they needed. As a menu they are scannable, and the consequence of
// each sits on its own line rather than buried mid-paragraph.
func (m *dnsSetupModel) renderManual(width int) string {
	half, sideBySide := columnWidth(width)

	// --- the record, with its copy keys on the rows themselves ---------------
	//
	// The number in front of each nameserver is the key that copies it. Putting
	// the affordance on the thing it acts on means there is nothing to look up:
	// the legend at the bottom tells you the keys exist, but here you can see
	// which row each one belongs to.
	var rec strings.Builder
	rec.WriteString(kvRow("NAME", m.zone, 7) + "\n")
	rec.WriteString(kvRow("TYPE", "NS", 7) +
		lipgloss.NewStyle().Foreground(mutedColor).Render("    TTL  ") +
		lipgloss.NewStyle().Foreground(fgColor).Render("300") + "\n\n")

	val := lipgloss.NewStyle().Foreground(lipgloss.Color("#60a5fa")).Bold(true)
	for i, ns := range m.nameservers {
		rec.WriteString(keycap(fmt.Sprintf("%d", i+1)) + " " + val.Render(ns) + "\n")
	}
	if len(m.nameservers) > 1 {
		rec.WriteString(keycap("c") + " " +
			lipgloss.NewStyle().Foreground(mutedColor).
				Render(fmt.Sprintf("copy all %d", len(m.nameservers))) + "\n")
	}

	rec.WriteString("\n")
	switch {
	case m.copiedNote != "":
		rec.WriteString(lipgloss.NewStyle().Foreground(successColor).
			Render("✓ " + truncateToWidth(m.copiedNote, half-8)))
	case m.checking:
		rec.WriteString(lipgloss.NewStyle().Foreground(accentColor).Render("⟳ checking…"))
	default:
		rec.WriteString(countdownRow(m.nextCheckIn, secondsBetweenDNSChecks, "rechecking in", half-8))
	}

	recordPanel := panel("delegation record · add to "+m.parent, rec.String(), half, accentColor)

	// --- what is happening, in the reader's terms ---------------------------
	//
	// The previous version said "ACM cert stalls 20m, then fails", which is the
	// mechanism as seen from inside the deploy. It assumes the reader knows what
	// ACM is, that a certificate needs validating, and why a DNS record is
	// involved at all. Start from what they asked for — a custom domain — and
	// walk to the consequence.
	var why strings.Builder
	why.WriteString(lipgloss.NewStyle().Foreground(fgColor).Render(wordWrap(
		"You asked for the custom domain "+m.zone+", so the deploy has to get an "+
			"HTTPS certificate for it. AWS only issues one after checking a DNS "+
			"record that proves you control the name — and meroku can only add that "+
			"record once "+m.parent+" answers from AWS.", half-6)) + "\n\n")

	why.WriteString(lipgloss.NewStyle().Foreground(mutedColor).Render(wordWrap(
		m.manualReason, half-6)) + "\n\n")

	why.WriteString(lipgloss.NewStyle().Foreground(warningColor).Render(wordWrap(
		"⚠ Until that is fixed the deploy stops at the certificate and finishes "+
			"nothing — no services, no database, none of it.", half-6)))

	// --- the ways out --------------------------------------------------------
	acts := renderActions([]action{
		{key: "r", title: "CHECK AGAIN NOW", tone: fgColor,
			detail: "If you have just added the record, do not wait for the countdown."},
		{key: "1-4", title: "ADD THE RECORD YOURSELF", tone: successColor,
			detail: "Copy it into " + m.parent + " wherever that domain is managed. " +
				"This screen keeps checking and carries on by itself once it resolves."},
		{key: "t", title: "MOVE " + strings.ToUpper(m.parent) + " TO ROUTE53", tone: accentColor,
			detail: "For providers that cannot add an NS record for a subdomain at all. " +
				"meroku copies your existing records over first, so mail and the website keep working."},
		{key: "s", title: "DEPLOY WITHOUT THE CUSTOM DOMAIN", tone: accentColor,
			detail: "Everything else goes up now. Turn the domain back on and deploy again whenever you like."},
		{key: "esc", title: "DEPLOY ANYWAY", tone: warningColor,
			detail: "It will stop at the certificate about 20 minutes in, having built nothing."},
		{key: "^C", title: "ABORT", tone: dangerColor,
			detail: "Stop here and change nothing."},
	}, half-6)

	// The record and the explanation together balance the menu, which is always
	// the longest panel. Stacked, the order is the one you read in: what to add,
	// why, then the ways out.
	left := recordPanel + "\n" + panel("what is happening", why.String(), half, warningColor)
	right := panel("what now", acts, half, successColor)

	if sideBySide {
		return joinColumns(left, right)
	}
	return left + "\n" + right
}
