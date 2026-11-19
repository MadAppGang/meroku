package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func blindBusYAML(region, bus, backendEvents, extras string) string {
	return "schema_version: 33\nproject: myapp\nenv: dev\nis_prod: false\nregion: " + region + `
account_id: "000000000000"
aws_profile: myapp-dev
state_bucket: myapp-terraform-state-dev
state_file: state.tfstate
state_lock_table: myapp-terraform-locks-dev
use_default_vpc: false
vpc_cidr: 10.20.0.0/16
egress_strategy: public_ip
ecr_strategy: local
domain:
  enabled: true
  create_domain_zone: true
  domain_name: example.com
workload:
  backend_image_port: 8080
  backend_cpu: "256"
  backend_memory: "512"
  backend_desired_count: 1
` + backendEvents + "\nevent_bus: " + bus + "\n" + extras
}

// REQ-1, REQ-9; TEST-1.
func TestBlindClassicBusIsTheDefault(t *testing.T) {
	implicitRoot, explicitRoot := newEventsRenderRoot(t), newEventsRenderRoot(t)
	implicit, _ := implicitRoot.mustRender(t, blindBusYAML("ap-southeast-2", "{enabled: true}", "", ""))
	explicit, _ := explicitRoot.mustRender(t, blindBusYAML("ap-southeast-2", "{enabled: true, type: classic}", "", ""))
	if implicit != explicit {
		t.Error("explicit classic changed main.tf relative to the implicit classic bus")
	}
	for _, forbidden := range []string{"awscc", "# meroku:events", "event_bus_enhanced", "event_fifo_queue_urls"} {
		if strings.Contains(implicit, forbidden) {
			t.Errorf("classic main.tf contains %q", forbidden)
		}
	}
}

// REQ-2, REQ-3, REQ-5, REQ-7, REQ-9; TEST-2.
func TestBlindEnhancedRenderPartitionsSubscriptions(t *testing.T) {
	main, _ := newEventsRenderRoot(t).mustRender(t, blindBusYAML("ap-southeast-2", "{enabled: true, type: enhanced}", `  events:
    subscribes:
      - {name: jobs, detail_type: "job.*"}
      - {name: ledger, detail_type_prefix: "ledger.", ordered: true}
      - {name: payouts, detail_type: "*.paid", ordered: true}
      - {name: ecs, detail_type: "ECS Task State Change", source: aws.ecs, bus: default}
`, ""))
	for _, want := range []string{
		`source\s*=\s*"hashicorp/awscc"`,
		`version\s*=\s*"~> 1\.110"`,
		`provider "awscc"\s*\{[^}]*region\s*=\s*"ap-southeast-2"`,
		`module "event_bus_enhanced"\s*\{[^}]*enabled\s*=\s*true`,
		`event_bus_arn\s*=\s*module\.event_bus_enhanced\.bus_arn`,
		`# meroku:events awscc=live`,
		`# meroku:events ordered=backend/ledger`,
		`# meroku:events ordered=backend/payouts`,
	} {
		if !regexp.MustCompile(want).MatchString(main) {
			t.Errorf("main.tf is missing %s", want)
		}
	}
	standard := hclTopBlocks(main, `module "event_subscriber_`)
	if len(standard) != 1 || hclModuleName(standard[0]) != "event_subscriber_backend_jobs" {
		t.Errorf("want only the jobs standard subscriber, got %v", standard)
	}
	fifo := hclTopBlocks(main, `module "event_fifo_subscriber_`)
	if len(fifo) != 1 || hclModuleName(fifo[0]) != "event_fifo_subscriber_backend" ||
		!regexp.MustCompile(`kind\s*=\s*"fifo"`).MatchString(fifo[0]) {
		t.Errorf("want one shared backend FIFO subscriber, got %v", fifo)
	}
}

