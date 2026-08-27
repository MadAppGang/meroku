package main

import (
	"strings"
	"testing"

	"github.com/aymerick/raymond"
)

// The helper as env/main.hbs actually calls it: five strings and a truthy
// value. Arity and argument types are the easy thing to get wrong here, because
// raymond resolves helpers at render time and a mismatch surfaces as a render
// error in generated Terraform rather than at compile time.
func TestAWSNameHelperRendersFromTemplate(t *testing.T) {
	registerCustomHelpers()

	tpl := `{{awsName @root.project @root.env name "" "sqs_queue" fifo}}|` +
		`{{awsName @root.project @root.env name "dlq" "sqs_queue" fifo}}|` +
		`{{awsName @root.project @root.env name "" "sns_topic" fifo}}`

	out, err := raymond.Render(tpl, map[string]interface{}{
		"project": "myapp",
		"env":     "dev",
		"name":    "orders",
		"fifo":    true,
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	parts := strings.Split(out, "|")
	if want := "myapp-dev-orders.fifo"; parts[0] != want {
		t.Errorf("queue name = %q, want %q", parts[0], want)
	}
	if want := "myapp-dev-orders-dlq.fifo"; parts[1] != want {
		t.Errorf("dlq name = %q, want %q", parts[1], want)
	}
	if want := "myapp-dev-orders.fifo"; parts[2] != want {
		t.Errorf("topic name = %q, want %q", parts[2], want)
	}
}

// A non-FIFO queue must not pick up the suffix.
func TestAWSNameHelperOmitsSuffixWhenNotFifo(t *testing.T) {
	registerCustomHelpers()

	out, err := raymond.Render(
		`{{awsName @root.project @root.env name "" "sqs_queue" fifo}}`,
		map[string]interface{}{
			"project": "myapp", "env": "dev", "name": "orders", "fifo": false,
		})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if out != "myapp-dev-orders" {
		t.Errorf("got %q, want myapp-dev-orders", out)
	}
}

// `fifo` is frequently absent from the YAML entirely. Absent must behave as
// false, not panic and not render "<nil>".
func TestAWSNameHelperHandlesMissingFifoKey(t *testing.T) {
	registerCustomHelpers()

	out, err := raymond.Render(
		`{{awsName @root.project @root.env name "" "sqs_queue" fifo}}`,
		map[string]interface{}{
			"project": "myapp", "env": "dev", "name": "orders",
		})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if out != "myapp-dev-orders" {
		t.Errorf("got %q, want myapp-dev-orders", out)
	}
}

// An unknown kind must render a marker loud enough to fail terraform validate,
// rather than silently emitting a name with no cap applied.
func TestAWSNameHelperRejectsUnknownKind(t *testing.T) {
	registerCustomHelpers()

	out, err := raymond.Render(
		`{{awsName @root.project @root.env name "" "bogus_kind" fifo}}`,
		map[string]interface{}{
			"project": "myapp", "env": "dev", "name": "orders", "fifo": false,
		})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "INVALID_AWS_NAME_KIND") {
		t.Errorf("got %q, want a loud marker", out)
	}
}

// A queue name long enough to blow the 80-character cap must lose the project
// and env, keep the identity, and keep ".fifo".
func TestAWSNameHelperCapsLongQueueName(t *testing.T) {
	registerCustomHelpers()

	out, err := raymond.Render(
		`{{awsName @root.project @root.env name "" "sqs_queue" fifo}}`,
		map[string]interface{}{
			"project": "an-extremely-long-project-name-for-testing",
			"env":     "production",
			"name":    "orders-reconciliation-batch-processor-queue",
			"fifo":    true,
		})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(out) > 80 {
		t.Errorf("got %q (%d chars), over the 80-character SQS cap", out, len(out))
	}
	if !strings.HasSuffix(out, ".fifo") {
		t.Errorf("got %q, must keep .fifo", out)
	}
	if !strings.HasPrefix(out, "orders-reconciliation") {
		t.Errorf("got %q, the identity must lead", out)
	}
}
