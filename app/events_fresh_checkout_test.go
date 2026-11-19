package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// freshCheckoutState is an S3 state another machine left: the enhanced bus,
// shared once (the guard), with the FIFO subscriber and the queues of orders.
const freshCheckoutState = `{
  "version": 4,
  "resources": [
    {"module": "module.event_bus_enhanced", "mode": "managed", "type": "awscc_eventsv2_event_bus", "name": "project",
     "instances": [{"attributes": {"event_bus_arn": "arn:aws:events:ap-southeast-2:000000000000:event-busv2/myapp-dev/b-1"}}]},
    {"module": "module.event_bus_enhanced", "mode": "managed", "type": "terraform_data", "name": "bus_guard",
     "instances": [{"attributes": {"input": {"value": {}, "type": "object"}}}]},
    {"module": "module.event_bus_enhanced", "mode": "managed", "type": "aws_ram_principal_association", "name": "bus",
     "instances": [{"index_key": "subscribe/account/000000000001", "attributes": {}}, {"index_key": "publish/account/000000000002", "attributes": {}}]},
    {"module": "module.event_fifo_subscriber_orders", "mode": "managed", "type": "awscc_eventsv2_subscriber", "name": "this",
     "instances": [{"attributes": {"name": "myapp-events-orders-fifo", "event_bus_arn": "arn:aws:events:ap-southeast-2:000000000000:event-busv2/myapp-dev/b-1"}}]},
    {"module": "module.events", "mode": "managed", "type": "aws_sqs_queue", "name": "fifo_main",
     "instances": [{"index_key": "orders", "attributes": {}}]},
    {"module": "module.events", "mode": "managed", "type": "aws_sqs_queue", "name": "main",
     "instances": [{"index_key": "orders", "attributes": {}}]}
  ]
}`

// freshCheckoutYAML is a dev.yaml with an S3 backend, the given event_bus
// line, and orders with the given events block.
func freshCheckoutYAML(eventBus, ordersEvents string) string {
	return `schema_version: 33
project: myapp
env: dev
region: ap-southeast-2
account_id: "000000000000"
aws_profile: none
state_bucket: fake-bucket
state_file: state.tfstate
use_default_vpc: false
ecr_strategy: local
` + eventBus + `
workload:
  backend_image_port: 8080
services:
  - name: orders
    container_port: 3000
` + ordersEvents
}

const freshCheckoutSubscribes = `    events:
      subscribes:
        - { name: a, detail_type: a.x }
`

// A fresh checkout: no env/dev/main.tf, no lock file, a classic or off YAML,
// and an S3 state with the enhanced bus. The render reads the state and
// decides from it, as it would on the machine that deployed.
func TestFreshCheckoutLeavingEnhancedReadsTheState(t *testing.T) {
	remote := func(Env) ([]byte, error) { return []byte(freshCheckoutState), nil }
	noPull := func() ([]byte, error) {
		t.Error("terraform state pull ran in an uninitialised directory")
		return nil, nil
	}

	t.Run("classic, the FIFO set not retired: refused", func(t *testing.T) {
		root := newEventsRenderRoot(t)
		_, remotes := stubEventsReaders(t, noPull, remote)
		_, _, _, err := root.render(t, freshCheckoutYAML("event_bus: { enabled: true }", freshCheckoutSubscribes))
		if *remotes != 1 {
			t.Errorf("the S3 state was read %d times, want once", *remotes)
		}
		if err == nil {
			t.Fatal("generate rendered over a deployed FIFO set that is not retired")
		}
		assertContains(t, "error", err.Error(), "fifo_retired: true", eventFifoQueueName("myapp", "dev", "orders"))
	})

	t.Run("classic, the FIFO set retired: cleanup", func(t *testing.T) {
		root := newEventsRenderRoot(t)
		stubEventsReaders(t, noPull, remote)
		mainTf, _ := root.mustRender(t, freshCheckoutYAML("event_bus: { enabled: true }",
			freshCheckoutSubscribes+"      queue: { fifo_retired: true }\n"))
		assertContains(t, "main.tf", mainTf,
			"# meroku:events awscc=cleanup\n",
			"# meroku:events tombstone="+eventFifoSubscriberModule("orders")+"\n",
			"# meroku:events fifo=orders retired\n",
		)
	})

	t.Run("off, orders without events: refused", func(t *testing.T) {
		root := newEventsRenderRoot(t)
		stubEventsReaders(t, noPull, remote)
		_, _, _, err := root.render(t, freshCheckoutYAML("event_bus: { enabled: false }", ""))
		if err == nil {
			t.Fatal("generate rendered over a deployed FIFO set that is not retired")
		}
		assertContains(t, "error", err.Error(), "fifo_retired: true")
	})

	t.Run("off, the FIFO set retired: cleanup", func(t *testing.T) {
		root := newEventsRenderRoot(t)
		stubEventsReaders(t, noPull, remote)
		mainTf, _ := root.mustRender(t, freshCheckoutYAML("event_bus: { enabled: false }",
			"    events:\n      queue: { fifo_retired: true }\n"))
		assertContains(t, "main.tf", mainTf, "# meroku:events awscc=cleanup\n", "# meroku:events fifo=orders retired\n")
	})

	t.Run("the state cannot be read: refused", func(t *testing.T) {
		root := newEventsRenderRoot(t)
		stubEventsReaders(t, noPull, func(Env) ([]byte, error) {
			return nil, errors.New("reading s3://fake-bucket/state.tfstate: AccessDenied")
		})
		_, _, _, err := root.render(t, freshCheckoutYAML("event_bus: { enabled: true }", freshCheckoutSubscribes))
		if err == nil || !errors.Is(err, errEventsStateUnreadable) {
			t.Fatalf("err = %v, want the unreadable-state refusal", err)
		}
		assertContains(t, "error", err.Error(), "AccessDenied", "env/dev/main.tf has not been generated on this machine")
		if _, statErr := os.Stat(filepath.Join(root.dir, "env", "dev", "main.tf")); statErr == nil {
			t.Error("the refused render wrote main.tf")
		}
	})

	// Once this machine has rendered the env with no events marker, the next
	// generate stays offline.
	t.Run("rendered here without markers: offline", func(t *testing.T) {
		root := newEventsRenderRoot(t)
		stubEventsReaders(t, noPull, func(Env) ([]byte, error) { return nil, errRemoteStateAbsent })
		root.mustRender(t, freshCheckoutYAML("event_bus: { enabled: true }", freshCheckoutSubscribes))
		_, remotes := stubEventsReaders(t, noPull, remote)
		root.mustRender(t, freshCheckoutYAML("event_bus: { enabled: true }", freshCheckoutSubscribes))
		if *remotes != 0 {
			t.Errorf("the S3 state was read %d times after a render without markers, want none", *remotes)
		}
	})
}
