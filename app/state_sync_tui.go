package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The sync screen: the work, as it happens, and what a deploy would then change.
//
// It starts only once the user has already said yes. The question itself is an
// ordinary inline prompt in performSync — taking over the terminal to ask a
// yes/no question is a heavy answer to a small question, and it arrives right
// after the environment picker, so a full-screen switch reads as the app having
// jumped somewhere unasked. A screen is the right shape for two minutes of
// terraform; it is the wrong shape for one question.
//
// It is built from the chrome in dns_chrome.go (header, panels, badges, key
// legend, the bouncing indeterminate track) rather than a second visual
// language. Two screens that do the same kind of work should look like the same
// program.
//
// Two rules this screen is designed around:
//
//   - No fake percentage. terraform init and terraform plan have no total to
//     divide by, so they get the bouncing marquee, never a filling bar. A bar
//     that fills on a guess teaches the operator to distrust the ones that mean
//     something.
//   - Destruction is the only outcome that can cost the user anything, so when
//     the plan contains any, it is the largest thing on the result screen.

// ---------------------------------------------------------------- the model ---

type syncScreenPhase int

const (
	// syncScreenWorking: a step is running.
	syncScreenWorking syncScreenPhase = iota
	// syncScreenFinished: every step has finished or failed.
	syncScreenFinished
)

// syncStepID identifies one unit of work. Which of them run is decided from the
// connection: a directory that is already on disk is never rewritten, and an
// already-linked environment only needs the comparison.
type syncStepID int

const (
	syncStepWrite syncStepID = iota
	syncStepInit
	syncStepCompare
)

var syncStepTitles = map[syncStepID]string{
	syncStepWrite:   "write env/",
	syncStepInit:    "terraform init",
	syncStepCompare: "terraform plan",
}

type syncScreenModel struct {
	req syncRequest

	phase  syncScreenPhase
	steps  []syncStepID
	states map[syncStepID]stepState
	notes  map[syncStepID]string
	active int // index into steps; -1 when nothing is running

	outcome reconnectOutcome
	err     error // a step that failed and stopped the sync
	plan    planSummary
	planErr error
	// stepOutput is what a failing step printed. terraform's own words about why
	// init failed are the most useful thing on the screen at that point.
	stepOutput string
	// cancelled records ctrl+c during the work. Without it the zero-valued plan
	// summary would be reported as "no changes", which is a lie about
	// infrastructure nobody ever compared.
	cancelled bool

	width   int
	height  int
	anim    int
	start   time.Time
	elapsed time.Duration
}

func newSyncScreenModel(req syncRequest) *syncScreenModel {
	m := &syncScreenModel{
		req:    req,
		states: map[syncStepID]stepState{},
		notes:  map[syncStepID]string{},
		active: -1,
		width:  100,
		height: 30,
		start:  time.Now(),
	}
	if !req.conn.Generated {
		m.steps = append(m.steps, syncStepWrite)
	}
	if req.conn.Status == stateDeployedButDisconnected {
		m.steps = append(m.steps, syncStepInit)
	}
	m.steps = append(m.steps, syncStepCompare)
	return m
}

// --------------------------------------------------------------- messages ---

type syncStepDoneMsg struct {
	step syncStepID
	err  error
	// output is whatever the step printed. It is carried in the message rather
	// than left to reach the terminal: see runStepQuietly.
	output string
}

type syncPlanDoneMsg struct {
	summary planSummary
	err     error
	output  string
}

// runStepQuietly runs a step with os.Stdout pointed at a pipe, and returns what
// it printed.
//
// The steps are ordinary meroku functions that report by printing —
// generateEnvironmentFiles prints a line per file it writes, terraformInitInDir
// prints terraform's output when init fails. Off the screen that is exactly
// right, and it must keep working that way, so the printing stays where it is
// and this redirects it for the duration of the call instead.
//
// Bubble Tea captured the real stdout when the program was constructed, so the
// renderer keeps drawing to the terminal while fmt.Print inside the step — which
// resolves os.Stdout at call time — goes to the pipe. Steps run strictly one at
// a time, so there is no window where a swapped stdout could catch anything
// else.
func runStepQuietly(fn func() error) (string, error) {
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		// Capturing is best-effort. A step that runs and prints is better than a
		// step that does not run.
		return "", fn()
	}

	original := os.Stdout
	os.Stdout = w

	// Drained concurrently: a step that printed more than the pipe buffer holds
	// would otherwise block forever on its own write.
	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()

	err := fn()

	os.Stdout = original
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out, err
}

