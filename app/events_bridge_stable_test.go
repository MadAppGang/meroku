package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// local.bridge is an output of the environment, so a key that comes and goes
// is a plan change of its own. Leaving the enhanced bus takes two generates:
// the first renders the awscc cleanup, the second (after that apply) drops
// it. The second must not change _bridge.tf, or its plan is never empty.
func TestEventsBridgeStableAfterLeavingEnhanced(t *testing.T) {
	yamlFor := func(eventBus string) string {
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
  - name: orders
    container_port: 3000
    events:
      subscribes:
        - { name: created, detail_type: order.created }
`
	}
	enhanced := yamlFor("event_bus: { enabled: true, type: enhanced }")
	deployedEnhanced := func() eventsAwsccState {
		s := emptyEventsState()
		s.Bus = true
		s.BusARN = fakeBusARN
		s.Consumers = []string{"orders"}
		s.Subscribers = map[string]string{
			"event_subscriber_orders_created": eventSubscriberName("myapp", "dev", "orders", "created"),
		}
		return s
	}
	cleanedUp := func() eventsAwsccState {
		s := emptyEventsState()
		s.Consumers = []string{"orders"}
		return s
	}

	for _, tc := range []struct {
		name, eventBus string
	}{
		{"to classic", "event_bus: { enabled: true }"},
		{"to off", "event_bus: { enabled: false }"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newEventsRenderRoot(t)

			stubEventsState(t, emptyEventsState)
			_, liveBridge := root.mustRender(t, enhanced)
			assertContains(t, "enhanced _bridge.tf", liveBridge, "    event_fifo_queue_urls = module.events.fifo_queue_urls\n")

			stubEventsState(t, deployedEnhanced)
			cleanupMain, cleanupBridge := root.mustRender(t, yamlFor(tc.eventBus))
			assertContains(t, "cleanup main.tf", cleanupMain, "# meroku:events awscc=cleanup\n")

			stubEventsState(t, cleanedUp)
			offMain, offBridge := root.mustRender(t, yamlFor(tc.eventBus))
			assertNotContains(t, "main.tf after cleanup", offMain, "awscc")
			if offBridge != cleanupBridge {
				t.Errorf("_bridge.tf changed on the generate after the cleanup apply:\n%s", eventsClassicFirstDiff(cleanupBridge, offBridge))
			}

			_, againBridge := root.mustRender(t, yamlFor(tc.eventBus))
			if againBridge != offBridge {
				t.Errorf("_bridge.tf changed on a later generate:\n%s", eventsClassicFirstDiff(offBridge, againBridge))
			}
			assertNotContains(t, "_bridge.tf after leaving enhanced", offBridge, "event_fifo_queue_urls")
		})
	}

	// A FIFO set kept through the move (fifo_retired) keeps its URLs in the
	// bridge on both generates.
	t.Run("to classic with a retired FIFO set", func(t *testing.T) {
		classic := readEventsEnhancedFixture(t, "classic")
		root := newEventsRenderRoot(t)
		stubEventsState(t, func() eventsAwsccState {
			s := emptyEventsState()
			s.Bus = true
			s.BusARN = fakeBusARN
			s.FifoQueues = []string{"orders"}
			s.Consumers = []string{"backend", "billing", "orders"}
			s.Subscribers = map[string]string{
				"event_fifo_subscriber_orders": eventFifoSubscriberName("myapp", "dev", "orders"),
				"event_subscriber_billing_t":   eventSubscriberName("myapp", "dev", "billing", "t"),
			}
			return s
		})
		_, cleanupBridge := root.mustRender(t, classic)
		stubEventsState(t, func() eventsAwsccState {
			s := emptyEventsState()
			s.FifoQueues = []string{"orders"}
			s.Consumers = []string{"backend", "billing", "orders"}
			return s
		})
		_, offBridge := root.mustRender(t, classic)
		assertContains(t, "_bridge.tf", offBridge, "    event_fifo_queue_urls = module.events.fifo_queue_urls\n")
		if offBridge != cleanupBridge {
			t.Errorf("_bridge.tf changed on the generate after the cleanup apply:\n%s", eventsClassicFirstDiff(cleanupBridge, offBridge))
		}
	})

	// An environment that was never enhanced keeps today's bridge, byte for
	// byte, and has no FIFO key.
	t.Run("never enhanced", func(t *testing.T) {
		body, err := os.ReadFile(filepath.Join(eventsClassicDir, "bus_on_consumers.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join(eventsClassicDir, "bus_on_consumers.bridge.tf.golden"))
		if err != nil {
			t.Fatal(err)
		}
		root := newEventsRenderRoot(t)
		stubEventsState(t, noLockEventsState)
		_, bridge := root.mustRender(t, string(body))
		if bridge != string(want) {
			t.Errorf("_bridge.tf differs from the classic golden:\n%s", eventsClassicFirstDiff(string(want), bridge))
		}
		if strings.Contains(bridge, "event_fifo_queue_urls") {
			t.Error("a never-enhanced classic bridge has event_fifo_queue_urls")
		}
	})
}
