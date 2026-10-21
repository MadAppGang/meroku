package main

import (
	"strings"
	"testing"
)

// The YAML -> HCL link for an event-processor task's container_command.
//
// EventProcessorTask.ContainerCommand is a []string, but the template rendered
// it through a raw stache (`{{{ container_command }}}`) rather than the `array`
// helper. Raymond stringifies a slice by concatenating its elements with no
// separator, so ["npm","run","cron"] reached main.tf as `npmruncron` — not a
// list, not quoted, and not valid HCL. Nothing caught it because the failure
// surfaces at `terraform` time, several steps removed from the YAML that caused
// it.
//
// This asserts the rendered value is a real HCL list. The `array` helper panics
// on a non-slice, so a type regression on the Go side fails loudly at render
// instead of silently emitting garbage again.

// eventBusTaskBlock returns the body of `module "event_bus_task_<name>" { ... }`.
func eventBusTaskBlock(t *testing.T, rendered, name string) string {
	t.Helper()

	header := `module "event_bus_task_` + name + `" {`
	start := strings.Index(rendered, header)
	if start < 0 {
		t.Fatalf("%s not found in the rendered template", header)
	}
	rest := rendered[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatalf("%s block is unterminated", header)
	}
	return rest[:end]
}

func TestEventProcessorContainerCommandRendersAsHCLList(t *testing.T) {
	overlay := map[string]interface{}{
		"event_processor_tasks": []interface{}{
			map[string]interface{}{
				"name":              "billing",
				"docker_image":      "public.ecr.aws/docker/library/alpine:latest",
				"container_command": []interface{}{"npm", "run", "cron"},
				"rule_name":         "billing_rule",
				"sources":           []interface{}{"custom.billing"},
				"detail_types":      []interface{}{"invoice.created"},
			},
		},
	}

	block := eventBusTaskBlock(t, renderMainHBS(t, overlay), "billing")

	const want = `container_command = ["npm","run","cron"]`
	if !strings.Contains(block, want) {
		t.Errorf("event task container_command did not render as an HCL list.\nwant substring: %s\ngot block:\n%s", want, block)
	}

	// The specific shape of the old bug: elements concatenated into one bare
	// token. Asserted separately so a regression names itself.
	if strings.Contains(block, "npmruncron") {
		t.Errorf("container_command rendered as a concatenated bare token (the raw-stache bug):\n%s", block)
	}
}

// An unset command must omit the argument entirely so the module default (and
// therefore the image's own ENTRYPOINT) still applies. Rendering an explicit []
// would override it.
func TestEventProcessorContainerCommandOmittedWhenUnset(t *testing.T) {
	overlay := map[string]interface{}{
		"event_processor_tasks": []interface{}{
			map[string]interface{}{
				"name":         "billing",
				"docker_image": "public.ecr.aws/docker/library/alpine:latest",
				"rule_name":    "billing_rule",
				"sources":      []interface{}{"custom.billing"},
				"detail_types": []interface{}{"invoice.created"},
			},
		},
	}

	block := eventBusTaskBlock(t, renderMainHBS(t, overlay), "billing")

	if strings.Contains(block, "container_command") {
		t.Errorf("container_command should be absent when unset, got block:\n%s", block)
	}
}
