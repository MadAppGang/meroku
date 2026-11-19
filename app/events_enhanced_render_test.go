package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// Render tests for the enhanced project bus: what env/main.hbs writes for an
// enhanced env, and for every way of leaving one. Fixtures are in
// testdata/events_enhanced/ (synthetic: project myapp, env dev, account
// 000000000000). The state probe is stubbed, so no Terraform runs.

const eventsEnhancedDir = "testdata/events_enhanced"

func readEventsEnhancedFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(eventsEnhancedDir, name+".yaml"))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return string(data)
}

// emptyEventsState is a readable state with nothing of the events modules in it.
func emptyEventsState() eventsAwsccState {
	return eventsAwsccState{Readable: true, Pulled: true, Tried: true, Subscribers: map[string]string{}}
}

// noLockEventsState is a state the probe did not read: a classic env with
// nothing of events in its markers or YAML, and no awscc in its lock file.
func noLockEventsState() eventsAwsccState { return eventsAwsccState{} }

// unreadableEventsState is a read that failed.
func unreadableEventsState() eventsAwsccState {
	return eventsAwsccState{Tried: true, Err: "terraform state pull: exit status 1: Error: SSO session expired"}
}

func assertContains(t *testing.T, label, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("%s does not contain %q", label, want)
		}
	}
}

func assertNotContains(t *testing.T, label, text string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(text, u) {
			t.Errorf("%s contains %q", label, u)
		}
	}
}

// subscriberChain returns the subscriber module blocks of a main.tf in file
// order.
func subscriberChain(mainTf string) []string {
	var out []string
	for _, b := range hclTopBlocks(mainTf, `module "event_`) {
		name := hclModuleName(b)
		if strings.HasPrefix(name, eventSubscriberModulePrefix) || strings.HasPrefix(name, eventFifoSubscriberModulePrefix) {
			out = append(out, b)
		}
	}
	return out
}

// assertChained checks that the blocks are chained in file order: the first
// has no depends_on, every other depends on the block before it.
func assertChained(t *testing.T, blocks []string) {
	t.Helper()
	for i, b := range blocks {
		deps := regexp.MustCompile(`(?m)^  depends_on = \[module\.([A-Za-z0-9_-]+)\]$`).FindStringSubmatch(b)
		switch {
		case i == 0 && deps != nil:
			t.Errorf("%s is first in the chain but depends on %s", hclModuleName(b), deps[1])
		case i > 0 && (deps == nil || deps[1] != hclModuleName(blocks[i-1])):
			t.Errorf("%s must depend on %s, the module before it", hclModuleName(b), hclModuleName(blocks[i-1]))
		}
	}
}

func moduleNames(blocks []string) []string {
	names := make([]string, 0, len(blocks))
	for _, b := range blocks {
		names = append(names, hclModuleName(b))
	}
	return names
}

