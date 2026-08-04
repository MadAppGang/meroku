package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The screen is driven directly here: Update is fed messages and the commands
// it returns are run inline. Nothing starts a Bubble Tea program, so no test
// can block on a terminal.

func syncScreenFixture(t *testing.T, generated bool, status stateConnectionStatus) (*syncScreenModel, *recordingGen, *recordingInit, *recordingPlan) {
	t.Helper()
	root := t.TempDir()
	gen := &recordingGen{root: root}
	init := &recordingInit{}
	plan := &recordingPlan{}

	c := stateConnection{
		Env:        "dev",
		Status:     status,
		Generated:  generated,
		AWSProfile: "example-dev",
		AccountID:  "000000000000",
		Summary: remoteStateSummary{
			Bucket: "state-bucket-example-dev-00000", Key: "state.tfstate",
			Region: "ap-southeast-2", ResourceCount: 85, ManagedCount: 67, DataCount: 18,
		},
	}
	m := newSyncScreenModel(syncRequest{
		conn: c, envDir: "env/dev", ask: true,
		gen: gen.run, init: init.run, plan: plan.run,
	})
	return m, gen, init, plan
}

func syncKey(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// pump runs a command and feeds whatever it produces back into the model, the
// way the Bubble Tea runtime would.
func pump(m *syncScreenModel, cmd tea.Cmd) {
	for i := 0; cmd != nil && i < 10; i++ {
		msg := cmd()
		if msg == nil {
			return
		}
		if _, ok := msg.(syncAnimMsg); ok {
			return
		}
		_, cmd = m.Update(msg)
	}
}

// --- which steps run -------------------------------------------------------

// A fresh checkout has to write env/dev first; a generated-but-unlinked
// directory does not; an already-linked one only needs the comparison.
func TestSyncScreenPicksItsStepsFromTheSituation(t *testing.T) {
	cases := map[string]struct {
		generated bool
		status    stateConnectionStatus
		want      []syncStepID
	}{
		"fresh checkout":      {false, stateDeployedButDisconnected, []syncStepID{syncStepWrite, syncStepInit, syncStepCompare}},
		"written, not linked": {true, stateDeployedButDisconnected, []syncStepID{syncStepInit, syncStepCompare}},
		"already linked":      {true, stateAlreadyInitialised, []syncStepID{syncStepCompare}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m, _, _, _ := syncScreenFixture(t, tc.generated, tc.status)
			if len(m.steps) != len(tc.want) {
				t.Fatalf("steps = %v, want %v", m.steps, tc.want)
			}
			for i := range tc.want {
				if m.steps[i] != tc.want[i] {
					t.Fatalf("steps = %v, want %v", m.steps, tc.want)
				}
			}
		})
	}
}

// --- the decision ----------------------------------------------------------

func TestSyncScreenRunsEveryStep(t *testing.T) {
	m, gen, init, plan := syncScreenFixture(t, false, stateDeployedButDisconnected)
	if m.phase != syncScreenWorking {
		t.Fatalf("phase = %v, want the screen to start on the work — the question is asked before it opens", m.phase)
	}

	pump(m, m.startNextStep())

	if len(gen.calls) != 1 || len(init.calls) != 1 || len(plan.calls) != 1 {
		t.Fatalf("steps = gen%v init%v plan%v, want one of each", gen.calls, init.calls, plan.calls)
	}
	if m.phase != syncScreenFinished {
		t.Fatalf("phase = %v, want finished", m.phase)
	}
	if !m.outcome.Generated || !m.outcome.Initialised {
		t.Fatalf("outcome = %+v, want generated and initialised", m.outcome)
	}
	if !strings.HasPrefix(m.outcome.Verdict, "Synced.") {
		t.Errorf("verdict = %q, want it to lead with Synced.", m.outcome.Verdict)
	}
}

// --- failures --------------------------------------------------------------

func TestSyncScreenStopsAtAFailedStep(t *testing.T) {
	m, gen, init, plan := syncScreenFixture(t, false, stateDeployedButDisconnected)
	gen.err = errors.New("environment file 'dev.yaml' not found")

	pump(m, m.startNextStep())

	if len(init.calls) != 0 || len(plan.calls) != 0 {
		t.Fatalf("terraform ran after a failed write: init%v plan%v", init.calls, plan.calls)
	}
	if m.states[syncStepWrite] != stepFailed {
		t.Fatalf("write state = %v, want failed", m.states[syncStepWrite])
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "untouched") {
		t.Errorf("a failure must say the infrastructure is untouched:\n%s", view)
	}
}

