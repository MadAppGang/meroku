package main

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v2"
)

// The YAML -> HCL link for ses.domains[].spf_record, spf_ttl and
// apex_txt_values: a key set in YAML reaches the module call and evaluates to
// exactly the YAML value; an unset key is not written at all.

func sesOverlay(domain map[string]interface{}) map[string]interface{} {
	entry := map[string]interface{}{
		"domain":  "example.com",
		"zone_id": "Z000000000000A",
	}
	for k, v := range domain {
		entry[k] = v
	}
	return map[string]interface{}{
		"ses": map[string]interface{}{
			"enabled": true,
			"domains": []interface{}{entry},
		},
	}
}

// sesModuleDomain parses the rendered main.tf, finds module "ses" and
// evaluates its domains attribute, returning the first entry.
func sesModuleDomain(t *testing.T, out string) cty.Value {
	t.Helper()
	file, diags := hclsyntax.ParseConfig([]byte(out), "main.tf", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("rendered main.tf does not parse: %s", diags.Error())
	}
	body := file.Body.(*hclsyntax.Body)
	for _, b := range body.Blocks {
		if b.Type != "module" || len(b.Labels) != 1 || b.Labels[0] != "ses" {
			continue
		}
		attr, ok := b.Body.Attributes["domains"]
		if !ok {
			t.Fatal(`module "ses" has no domains attribute`)
		}
		val, diags := attr.Expr.Value(nil)
		if diags.HasErrors() {
			t.Fatalf("evaluating ses domains: %s", diags.Error())
		}
		if !val.CanIterateElements() || val.LengthInt() < 1 {
			t.Fatalf("ses domains is empty: %#v", val)
		}
		return val.Index(cty.NumberIntVal(0))
	}
	t.Fatal(`rendered main.tf has no module "ses"`)
	return cty.NilVal
}

func sesModuleBlockText(t *testing.T, out string) string {
	t.Helper()
	i := strings.Index(out, `module "ses" {`)
	if i < 0 {
		t.Fatal(`rendered main.tf has no module "ses"`)
	}
	rest := out[i:]
	if j := strings.Index(rest, "\n}\n"); j >= 0 {
		return rest[:j+3]
	}
	return rest
}

// R5: unset keys are not written.
func TestSESSpfTemplate_UnsetKeysAbsent(t *testing.T) {
	out := renderMainHBS(t, sesOverlay(nil))
	block := sesModuleBlockText(t, out)
	for _, key := range []string{"spf_record", "spf_ttl", "apex_txt_values"} {
		if strings.Contains(block, key) {
			t.Errorf("unset %s must not appear in module \"ses\":\n%s", key, block)
		}
	}
	if err := validateGeneratedHCL("main.tf", out); err != nil {
		t.Fatalf("validateGeneratedHCL: %v", err)
	}
}

// R5/R11: apex_txt_values: [] renders byte-identical to the key being absent.
func TestSESSpfTemplate_EmptyApexListIsAbsent(t *testing.T) {
	without := renderMainHBS(t, sesOverlay(nil))
	empty := renderMainHBS(t, sesOverlay(map[string]interface{}{"apex_txt_values": []interface{}{}}))
	if strings.Contains(sesModuleBlockText(t, empty), "apex_txt_values") {
		t.Errorf("apex_txt_values: [] must not be written")
	}
	if without != empty {
		t.Errorf("apex_txt_values: [] must render byte-identical to an absent key")
	}
}

// R1: keys render when set and evaluate to the YAML values.
func TestSESSpfTemplate_AllKeysRender(t *testing.T) {
	out := renderMainHBS(t, sesOverlay(map[string]interface{}{
		"spf_record":      "v=spf1 include:_spf.google.com include:amazonses.com ~all",
		"spf_ttl":         300,
		"apex_txt_values": []interface{}{"google-site-verification=fake-token", "ms=ms00000000"},
	}))
	if err := validateGeneratedHCL("main.tf", out); err != nil {
		t.Fatalf("validateGeneratedHCL: %v", err)
	}
	d := sesModuleDomain(t, out)
	if got := d.GetAttr("spf_record").AsString(); got != "v=spf1 include:_spf.google.com include:amazonses.com ~all" {
		t.Errorf("spf_record = %q", got)
	}
	ttl, _ := d.GetAttr("spf_ttl").AsBigFloat().Int64()
	if ttl != 300 {
		t.Errorf("spf_ttl = %d, want 300", ttl)
	}
	list := d.GetAttr("apex_txt_values")
	if list.LengthInt() != 2 {
		t.Fatalf("apex_txt_values length = %d, want 2", list.LengthInt())
	}
	if got := list.Index(cty.NumberIntVal(0)).AsString(); got != "google-site-verification=fake-token" {
		t.Errorf("apex_txt_values[0] = %q", got)
	}
	if got := list.Index(cty.NumberIntVal(1)).AsString(); got != "ms=ms00000000" {
		t.Errorf("apex_txt_values[1] = %q", got)
	}
}

