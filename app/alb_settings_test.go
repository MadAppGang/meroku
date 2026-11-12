package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// The optional ALB settings: alb.ssl_policy, alb.deregistration_delay,
// alb.remove_api_gateway and workload.backend_health_check_grace_period. This
// file covers the YAML -> main.tf hop, the refusals before rendering, the typed
// config round trip, and a textual guard on the module's moved blocks. The
// module half lives in modules/workloads/tests/alb_settings.tftest.hcl.

const albSettingsTLS13Policy = "ELBSecurityPolicy-TLS13-1-2-2021-06"

// albSettingsDoc returns project/dev.yaml as a string-keyed document.
func albSettingsDoc(t *testing.T) map[string]interface{} {
	t.Helper()
	base, err := os.ReadFile(filepath.Join("..", "project", "dev.yaml"))
	if err != nil {
		t.Fatalf("reading project/dev.yaml: %v", err)
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(base, &doc); err != nil {
		t.Fatalf("parsing project/dev.yaml: %v", err)
	}
	return doc
}

// albSettingsOverlay is one test's change to project/dev.yaml. alb replaces
// the whole alb section; workload keys are added to dev.yaml's own workload;
// services and extensions, when non-nil, replace those sections.
type albSettingsOverlay struct {
	alb        map[interface{}]interface{}
	workload   map[interface{}]interface{}
	services   []interface{}
	extensions map[interface{}]interface{}
}

func (o albSettingsOverlay) apply(t *testing.T, doc map[string]interface{}) {
	t.Helper()
	if o.alb != nil {
		doc["alb"] = o.alb
	}
	workload, ok := doc["workload"].(map[interface{}]interface{})
	if !ok {
		t.Fatal("project/dev.yaml has no workload section")
	}
	delete(workload, "backend_health_check_grace_period")
	for k, v := range o.workload {
		workload[k] = v
	}
	if o.services != nil {
		doc["services"] = o.services
	}
	if o.extensions != nil {
		doc["extensions"] = o.extensions
	}
}

// albSettingsPreprocess runs the real pre-processing over project/dev.yaml with
// the overlay and returns its error.
func albSettingsPreprocess(t *testing.T, o albSettingsOverlay) error {
	t.Helper()
	doc := albSettingsDoc(t)
	o.apply(t, doc)
	envMap, ok := convertToJSONCompatible(doc).(map[string]interface{})
	if !ok {
		t.Fatal("fixture yaml is not a mapping")
	}
	return preprocessEnvMap(envMap)
}

// albSettingsRender renders env/main.hbs over project/dev.yaml with the overlay.
func albSettingsRender(t *testing.T, o albSettingsOverlay) string {
	t.Helper()
	doc := albSettingsDoc(t)
	o.apply(t, doc)
	overlay := map[string]interface{}{
		"alb":      doc["alb"],
		"workload": doc["workload"],
	}
	if o.services != nil {
		overlay["services"] = doc["services"]
	}
	if o.extensions != nil {
		overlay["extensions"] = doc["extensions"]
	}
	return renderMainHBS(t, overlay)
}

// albSettingsArg matches `<name> = <value>` at the start of a line.
func albSettingsArg(name, value string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s*=\s*` + regexp.QuoteMeta(value) + `\s*$`)
}

// albSettingsHasArg reports whether `<name> =` appears as an argument anywhere.
func albSettingsHasArg(rendered, name string) bool {
	return regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s*=`).MatchString(rendered)
}

// Every new key is optional: absent keys render nothing, so main.tf is
// unchanged for every environment that sets none of them.
func TestALBSettingsAbsentKeysRenderNothing(t *testing.T) {
	out := albSettingsRender(t, albSettingsOverlay{})

	for _, name := range []string{"alb_ssl_policy", "alb_deregistration_delay", "remove_api_gateway", "backend_health_check_grace_period"} {
		if albSettingsHasArg(out, name) {
			t.Errorf("a YAML without the key must render no %s argument in main.tf", name)
		}
	}
}

func TestALBSSLPolicyRendersQuoted(t *testing.T) {
	out := albSettingsRender(t, albSettingsOverlay{
		alb: map[interface{}]interface{}{"enabled": true, "ssl_policy": albSettingsTLS13Policy},
	})
	block := workloadsModuleBlock(t, out)

	if !albSettingsArg("alb_ssl_policy", `"`+albSettingsTLS13Policy+`"`).MatchString(block) {
		t.Errorf("alb.ssl_policy must render as alb_ssl_policy = %q inside module \"workloads\"; got %q", albSettingsTLS13Policy, argumentValue(t, out, "alb_ssl_policy"))
	}
}

