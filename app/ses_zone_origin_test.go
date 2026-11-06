package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// sesZoneEnvMap builds the env map renderMainHBS renders, for the same
// overlay, so the resolver's output can be read back.
func sesZoneEnvMap(t *testing.T, overlay map[string]interface{}) map[string]interface{} {
	t.Helper()
	base, err := os.ReadFile(filepath.Join("..", "project", "dev.yaml"))
	if err != nil {
		t.Fatalf("reading project/dev.yaml: %v", err)
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(base, &doc); err != nil {
		t.Fatalf("parsing project/dev.yaml: %v", err)
	}
	for k, v := range overlay {
		doc[k] = v
	}
	envMap, ok := convertToJSONCompatible(doc).(map[string]interface{})
	if !ok {
		t.Fatal("fixture yaml is not a mapping")
	}
	if err := preprocessEnvMap(envMap); err != nil {
		t.Fatalf("preprocessEnvMap: %v", err)
	}
	return envMap
}

var sesRenderedZoneLine = regexp.MustCompile(`(?m)^\s*zone_id = (\S+)`)

// The resolver must name the zone the template prints, on every row: the
// template is still what writes zone_id, and the resolver is what decides
// merge mode, so a disagreement would put records into one zone under the
// other zone's rules.
func TestSESZoneTemplateAgreement(t *testing.T) {
	for _, row := range sesZoneRows() {
		t.Run(row.name, func(t *testing.T) {
			envMap := sesZoneEnvMap(t, row.overlay)
			block := sesModuleBlockText(t, renderMainHBS(t, row.overlay))
			ses, _ := computeNodeMap(envMap["ses"])
			domains, _ := ses["domains"].([]interface{})

			if len(domains) == 0 {
				got, _ := envMap[sesLegacyZoneOriginKey].(string)
				if got != row.wantOrigin[0] {
					t.Errorf("legacy origin = %q, want %q", got, row.wantOrigin[0])
				}
				printed := sesRenderedZoneLine.FindStringSubmatch(block)
				if (printed != nil) != (got != "") {
					t.Errorf("legacy: origin %q but template zone_id line %v", got, printed)
				}
				return
			}

			// One chunk per rendered domain entry.
			chunks := strings.Split(block, "      domain = ")[1:]
			if len(chunks) != len(domains) {
				t.Fatalf("rendered %d domain entries, want %d", len(chunks), len(domains))
			}
			for i, raw := range domains {
				d := raw.(map[string]interface{})
				origin, _ := d[sesZoneOriginKey].(string)
				expr, _ := d[sesZoneExprKey].(string)
				if origin != row.wantOrigin[i] {
					t.Errorf("domain %d origin = %q, want %q", i, origin, row.wantOrigin[i])
				}
				printed := sesRenderedZoneLine.FindStringSubmatch(chunks[i])
				switch {
				case printed == nil && expr != "":
					t.Errorf("domain %d: resolver says %s, template prints no zone_id", i, expr)
				case printed != nil && printed[1] != expr:
					t.Errorf("domain %d: resolver says %q, template prints %q", i, expr, printed[1])
				}
			}
		})
	}
}

// The zone-origin matrix: one fixture per branch the template used to take to
// decide which Route53 zone an SES domain's records go into. The goldens in
// testdata/ses_zone_*.golden were rendered by the template before the decision
// moved into Go (resolveSESZones). Every row whose zone meroku created must
// still render those bytes exactly; a row in an existing zone may differ only
// by the lines that switch it to merge mode.
//
// Rewrite the goldens with: go test -run TestSESZoneOriginGolden -update
// (only ever from a template that is known to be right).

type sesZoneRow struct {
	name    string
	overlay map[string]interface{}
	// wantOrigin per SES domain, in list order ("" = no zone).
	wantOrigin []string
}

func sesZoneDomainBlock(fields map[string]interface{}) map[string]interface{} {
	d := map[string]interface{}{
		"enabled":            true,
		"domain_name":        "example.com",
		"create_domain_zone": true,
	}
	for k, v := range fields {
		d[k] = v
	}
	return d
}

func sesZoneSES(domains ...map[string]interface{}) map[string]interface{} {
	list := make([]interface{}, 0, len(domains))
	for _, d := range domains {
		list = append(list, d)
	}
	return map[string]interface{}{"enabled": true, "domains": list}
}

func sesZoneRows() []sesZoneRow {
	return []sesZoneRow{
		{
			name: "explicit_zone_id",
			overlay: map[string]interface{}{
				"domain": sesZoneDomainBlock(nil),
				"ses":    sesZoneSES(map[string]interface{}{"domain": "example.com", "zone_id": "Z000000000000A"}),
			},
			wantOrigin: []string{"existing"},
		},
		{
			name: "additional_zone_id",
			overlay: map[string]interface{}{
				"domain": sesZoneDomainBlock(map[string]interface{}{
					"additional_domains": []interface{}{
						map[string]interface{}{"domain": "example.org", "zone_id": "Z000000000000B"},
					},
				}),
				"ses": sesZoneSES(map[string]interface{}{"domain": "mail.example.org"}),
			},
			wantOrigin: []string{"existing"},
		},
		{
			name: "additional_create_zone",
			overlay: map[string]interface{}{
				"domain": sesZoneDomainBlock(map[string]interface{}{
					"additional_domains": []interface{}{
						map[string]interface{}{"domain": "example.org", "zone_id": "Z000000000000B"},
						map[string]interface{}{"domain": "example.net", "create_zone": true},
					},
				}),
				"ses": sesZoneSES(map[string]interface{}{"domain": "example.net"}),
			},
			wantOrigin: []string{"created"},
		},
		{
			name: "prod_main_created",
			overlay: map[string]interface{}{
				"env":    "prod",
				"domain": sesZoneDomainBlock(nil),
				"ses":    sesZoneSES(map[string]interface{}{"domain": "mail.example.com"}),
			},
			wantOrigin: []string{"created"},
		},
		{
			name: "prod_main_existing",
			overlay: map[string]interface{}{
				"env":    "prod",
				"domain": sesZoneDomainBlock(map[string]interface{}{"create_domain_zone": false}),
				"ses":    sesZoneSES(map[string]interface{}{"domain": "example.com"}),
			},
			wantOrigin: []string{"existing"},
		},
		{
			name: "dev_delegated",
			overlay: map[string]interface{}{
				"domain": sesZoneDomainBlock(map[string]interface{}{
					"create_domain_zone": false,
					"root_zone_id":       "Z000000000000R",
					"root_account_id":    "000000000000",
				}),
				"ses": sesZoneSES(map[string]interface{}{"domain": "mail.example.com"}),
			},
			wantOrigin: []string{"created"},
		},
		{
			name: "dev_main_created",
			overlay: map[string]interface{}{
				"domain": sesZoneDomainBlock(nil),
				"ses":    sesZoneSES(map[string]interface{}{"domain": "mail.example.com"}),
			},
			wantOrigin: []string{"created"},
		},
		{
			name: "dev_main_existing",
			overlay: map[string]interface{}{
				"domain": sesZoneDomainBlock(map[string]interface{}{"create_domain_zone": false}),
				"ses":    sesZoneSES(map[string]interface{}{"domain": "mail.example.com"}),
			},
			wantOrigin: []string{"existing"},
		},
		{
			name: "no_zone",
			overlay: map[string]interface{}{
				"domain": map[string]interface{}{"enabled": false},
				"ses":    sesZoneSES(map[string]interface{}{"domain": "example.com"}),
			},
			wantOrigin: []string{""},
		},
		{
			name: "outside_main_domain",
			overlay: map[string]interface{}{
				"domain": sesZoneDomainBlock(nil),
				"ses":    sesZoneSES(map[string]interface{}{"domain": "example.org"}),
			},
			wantOrigin: []string{""},
		},
		{
			name: "mixed_domains_with_keys",
			overlay: map[string]interface{}{
				"domain": sesZoneDomainBlock(nil),
				"ses": map[string]interface{}{
					"enabled":             true,
					"dmarc_policy":        "quarantine",
					"dmarc_rua_email":     "dmarc@example.com",
					"mail_from_subdomain": "mail",
					"domains": []interface{}{
						map[string]interface{}{
							"domain":           "mail.example.com",
							"spf_record":       "v=spf1 include:_spf.google.com include:amazonses.com ~all",
							"spf_ttl":          300,
							"apex_txt_values":  []interface{}{"google-site-verification=fake-token-do-not-use"},
							"enable_mail_from": false,
							"dmarc_policy":     "reject",
						},
						map[string]interface{}{"domain": "example.org"},
					},
				},
			},
			wantOrigin: []string{"created", ""},
		},
		{
			name: "legacy_created",
			overlay: map[string]interface{}{
				"domain": sesZoneDomainBlock(nil),
				"ses":    map[string]interface{}{"enabled": true, "domain_name": "mail.example.com"},
			},
			wantOrigin: []string{"created"},
		},
		{
			name: "legacy_existing",
			overlay: map[string]interface{}{
				"domain": sesZoneDomainBlock(map[string]interface{}{"create_domain_zone": false}),
				"ses":    map[string]interface{}{"enabled": true, "domain_name": "mail.example.com"},
			},
			wantOrigin: []string{"existing"},
		},
		{
			name: "legacy_no_domain",
			overlay: map[string]interface{}{
				"domain": map[string]interface{}{"enabled": false},
				"ses":    map[string]interface{}{"enabled": true, "domain_name": "mail.example.com"},
			},
			wantOrigin: []string{""},
		},
	}
}

// sesRenderedSection returns the part of a rendered main.tf the SES block
// owns: from its heading comment to the first top-level block after
// module "ses" that is not a moved or removed block.
func sesRenderedSection(t *testing.T, out string) string {
	t.Helper()
	start := strings.Index(out, "# Email domain configuration")
	if start < 0 {
		t.Fatal("rendered main.tf has no SES heading")
	}
	rest := out[start:]
	mod := strings.Index(rest, "module \"ses\" {")
	if mod < 0 {
		t.Fatal(`rendered main.tf has no module "ses"`)
	}
	closeIdx := strings.Index(rest[mod:], "\n}\n")
	if closeIdx < 0 {
		t.Fatal(`module "ses" is not closed`)
	}
	pos := mod + closeIdx + 3
	next := regexp.MustCompile(`(?m)^[a-z_]+ [{"]`)
	for {
		loc := next.FindStringIndex(rest[pos:])
		if loc == nil {
			return rest
		}
		word := strings.Fields(rest[pos+loc[0]:])[0]
		if word != "moved" && word != "removed" {
			return rest[:pos+loc[0]]
		}
		end := strings.Index(rest[pos+loc[0]:], "\n}\n")
		if end < 0 {
			return rest
		}
		pos = pos + loc[0] + end + 3
	}
}

// sesMergeModeLines are the lines and blocks that switch a domain in an
// existing zone to merge mode. Removing them from a rendered section must give
// back the bytes the template produced before merge mode existed.
var sesMergeModeLine = regexp.MustCompile(`(?m)^ *zone_origin *= *"existing"\n`)

func stripSESMergeMode(section string) string {
	section = sesMergeModeLine.ReplaceAllString(section, "")
	if i := strings.Index(section, sesMergeMigrationHeading); i >= 0 {
		// The migration comment and blocks are rendered as one unit, directly
		// before the closing {{/if}}, so everything from the heading to the
		// end of the section belongs to it.
		tail := section[i:]
		end := strings.Index(tail, sesMergeMigrationEnd)
		if end >= 0 {
			section = section[:i] + tail[end+len(sesMergeMigrationEnd):]
		}
	}
	return section
}

func sesSignificantLines(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func TestSESZoneOriginGolden(t *testing.T) {
	for _, row := range sesZoneRows() {
		t.Run(row.name, func(t *testing.T) {
			out := renderMainHBS(t, row.overlay)
			section := sesRenderedSection(t, out)
			path := filepath.Join("testdata", "ses_zone_"+row.name+".golden")
			if *updateGolden {
				if err := os.WriteFile(path, []byte(stripSESMergeMode(section)), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading golden: %v", err)
			}
			existing := false
			for _, o := range row.wantOrigin {
				existing = existing || o == "existing"
			}
			if !existing {
				if section != string(want) {
					t.Errorf("rendered SES section differs from the golden.\n--- got ---\n%s\n--- want ---\n%s", section, want)
				}
				if strings.Contains(section, "zone_origin") || strings.Contains(section, "_released") {
					t.Errorf("a row with no existing zone must render no merge-mode lines:\n%s", section)
				}
				return
			}
			// An existing-zone row is meant to change. The inserted block also
			// moves some of the template's stray indentation, so this compares
			// lines with surrounding whitespace and blank lines dropped: every
			// other token must be exactly as before.
			if got := sesSignificantLines(stripSESMergeMode(section)); got != sesSignificantLines(string(want)) {
				t.Errorf("rendered SES section, merge-mode lines removed, differs from the golden.\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
			if !sesMergeModeLine.MatchString(section) {
				t.Errorf("an existing-zone row must render zone_origin = \"existing\":\n%s", section)
			}
		})
	}
}
