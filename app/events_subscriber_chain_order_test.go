package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The offline proof that the subscriber chain env/main.hbs renders gives what
// AWS and the design need, under Terraform's real graph:
//
//   - one subscriber create, update or delete at a time on the bus (AWS
//     refuses a second concurrent one, and awscc does not retry it);
//   - every new or changed subscriber is in place before an old one is
//     deleted, so a rename or an ordered toggle loses no event;
//   - leaving the enhanced bus, and terraform destroy, revoke other accounts'
//     access only after RAM stopped granting it, and before the bus is gone.
//
// Each step renders an env YAML through applyTemplate with the real
// template. The rendered module "event_bus_enhanced" and every subscriber
// block are kept verbatim, chain and depends_on included; only their source
// points at the fakes in testdata/subscriber_chain_fake, which log every
// subscriber create and delete with a one-second body. A fake module "events"
// takes the rendered consumers list. Then `terraform apply -json` runs with
// the default parallelism, and the state probe reads the fake state back for
// the next render, as a deploy does. The order of the stream is the clock: a
// fake's provisioner lines, and the apply_start and apply_complete of each
// in-place subscriber update.
//
// An update takes no time of its own, so an update that the graph left free
// to run beside a create or delete shows as an overlap only when it happens
// to start inside that second; creates and deletes are always caught.
//
// Needs terraform on PATH (CI has it); no provider is downloaded.

const chainFakeDir = "testdata/subscriber_chain_fake"

// chainStepYAML is a whole env YAML with the given event_bus block and
// services.
func chainStepYAML(eventBus, services string) string {
	return `schema_version: 33
project: myapp
env: dev
region: ap-southeast-2
account_id: "000000000000"
aws_profile: none
use_default_vpc: false
ecr_strategy: local
` + eventBus + `
workload:
  backend_image_port: 8080
services:
` + services
}

const chainEnhancedShared = `event_bus:
  enabled: true
  type: enhanced
  share:
    - { account: "000000000001", access: subscribe }
    - { account: "000000000002", access: publish }`

const chainEnhancedPublishOnly = `event_bus:
  enabled: true
  type: enhanced
  share:
    - { account: "000000000002", access: publish }`

// chainLog is one apply's log.
type chainLog []string

func (l chainLog) index(prefix string) int {
	for i, line := range l {
		if line == prefix || strings.HasPrefix(line, prefix+" ") {
			return i
		}
	}
	return -1
}

// indices returns every line that starts with "<kind> <op> " (kind start or
// end).
func (l chainLog) indices(kind, op string) []int {
	var out []int
	for i, line := range l {
		if strings.HasPrefix(line, kind+" "+op+" ") {
			out = append(out, i)
		}
	}
	return out
}

func isSubscriberOp(op string) bool {
	return op == "create" || op == "update" || op == "delete"
}

// mustBefore asserts that line a (a full prefix such as "end create m") comes
// before line b.
func (l chainLog) mustBefore(t *testing.T, step, a, b string) {
	t.Helper()
	ia, ib := l.index(a), l.index(b)
	switch {
	case ia < 0:
		t.Errorf("%s: no %q in the log:\n%s", step, a, strings.Join(l, "\n"))
	case ib < 0:
		t.Errorf("%s: no %q in the log:\n%s", step, b, strings.Join(l, "\n"))
	case ia > ib:
		t.Errorf("%s: %q must come before %q:\n%s", step, a, b, strings.Join(l, "\n"))
	}
}

func (l chainLog) mustHave(t *testing.T, step string, lines ...string) {
	t.Helper()
	for _, line := range lines {
		if l.index(line) < 0 {
			t.Errorf("%s: no %q in the log:\n%s", step, line, strings.Join(l, "\n"))
		}
	}
}

// checkGeneric asserts what holds in every apply: subscriber operations never
// overlap; every create and update ends before the first delete starts; and
// every create ends before the first update starts.
func (l chainLog) checkGeneric(t *testing.T, step string) {
	t.Helper()
	t.Logf("%s:\n  %s", step, strings.Join(l, "\n  "))
	open := ""
	for _, line := range l {
		f := strings.Fields(line)
		if len(f) < 3 || !isSubscriberOp(f[1]) {
			continue
		}
		switch f[0] {
		case "start":
			if open != "" {
				t.Errorf("%s: %q starts while %q is still running:\n%s", step, line, open, strings.Join(l, "\n"))
			}
			open = line
		case "end":
			open = ""
		}
	}

	last := func(kind string, ops ...string) int {
		n := -1
		for _, op := range ops {
			for _, i := range l.indices(kind, op) {
				if i > n {
					n = i
				}
			}
		}
		return n
	}
	first := func(kind, op string) int {
		if idx := l.indices(kind, op); len(idx) > 0 {
			return idx[0]
		}
		return -1
	}
	if d := first("start", "delete"); d >= 0 {
		if e := last("end", "create", "update"); e > d {
			t.Errorf("%s: a create or update ends after the first delete starts:\n%s", step, strings.Join(l, "\n"))
		}
	}
	if u := first("start", "update"); u >= 0 {
		if e := last("end", "create"); e > u {
			t.Errorf("%s: a create ends after the first update starts:\n%s", step, strings.Join(l, "\n"))
		}
	}
}

