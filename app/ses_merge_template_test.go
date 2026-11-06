package main

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v2"
)

// The YAML -> HCL link for ses.domains[].txt_records and for zone origin:
// the zone_origin line, and the moved/removed blocks that take the whole-set
// records of a zone meroku did not create out of state without a destroy.

func sesTopBlocks(t *testing.T, out string) []*hclsyntax.Block {
	t.Helper()
	file, diags := hclsyntax.ParseConfig([]byte(out), "main.tf", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("rendered main.tf does not parse: %s", diags.Error())
	}
	return file.Body.(*hclsyntax.Body).Blocks
}

func sesExprText(out string, e hclsyntax.Expression) string {
	r := e.Range()
	return out[r.Start.Byte:r.End.Byte]
}

// sesMovesAndRemovals lists "from -> to" of every moved block and "from" of
// every removed block that names module.ses.
func sesMovesAndRemovals(t *testing.T, out string) (moves []string, removals map[string]*hclsyntax.Block) {
	t.Helper()
	removals = map[string]*hclsyntax.Block{}
	for _, b := range sesTopBlocks(t, out) {
		from, ok := b.Body.Attributes["from"]
		if !ok {
			continue
		}
		f := sesExprText(out, from.Expr)
		if !strings.HasPrefix(f, "module.ses.") {
			continue
		}
		switch b.Type {
		case "moved":
			moves = append(moves, f+" -> "+sesExprText(out, b.Body.Attributes["to"].Expr))
		case "removed":
			removals[f] = b
		}
	}
	return moves, removals
}

func sesRemovedDestroys(t *testing.T, b *hclsyntax.Block) bool {
	t.Helper()
	for _, inner := range b.Body.Blocks {
		if inner.Type != "lifecycle" {
			continue
		}
		v, diags := inner.Body.Attributes["destroy"].Expr.Value(nil)
		if diags.HasErrors() {
			t.Fatalf("lifecycle.destroy: %s", diags.Error())
		}
		return v.True()
	}
	t.Fatal("removed block has no lifecycle block")
	return false
}

func TestSESTXTRecordsTemplate_AbsentAndEmptyAreIdentical(t *testing.T) {
	without := renderMainHBS(t, sesOverlay(nil))
	empty := renderMainHBS(t, sesOverlay(map[string]interface{}{"txt_records": []interface{}{}}))
	if strings.Contains(sesModuleBlockText(t, without), "txt_records") {
		t.Error("absent txt_records must not be written")
	}
	if without != empty {
		t.Error("txt_records: [] must render byte-identical to an absent key")
	}
}

func TestSESTXTRecordsTemplate_RendersAndDecodesExactly(t *testing.T) {
	long := "v=DKIM1; k=rsa; p=" + strings.Repeat("fake", 100)
	out := renderMainHBS(t, sesOverlay(map[string]interface{}{
		"txt_records": []interface{}{
			map[string]interface{}{"name": "google._domainkey", "ttl": 3600, "values": []interface{}{long}},
			map[string]interface{}{"name": "_acme", "ttl": 0, "values": []interface{}{"a=${x} %{y} &'<>", "b=2"}},
			map[string]interface{}{"name": "plain", "values": []interface{}{"c=3"}},
		},
	}))
	if err := validateGeneratedHCL("main.tf", out); err != nil {
		t.Fatalf("validateGeneratedHCL: %v", err)
	}
	records := sesModuleDomain(t, out).GetAttr("txt_records")
	if records.LengthInt() != 3 {
		t.Fatalf("txt_records length = %d, want 3", records.LengthInt())
	}
	first := records.Index(cty.NumberIntVal(0))
	if got := first.GetAttr("name").AsString(); got != "google._domainkey" {
		t.Errorf("name = %q", got)
	}
	if got := first.GetAttr("values").Index(cty.NumberIntVal(0)).AsString(); got != long {
		t.Errorf("long value decoded differently")
	}
	if n, _ := first.GetAttr("ttl").AsBigFloat().Int64(); n != 3600 {
		t.Errorf("ttl = %d, want 3600", n)
	}
	second := records.Index(cty.NumberIntVal(1))
	if got := second.GetAttr("values").Index(cty.NumberIntVal(0)).AsString(); got != "a=${x} %{y} &'<>" {
		t.Errorf("value with template openers decoded to %q", got)
	}
	if ttl := second.GetAttr("ttl"); ttl.IsNull() {
		t.Error("ttl: 0 must render, not be dropped")
	} else if n, _ := ttl.AsBigFloat().Int64(); n != 0 {
		t.Errorf("ttl = %d, want 0", n)
	}
	if third := records.Index(cty.NumberIntVal(2)); third.Type().HasAttribute("ttl") {
		t.Error("an unset ttl must not be written")
	}
}