// 0 is a valid delay and must not be swallowed as "absent".
func TestALBDeregistrationDelayRendersZero(t *testing.T) {
	for _, v := range []int{0, 3600} {
		t.Run(strconv.Itoa(v), func(t *testing.T) {
			out := albSettingsRender(t, albSettingsOverlay{
				alb: map[interface{}]interface{}{"enabled": true, "deregistration_delay": v},
			})
			block := workloadsModuleBlock(t, out)
			if !albSettingsArg("alb_deregistration_delay", strconv.Itoa(v)).MatchString(block) {
				t.Errorf("alb.deregistration_delay: %d must render as alb_deregistration_delay = %d inside module \"workloads\"", v, v)
			}
		})
	}
}

// An explicit false renders too, not only true.
func TestALBRemoveAPIGatewayRendersExplicitValue(t *testing.T) {
	for _, v := range []bool{true, false} {
		want := "false"
		if v {
			want = "true"
		}
		t.Run(want, func(t *testing.T) {
			out := albSettingsRender(t, albSettingsOverlay{
				alb: map[interface{}]interface{}{"enabled": true, "remove_api_gateway": v},
			})
			block := workloadsModuleBlock(t, out)
			if !albSettingsArg("remove_api_gateway", want).MatchString(block) {
				t.Errorf("alb.remove_api_gateway: %s must render as remove_api_gateway = %s inside module \"workloads\"", want, want)
			}
		})
	}
}

func TestBackendHealthCheckGracePeriodRenders(t *testing.T) {
	for _, v := range []int{120, 0} {
		t.Run(strconv.Itoa(v), func(t *testing.T) {
			out := albSettingsRender(t, albSettingsOverlay{
				workload: map[interface{}]interface{}{"backend_health_check_grace_period": v},
			})
			block := workloadsModuleBlock(t, out)
			if !albSettingsArg("backend_health_check_grace_period", strconv.Itoa(v)).MatchString(block) {
				t.Errorf("workload.backend_health_check_grace_period: %d must render as backend_health_check_grace_period = %d inside module \"workloads\"", v, v)
			}
		})
	}
}

func albOn(extra map[interface{}]interface{}) map[interface{}]interface{} {
	m := map[interface{}]interface{}{"enabled": true}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func albSettingsService(name, access string, enabled bool) map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"name":           name,
		"enabled":        enabled,
		"network_access": access,
		"container_port": 3000,
		"host_port":      3000,
		"cpu":            256,
		"memory":         512,
		"desired_count":  1,
		"runtime":        "fargate",
		"ecr_config":     map[interface{}]interface{}{"mode": "create_ecr"},
	}
}