// A locked state is information, not a failed sync: the link still happened.
func TestSyncScreenReportsASkippedComparison(t *testing.T) {
	m, _, _, plan := syncScreenFixture(t, false, stateDeployedButDisconnected)
	plan.err = &stateLockedError{ID: "00000000-0000-0000-0000-000000000000"}

	pump(m, m.startNextStep())

	if m.err != nil {
		t.Fatalf("a locked state must not fail the sync, got: %v", m.err)
	}
	if m.states[syncStepCompare] != stepSkipped {
		t.Fatalf("compare state = %v, want skipped", m.states[syncStepCompare])
	}
	if !strings.Contains(m.outcome.Verdict, "locked") {
		t.Errorf("verdict = %q, want it to name the lock", m.outcome.Verdict)
	}
}

// --- the result ------------------------------------------------------------

// Destruction is the only outcome that can cost the user something, so it leads
// the result and is the loudest thing on it.
func TestSyncScreenLeadsWithDestruction(t *testing.T) {
	m, _, _, plan := syncScreenFixture(t, false, stateDeployedButDisconnected)
	plan.summary = planSummary{Add: 16, Change: 2, Destroy: 3, Replace: 2}

	pump(m, m.startNextStep())

	view := stripANSI(m.View())
	destroy := strings.Index(view, "WOULD DESTROY 3")
	counts := strings.Index(view, "destroy 3")
	if destroy < 0 {
		t.Fatalf("destruction was not called out:\n%s", view)
	}
	if counts < 0 {
		t.Fatalf("the split was not labelled:\n%s", view)
	}
	if destroy > counts {
		t.Errorf("destruction must lead the numbers:\n%s", view)
	}
	if !strings.Contains(view, "NOTHING HAS BEEN APPLIED") {
		t.Errorf("the screen must make clear nothing was applied:\n%s", view)
	}
	if !strings.Contains(view, "2 of those are replacements") {
		t.Errorf("replacements were not distinguished from outright destroys:\n%s", view)
	}
}

func TestSyncScreenNoChangesDoesNotShout(t *testing.T) {
	m, _, _, _ := syncScreenFixture(t, false, stateDeployedButDisconnected)

	pump(m, m.startNextStep())

	view := stripANSI(m.View())
	if strings.Contains(view, "DESTROY") {
		t.Errorf("nothing would be destroyed; do not say so:\n%s", view)
	}
	if !strings.Contains(view, "matches all 85 deployed resources") {
		t.Errorf("a matching configuration was not reported:\n%s", view)
	}
}

// --- the drift bar ---------------------------------------------------------

// The bar is exactly as wide as asked for, at every width and every mix.
func TestDriftBarIsExactlyTheRequestedWidth(t *testing.T) {
	summaries := []planSummary{
		{},
		{Add: 16, Change: 2, Destroy: 3},
		{Add: 1},
		{Destroy: 1},
		{Add: 200, Change: 1, Destroy: 1},
		{Change: 7, Destroy: 7},
	}
	for _, s := range summaries {
		for _, w := range []int{8, 12, 20, 33, 64, 91} {
			if got := lipgloss.Width(driftBar(s, w)); got != w {
				t.Errorf("driftBar(%+v, %d) width = %d", s, w, got)
			}
		}
	}
}

// One destroy in a plan of two hundred still has to be visible.
func TestDriftBarAlwaysShowsASingleDestroy(t *testing.T) {
	bar := driftBar(planSummary{Add: 200, Destroy: 1}, 40)
	if !strings.Contains(bar, string(dangerColorANSI())) {
		t.Errorf("a lone destroy vanished from the bar: %q", bar)
	}
}

func dangerColorANSI() []rune {
	return []rune(lipgloss.NewStyle().Foreground(dangerColor).Render("█"))
}

// --- layout ----------------------------------------------------------------