func TestSESMergeTemplate_CreatedZoneRendersNoMergeMode(t *testing.T) {
	out := renderMainHBS(t, map[string]interface{}{
		"domain": sesZoneDomainBlock(nil),
		"ses":    sesZoneSES(map[string]interface{}{"domain": "mail.example.com"}),
	})
	if strings.Contains(out, "zone_origin") || strings.Contains(out, "_released") {
		t.Errorf("a created zone must render no zone_origin and no release blocks")
	}
	if _, removals := sesMovesAndRemovals(t, out); len(removals) != 0 {
		t.Errorf("a created zone must render no removed blocks, got %d", len(removals))
	}
}

func TestSESMergeTemplate_ExistingZoneRendersOriginAndReleases(t *testing.T) {
	out := renderMainHBS(t, map[string]interface{}{
		"domain": sesZoneDomainBlock(nil),
		"ses": sesZoneSES(
			map[string]interface{}{"domain": "example.org", "zone_id": "Z000000000000B"},
			// No zone: nothing to release.
			map[string]interface{}{"domain": "example.net"},
		),
	})
	if err := validateGeneratedHCL("main.tf", out); err != nil {
		t.Fatalf("validateGeneratedHCL: %v", err)
	}
	d := sesModuleDomain(t, out)
	if got := d.GetAttr("zone_origin").AsString(); got != "existing" {
		t.Errorf("zone_origin = %q, want existing", got)
	}

	moves, removals := sesMovesAndRemovals(t, out)
	for _, want := range []string{
		`module.ses.aws_route53_record.spf["example.org"] -> module.ses.aws_route53_record.spf_released["example.org"]`,
		`module.ses.aws_route53_record.domain_amazonses_verification_record["example.org"] -> module.ses.aws_route53_record.domain_amazonses_verification_record_released["example.org"]`,
		`module.ses.aws_route53_record.dmarc["example.org"] -> module.ses.aws_route53_record.dmarc_released["example.org"]`,
	} {
		found := false
		for _, m := range moves {
			found = found || m == want
		}
		if !found {
			t.Errorf("missing move %s\nmoves: %v", want, moves)
		}
	}
	for _, m := range moves {
		if strings.Contains(m, `"example.net"]`) {
			t.Errorf("a domain without a zone must not be released: %s", m)
		}
	}
	for _, from := range []string{
		"module.ses.aws_route53_record.spf_released",
		"module.ses.aws_route53_record.domain_amazonses_verification_record_released",
		"module.ses.aws_route53_record.dmarc_released",
	} {
		b, ok := removals[from]
		if !ok {
			t.Errorf("missing removed block for %s", from)
			continue
		}
		if sesRemovedDestroys(t, b) {
			t.Errorf("removed %s must not destroy: that would delete the live record set", from)
		}
	}
	if _, ok := removals["module.ses.terraform_data.dns_value"]; ok {
		t.Error("with SES on, the module declares dns_value: a removed block for it is an error")
	}
}

func TestSESMergeTemplate_LegacyExistingZone(t *testing.T) {
	out := renderMainHBS(t, map[string]interface{}{
		"domain": sesZoneDomainBlock(map[string]interface{}{"create_domain_zone": false}),
		"ses":    map[string]interface{}{"enabled": true, "domain_name": "mail.example.com"},
	})
	if err := validateGeneratedHCL("main.tf", out); err != nil {
		t.Fatalf("validateGeneratedHCL: %v", err)
	}
	if !strings.Contains(sesModuleBlockText(t, out), "\n  zone_origin = \"existing\"\n") {
		t.Errorf("the legacy form must pass zone_origin = \"existing\":\n%s", sesModuleBlockText(t, out))
	}
	moves, _ := sesMovesAndRemovals(t, out)
	want := `module.ses.aws_route53_record.spf["mail.example.com"] -> module.ses.aws_route53_record.spf_released["mail.example.com"]`
	found := false
	for _, m := range moves {
		found = found || m == want
	}
	if !found {
		t.Errorf("legacy existing zone: missing %s\nmoves: %v", want, moves)
	}
}