type syncAnimMsg time.Time

func syncAnimCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg { return syncAnimMsg(t) })
}

func (m *syncScreenModel) Init() tea.Cmd {
	return tea.Batch(syncAnimCmd(), m.startNextStep())
}

// startNextStep advances to the next pending step and returns the command that
// performs it. It returns tea.Quit when the list is exhausted.
func (m *syncScreenModel) startNextStep() tea.Cmd {
	m.active++
	if m.active >= len(m.steps) {
		m.phase = syncScreenFinished
		m.active = -1
		return nil
	}

	step := m.steps[m.active]
	m.states[step] = stepActive

	switch step {
	case syncStepWrite:
		env := m.req.conn.Env
		gen := m.req.gen
		return func() tea.Msg {
			out, err := runStepQuietly(func() error { return gen(env) })
			return syncStepDoneMsg{step: syncStepWrite, err: err, output: out}
		}
	case syncStepInit:
		dir := m.req.envDir
		init := m.req.init
		return func() tea.Msg {
			out, err := runStepQuietly(func() error { return init(dir) })
			return syncStepDoneMsg{step: syncStepInit, err: err, output: out}
		}
	default:
		dir := m.req.envDir
		plan := m.req.plan
		return func() tea.Msg {
			var s planSummary
			var planErr error
			out, _ := runStepQuietly(func() error {
				s, planErr = plan(dir)
				return nil
			})
			return syncPlanDoneMsg{summary: s, err: planErr, output: out}
		}
	}
}

func (m *syncScreenModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case syncAnimMsg:
		m.anim++
		m.elapsed = time.Since(m.start)
		return m, syncAnimCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)

	case syncStepDoneMsg:
		if msg.err != nil {
			m.states[msg.step] = stepFailed
			m.err = msg.err
			m.stepOutput = msg.output
			m.phase = syncScreenFinished
			m.active = -1
			m.outcome.Verdict = syncStepFailureVerdict(msg.step, m.req.conn.Env, msg.err)
			return m, nil
		}
		m.states[msg.step] = stepOK
		switch msg.step {
		case syncStepWrite:
			m.outcome.Generated = true
			m.notes[msg.step] = fmt.Sprintf("wrote env/%s/ from %s.yaml", m.req.conn.Env, m.req.conn.Env)
		case syncStepInit:
			m.outcome.Initialised = true
			m.notes[msg.step] = fmt.Sprintf("linked to s3://%s", m.req.conn.Summary.Bucket)
		}
		return m, m.startNextStep()

	case syncPlanDoneMsg:
		m.plan, m.planErr = msg.summary, msg.err
		if msg.err != nil {
			m.stepOutput = msg.output
		}
		if msg.err != nil {
			m.states[syncStepCompare] = stepSkipped
			m.notes[syncStepCompare] = shortErrorNote(msg.err)
		} else {
			m.states[syncStepCompare] = stepOK
			m.notes[syncStepCompare] = planCountsNote(msg.summary)
		}
		m.outcome.Verdict = syncVerdictLead(m.outcome.Initialised) + driftVerdict(msg.summary, msg.err)
		m.phase = syncScreenFinished
		m.active = -1
		return m, nil
	}

	return m, nil
}

func (m *syncScreenModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		if m.phase == syncScreenWorking {
			m.cancelled = true
			m.outcome.Verdict = fmt.Sprintf("Cancelled part way through. Run `meroku sync %s` to finish.", m.req.conn.Env)
		}
		return m, tea.Quit
	}

	if m.phase == syncScreenFinished {
		switch key {
		case "enter", "q", "esc", " ":
			return m, tea.Quit
		}
	}
	return m, nil
}

// ------------------------------------------------------------------- view ---

func (m *syncScreenModel) View() string {
	w := m.width
	if w < 60 {
		w = 60
	}

	context := m.req.conn.AWSProfile
	if m.req.conn.AccountID != "" {
		if context != "" {
			context += " · "
		}
		context += m.req.conn.AccountID
	}
	clock := fmt.Sprintf("%02d:%02d", int(m.elapsed.Minutes()), int(m.elapsed.Seconds())%60)

	var b strings.Builder
	b.WriteString(renderAppHeader("SYNC", m.req.conn.Env, context, clock, w) + "\n\n")
	b.WriteString(m.renderFactsRow(w) + "\n\n")

	b.WriteString(m.renderSteps(w) + "\n\n")
	if m.phase == syncScreenFinished {
		b.WriteString(m.renderResult(w) + "\n\n")
	}

	b.WriteString(renderKeyLegend(m.legend(), w))
	return lipgloss.Place(w, m.height, lipgloss.Left, lipgloss.Top, b.String())
}