// checkTeardown asserts the order of leaving the bus: every RAM principal
// association is gone before the guard's teardown revoke starts, the revoke
// gets the bus ARN from its own input in teardown mode, and it and every
// subscriber delete end before the bus delete starts.
func (l chainLog) checkTeardown(t *testing.T, step, busID string) {
	t.Helper()
	l.mustHave(t, step, "start teardown "+busID+" mode=teardown", "end teardown "+busID, "start bus-delete "+busID)
	teardown := l.index("start teardown " + busID)
	for _, i := range l.indices("end", "principal-delete") {
		if i > teardown {
			t.Errorf("%s: %q ends after the teardown revoke starts:\n%s", step, l[i], strings.Join(l, "\n"))
		}
	}
	busDelete := l.index("start bus-delete " + busID)
	if l.index("end teardown "+busID) > busDelete {
		t.Errorf("%s: the teardown revoke ends after the bus delete starts:\n%s", step, strings.Join(l, "\n"))
	}
	for _, i := range l.indices("end", "delete") {
		if i > busDelete {
			t.Errorf("%s: %q ends after the bus delete starts:\n%s", step, l[i], strings.Join(l, "\n"))
		}
	}
}

// busCreated returns the id the fake bus logged when it was created.
func (l chainLog) busCreated(t *testing.T, step string) string {
	t.Helper()
	i := l.index("end bus-create")
	if i < 0 {
		t.Fatalf("%s: the bus was not created:\n%s", step, strings.Join(l, "\n"))
	}
	return strings.Fields(l[i])[2]
}

// chainHarness runs the steps: render, rewrite to the fakes, apply, read
// state back.
type chainHarness struct {
	t         *testing.T
	terraform string
	fakes     string
	work      string
	root      *eventsRenderRoot
	state     eventsAwsccState
}

var renderedModuleSource = regexp.MustCompile(`(?m)^(  source\s+= )"[^"]*/(event_bus_enhanced|event_subscriber)"$`)

// fakeMain keeps the bus module and the subscriber chain of a rendered
// main.tf, sources pointed at the fakes, and adds a fake module "events" fed
// the rendered consumers list.
func (h *chainHarness) fakeMain(mainTf string) string {
	h.t.Helper()
	events := hclTopBlocks(mainTf, `module "events" {`)
	if len(events) != 1 {
		h.t.Fatalf("want one module \"events\" in the render, got %d", len(events))
	}
	consumers := regexp.MustCompile(`(?m)^  consumers   = (.*)$`).FindStringSubmatch(events[0])
	if consumers == nil {
		h.t.Fatalf("no consumers line in module \"events\":\n%s", events[0])
	}

	var b strings.Builder
	fmt.Fprintf(&b, "module \"events\" {\n  source    = %q\n  consumers = %s\n}\n", filepath.Join(h.fakes, "events"), consumers[1])
	blocks := append(hclTopBlocks(mainTf, `module "event_bus_enhanced" {`), subscriberChain(mainTf)...)
	for _, block := range blocks {
		name := hclModuleName(block)
		block = renderedModuleSource.ReplaceAllStringFunc(block, func(line string) string {
			m := renderedModuleSource.FindStringSubmatch(line)
			out := fmt.Sprintf("%s%q", m[1], filepath.Join(h.fakes, m[2]))
			if m[2] == "event_subscriber" {
				out += fmt.Sprintf("\n  log_name = %q", name)
			}
			return out
		})
		b.WriteString("\n" + block + "\n")
	}
	return b.String()
}

func (h *chainHarness) run(args ...string) string {
	h.t.Helper()
	cmd := exec.Command(h.terraform, args...)
	cmd.Dir = h.work
	cmd.Env = append(os.Environ(), "TF_IN_AUTOMATION=1", "CHECKPOINT_DISABLE=1")
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		h.t.Fatalf("terraform %s: %v\n%s\n%s", strings.Join(args, " "), err, out.String(), stderr.String())
	}
	return out.String()
}

