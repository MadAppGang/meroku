package main

import (
	"regexp"
	"strings"
	"testing"
)

// A topic in extensions.sns_topics with add_to_backend_env hands its ARN to the
// backend. The backend's task role must then be allowed to publish to it, which
// env/main.hbs does by passing the same ARNs as backend_sns_publish_topic_arns
// (modules/workloads/sns.tf). These tests pin that every injected topic ARN has
// its grant, and that no grant appears for a topic the backend is not given.

var snsPublishArg = regexp.MustCompile(`(?s)backend_sns_publish_topic_arns = \[(.*?)\]`)

func snsTopicsOverlay(topics ...map[string]interface{}) map[string]interface{} {
	list := make([]interface{}, 0, len(topics))
	for _, t := range topics {
		list = append(list, t)
	}
	return map[string]interface{}{"extensions": map[string]interface{}{"sns_topics": list}}
}

func TestTemplate_BackendMayPublishToTopicsInItsEnv(t *testing.T) {
	out := renderMainHBS(t, snsTopicsOverlay(
		map[string]interface{}{"name": "orders", "add_to_backend_env": "ORDERS_TOPIC_ARN"},
		map[string]interface{}{"name": "audit", "fifo": true, "content_based_dedup": false, "add_to_backend_env": "AUDIT_TOPIC_ARN"},
		map[string]interface{}{"name": "internal"},
	))
	if err := validateGeneratedHCL("env/dev/main.tf", out); err != nil {
		t.Fatalf("generated terraform does not parse:\n%v", err)
	}

	m := snsPublishArg.FindStringSubmatch(out)
	if m == nil {
		t.Fatal("backend_sns_publish_topic_arns is not passed to module.workloads")
	}
	for _, want := range []string{"aws_sns_topic.ext_orders.arn", "aws_sns_topic.ext_audit.arn"} {
		if !strings.Contains(m[1], want) {
			t.Errorf("backend_sns_publish_topic_arns lacks %s, whose ARN the backend receives:\n%s", want, m[0])
		}
	}
	if strings.Contains(m[1], "ext_internal") {
		t.Errorf("a topic without add_to_backend_env must get no publish grant:\n%s", m[0])
	}

	// Every topic ARN in the backend's environment must be in the grant.
	injected := regexp.MustCompile(`value = (aws_sns_topic\.ext_\w+\.arn)`).FindAllStringSubmatch(out, -1)
	if len(injected) != 2 {
		t.Fatalf("expected 2 topic ARNs in backend_env, found %d", len(injected))
	}
	for _, inj := range injected {
		if !strings.Contains(m[1], inj[1]) {
			t.Errorf("%s is injected into the backend's environment with no publish grant", inj[1])
		}
	}
}

func TestTemplate_NoTopicsInBackendEnvNoPublishGrant(t *testing.T) {
	out := renderMainHBS(t, snsTopicsOverlay(
		map[string]interface{}{"name": "internal"},
	))
	if err := validateGeneratedHCL("env/dev/main.tf", out); err != nil {
		t.Fatalf("generated terraform does not parse:\n%v", err)
	}
	if m := snsPublishArg.FindStringSubmatch(out); m != nil && strings.TrimSpace(m[1]) != "" {
		t.Errorf("no topic sets add_to_backend_env, yet a grant is rendered:\n%s", m[0])
	}

	// An environment without extension topics renders module.workloads exactly
	// as before.
	if strings.Contains(renderMainHBS(t, map[string]interface{}{}), "backend_sns_publish_topic_arns") {
		t.Error("an environment without extensions.sns_topics must not render backend_sns_publish_topic_arns")
	}
}