// Nothing may spill past the terminal edge, at any phase or width. A footer
// that overflowed at 114 columns is why this test exists.
func TestSyncScreenFitsItsTerminal(t *testing.T) {
	for _, size := range []struct{ w, h int }{{80, 24}, {100, 30}, {114, 40}, {160, 50}} {
		build := func() []*syncScreenModel {
			working, _, _, _ := syncScreenFixture(t, false, stateDeployedButDisconnected)
			working.phase = syncScreenWorking
			working.states[syncStepWrite] = stepOK
			working.notes[syncStepWrite] = "wrote env/dev/main.tf from dev.yaml"
			working.states[syncStepInit] = stepActive
			working.active = 1

			destroyed, _, _, dplan := syncScreenFixture(t, false, stateDeployedButDisconnected)
			dplan.summary = planSummary{Add: 16, Change: 2, Destroy: 3, Replace: 2}
			pump(destroyed, destroyed.startNextStep())

			matched, _, _, _ := syncScreenFixture(t, false, stateDeployedButDisconnected)
			pump(matched, matched.startNextStep())

			failed, gen, _, _ := syncScreenFixture(t, false, stateDeployedButDisconnected)
			gen.err = errors.New("environment file 'dev.yaml' not found in this directory, and no template could be rendered without it")
			pump(failed, failed.startNextStep())

			return []*syncScreenModel{working, destroyed, matched, failed}
		}

		for i, m := range build() {
			m.width, m.height = size.w, size.h
			for n, line := range strings.Split(m.View(), "\n") {
				if got := lipgloss.Width(line); got > size.w {
					t.Errorf("%dx%d screen %d line %d overflows: %d > %d\n%s",
						size.w, size.h, i, n, got, size.w, stripANSI(line))
				}
			}
		}
	}
}

// --- visual QA -------------------------------------------------------------

// Writes colour ANSI frames for conversion to PNG, the same way the DNS screens
// are checked. Gated so ordinary `go test` stays quiet.
//
//	MEROKU_TUI_SHOTS=1 go test -run TestRenderSyncScreens ./app
func TestRenderSyncScreens(t *testing.T) {
	if os.Getenv("MEROKU_TUI_SHOTS") != "1" {
		t.Skip("set MEROKU_TUI_SHOTS=1 to render sync screenshots")
	}
	lipglossForceTrueColor()

	widths := map[string]int{"wide": 120, "narrow": 80}

	frames := map[string]func() *syncScreenModel{
		"sync-1-decide": func() *syncScreenModel {
			m, _, _, _ := syncScreenFixture(t, false, stateDeployedButDisconnected)
			return m
		},
		"sync-2-working": func() *syncScreenModel {
			m, _, _, _ := syncScreenFixture(t, false, stateDeployedButDisconnected)
			m.phase = syncScreenWorking
			m.states[syncStepWrite] = stepOK
			m.notes[syncStepWrite] = "wrote env/dev/main.tf from dev.yaml"
			m.states[syncStepInit] = stepOK
			m.notes[syncStepInit] = "linked to s3://state-bucket-example-dev-00000"
			m.states[syncStepCompare] = stepActive
			m.active = 2
			m.anim = 11
			m.elapsed = 74e9
			return m
		},
		"sync-3-destroy": func() *syncScreenModel {
			m, _, _, plan := syncScreenFixture(t, false, stateDeployedButDisconnected)
			plan.summary = planSummary{Add: 16, Change: 2, Destroy: 3, Replace: 2}
			_, cmd := m.Update(syncKey("s"))
			pump(m, cmd)
			return m
		},
		"sync-4-match": func() *syncScreenModel {
			m, _, _, _ := syncScreenFixture(t, false, stateDeployedButDisconnected)
			_, cmd := m.Update(syncKey("s"))
			pump(m, cmd)
			return m
		},
	}

	for name, build := range frames {
		for label, w := range widths {
			m := build()
			m.width, m.height = w, 34
			path := "/tmp/meroku-tui-" + name + "-" + label + ".ansi"
			if err := os.WriteFile(path, []byte(m.View()), 0o644); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}
			t.Logf("wrote %s", path)
		}
	}
}

// Ctrl+C part way through must not claim the comparison came back clean: an
// empty plan summary is "nobody looked", not "no changes".
func TestSyncScreenCancelDoesNotClaimAMatch(t *testing.T) {
	m, _, _, _ := syncScreenFixture(t, false, stateDeployedButDisconnected)
	m.phase = syncScreenWorking
	m.states[syncStepWrite] = stepOK
	m.outcome.Generated = true
	m.states[syncStepInit] = stepActive
	m.active = 1

	if _, cmd := m.Update(syncKey("ctrl+c")); cmd == nil {
		t.Fatal("ctrl+c should quit")
	}
	if !m.cancelled {
		t.Fatal("cancelled = false after ctrl+c during the work")
	}

	out := captureOutput(t, m.printRecord)
	if strings.Contains(out, "matches") {
		t.Errorf("a cancelled run must not claim a match:\n%s", out)
	}
	if !strings.Contains(out, "Cancelled") || !strings.Contains(out, "meroku sync dev") {
		t.Errorf("a cancelled run must say so and how to finish:\n%s", out)
	}
}