func TestEnhancedEventsRender(t *testing.T) {
	yamlBody := readEventsEnhancedFixture(t, "live")
	root := newEventsRenderRoot(t)
	stubEventsState(t, emptyEventsState)
	mainTf, bridgeTf := root.mustRender(t, yamlBody)

	assertContains(t, "main.tf", mainTf,
		"    awscc = {\n      source  = \"hashicorp/awscc\"\n      version = \"~> 1.110\"\n    }\n",
		"provider \"awscc\" {\n  region = \"ap-southeast-2\"\n}\n",
		"# meroku:events awscc=live\n",
		"# meroku:events ordered=orders/a\n",
		"# meroku:events ordered=orders/b\n",
		"# meroku:events fifo=orders\n",
		"# meroku:events share=account:000000000001:subscribe\n",
		"  bus_enabled = true\n  bus_type         = \"enhanced\"\n  enhanced_bus_arn = module.event_bus_enhanced.bus_arn\n",
	)
	assertNotContains(t, "main.tf", mainTf, "aws_cloudwatch_event_bus")

	bus := hclTopBlocks(mainTf, `module "event_bus_enhanced" {`)
	if len(bus) != 1 {
		t.Fatalf("want one module \"event_bus_enhanced\", got %d", len(bus))
	}
	assertContains(t, "module event_bus_enhanced", bus[0],
		"  enabled = true\n",
		`  share   = [{"access":"publish","id":"000000000002","kind":"account"},{"access":"subscribe","id":"000000000001","kind":"account"}]`,
		`  revoke  = {"enabled":true,"granted":["account:000000000001"],"keep_accounts":["000000000001"],"keep_org":false,"keep_ous":[],"lost":[]}`,
		"  guard   = true\n",
	)

	// Every workload module gets the bus ARN beside EVENT_BUS_NAME.
	arnLine := "  event_bus_name = module.events.bus_name\n  event_bus_arn  = module.event_bus_enhanced.bus_arn\n"
	for _, header := range []string{`module "workloads" {`, `module "schedule_task_cleanup" {`, `module "event_bus_task_mailer" {`} {
		blocks := hclTopBlocks(mainTf, header)
		if len(blocks) != 1 {
			t.Fatalf("want one %s, got %d", header, len(blocks))
		}
		assertContains(t, header, blocks[0], arnLine)
	}

	// One standard subscriber per unordered project-bus subscription, one FIFO
	// subscriber for the ordered ones, chained in sorted order (nothing is in
	// state yet, so all are new).
	chain := subscriberChain(mainTf)
	wantChain := []string{
		"event_fifo_subscriber_orders",
		"event_subscriber_backend_user-created",
		"event_subscriber_billing_t",
		"event_subscriber_orders_s",
	}
	if got := moduleNames(chain); !reflect.DeepEqual(got, wantChain) {
		t.Fatalf("subscriber chain = %v, want %v", got, wantChain)
	}
	assertChained(t, chain)
	assertContains(t, "event_fifo_subscriber_orders", chain[0],
		"  kind         = \"fifo\"\n",
		"  subscription = module.events.fifo_subscribers[\"orders\"]\n",
		"  bus_arn      = module.event_bus_enhanced.bus_arn\n",
	)
	assertContains(t, "event_subscriber_orders_s", chain[3],
		"  kind         = \"standard\"\n",
		"  subscription = module.events.subscribers[\"orders/s\"]\n",
	)
	for _, b := range chain {
		assertNotContains(t, hclModuleName(b), b, "ordered = ", "enabled = false")
	}
	assertContains(t, "module events", hclTopBlocks(mainTf, `module "events" {`)[0], `"key":"a","ordered":true`)

	assertContains(t, "_bridge.tf", bridgeTf, "    event_fifo_queue_urls = module.events.fifo_queue_urls\n")
}

// A subscriber module new to state renders before every module that is in
// state; a deployed subscriber that is no longer wanted is a tombstone at the
// head of the chain.
func TestEnhancedEventsRenderChainGroups(t *testing.T) {
	yamlBody := readEventsEnhancedFixture(t, "live")
	root := newEventsRenderRoot(t)
	stubEventsState(t, func() eventsAwsccState {
		s := emptyEventsState()
		s.Bus = true
		s.Subscribers = map[string]string{
			"event_subscriber_backend_user-created": eventSubscriberName("myapp", "dev", "backend", "user-created"),
			"event_subscriber_orders_s":             eventSubscriberName("myapp", "dev", "orders", "s"),
			"event_subscriber_orders_gone":          eventSubscriberName("myapp", "dev", "orders", "gone"),
		}
		return s
	})
	mainTf, _ := root.mustRender(t, yamlBody)

	chain := subscriberChain(mainTf)
	want := []string{
		"event_subscriber_orders_gone",          // tombstone
		"event_fifo_subscriber_orders",          // new
		"event_subscriber_billing_t",            // new
		"event_subscriber_backend_user-created", // in state
		"event_subscriber_orders_s",             // in state
	}
	if got := moduleNames(chain); !reflect.DeepEqual(got, want) {
		t.Fatalf("subscriber chain = %v, want %v", got, want)
	}
	assertChained(t, chain)
	assertContains(t, "tombstone", chain[0], "  enabled = false\n", "  kind    = \"standard\"\n", "  bus_arn = module.event_bus_enhanced.bus_arn\n")
	assertNotContains(t, "tombstone", chain[0], "subscription")
	assertContains(t, "main.tf", mainTf, "# meroku:events tombstone=event_subscriber_orders_gone\n")
}