// renderFactsRow is the headline: deployed, how much, where. It stays on screen
// for the whole flow, so the number the user is deciding about never scrolls
// away.
func (m *syncScreenModel) renderFactsRow(width int) string {
	s := m.req.conn.Summary
	chips := []string{
		badge("DEPLOYED", successColor),
		statChip("resources", fmt.Sprintf("%d", s.ResourceCount), fgColor),
	}
	if s.Region != "" {
		chips = append(chips, statChip("region", s.Region, accentColor))
	}
	if s.Bucket != "" {
		chips = append(chips, statChip("state", "s3://"+s.Bucket, mutedColor))
	}

	row := strings.Join(chips, lipgloss.NewStyle().Foreground(borderColor).Render("   "))
	if lipgloss.Width(row) > width {
		// Drop the bucket first: it is the least useful of the four when the
		// terminal is too narrow to hold them all.
		row = strings.Join(chips[:len(chips)-1],
			lipgloss.NewStyle().Foreground(borderColor).Render("   "))
	}
	return " " + row
}

// renderSteps lists the work with a badge each, and gives the running one the
// bouncing track — the honest shape for something with no total.
func (m *syncScreenModel) renderSteps(width int) string {
	inner := width - 6
	if inner < 20 {
		inner = 20
	}

	var b strings.Builder
	for i, step := range m.steps {
		state := m.states[step]
		var chip string
		switch state {
		case stepOK:
			chip = badgeFixed("DONE", successColor, 8)
		case stepActive:
			chip = badgeFixed("RUNNING", accentColor, 8)
		case stepFailed:
			chip = badgeFixed("FAILED", dangerColor, 8)
		case stepSkipped:
			chip = badgeFixed("SKIPPED", warningColor, 8)
		default:
			chip = lipgloss.NewStyle().Foreground(mutedColor).Width(8).Align(lipgloss.Center).Render("waiting")
		}

		title := syncStepTitles[step]
		if step == syncStepWrite {
			title = "write env/" + m.req.conn.Env
		}
		titleTone := fgColor
		if state == stepPending {
			titleTone = mutedColor
		}

		line := chip + " " + lipgloss.NewStyle().Foreground(titleTone).Render(title)
		if note := m.notes[step]; note != "" {
			line += lipgloss.NewStyle().Foreground(mutedColor).
				Render("  " + truncateToWidth(note, inner-lipgloss.Width(line)-2))
		}
		b.WriteString(line + "\n")

		if state == stepActive {
			b.WriteString(indeterminateRow(inner, m.anim,
				fmt.Sprintf("%s (%ds)", syncStepTitles[step], int(m.elapsed.Seconds()))) + "\n")
			if step == syncStepCompare {
				// A minute of no output is where people start pressing ctrl+c.
				// Saying so costs one dim line.
				b.WriteString(lipgloss.NewStyle().Foreground(mutedColor).
					Render("         this takes a minute or two on a large environment") + "\n")
			}
		}
		if i < len(m.steps)-1 {
			b.WriteString("\n")
		}
	}

	accent := accentColor
	if m.phase == syncScreenFinished {
		accent = borderColor
	}
	return panel("steps", strings.TrimRight(b.String(), "\n"), width, accent)
}