// Each refusal must name the key at fault and the way out.
func TestALBSettingsRefusals(t *testing.T) {
	cases := []struct {
		name    string
		overlay albSettingsOverlay
		want    []string
	}{
		{
			name:    "remove_api_gateway with the ALB off",
			overlay: albSettingsOverlay{alb: map[interface{}]interface{}{"enabled": false, "remove_api_gateway": true}},
			want:    []string{"remove_api_gateway", "alb.enabled"},
		},
		{
			name: "remove_api_gateway with an enabled full_open service",
			overlay: albSettingsOverlay{
				alb:      albOn(map[interface{}]interface{}{"remove_api_gateway": true}),
				services: []interface{}{albSettingsService("ledger", "full_open", true)},
			},
			want: []string{"remove_api_gateway", "ledger", "full_open", "public"},
		},
		{
			name: "remove_api_gateway with SNS webhooks",
			overlay: albSettingsOverlay{
				alb: albOn(map[interface{}]interface{}{"remove_api_gateway": true}),
				extensions: map[interface{}]interface{}{
					"sns_topics": []interface{}{
						map[interface{}]interface{}{
							"name":     "orders",
							"webhooks": []interface{}{map[interface{}]interface{}{"path": "/hook"}},
						},
					},
				},
			},
			want: []string{"remove_api_gateway", "orders", "webhooks"},
		},
		{
			name:    "ssl_policy empty",
			overlay: albSettingsOverlay{alb: albOn(map[interface{}]interface{}{"ssl_policy": ""})},
			want:    []string{"ssl_policy", "ELBSecurityPolicy-2016-08"},
		},
		{
			name:    "ssl_policy with a space",
			overlay: albSettingsOverlay{alb: albOn(map[interface{}]interface{}{"ssl_policy": "bad policy"})},
			want:    []string{"ssl_policy", "ELBSecurityPolicy-2016-08"},
		},
		{
			name:    "ssl_policy a number",
			overlay: albSettingsOverlay{alb: albOn(map[interface{}]interface{}{"ssl_policy": 42})},
			want:    []string{"ssl_policy"},
		},
		{
			name:    "deregistration_delay negative",
			overlay: albSettingsOverlay{alb: albOn(map[interface{}]interface{}{"deregistration_delay": -1})},
			want:    []string{"deregistration_delay", "3600"},
		},
		{
			name:    "deregistration_delay above 3600",
			overlay: albSettingsOverlay{alb: albOn(map[interface{}]interface{}{"deregistration_delay": 3601})},
			want:    []string{"deregistration_delay", "3600"},
		},
		{
			name:    "deregistration_delay fractional",
			overlay: albSettingsOverlay{alb: albOn(map[interface{}]interface{}{"deregistration_delay": 1.5})},
			want:    []string{"deregistration_delay"},
		},
		{
			name:    "deregistration_delay a string",
			overlay: albSettingsOverlay{alb: albOn(map[interface{}]interface{}{"deregistration_delay": "300"})},
			want:    []string{"deregistration_delay"},
		},
		{
			name:    "grace period negative",
			overlay: albSettingsOverlay{workload: map[interface{}]interface{}{"backend_health_check_grace_period": -1}},
			want:    []string{"backend_health_check_grace_period"},
		},
		{
			name:    "grace period fractional",
			overlay: albSettingsOverlay{workload: map[interface{}]interface{}{"backend_health_check_grace_period": 1.5}},
			want:    []string{"backend_health_check_grace_period"},
		},
		{
			name:    "remove_api_gateway not a bool",
			overlay: albSettingsOverlay{alb: albOn(map[interface{}]interface{}{"remove_api_gateway": "yes"})},
			want:    []string{"remove_api_gateway"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := albSettingsPreprocess(t, tc.overlay)
			if err == nil {
				t.Fatal("must be refused before rendering, got no error")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error must mention %q:\n%s", w, err)
				}
			}
		})
	}
}

func TestALBSettingsAccepts(t *testing.T) {
	cases := []struct {
		name    string
		overlay albSettingsOverlay
	}{
		{
			name: "remove_api_gateway with the ALB on, public services, a disabled full_open service and a topic without webhooks",
			overlay: albSettingsOverlay{
				alb: albOn(map[interface{}]interface{}{"remove_api_gateway": true}),
				services: []interface{}{
					albSettingsService("web", "public", true),
					albSettingsService("ledger", "full_open", false),
				},
				extensions: map[interface{}]interface{}{
					"sns_topics": []interface{}{
						map[interface{}]interface{}{"name": "orders"},
					},
				},
			},
		},
		{
			name:    "remove_api_gateway false with the ALB off",
			overlay: albSettingsOverlay{alb: map[interface{}]interface{}{"enabled": false, "remove_api_gateway": false}},
		},
		{
			name:    "deregistration_delay 0",
			overlay: albSettingsOverlay{alb: albOn(map[interface{}]interface{}{"deregistration_delay": 0})},
		},
		{
			name:    "deregistration_delay 3600",
			overlay: albSettingsOverlay{alb: albOn(map[interface{}]interface{}{"deregistration_delay": 3600})},
		},
		{
			name:    "grace period 0",
			overlay: albSettingsOverlay{workload: map[interface{}]interface{}{"backend_health_check_grace_period": 0}},
		},
		{
			name:    "ssl_policy a policy name",
			overlay: albSettingsOverlay{alb: albOn(map[interface{}]interface{}{"ssl_policy": albSettingsTLS13Policy})},
		},
		{
			// Inert while the ALB is off, and not refused: an operator may
			// stage them before turning the ALB on.
			name: "ssl_policy, deregistration_delay and grace period with the ALB off",
			overlay: albSettingsOverlay{
				alb:      map[interface{}]interface{}{"enabled": false, "ssl_policy": albSettingsTLS13Policy, "deregistration_delay": 30},
				workload: map[interface{}]interface{}{"backend_health_check_grace_period": 120},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := albSettingsPreprocess(t, tc.overlay); err != nil {
				t.Errorf("must be accepted, got: %v", err)
			}
		})
	}
}