// subscriberAddr matches a subscriber fake's resource in the JSON stream.
var subscriberAddr = regexp.MustCompile(`^module\.([A-Za-z0-9_-]+)\.terraform_data\.sub\[0\]$`)

// streamLog turns the `terraform apply -json` stream into the log, in stream
// order: every line a fake's provisioner printed, and a start and an end line
// for every in-place update of a subscriber, which runs no provisioner.
func streamLog(t *testing.T, stream string) chainLog {
	t.Helper()
	var log chainLog
	for _, raw := range strings.Split(stream, "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var msg struct {
			Type string `json:"type"`
			Hook struct {
				Resource struct {
					Addr string `json:"addr"`
				} `json:"resource"`
				Action string `json:"action"`
				Output string `json:"output"`
			} `json:"hook"`
		}
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			t.Fatalf("terraform -json line is not JSON: %v\n%s", err, raw)
		}
		switch msg.Type {
		case "provision_progress":
			line := strings.TrimSpace(msg.Hook.Output)
			if strings.HasPrefix(line, "start ") || strings.HasPrefix(line, "end ") {
				log = append(log, line)
			}
		case "apply_start", "apply_complete":
			m := subscriberAddr.FindStringSubmatch(msg.Hook.Resource.Addr)
			if m == nil || msg.Hook.Action != "update" {
				continue
			}
			kind := "start"
			if msg.Type == "apply_complete" {
				kind = "end"
			}
			log = append(log, kind+" update "+m[1])
		}
	}
	return log
}

// apply runs one terraform command over main.tf and returns its log.
func (h *chainHarness) apply(mainTf string, args ...string) chainLog {
	h.t.Helper()
	if err := os.WriteFile(filepath.Join(h.work, "main.tf"), []byte(mainTf), 0o644); err != nil {
		h.t.Fatal(err)
	}
	h.run("init", "-input=false", "-no-color")
	stream := h.run(append(append([]string{"apply"}, args...), "-auto-approve", "-input=false", "-json")...)
	h.state = h.readState()
	return streamLog(h.t, stream)
}

// step renders yamlBody and applies it.
func (h *chainHarness) step(yamlBody string) (chainLog, string) {
	h.t.Helper()
	mainTf, _ := h.root.mustRender(h.t, yamlBody)
	fake := h.fakeMain(mainTf)
	return h.apply(fake), mainTf
}