// renderResult is the payload of the whole flow: what a terraform apply would
// now do to the deployed infrastructure.
func (m *syncScreenModel) renderResult(width int) string {
	c := m.req.conn
	inner := width - 6
	if inner < 20 {
		inner = 20
	}

	if m.err != nil {
		body := badge("FAILED", dangerColor) + " " +
			lipgloss.NewStyle().Foreground(fgColor).Render("your infrastructure and its state are untouched") +
			"\n\n" + lipgloss.NewStyle().Foreground(dimColor).Render(wordWrap(m.err.Error(), inner)) +
			m.renderStepOutput(inner)
		return panel("sync failed", body, width, dangerColor)
	}

	if m.planErr != nil {
		title := "comparison skipped"
		lead := badge("SKIPPED", warningColor) + " " +
			lipgloss.NewStyle().Foreground(fgColor).Render("the sync finished; the comparison did not run")
		body := lead + "\n\n" +
			lipgloss.NewStyle().Foreground(dimColor).Render(wordWrap(m.planErr.Error(), inner)) +
			m.renderStepOutput(inner) + "\n\n" +
			lipgloss.NewStyle().Foreground(mutedColor).Render(
				fmt.Sprintf("retry with: terraform -chdir=%s plan", m.req.envDir))
		return panel(title, body, width, warningColor)
	}

	accent := successColor
	var lead string
	switch {
	case m.plan.Destroy > 0:
		accent = dangerColor
		lead = badge(fmt.Sprintf("WOULD DESTROY %d", m.plan.Destroy), dangerColor) + " " +
			lipgloss.NewStyle().Foreground(fgColor).Bold(true).
				Render("applying this configuration would remove deployed resources")
	case m.plan.Total() > 0:
		accent = warningColor
		lead = badge("DIFFERS", warningColor) + " " +
			lipgloss.NewStyle().Foreground(fgColor).
				Render("the local configuration differs from what is deployed")
	default:
		lead = badge("MATCHES", successColor) + " " +
			lipgloss.NewStyle().Foreground(fgColor).
				Render(fmt.Sprintf("the local configuration matches all %d deployed resources", c.Summary.ResourceCount))
	}

	body := lead + "\n\n" + driftBar(m.plan, inner) + "\n" + driftLegend(m.plan)
	if m.plan.Replace > 0 {
		body += "\n" + lipgloss.NewStyle().Foreground(dangerColor).
			Render(fmt.Sprintf("   %d of those are replacements — destroyed and recreated", m.plan.Replace))
	}
	if m.plan.Total() > 0 {
		body += "\n\n" + lipgloss.NewStyle().Foreground(fgColor).Bold(true).Render("NOTHING HAS BEEN APPLIED.") +
			lipgloss.NewStyle().Foreground(mutedColor).
				Render(fmt.Sprintf(" Review with: terraform -chdir=%s plan", m.req.envDir))
	}

	return panel("what a deploy would change", body, width, accent)
}

// renderStepOutput shows the last few lines a failing step printed. They were
// captured off the terminal so they could not tear the screen; they are still
// the most useful thing to read when init fails, so they belong inside the
// panel rather than nowhere.
func (m *syncScreenModel) renderStepOutput(inner int) string {
	var kept []string
	for _, line := range strings.Split(m.stepOutput, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			kept = append(kept, truncateToWidth(trimmed, inner))
		}
	}
	if len(kept) == 0 {
		return ""
	}
	const maxLines = 6
	if len(kept) > maxLines {
		kept = kept[len(kept)-maxLines:]
	}
	return "\n\n" + lipgloss.NewStyle().Foreground(mutedColor).Render(strings.Join(kept, "\n"))
}

func (m *syncScreenModel) legend() []keyHint {
	if m.phase == syncScreenWorking {
		return []keyHint{{"ctrl+c", "cancel"}}
	}
	return []keyHint{{"enter", "close"}}
}

// --------------------------------------------------------------- drift bar ---

// driftBar renders the plan as one bar split proportionally by action, coloured
// green/amber/red for add/change/destroy.
//
// A bare "16 / 2 / 3" makes three numbers of equal visual weight. Only one of
// them can lose you something, so the bar gives destruction a size on screen
// proportional to how much of the plan it is — and a colour that means the same
// thing everywhere else in meroku.
func driftBar(s planSummary, width int) string {
	if width < 8 {
		width = 8
	}
	if s.Total() == 0 {
		return lipgloss.NewStyle().Foreground(successColor).Render(strings.Repeat("█", width))
	}

	type seg struct {
		count int
		tone  lipgloss.TerminalColor
	}
	segs := []seg{
		{s.Add, successColor},
		{s.Change, warningColor},
		{s.Destroy, dangerColor},
	}

	// Every non-zero category gets at least one cell: a single destroy in a plan
	// of two hundred still has to be visible.
	cells := make([]int, len(segs))
	used := 0
	for i, sg := range segs {
		if sg.count > 0 {
			cells[i] = 1
			used++
		}
	}
	remaining := width - used
	if remaining < 0 {
		remaining = 0
	}
	for i, sg := range segs {
		if sg.count == 0 {
			continue
		}
		cells[i] += remaining * sg.count / s.Total()
	}
	// Hand any rounding leftovers to the largest category so the bar is exactly
	// as wide as asked for.
	for sum := 0; ; {
		sum = 0
		for _, n := range cells {
			sum += n
		}
		if sum >= width {
			break
		}
		big, bigN := 0, -1
		for i, sg := range segs {
			if sg.count > bigN {
				big, bigN = i, sg.count
			}
		}
		cells[big]++
	}

	var b strings.Builder
	for i, sg := range segs {
		if cells[i] == 0 {
			continue
		}
		b.WriteString(lipgloss.NewStyle().Foreground(sg.tone).Render(strings.Repeat("█", cells[i])))
	}
	return b.String()
}