// --- frame integrity -------------------------------------------------------
//
// The width test below only proves nothing spills past the edge. These prove
// the frame is the right frame: one heading per panel, one row per step, and no
// stale row left over from an earlier phase. A duplicated heading and a
// contradictory `RUNNING`/`DONE` pair for the same step both got through a
// passing suite once.

// frameFixtures builds one deterministic model per phase worth locking down.
func frameFixtures(t *testing.T) map[string]*syncScreenModel {
	t.Helper()

	working, _, _, _ := syncScreenFixture(t, false, stateDeployedButDisconnected)
	working.phase = syncScreenWorking
	working.states[syncStepWrite] = stepOK
	working.notes[syncStepWrite] = "wrote env/dev/ from dev.yaml"
	working.states[syncStepInit] = stepActive
	working.active = 1
	working.anim = 7
	working.elapsed = 31 * time.Second

	destroyed, _, _, dplan := syncScreenFixture(t, false, stateDeployedButDisconnected)
	dplan.summary = planSummary{Add: 16, Change: 2, Destroy: 3, Replace: 2}
	pump(destroyed, destroyed.startNextStep())
	destroyed.elapsed = 148 * time.Second

	matched, _, _, _ := syncScreenFixture(t, false, stateDeployedButDisconnected)
	pump(matched, matched.startNextStep())
	matched.elapsed = 142 * time.Second

	return map[string]*syncScreenModel{
		"working": working,
		"destroy": destroyed,
		"match":   matched,
	}
}

// Every panel heading appears exactly once, and no step is listed twice. A
// second "STEPS" or a stale row is the signature of a torn frame.
func TestSyncScreenFrameHasNoDuplicateHeadingsOrRows(t *testing.T) {
	for name, m := range frameFixtures(t) {
		t.Run(name, func(t *testing.T) {
			m.width, m.height = 120, 34
			frame := stripANSI(m.View())

			headings := map[string]int{
				"STEPS":                      0,
				"WHAT A DEPLOY WOULD CHANGE": 0,
			}
			for _, line := range strings.Split(frame, "\n") {
				for h := range headings {
					if strings.Contains(line, h) {
						headings[h]++
					}
				}
			}
			for h, n := range headings {
				if n > 1 {
					t.Errorf("heading %q appears %d times in the %s frame:\n%s", h, n, name, frame)
				}
			}

			// One row per step, whatever its state.
			for _, step := range m.steps {
				title := syncStepTitles[step]
				if step == syncStepWrite {
					title = "write env/" + m.req.conn.Env
				}
				rows := 0
				for _, line := range strings.Split(frame, "\n") {
					// The marquee label repeats the step name; only count status rows.
					if strings.Contains(line, title) &&
						(strings.Contains(line, "DONE") || strings.Contains(line, "RUNNING") ||
							strings.Contains(line, "FAILED") || strings.Contains(line, "SKIPPED") ||
							strings.Contains(line, "waiting")) {
						rows++
					}
				}
				if rows > 1 {
					t.Errorf("step %q has %d status rows in the %s frame:\n%s", title, rows, name, frame)
				}
			}

			// And no step can be running and done at the same time.
			for _, step := range m.steps {
				if m.states[step] == stepActive && m.phase == syncScreenFinished {
					t.Errorf("step %v is still marked running on a finished screen", step)
				}
			}
		})
	}
}

// Golden snapshot of each frame. Any unintended change to what is drawn — a
// duplicated line, a lost row, a shifted panel — shows up as a diff.
//
//	go test -run TestSyncScreenGoldenFrames -update ./app
var updateGolden = flag.Bool("update", false, "rewrite the sync screen golden frames")