// readState maps the fake state onto what the real probe reads from the
// real modules (app/events_enhanced.go, parseEventsAwsccState).
func (h *chainHarness) readState() eventsAwsccState {
	h.t.Helper()
	cmd := exec.Command(h.terraform, "show", "-json", "-no-color")
	cmd.Dir = h.work
	cmd.Env = append(os.Environ(), "CHECKPOINT_DISABLE=1")
	out, err := cmd.Output()
	if err != nil {
		h.t.Fatalf("terraform show -json: %v", err)
	}
	var doc struct {
		Values struct {
			RootModule struct {
				ChildModules []struct {
					Address   string `json:"address"`
					Resources []struct {
						Name   string          `json:"name"`
						Index  json.RawMessage `json:"index"`
						Values struct {
							Input json.RawMessage `json:"input"`
						} `json:"values"`
					} `json:"resources"`
				} `json:"child_modules"`
			} `json:"root_module"`
		} `json:"values"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		h.t.Fatalf("parsing terraform show -json: %v\n%s", err, out)
	}
	s := emptyEventsState()
	for _, m := range doc.Values.RootModule.ChildModules {
		module := strings.TrimPrefix(m.Address, "module.")
		for _, r := range m.Resources {
			var key string
			_ = json.Unmarshal(r.Index, &key)
			switch {
			case module == "event_bus_enhanced" && r.Name == "bus":
				s.Bus = true
			case module == "event_bus_enhanced" && r.Name == "bus_guard":
				s.Guard = true
			case module == "event_bus_enhanced" && r.Name == "principal":
				s.RamPrincipals = append(s.RamPrincipals, key)
			case module == "event_bus_enhanced" && (r.Name == "revoke" || r.Name == "revoke_record"):
				var in struct {
					Granted []string `json:"granted"`
					Lost    []string `json:"lost"`
					Round   int      `json:"round"`
				}
				_ = json.Unmarshal(r.Values.Input, &in)
				rec := &eventsRevokeRecord{Granted: in.Granted, Lost: in.Lost, Round: in.Round}
				if r.Name == "revoke" {
					s.Revoke = rec
				} else {
					s.RevokeRecord = rec
				}
			case module == "events" && r.Name == "queue":
				s.Consumers = append(s.Consumers, key)
			case module == "events" && r.Name == "fifo_queue":
				s.FifoQueues = append(s.FifoQueues, key)
			case r.Name == "sub":
				if name, ok := eventModuleSubscriberName("myapp", "dev", module); ok {
					s.Subscribers[module] = name
				}
			}
		}
	}
	sort.Strings(s.RamPrincipals)
	sort.Strings(s.Consumers)
	sort.Strings(s.FifoQueues)
	return s
}

func TestEventSubscriberChainOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("runs terraform applies with one-second operations")
	}
	terraform, err := exec.LookPath("terraform")
	if err != nil {
		t.Skip("terraform is not on PATH")
	}
	fakes, err := filepath.Abs(chainFakeDir)
	if err != nil {
		t.Fatal(err)
	}
	h := &chainHarness{
		t:         t,
		terraform: terraform,
		fakes:     fakes,
		work:      t.TempDir(),
		root:      newEventsRenderRoot(t),
	}
	h.state = emptyEventsState()
	stubEventsState(t, func() eventsAwsccState { return h.state })

	sub := func(consumer, key string) string { return eventSubscriberModule(consumer, key) }
	fifoOrders := eventFifoSubscriberModule("orders")

	// 1. Two consumers, a share with two principals.
	log, _ := h.step(chainStepYAML(chainEnhancedShared, `
  - name: orders
    container_port: 3000
    events:
      subscribes:
        - { name: a, detail_type: a.x, ordered: true }
        - { name: b, detail_type: b.x, ordered: true }
        - { name: s, detail_type: s.x }
  - name: billing
    container_port: 3001
    events:
      subscribes:
        - { name: t, detail_type: t.x }
`))
	log.checkGeneric(t, "step 1")
	log.mustHave(t, "step 1", "end create "+fifoOrders, "end create "+sub("orders", "s"), "end create "+sub("billing", "t"),
		"end principal-create subscribe/account/000000000001", "end principal-create publish/account/000000000002",
		"start revoke granted=account:000000000001 lost=")
	busID := log.busCreated(t, "step 1")

	// 2. Rename t, add u, remove the subscribe principal.
	log, _ = h.step(chainStepYAML(chainEnhancedPublishOnly, `
  - name: orders
    container_port: 3000
    events:
      subscribes:
        - { name: a, detail_type: a.x, ordered: true }
        - { name: b, detail_type: b.x, ordered: true }
        - { name: s, detail_type: s.x }
  - name: billing
    container_port: 3001
    events:
      subscribes:
        - { name: t2, detail_type: t.x }
        - { name: u, detail_type: u.x }
`))
	log.checkGeneric(t, "step 2")
	log.mustBefore(t, "step 2", "end create "+sub("billing", "t2"), "start delete "+sub("billing", "t"))
	log.mustBefore(t, "step 2", "end create "+sub("billing", "u"), "start delete "+sub("billing", "t"))
	log.mustBefore(t, "step 2", "end principal-delete subscribe/account/000000000001", "start revoke")
	log.mustHave(t, "step 2", "start revoke granted= lost=account:000000000001")

	// 3. Update plus delete: s turns ordered, so the FIFO subscriber widens and
	// the standard one goes.
	log, _ = h.step(chainStepYAML(chainEnhancedPublishOnly, `
  - name: orders
    container_port: 3000
    events:
      subscribes:
        - { name: a, detail_type: a.x, ordered: true }
        - { name: b, detail_type: b.x, ordered: true }
        - { name: s, detail_type: s.x, ordered: true }
  - name: billing
    container_port: 3001
    events:
      subscribes:
        - { name: t2, detail_type: t.x }
        - { name: u, detail_type: u.x }
`))
	log.checkGeneric(t, "step 3")
	log.mustBefore(t, "step 3", "end update "+fifoOrders, "start delete "+sub("orders", "s"))

	// 4. Create plus a narrowing update: a turns ordered off while b stays.
	log, _ = h.step(chainStepYAML(chainEnhancedPublishOnly, `
  - name: orders
    container_port: 3000
    events:
      subscribes:
        - { name: a, detail_type: a.x }
        - { name: b, detail_type: b.x, ordered: true }
        - { name: s, detail_type: s.x, ordered: true }
  - name: billing
    container_port: 3001
    events:
      subscribes:
        - { name: t2, detail_type: t.x }
        - { name: u, detail_type: u.x }
`))
	log.checkGeneric(t, "step 4")
	log.mustBefore(t, "step 4", "end create "+sub("orders", "a"), "start update "+fifoOrders)

	// 5. A new module heads the chain as a create, then sorts last among the
	// modules in state, so the stored dependencies predate the new order; in
	// that second apply two subscribers are removed.
	log, _ = h.step(chainStepYAML(chainEnhancedPublishOnly, `
  - name: orders
    container_port: 3000
    events:
      subscribes:
        - { name: a, detail_type: a.x }
        - { name: b, detail_type: b.x, ordered: true }
        - { name: s, detail_type: s.x, ordered: true }
  - name: billing
    container_port: 3001
    events:
      subscribes:
        - { name: t2, detail_type: t.x }
        - { name: u, detail_type: u.x }
  - name: zeta
    container_port: 3002
    events:
      subscribes:
        - { name: q, detail_type: q.x }
`))
	log.checkGeneric(t, "step 5a")
	log.mustHave(t, "step 5a", "end create "+sub("zeta", "q"))

	log, _ = h.step(chainStepYAML(chainEnhancedPublishOnly, `
  - name: orders
    container_port: 3000
    events:
      subscribes:
        - { name: b, detail_type: b.x, ordered: true }
        - { name: s, detail_type: s.x, ordered: true }
  - name: billing
    container_port: 3001
    events:
      subscribes:
        - { name: t2, detail_type: t.x }
  - name: zeta
    container_port: 3002
    events:
      subscribes:
        - { name: q, detail_type: q.x }
`))
	log.checkGeneric(t, "step 5b")
	log.mustHave(t, "step 5b", "end delete "+sub("orders", "a"), "end delete "+sub("billing", "u"))

	// 6. A whole consumer goes (its queue too), with a rename elsewhere.
	log, _ = h.step(chainStepYAML(chainEnhancedPublishOnly, `
  - name: orders
    container_port: 3000
    events:
      subscribes:
        - { name: b, detail_type: b.x, ordered: true }
        - { name: s, detail_type: s.x, ordered: true }
  - name: zeta
    container_port: 3002
    events:
      subscribes:
        - { name: q2, detail_type: q.x }
`))
	log.checkGeneric(t, "step 6")
	log.mustBefore(t, "step 6", "end create "+sub("zeta", "q2"), "start delete "+sub("zeta", "q"))
	log.mustHave(t, "step 6", "end delete "+sub("billing", "t2"))

	if !containsString(h.state.FifoQueues, "orders") {
		t.Fatalf("step 6: the fake state holds no FIFO queue of orders: %+v", h.state)
	}

	// 7. Back to classic: cleanup deletes every subscriber and the bus. orders
	// keeps its ordered queue with fifo_retired, the first of the two deploys
	// that delete a FIFO set.
	log, mainTf := h.step(chainStepYAML("event_bus: { enabled: true }", `
  - name: orders
    container_port: 3000
    events:
      subscribes:
        - { name: b, detail_type: b.x }
        - { name: s, detail_type: s.x }
      queue: { fifo_retired: true }
  - name: zeta
    container_port: 3002
    events:
      subscribes:
        - { name: q2, detail_type: q.x }
`))
	log.checkGeneric(t, "step 7")
	log.mustHave(t, "step 7", "end delete "+fifoOrders, "end delete "+sub("zeta", "q2"), "end principal-delete publish/account/000000000002")
	log.checkTeardown(t, "step 7", busID)
	if h.state.Bus || len(h.state.Subscribers) > 0 {
		t.Errorf("step 7: state still holds the bus or subscribers: %+v", h.state)
	}
	assertContains(t, "step 7 main.tf", mainTf, "# meroku:events fifo=orders retired\n")
	if !containsString(h.state.FifoQueues, "orders") {
		t.Errorf("step 7: fifo_retired did not keep the FIFO queue of orders: %+v", h.state)
	}

	// 8. Enhanced again with a share, then terraform destroy.
	log, mainTf = h.step(chainStepYAML(chainEnhancedShared, `
  - name: orders
    container_port: 3000
    events:
      subscribes:
        - { name: b, detail_type: b.x, ordered: true }
        - { name: s, detail_type: s.x }
`))
	log.checkGeneric(t, "step 8")
	busID = log.busCreated(t, "step 8")
	log = h.apply(h.fakeMain(mainTf), "-destroy")
	log.checkGeneric(t, "step 8 destroy")
	log.mustHave(t, "step 8 destroy", "end delete "+fifoOrders, "end delete "+sub("orders", "s"),
		"end principal-delete subscribe/account/000000000001", "end principal-delete publish/account/000000000002")
	log.checkTeardown(t, "step 8 destroy", busID)
}