// Leaving the enhanced bus: the bus and every subscriber are deleted through
// cleanup mode, and only while something of them is left.
func TestEnhancedEventsReverseTransitionRender(t *testing.T) {
	deployed := func() eventsAwsccState {
		s := emptyEventsState()
		s.Bus = true
		s.Guard = true
		s.FifoQueues = []string{"orders"}
		s.Consumers = []string{"backend", "billing", "orders"}
		s.Subscribers = map[string]string{
			"event_fifo_subscriber_orders": eventFifoSubscriberName("myapp", "dev", "orders"),
			"event_subscriber_billing_t":   eventSubscriberName("myapp", "dev", "billing", "t"),
		}
		return s
	}
	classic := readEventsEnhancedFixture(t, "classic")
	off := strings.Replace(classic, "event_bus: { enabled: true }", "event_bus: { enabled: false }", 1)
	if off == classic {
		t.Fatal("classic fixture no longer has the bus line the off variant edits")
	}

	assertCleanup := func(t *testing.T, mainTf string, tombstones []string) {
		t.Helper()
		assertContains(t, "main.tf", mainTf,
			"      source  = \"hashicorp/awscc\"\n",
			"provider \"awscc\" {\n  region = \"ap-southeast-2\"\n}\n",
			"# meroku:events awscc=cleanup\n",
		)
		assertNotContains(t, "main.tf", mainTf, "bus_type", "event_bus_arn  = module.event_bus_enhanced", "subscription =", "event_fifo_queue_urls =")
		bus := hclTopBlocks(mainTf, `module "event_bus_enhanced" {`)
		if len(bus) != 1 {
			t.Fatalf("want one module \"event_bus_enhanced\", got %d", len(bus))
		}
		assertContains(t, "module event_bus_enhanced", bus[0], "  enabled = false\n", "  share   = []\n", "  guard   = false\n", `"enabled":false`)
		chain := subscriberChain(mainTf)
		if got := moduleNames(chain); !reflect.DeepEqual(got, tombstones) {
			t.Fatalf("tombstones = %v, want %v", got, tombstones)
		}
		assertChained(t, chain)
		for _, b := range chain {
			assertContains(t, hclModuleName(b), b, "  enabled = false\n")
		}
	}

	t.Run("to classic, state lists the bus and two subscribers", func(t *testing.T) {
		root := newEventsRenderRoot(t)
		stubEventsState(t, deployed)
		mainTf, _ := root.mustRender(t, classic)
		assertCleanup(t, mainTf, []string{"event_fifo_subscriber_orders", "event_subscriber_billing_t"})
		assertContains(t, "main.tf", mainTf, "# meroku:events fifo=orders retired\n")
	})

	t.Run("to off, state lists the bus and two subscribers", func(t *testing.T) {
		root := newEventsRenderRoot(t)
		stubEventsState(t, deployed)
		mainTf, _ := root.mustRender(t, off)
		assertCleanup(t, mainTf, []string{"event_fifo_subscriber_orders", "event_subscriber_billing_t"})
		assertContains(t, "main.tf", mainTf, "  bus_enabled = false\n")
	})

	// The markers record a render, not an apply, so they never stand in for
	// the state on a render that writes main.tf.
	t.Run("unreadable state with awscc markers is refused", func(t *testing.T) {
		root := newEventsRenderRoot(t)
		stubEventsState(t, emptyEventsState)
		first, _ := root.mustRender(t, readEventsEnhancedFixture(t, "live"))
		stubEventsState(t, unreadableEventsState)
		_, _, _, err := root.render(t, classic)
		if err == nil {
			t.Fatal("generate wrote main.tf from markers while the state could not be read")
		}
		if !errors.Is(err, errEventsStateUnreadable) {
			t.Errorf("the refusal does not carry errEventsStateUnreadable: %v", err)
		}
		assertContains(t, "error", err.Error(), "could not be read (terraform state pull: exit status 1: Error: SSO session expired)", "aws sso login --profile")
		kept, readErr := os.ReadFile(filepath.Join(root.dir, "env", "dev", "main.tf"))
		if readErr != nil || string(kept) != first {
			t.Error("the refused render changed main.tf")
		}
	})

	// With nothing of awscc left, a classic env renders as if it had never
	// been enhanced: the classic golden, byte for byte.
	golden := func(t *testing.T) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(eventsClassicDir, "bus_on_consumers.main.tf.golden"))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	classicFixture := func(t *testing.T) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(eventsClassicDir, "bus_on_consumers.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	t.Run("readable state without awscc renders the classic golden", func(t *testing.T) {
		want, body := golden(t), classicFixture(t)
		root := newEventsRenderRoot(t)
		stubEventsState(t, emptyEventsState)
		mainTf, _ := root.mustRender(t, body)
		if mainTf != want {
			t.Errorf("main.tf differs from the classic golden:\n%s", eventsClassicFirstDiff(want, mainTf))
		}
		assertNotContains(t, "main.tf", mainTf, "awscc")
	})

	t.Run("unreadable state without markers renders the classic golden", func(t *testing.T) {
		want, body := golden(t), classicFixture(t)
		root := newEventsRenderRoot(t)
		stubEventsState(t, noLockEventsState)
		mainTf, _ := root.mustRender(t, body)
		if mainTf != want {
			t.Errorf("main.tf differs from the classic golden:\n%s", eventsClassicFirstDiff(want, mainTf))
		}
	})
}