// The TUI and web save paths round-trip through the typed model; the keys,
// zeros included, must survive.
func TestALBSettingsSurviveLoadSave(t *testing.T) {
	var a ALB
	if err := yaml.Unmarshal([]byte(`
enabled: true
ssl_policy: `+albSettingsTLS13Policy+`
deregistration_delay: 0
remove_api_gateway: true
`), &a); err != nil {
		t.Fatalf("unmarshalling into ALB: %v", err)
	}
	out, err := yaml.Marshal(a)
	if err != nil {
		t.Fatalf("marshalling ALB: %v", err)
	}
	var raw map[interface{}]interface{}
	if err := yaml.Unmarshal(out, &raw); err != nil {
		t.Fatalf("re-reading marshalled ALB: %v\n%s", err, out)
	}
	m := toStringKeyed(raw).(map[string]interface{})

	if m["ssl_policy"] != albSettingsTLS13Policy {
		t.Errorf("ssl_policy must stay %q, got %#v", albSettingsTLS13Policy, m["ssl_policy"])
	}
	if v, present := m["deregistration_delay"]; !present || v != 0 {
		t.Errorf("deregistration_delay: 0 must stay 0, got %#v (present=%v)\n%s", v, present, out)
	}
	if m["remove_api_gateway"] != true {
		t.Errorf("remove_api_gateway: true must stay true, got %#v", m["remove_api_gateway"])
	}

	w := roundTripWorkload(t, "backend_health_check_grace_period: 0\n")
	if v, present := w["backend_health_check_grace_period"]; !present || v != 0 {
		t.Errorf("backend_health_check_grace_period: 0 must stay 0, got %#v (present=%v)", v, present)
	}
	w = roundTripWorkload(t, "backend_health_check_grace_period: 120\n")
	if w["backend_health_check_grace_period"] != 120 {
		t.Errorf("backend_health_check_grace_period: 120 must stay 120, got %#v", w["backend_health_check_grace_period"])
	}
}

// Absent keys stay absent, so a save does not add them to every environment.
func TestALBSettingsAbsentKeysStayAbsentOnSave(t *testing.T) {
	var a ALB
	if err := yaml.Unmarshal([]byte("enabled: true\n"), &a); err != nil {
		t.Fatalf("unmarshalling into ALB: %v", err)
	}
	out, err := yaml.Marshal(a)
	if err != nil {
		t.Fatalf("marshalling ALB: %v", err)
	}
	for _, k := range []string{"ssl_policy", "deregistration_delay", "remove_api_gateway"} {
		if strings.Contains(string(out), k) {
			t.Errorf("an ALB without %s must marshal without it, got:\n%s", k, out)
		}
	}
	w := roundTripWorkload(t, "backend_auto_deploy: true\n")
	if _, present := w["backend_health_check_grace_period"]; present {
		t.Errorf("a workload without backend_health_check_grace_period must marshal without it")
	}
}

// The moved blocks keep existing environments' gateway resources and ALB-mode
// api_domain record at their objects. CI's Terraform cannot plan a
// legacy-shaped state, so this guards their presence textually.
func TestWorkloadsKeepsGatewayAndRecordMovedBlocks(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "modules", "workloads", "*.tf"))
	if err != nil || len(files) == 0 {
		t.Fatalf("listing modules/workloads/*.tf: %v (%d files)", err, len(files))
	}
	var all strings.Builder
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		// Drop full-line comments so a commented-out block does not count.
		for _, line := range strings.Split(string(b), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
				continue
			}
			all.WriteString(line)
			all.WriteByte('\n')
		}
	}
	src := all.String()

	pairs := [][2]string{
		{"aws_apigatewayv2_api.api_gateway", "aws_apigatewayv2_api.api_gateway[0]"},
		{"aws_apigatewayv2_vpc_link.backend", "aws_apigatewayv2_vpc_link.backend[0]"},
		{"aws_cloudwatch_log_group.api_gateway_logs", "aws_cloudwatch_log_group.api_gateway_logs[0]"},
		{"aws_apigatewayv2_stage.backend", "aws_apigatewayv2_stage.backend[0]"},
		{"aws_route53_record.alb_alias", "aws_route53_record.backend"},
	}
	for _, p := range pairs {
		re := regexp.MustCompile(`moved\s*\{\s*from\s*=\s*` + regexp.QuoteMeta(p[0]) + `\s+to\s*=\s*` + regexp.QuoteMeta(p[1]) + `\s*\}`)
		if !re.MatchString(src) {
			t.Errorf("modules/workloads must keep moved { from = %s  to = %s }", p[0], p[1])
		}
	}

	// A moved block whose `from` is still declared is a Terraform error.
	if regexp.MustCompile(`resource\s+"aws_route53_record"\s+"alb_alias"`).MatchString(src) {
		t.Error(`resource "aws_route53_record" "alb_alias" must no longer be declared; the api_domain record is aws_route53_record.backend in both modes`)
	}
}