// REQ-1, REQ-3, REQ-7, REQ-8, REQ-10; TEST-3.
func TestBlindEventsRefusalsDoNotWriteMain(t *testing.T) {
	const classic = "{enabled: true}"
	const enhanced = "{enabled: true, type: enhanced}"
	const entry = "  events:\n    subscribes:\n      - "
	for _, tc := range []struct{ name, region, bus, events, extra, want string }{
		{"unknown kind", "ap-southeast-2", "{enabled: true, type: other}", "", "", "event_bus.type must be classic or enhanced, got other"},
		{"disabled enhanced", "ap-southeast-2", "{enabled: false, type: enhanced}", "", "", "event_bus.type is enhanced, but event_bus.enabled is not true"},
		{"unsupported region", "sa-east-1", enhanced, "", "", "event_bus.type enhanced is not available in sa-east-1"},
		{"share on classic", "ap-southeast-2", "{enabled: true, share: [{organization: true, access: publish}]}", "", "", "event_bus.share needs event_bus.type: enhanced"},
		{"numeric share account", "ap-southeast-2", "{enabled: true, type: enhanced, share: [{account: 111111111111, access: publish}]}", "", "", "an AWS account id is 12 digits, written in quotes"},
		{"own share account", "ap-southeast-2", "{enabled: true, type: enhanced, share: [{account: \"000000000000\", access: publish}]}", "", "", "names this environment's own account"},
		{"missing share access", "ap-southeast-2", "{enabled: true, type: enhanced, share: [{organization: true}]}", "", "", "access must be publish, subscribe or full"},
		{"ordered classic", "ap-southeast-2", classic, entry + "{detail_type: ledger, ordered: true}\n", "", "ordered needs the enhanced project bus"},
		{"ordered default", "ap-southeast-2", enhanced, entry + "{detail_type: ledger, bus: default, ordered: true}\n", "", "ordered is on the default bus, which cannot keep order"},
		{"infix wildcard", "ap-southeast-2", enhanced, entry + "{detail_type: \"job.*.done\"}\n", "", "uses * in a way the enhanced bus cannot match"},
		{"lone wildcard", "ap-southeast-2", enhanced, entry + "{detail_type: \"*\"}\n", "", "uses * in a way the enhanced bus cannot match"},
		{"retired and ordered", "ap-southeast-2", enhanced, entry + "{detail_type: ledger, ordered: true}\n    queue: {fifo_retired: true}\n", "", "fifo_retired keeps a queue that no subscription fills"},
		{"project task rule", "ap-southeast-2", enhanced, "", "event_processor_tasks:\n  - name: mailer\n    rules:\n      - {name: r1, bus: project, sources: [myapp.backend], detail_types: [job.done]}\n", "an enhanced bus cannot start ECS tasks"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newEventsRenderRoot(t)
			_, _, output, err := root.render(t, blindBusYAML(tc.region, tc.bus, tc.events, tc.extra))
			matches := err != nil && strings.Contains(err.Error(), tc.want)
			if tc.name == "unknown kind" && err != nil {
				// E1 specifies the rejected value, not how it is quoted.
				matches = regexp.MustCompile(`(?m)^\s*event_bus\.type must be classic or enhanced, got (other|"other")\s*$`).MatchString(err.Error())
			}
			if !matches {
				t.Fatalf("want generate error containing %q, got %v\n%s", tc.want, err, output)
			}
			if _, err := os.Stat(filepath.Join(root.dir, "env", "dev", "main.tf")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("refused generate must leave main.tf absent, stat error: %v", err)
			}
		})
	}
}

// REQ-7, REQ-10; TEST-4.
func TestBlindActiveFifoRequiresRetirementEvenWithoutEventsKey(t *testing.T) {
	// A previous render is not a deployment. Return state for an applied FIFO set.
	previousReader := eventsRemoteStateReader
	t.Cleanup(func() { eventsRemoteStateReader = previousReader })
	eventsRemoteStateReader = func(context.Context, Env) ([]byte, error) {
		return []byte(`{
		  "version": 4,
		  "resources": [
		    {
		      "module": "module.event_fifo_subscriber_backend",
		      "mode": "managed",
		      "type": "awscc_eventsv2_subscriber",
		      "name": "this",
		      "instances": [{
		        "index_key": 0,
		        "attributes": {
		          "name": "myapp-backend-fifo-dev",
		          "event_bus_arn": "arn:aws:events:ap-southeast-2:000000000000:event-busv2/myapp-dev/fake-bus-id"
		        }
		      }]
		    },
		    {
		      "module": "module.events",
		      "mode": "managed",
		      "type": "aws_sqs_queue",
		      "name": "fifo_main",
		      "instances": [{
		        "index_key": "backend",
		        "attributes": {"name": "myapp-backend-events-dev.fifo"}
		      }]
		    }
		  ]
		}`), nil
	}
	const bus = "{enabled: true, type: enhanced}"
	root := newEventsRenderRoot(t)
	_, _, output, err := root.render(t, blindBusYAML("ap-southeast-2", bus, "", ""))
	if err == nil || !strings.Contains(err.Error(), "backend no longer has an ordered subscription") {
		t.Fatalf("want removal refusal for active FIFO, got %v\n%s", err, output)
	}
	retired, _ := root.mustRender(t, blindBusYAML("ap-southeast-2", bus, "  events:\n    queue: {fifo_retired: true}\n", ""))
	if !strings.Contains(retired, "# meroku:events fifo=backend retired") {
		t.Error("main.tf is missing the backend FIFO retirement marker")
	}
	match := regexp.MustCompile(`(?m)^[\t ]*consumers[\t ]*=[\t ]*(\[[^\r\n]*\])[\t ]*$`).FindStringSubmatch(retired)
	if match == nil {
		t.Fatal("main.tf is missing the rendered consumers array")
	}
	var consumers []struct {
		Name          string            `json:"name"`
		FIFO          bool              `json:"fifo"`
		FIFORetired   bool              `json:"fifo_retired"`
		Subscriptions []json.RawMessage `json:"subscriptions"`
	}
	if err := json.Unmarshal([]byte(match[1]), &consumers); err != nil {
		t.Fatalf("decode rendered consumers: %v", err)
	}
	found := false
	for _, consumer := range consumers {
		if consumer.Name != "backend" {
			continue
		}
		found = true
		if !consumer.FIFO || !consumer.FIFORetired || consumer.Subscriptions == nil || len(consumer.Subscriptions) != 0 {
			t.Errorf("backend must retain a retired FIFO set with an empty subscriptions array: %+v", consumer)
		}
	}
	if !found {
		t.Error("retirement must retain the backend consumer")
	}
	for _, block := range hclTopBlocks(retired, `module "event_fifo_subscriber_`) {
		if !regexp.MustCompile(`enabled\s*=\s*false`).MatchString(block) {
			t.Errorf("retired FIFO must not have a live subscriber: %s", block)
		}
	}
}