// R3: spf_ttl: 0 renders spf_ttl = 0.
func TestSESSpfTemplate_TTLZeroRenders(t *testing.T) {
	out := renderMainHBS(t, sesOverlay(map[string]interface{}{"spf_ttl": 0}))
	if err := validateGeneratedHCL("main.tf", out); err != nil {
		t.Fatalf("validateGeneratedHCL: %v", err)
	}
	if !strings.Contains(sesModuleBlockText(t, out), "spf_ttl = 0") {
		t.Errorf("spf_ttl: 0 must render spf_ttl = 0:\n%s", sesModuleBlockText(t, out))
	}
	d := sesModuleDomain(t, out)
	ttl := d.GetAttr("spf_ttl")
	if ttl.IsNull() {
		t.Fatal("spf_ttl evaluated to null, want 0")
	}
	if n, _ := ttl.AsBigFloat().Int64(); n != 0 {
		t.Errorf("spf_ttl = %d, want 0", n)
	}
}

// R4: values evaluate as HCL to exactly the YAML input.
func TestSESSpfTemplate_ValuesDecodeExactly(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"spf punctuation ~-=:.", "v=spf1 a:mail.example.com ip4:192.0.2.0/24 -include:x.example.com ~all"},
		{"html-escapable &'<>", "v=spf1 include:a&b.example.com 'x' <y> ~all"},
		{"spf macro %{i}", "v=spf1 exists:%{i}._spf.example.com ~all"},
		{"quote backslash interpolation", `v=spf1 a" "b \ ${x} ~all`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			extra := strings.Replace(tc.value, "v=spf1", "extra=1", 1)
			out := renderMainHBS(t, sesOverlay(map[string]interface{}{
				"spf_record":      tc.value,
				"apex_txt_values": []interface{}{extra},
			}))
			if err := validateGeneratedHCL("main.tf", out); err != nil {
				t.Fatalf("validateGeneratedHCL: %v", err)
			}
			d := sesModuleDomain(t, out)
			if got := d.GetAttr("spf_record").AsString(); got != tc.value {
				t.Errorf("spf_record decoded = %q, want %q", got, tc.value)
			}
			if got := d.GetAttr("apex_txt_values").Index(cty.NumberIntVal(0)).AsString(); got != extra {
				t.Errorf("apex_txt_values[0] decoded = %q, want %q", got, extra)
			}
		})
	}
}

// R10: SESDomain YAML round-trip.
func TestSESDomainSpfYAMLRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantKeys map[string]interface{}
		absent   []string
	}{
		{
			name:   "unset",
			in:     "domain: example.com\n",
			absent: []string{"spf_record", "spf_ttl", "apex_txt_values"},
		},
		{
			name:     "spf_ttl zero",
			in:       "domain: example.com\nspf_ttl: 0\n",
			wantKeys: map[string]interface{}{"spf_ttl": 0},
			absent:   []string{"spf_record", "apex_txt_values"},
		},
		{
			name: "all three set",
			in:   "domain: example.com\nspf_record: v=spf1 include:_spf.google.com ~all\nspf_ttl: 300\napex_txt_values:\n- google-site-verification=abc\n- ms=ms00000000\n",
			wantKeys: map[string]interface{}{
				"spf_record":      "v=spf1 include:_spf.google.com ~all",
				"spf_ttl":         300,
				"apex_txt_values": []interface{}{"google-site-verification=abc", "ms=ms00000000"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var d SESDomain
			if err := yaml.Unmarshal([]byte(tc.in), &d); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			raw, err := yaml.Marshal(d)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var back map[string]interface{}
			if err := yaml.Unmarshal(raw, &back); err != nil {
				t.Fatalf("re-parse: %v", err)
			}
			for k, want := range tc.wantKeys {
				got, ok := back[k]
				if !ok {
					t.Errorf("%s missing after round-trip:\n%s", k, raw)
					continue
				}
				if !yamlEqual(got, want) {
					t.Errorf("%s = %#v, want %#v", k, got, want)
				}
			}
			for _, k := range tc.absent {
				if _, ok := back[k]; ok {
					t.Errorf("%s written although never set:\n%s", k, raw)
				}
			}
		})
	}
}

func yamlEqual(a, b interface{}) bool {
	as, aok := a.([]interface{})
	bs, bok := b.([]interface{})
	if aok || bok {
		if !aok || !bok || len(as) != len(bs) {
			return false
		}
		for i := range as {
			if !yamlEqual(as[i], bs[i]) {
				return false
			}
		}
		return true
	}
	return a == b
}