func TestSyncScreenGoldenFrames(t *testing.T) {
	for name, m := range frameFixtures(t) {
		t.Run(name, func(t *testing.T) {
			m.width, m.height = 120, 34
			got := strings.TrimRight(stripANSI(m.View()), "\n \t")

			path := filepath.Join("testdata", "sync-frame-"+name+".golden")
			if *updateGolden {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatalf("creating testdata: %v", err)
				}
				if err := os.WriteFile(path, []byte(got+"\n"), 0o644); err != nil {
					t.Fatalf("writing %s: %v", path, err)
				}
				return
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %s (run with -update to create it): %v", path, err)
			}
			if got != strings.TrimRight(string(want), "\n \t") {
				t.Errorf("frame %q changed.\n--- got ---\n%s\n--- want ---\n%s", name, got, string(want))
			}
		})
	}
}

// --- stdout ----------------------------------------------------------------

// Nothing may reach stdout while the program owns the terminal. The steps are
// ordinary meroku functions that report by printing — generateEnvironmentFiles
// prints a line per file it writes — and a stray line under a Bubble Tea
// program tears the frame it is drawing.
func TestSyncScreenStepsWriteNothingToStdout(t *testing.T) {
	root := t.TempDir()

	gen := &recordingGen{root: root}
	noisyGen := func(env string) error {
		fmt.Printf("✓ Generated: env/%s/_bridge.tf\n", env)
		fmt.Printf("✓ Generated: env/%s/main.tf\n", env)
		return gen.run(env)
	}
	init := &recordingInit{}
	noisyInit := func(dir string) error {
		fmt.Println("   Initializing the backend...")
		return init.run(dir)
	}
	plan := &recordingPlan{}
	noisyPlan := func(dir string) (planSummary, error) {
		fmt.Println("   Refreshing state...")
		return plan.run(dir)
	}

	m := newSyncScreenModel(syncRequest{
		conn: stateConnection{
			Env: "dev", Status: stateDeployedButDisconnected,
			Summary: remoteStateSummary{Bucket: "b", Key: "k", Region: "r", ResourceCount: 85},
		},
		envDir: "env/dev", ask: true,
		gen: noisyGen, init: noisyInit, plan: noisyPlan,
	})

	leaked := captureOutput(t, func() {
		pump(m, m.startNextStep())
	})

	if leaked != "" {
		t.Errorf("the screen let %d bytes reach stdout while it owned the terminal:\n%q", len(leaked), leaked)
	}
	if len(gen.calls) != 1 || len(init.calls) != 1 || len(plan.calls) != 1 {
		t.Fatalf("steps did not all run: gen%v init%v plan%v", gen.calls, init.calls, plan.calls)
	}
}

// The capture is for the screen only. Off it, the same functions must print
// exactly as they always have — `meroku generate` still tells you what it wrote.
func TestPlainSyncStillPrintsStepOutput(t *testing.T) {
	isolateAWSEnv(t)
	root, envDir, c := disconnectedFixture(t)

	gen := &recordingGen{root: root}
	noisyGen := func(env string) error {
		fmt.Printf("✓ Generated: env/%s/_bridge.tf\n", env)
		return gen.run(env)
	}

	out := captureOutput(t, func() {
		if _, err := performSync(syncRequest{
			conn: c, envDir: envDir, ask: true,
			gen: noisyGen, init: (&recordingInit{}).run, plan: (&recordingPlan{}).run,
			decide: func(stateConnection) syncDecision { return syncApproved },
		}); err != nil {
			t.Errorf("performSync: %v", err)
		}
	})

	if !strings.Contains(out, "✓ Generated: env/dev/_bridge.tf") {
		t.Errorf("the plain path must keep printing what the generator printed:\n%s", out)
	}
}

// A failing step's output is not thrown away — it is shown inside the panel,
// which is where terraform's explanation of an init failure belongs.
func TestSyncScreenShowsAFailedStepsOutput(t *testing.T) {
	m, _, init, _ := syncScreenFixture(t, true, stateDeployedButDisconnected)
	init.err = errors.New("terraform init failed: exit status 1")
	m.req.init = func(dir string) error {
		fmt.Println("Error: Failed to get existing workspaces: S3 bucket does not exist.")
		return init.run(dir)
	}

	var leaked string
	leaked = captureOutput(t, func() {
		pump(m, m.startNextStep())
	})
	if leaked != "" {
		t.Errorf("a failing step leaked to stdout:\n%q", leaked)
	}

	view := stripANSI(m.View())
	if !strings.Contains(view, "S3 bucket does not exist") {
		t.Errorf("the failure output was captured but never shown:\n%s", view)
	}
}