// The markers a render writes parse back to what that render was, and a
// second render of the same YAML over the same state is byte-identical.
func TestEventMarkersRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		yaml  string
		state func() eventsAwsccState
		want  eventPrevious
	}{
		{
			name:  "live and shared",
			yaml:  readEventsEnhancedFixture(t, "live"),
			state: emptyEventsState,
			want: eventPrevious{
				Mode: awsccModeLive,
				Subscribers: []string{
					"event_fifo_subscriber_orders",
					"event_subscriber_backend_user-created",
					"event_subscriber_billing_t",
					"event_subscriber_orders_s",
				},
				Ordered: []string{"orders/a", "orders/b"},
				Fifo:    map[string]bool{"orders": false},
				Share:   []string{"account:000000000001:subscribe", "account:000000000002:publish"},
				Guard:   true,
			},
		},
		{
			name: "cleanup",
			yaml: readEventsEnhancedFixture(t, "classic"),
			state: func() eventsAwsccState {
				s := emptyEventsState()
				s.Bus = true
				s.FifoQueues = []string{"orders"}
				s.Subscribers = map[string]string{"event_subscriber_billing_t": eventSubscriberName("myapp", "dev", "billing", "t")}
				return s
			},
			want: eventPrevious{
				Mode:       awsccModeCleanup,
				Tombstones: []string{"event_subscriber_billing_t"},
				Fifo:       map[string]bool{"orders": true},
			},
		},
		{
			name: "classic with a retired FIFO set",
			yaml: readEventsEnhancedFixture(t, "classic"),
			state: func() eventsAwsccState {
				s := emptyEventsState()
				s.FifoQueues = []string{"orders"}
				return s
			},
			want: eventPrevious{Fifo: map[string]bool{"orders": true}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newEventsRenderRoot(t)
			stubEventsState(t, tc.state)
			first, _ := root.mustRender(t, tc.yaml)

			got := parseEventMarkers([]byte(first))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("markers parse to\n  %+v\nwant\n  %+v", got, tc.want)
			}
			// Every marker line is one the parser reads.
			for _, line := range strings.Split(first, "\n") {
				if strings.HasPrefix(line, eventMarkerPrefix) && !eventMarkerPattern.MatchString(line) {
					t.Errorf("marker line %q does not parse", line)
				}
			}
			if tc.want.Mode == "" {
				assertNotContains(t, "main.tf", first, "awscc")
			}

			second, _ := root.mustRender(t, tc.yaml)
			if second != first {
				t.Errorf("a second render differs:\n%s", eventsClassicFirstDiff(first, second))
			}
		})
	}
}