// driftLegend labels the bar's segments. Colour carries the category, the
// number carries the size.
func driftLegend(s planSummary) string {
	if s.Total() == 0 {
		return lipgloss.NewStyle().Foreground(mutedColor).Render("no changes")
	}
	parts := []string{}
	if s.Add > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(successColor).Render("█ ")+
			statChip("add", fmt.Sprintf("%d", s.Add), fgColor))
	}
	if s.Change > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(warningColor).Render("█ ")+
			statChip("change", fmt.Sprintf("%d", s.Change), fgColor))
	}
	if s.Destroy > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(dangerColor).Render("█ ")+
			statChip("destroy", fmt.Sprintf("%d", s.Destroy), dangerColor))
	}
	return strings.Join(parts, "   ")
}

// ------------------------------------------------------------ entry point ---

// runSyncScreen shows the screen, runs whatever the user agreed to, and then
// leaves a plain-text record behind.
//
// The record matters: the alt screen is gone the moment the program exits, and
// the resource counts and the next command are worth keeping in the scrollback.
func runSyncScreen(req syncRequest) (reconnectOutcome, error) {
	m := newSyncScreenModel(req)

	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		// The screen could not start. Do the sync as lines rather than lose it.
		return plainSync(req)
	}

	fm, ok := final.(*syncScreenModel)
	if !ok {
		return m.outcome, m.err
	}
	fm.printRecord()
	return fm.outcome, fm.err
}

// printRecord writes the outcome as plain lines after the screen has closed.
func (m *syncScreenModel) printRecord() {
	c := m.req.conn
	fmt.Printf("\n📦 %s\n", deployedLine(c))

	if m.err != nil {
		fmt.Printf("   ✗ %s\n", m.outcome.Verdict)
		fmt.Println("   Your infrastructure and its state are untouched.")
		return
	}

	if m.outcome.Generated {
		fmt.Printf("   ✓ wrote env/%s from %s.yaml\n", c.Env, c.Env)
	}
	if m.outcome.Initialised {
		fmt.Printf("   ✓ linked env/%s to s3://%s/%s\n", c.Env, c.Summary.Bucket, c.Summary.Key)
	}

	// Nothing was compared, so say nothing about what a deploy would do.
	if m.cancelled {
		fmt.Printf("   Cancelled part way through. Run `meroku sync %s` to finish.\n", c.Env)
		return
	}

	switch {
	case m.planErr != nil:
		fmt.Printf("   • comparison skipped: %s\n", shortErrorNote(m.planErr))
		fmt.Printf("     retry with: terraform -chdir=%s plan\n", m.req.envDir)
	case m.plan.Total() == 0:
		fmt.Printf("   ✓ the local configuration matches all %d deployed resources\n", c.Summary.ResourceCount)
	default:
		fmt.Printf("   • %s\n", planCountsNote(m.plan))
		fmt.Println("     NOTHING HAS BEEN APPLIED. Review before deploying:")
		fmt.Printf("       terraform -chdir=%s plan\n", m.req.envDir)
	}
}

// ----------------------------------------------------------------- phrases ---

func planCountsNote(s planSummary) string {
	return fmt.Sprintf("%d to add, %d to change, %d to destroy", s.Add, s.Change, s.Destroy)
}

// shortErrorNote keeps an error to one readable line.
func shortErrorNote(err error) string {
	if err == nil {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(err.Error(), "\n", 2)[0])
	return truncateToWidth(line, 90)
}

func syncStepFailureVerdict(step syncStepID, env string, err error) string {
	switch step {
	case syncStepWrite:
		return fmt.Sprintf("env/%s could not be written, so it is still unlinked.", env)
	default:
		if err != nil && strings.Contains(err.Error(), errTerraformNotInstalled.Error()) {
			return "Terraform is not installed, so this environment is still unlinked."
		}
		return "terraform init failed, so this environment is still unlinked."
	}
}