func TestSESMergeTemplate_DisabledSESRemovesOwnedValues(t *testing.T) {
	out := renderMainHBS(t, map[string]interface{}{
		"domain": sesZoneDomainBlock(nil),
		"ses": map[string]interface{}{
			"enabled": false,
			"domains": []interface{}{map[string]interface{}{"domain": "example.org", "zone_id": "Z000000000000B"}},
		},
	})
	if err := validateGeneratedHCL("main.tf", out); err != nil {
		t.Fatalf("validateGeneratedHCL: %v", err)
	}
	if strings.Contains(out, `module "ses" {`) {
		t.Fatal("SES off must not render the module")
	}
	_, removals := sesMovesAndRemovals(t, out)
	for _, from := range []string{"module.ses.terraform_data.dns_slot", "module.ses.terraform_data.dns_value"} {
		b, ok := removals[from]
		if !ok {
			t.Errorf("missing removed block for %s", from)
			continue
		}
		if !sesRemovedDestroys(t, b) {
			t.Errorf("removed %s must destroy, so the provisioner removes meroku's values", from)
		}
		var prov *hclsyntax.Block
		for _, inner := range b.Body.Blocks {
			if inner.Type == "provisioner" {
				prov = inner
			}
		}
		if prov == nil {
			t.Errorf("removed %s has no provisioner", from)
			continue
		}
		if got := sesExprText(out, prov.Body.Attributes["command"].Expr); got != `"remove"` {
			t.Errorf("provisioner command = %s, want \"remove\"", got)
		}
		if got := sesExprText(out, prov.Body.Attributes["when"].Expr); got != "destroy" {
			t.Errorf("provisioner when = %s, want destroy", got)
		}
	}
}

func TestSESMergeTemplate_DisabledSESWithCreatedZonesRendersNothing(t *testing.T) {
	base := map[string]interface{}{
		"domain": sesZoneDomainBlock(nil),
		"ses": map[string]interface{}{
			"enabled": false,
			"domains": []interface{}{map[string]interface{}{"domain": "mail.example.com"}},
		},
	}
	out := renderMainHBS(t, base)
	if strings.Contains(out, "module.ses.") {
		t.Errorf("SES off with only created zones must render nothing about module.ses")
	}
}

func TestSESTXTRecordYAMLRoundTrip(t *testing.T) {
	in := "domain: example.com\ntxt_records:\n- name: google._domainkey\n  values:\n  - v=DKIM1; k=rsa; p=fake\n  ttl: 0\n- name: other\n  values:\n  - a\n  - b\n"
	var d SESDomain
	if err := yaml.Unmarshal([]byte(in), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.TXTRecords) != 2 || d.TXTRecords[0].TTL == nil || *d.TXTRecords[0].TTL != 0 || d.TXTRecords[1].TTL != nil {
		t.Fatalf("decoded %+v", d.TXTRecords)
	}
	raw, err := yaml.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var again SESDomain
	if err := yaml.Unmarshal(raw, &again); err != nil {
		t.Fatal(err)
	}
	if len(again.TXTRecords) != 2 || again.TXTRecords[0].TTL == nil || *again.TXTRecords[0].TTL != 0 ||
		again.TXTRecords[1].Values[1] != "b" || again.TXTRecords[1].TTL != nil {
		t.Errorf("round trip lost data:\n%s", raw)
	}
	if strings.Contains(string(raw), "ttl: null") {
		t.Errorf("an unset ttl must not be written:\n%s", raw)
	}

	var none SESDomain
	if err := yaml.Unmarshal([]byte("domain: example.com\n"), &none); err != nil {
		t.Fatal(err)
	}
	raw, _ = yaml.Marshal(none)
	if strings.Contains(string(raw), "txt_records") {
		t.Errorf("absent txt_records must not be written:\n%s", raw)
	}
}
