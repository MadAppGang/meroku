package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// The YAML -> HCL link for EC2 compute pools (schema v26).
//
// Two properties are load-bearing and neither is visible by reading the
// template. The first is absence: an environment with no `compute:` block must
// render exactly what v25 rendered plus the backend_runtime default, because a
// single extra argument in module "workloads" is a plan diff in every existing
// environment. The second is escaping: pool fields carry free-form strings a
// user typed into a textarea, they land in generated HCL, and HCL2 treats `${`
// and `%{` as syntax inside quoted strings and heredocs alike -- so an
// unescaped value is not a rendering nit, it is config content becoming
// Terraform code.
//
// These render the real env/main.hbs through the same helpers and the same
// pre-processing applyTemplate uses (renderMainHBS, autodeploy_template_test.go).

// hclSpaceRun collapses the alignment padding main.hbs emits, so a `want` can
// say `network_mode = "bridge"` without pinning the column the = sits in.
var hclSpaceRun = regexp.MustCompile(`[ \t]+`)

func normalizeHCL(s string) string {
	return hclSpaceRun.ReplaceAllString(s, " ")
}

// computePool builds one pool over the minimum a pool needs, so a case states
// only the field it is about.
func computePool(extra map[string]interface{}) map[string]interface{} {
	pool := map[string]interface{}{
		"name":           "general",
		"instance_types": []interface{}{"m7i.large"},
	}
	for k, v := range extra {
		pool[k] = v
	}
	return pool
}

// computeOverlay wraps pools in the `compute:` block renderMainHBS merges over
// the shipped project/dev.yaml.
func computeOverlay(pools ...map[string]interface{}) map[string]interface{} {
	list := make([]interface{}, 0, len(pools))
	for _, p := range pools {
		list = append(list, p)
	}
	return map[string]interface{}{
		"compute": map[string]interface{}{"pools": list},
	}
}

// workloadOverlay merges keys into dev.yaml's own workload block rather than
// replacing it: main.hbs reads two dozen workload keys, and a replacement
// fixture would silently change what the rest of the module block renders.
func workloadOverlay(t *testing.T, extra map[string]interface{}) map[string]interface{} {
	t.Helper()

	base, err := os.ReadFile(filepath.Join("..", "project", "dev.yaml"))
	if err != nil {
		t.Fatalf("reading project/dev.yaml: %v", err)
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(base, &doc); err != nil {
		t.Fatalf("parsing project/dev.yaml: %v", err)
	}

	workload, _ := convertToJSONCompatible(doc["workload"]).(map[string]interface{})
	if workload == nil {
		workload = map[string]interface{}{}
	}
	for k, v := range extra {
		workload[k] = v
	}
	return map[string]interface{}{"workload": workload}
}

func mergeOverlays(overlays ...map[string]interface{}) map[string]interface{} {
	merged := map[string]interface{}{}
	for _, o := range overlays {
		for k, v := range o {
			merged[k] = v
		}
	}
	return merged
}

// renderComputeBlock returns the module "workloads" body, whitespace-normalized.
func renderComputeBlock(t *testing.T, overlay map[string]interface{}) string {
	t.Helper()
	return normalizeHCL(workloadsModuleBlock(t, renderMainHBS(t, overlay)))
}

// renderPoolList is computePoolsList, whitespace-normalized. Every assertion
// about a pool ARGUMENT uses this rather than the whole module block: the
// block's own header comment names min_size, network_mode and the rest in
// prose, so a `want: ""` absence check run over the block would match the
// documentation instead of the HCL.
func renderPoolList(t *testing.T, overlay map[string]interface{}) string {
	t.Helper()
	return normalizeHCL(computePoolsList(t, overlay))
}

// computePoolsList returns just the `compute_pools = [ ... ]` argument, so an
// escaping assertion cannot accidentally pass or fail on HCL the rest of the
// module block emits. Raw, not normalized: the heredoc body is in here.
func computePoolsList(t *testing.T, overlay map[string]interface{}) string {
	t.Helper()

	block := workloadsModuleBlock(t, renderMainHBS(t, overlay))
	start := strings.Index(block, "compute_pools = [")
	if start < 0 {
		t.Fatalf("compute_pools not found in the rendered module block:\n%s", block)
	}
	rest := block[start:]
	end := strings.Index(rest, "\n  ]")
	if end < 0 {
		t.Fatalf("compute_pools list is unterminated:\n%s", rest)
	}
	return rest[:end]
}

// An environment that has never heard of compute pools must render the Fargate
// default and nothing else.
//
// backend_runtime is the one compute argument emitted unconditionally, because
// its value is the variable's own default -- so it is the one that must be
// pinned to "fargate" rather than pinned to absent.
func TestComputeTemplate_BackendRuntime(t *testing.T) {
	tests := []struct {
		name    string
		overlay map[string]interface{}
		want    string
	}{
		{
			name:    "the shipped dev.yaml has no backend_runtime, so the default renders",
			overlay: nil,
			want:    `backend_runtime = "fargate"`,
		},
		{
			name:    "an explicit fargate renders identically to the default",
			overlay: workloadOverlay(t, map[string]interface{}{"backend_runtime": "fargate"}),
			want:    `backend_runtime = "fargate"`,
		},
		{
			name:    "ec2 renders verbatim",
			overlay: workloadOverlay(t, map[string]interface{}{"backend_runtime": "ec2"}),
			want:    `backend_runtime = "ec2"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			block := renderComputeBlock(t, tc.overlay)
			if !strings.Contains(block, tc.want) {
				t.Errorf("want %q, got:\n%s", tc.want, block)
			}
		})
	}
}

// compute_pools is absent unless there is something to put in it. An empty
// argument would still be an argument, and every existing environment would
// plan a change on the first apply after upgrading.
func TestComputeTemplate_ComputePoolsAbsentByDefault(t *testing.T) {
	tests := []struct {
		name    string
		overlay map[string]interface{}
		want    string // "" means compute_pools must be absent entirely
	}{
		{
			name:    "no compute block",
			overlay: nil,
			want:    "",
		},
		{
			name:    "a compute block with no pools key",
			overlay: map[string]interface{}{"compute": map[string]interface{}{}},
			want:    "",
		},
		{
			name:    "an empty pool list",
			overlay: computeOverlay(),
			want:    "",
		},
		{
			name:    "one pool",
			overlay: computeOverlay(computePool(nil)),
			want:    `compute_pools = [`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			block := renderComputeBlock(t, tc.overlay)

			if tc.want == "" {
				if strings.Contains(block, "compute_pools") {
					t.Errorf("compute_pools should be absent, got:\n%s", block)
				}
				return
			}
			if !strings.Contains(block, tc.want) {
				t.Errorf("want %q, got:\n%s", tc.want, block)
			}
		})
	}
}

// backend_compute_pool is only meaningful on the EC2 branch, and an empty
// string is the variable's default, so an unset one is omitted rather than
// rendered as "".
func TestComputeTemplate_EC2Backend(t *testing.T) {
	overlay := mergeOverlays(
		workloadOverlay(t, map[string]interface{}{
			"backend_runtime":      "ec2",
			"backend_compute_pool": "general",
		}),
		computeOverlay(computePool(nil)),
	)
	block := renderComputeBlock(t, overlay)

	for _, want := range []string{
		`backend_runtime = "ec2"`,
		`backend_compute_pool = "general"`,
		`compute_pools = [`,
		`name = "general"`,
		`instance_types = ["m7i.large"]`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("want %q, got:\n%s", want, block)
		}
	}
}

func TestComputeTemplate_BackendComputePoolOmittedWhenUnset(t *testing.T) {
	tests := []struct {
		name    string
		overlay map[string]interface{}
		want    string // "" means backend_compute_pool must be absent
	}{
		{
			name:    "unset",
			overlay: nil,
			want:    "",
		},
		{
			name:    "empty string is what an unset pool looks like after migration",
			overlay: workloadOverlay(t, map[string]interface{}{"backend_compute_pool": ""}),
			want:    "",
		},
		{
			name:    "a real name renders",
			overlay: workloadOverlay(t, map[string]interface{}{"backend_compute_pool": "general"}),
			want:    `backend_compute_pool = "general"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			block := renderComputeBlock(t, tc.overlay)

			if tc.want == "" {
				if strings.Contains(block, "backend_compute_pool") {
					t.Errorf("backend_compute_pool should be absent, got:\n%s", block)
				}
				return
			}
			if !strings.Contains(block, tc.want) {
				t.Errorf("want %q, got:\n%s", tc.want, block)
			}
		})
	}
}

// bridge is the default in three places -- the Go structs, this template, and
// optional(string, "bridge") in modules/workloads/variables.tf. A pool that
// omits the key has to render and plan identically whichever layer supplied
// it, and the argument is emitted unconditionally so the generated main.tf
// says which mode a pool is in without the reader knowing the default.
func TestComputeTemplate_NetworkModeDefault(t *testing.T) {
	tests := []struct {
		name    string
		overlay map[string]interface{}
		want    string
	}{
		{
			name:    "omitted renders bridge",
			overlay: computeOverlay(computePool(nil)),
			want:    `network_mode = "bridge"`,
		},
		{
			name:    "an explicit bridge renders identically",
			overlay: computeOverlay(computePool(map[string]interface{}{"network_mode": "bridge"})),
			want:    `network_mode = "bridge"`,
		},
		{
			// assume_egress is what unlocks awsvpc at generate time (D-7). It
			// is an assertion about the operator's VPC, read by nothing under
			// modules/, so it travels no further than the validator — see
			// TestComputeTemplate_PoolFields for the absence assertion.
			name: "awsvpc renders verbatim",
			overlay: computeOverlay(computePool(map[string]interface{}{
				"network_mode":  "awsvpc",
				"assume_egress": true,
			})),
			want: `network_mode = "awsvpc"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			block := renderComputeBlock(t, tc.overlay)
			if !strings.Contains(block, tc.want) {
				t.Errorf("want %q, got:\n%s", tc.want, block)
			}
		})
	}
}

// 0 is a real value for both of these -- min_size: 0 is a pool that scales to
// nothing, on_demand_base: 0 is a pure-spot pool -- so a truthiness test would
// drop the argument and let the variable default (1 and 0) take over. That is
// silent, and for min_size it is also wrong.
func TestComputeTemplate_MinSizeZero(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		overlay map[string]interface{}
		want    string // "" means the argument must be absent
	}{
		{
			name:    "min_size absent omits the argument",
			field:   "min_size",
			overlay: computeOverlay(computePool(nil)),
			want:    "",
		},
		{
			name:    "min_size zero survives as zero",
			field:   "min_size",
			overlay: computeOverlay(computePool(map[string]interface{}{"min_size": 0})),
			want:    "min_size = 0",
		},
		{
			name:    "a real min_size renders",
			field:   "min_size",
			overlay: computeOverlay(computePool(map[string]interface{}{"min_size": 3})),
			want:    "min_size = 3",
		},
		{
			name:    "on_demand_base absent omits the argument",
			field:   "on_demand_base",
			overlay: computeOverlay(computePool(nil)),
			want:    "",
		},
		{
			// on_demand_base only means anything under spot_with_base, and the
			// validator refuses the pairing that reads as "one guaranteed
			// instance, the rest spot" but silently is not.
			name:  "on_demand_base zero survives as zero",
			field: "on_demand_base",
			overlay: computeOverlay(computePool(map[string]interface{}{
				"capacity_type":  "spot_with_base",
				"on_demand_base": 0,
			})),
			want: "on_demand_base = 0",
		},
		{
			name:  "a real on_demand_base renders",
			field: "on_demand_base",
			overlay: computeOverlay(computePool(map[string]interface{}{
				"capacity_type":  "spot_with_base",
				"on_demand_base": 2,
			})),
			want: "on_demand_base = 2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			list := renderPoolList(t, tc.overlay)

			if tc.want == "" {
				if strings.Contains(list, tc.field) {
					t.Errorf("%s should be absent when unset, got:\n%s", tc.field, list)
				}
				return
			}
			if !strings.Contains(list, tc.want) {
				t.Errorf("want %q, got:\n%s", tc.want, list)
			}
		})
	}
}

// Every §5.1 key reaches the module call, and the ones with defaults render
// their default rather than vanishing. A field declared in variables.tf but
// never emitted here is the silent failure this whole file exists to catch:
// Terraform's object type conversion drops an undeclared attribute without a
// word, and a never-emitted one simply takes its default forever.
func TestComputeTemplate_PoolFields(t *testing.T) {
	pool := computePool(map[string]interface{}{
		"enabled":         true,
		"instance_types":  []interface{}{"m7g.large", "m6g.large"},
		"capacity_type":   "spot_with_base",
		"on_demand_base":  1,
		"min_size":        2,
		"max_size":        9,
		"target_capacity": 80,
		"network_mode":    "awsvpc",
		"assume_egress":   true,
		"ami_family":      "al2023_arm64",
		"ami_id":          "ami-00112233445566778",
		"root_volume_gb":  50,
		"user_data_extra": "echo hello",
		"extra_volumes": []interface{}{
			map[string]interface{}{"device_name": "/dev/xvdb", "size_gb": 100, "type": "gp3"},
		},
		"instance_policies": []interface{}{
			map[string]interface{}{
				"actions":   []interface{}{"s3:GetObject"},
				"resources": []interface{}{"arn:aws:s3:::example/*"},
			},
		},
	})
	list := renderPoolList(t, computeOverlay(pool))

	for _, want := range []string{
		`name = "general"`,
		`enabled = true`,
		`instance_types = ["m7g.large","m6g.large"]`,
		`capacity_type = "spot_with_base"`,
		`on_demand_base = 1`,
		`min_size = 2`,
		`max_size = 9`,
		`target_capacity = 80`,
		`network_mode = "awsvpc"`,
		`ami_family = "al2023_arm64"`,
		`ami_id = "ami-00112233445566778"`,
		`root_volume_gb = 50`,
		`user_data_extra = <<-EOT`,
		`"device_name":"/dev/xvdb"`,
		`"actions":["s3:GetObject"]`,
	} {
		if !strings.Contains(list, want) {
			t.Errorf("want %q, got:\n%s", want, list)
		}
	}

	// assume_egress is a generate-time assertion (D-7). Nothing under modules/
	// reads it and variables.tf does not declare it, so emitting it would make
	// terraform fail on an unsupported attribute.
	if strings.Contains(list, "assume_egress") {
		t.Errorf("assume_egress must not be rendered, got:\n%s", list)
	}
}

// The defaults a pool inherits when it says nothing.
func TestComputeTemplate_PoolDefaults(t *testing.T) {
	list := renderPoolList(t, computeOverlay(computePool(nil)))

	for _, want := range []string{
		`enabled = true`,
		`capacity_type = "on_demand"`,
		`network_mode = "bridge"`,
		`max_size = 6`,
		`target_capacity = 100`,
		`ami_family = "al2023"`,
		`root_volume_gb = 30`,
	} {
		if !strings.Contains(list, want) {
			t.Errorf("want %q, got:\n%s", want, list)
		}
	}

	// ami_id, user_data_extra, extra_volumes and instance_policies all default
	// to "" or [] in variables.tf, so emitting them empty would be noise.
	for _, absent := range []string{"ami_id", "user_data_extra", "extra_volumes", "instance_policies"} {
		if strings.Contains(list, absent) {
			t.Errorf("%s should be absent when unset, got:\n%s", absent, list)
		}
	}
}

func TestComputeTemplate_MultiplePoolsAreCommaSeparated(t *testing.T) {
	overlay := computeOverlay(
		computePool(map[string]interface{}{"name": "general"}),
		computePool(map[string]interface{}{
			"name":           "gpu",
			"ami_family":     "al2023_gpu",
			"instance_types": []interface{}{"g5.xlarge"},
		}),
	)
	list := normalizeHCL(computePoolsList(t, overlay))

	if !strings.Contains(list, `name = "general"`) || !strings.Contains(list, `name = "gpu"`) {
		t.Errorf("both pools should render, got:\n%s", list)
	}
	// Exactly one separator between two objects, and none after the last.
	if n := strings.Count(list, "},"); n != 1 {
		t.Errorf("want exactly one object separator between two pools, got %d:\n%s", n, list)
	}
}

// rawHCLOpener matches a `${` or `%{` that is NOT already escaped. HCL escapes
// by doubling the sigil, so `$${` and `%%{` are literals and anything else is
// syntax Terraform will try to evaluate.
var rawHCLOpener = regexp.MustCompile(`(^|[^$])\$\{|(^|[^%])%\{`)

// The injection fixture, and the reason this phase owns raymond.go as well as
// the template.
//
// A user pastes a boot script into a textarea. It contains `${HOSTNAME}`,
// because essentially every non-trivial user-data script does. json.Marshal --
// which is all the plain `array` helper does -- escapes <, > and & but not $ or
// %, and a bare {{value}} would only HTML-escape. Either way the sequence
// reaches generated HCL intact, where `${` opens an interpolation inside quoted
// strings and heredocs alike. terraform plan then fails with "There is no
// variable named HOSTNAME" -- but the failure is the mild outcome. The real
// defect is that config content is being parsed as Terraform code, and a `"`
// in a quoted-string position ends the string and hands the parser whatever
// follows.
//
// So the assertion is not "the render succeeded", it is: inside compute_pools,
// no unescaped opener survives anywhere, and the quote is backslash-escaped.
func TestComputeTemplate_UserDataEscaping(t *testing.T) {
	pool := computePool(map[string]interface{}{
		// A double quote and a ${ in the same value, in the heredoc position.
		"user_data_extra": "echo \"${HOSTNAME} booted\" >> /var/log/pool\n%{ if true }danger%{ endif }\n",
		// The same two hazards in a quoted-string position, where an
		// unescaped quote is a parser breakout rather than a bad value.
		//
		// ami_id rather than name deliberately: `name` is subject to a format
		// rule in the validator, so a hostile name would be refused before it
		// ever reached the template and this test would be asserting the
		// validator's behaviour instead of the helper's. hclString's handling
		// of a quoted scalar is pinned directly in TestHCLStringHelper.
		"ami_id": `ami-0abc" ${var.injected}`,
		// IAM documents carry ${aws:PrincipalTag/...} routinely.
		"instance_policies": []interface{}{
			map[string]interface{}{
				"actions":   []interface{}{"s3:GetObject"},
				"resources": []interface{}{"arn:aws:s3:::bucket/${aws:PrincipalTag/team}/*"},
			},
		},
		"extra_volumes": []interface{}{
			map[string]interface{}{"device_name": "/dev/xvdb${x}", "size_gb": 100},
		},
	})
	list := computePoolsList(t, computeOverlay(pool))

	if loc := rawHCLOpener.FindStringIndex(list); loc != nil {
		t.Errorf("unescaped HCL template opener %q at %d in the rendered pool:\n%s",
			list[loc[0]:loc[1]], loc[0], list)
	}

	for _, want := range []string{
		// Heredoc: openers doubled, the quote left alone. HCL processes
		// backslash escapes only in quoted templates, so a \" emitted here
		// would be a literal backslash in the user's boot script.
		`echo "$${HOSTNAME} booted" >> /var/log/pool`,
		`%%{ if true }danger%%{ endif }`,
		// Quoted string: the quote backslash-escaped, the opener doubled.
		`ami_id          = "ami-0abc\" $${var.injected}"`,
		// Lists: escaped inside the JSON, which json.Marshal does not do.
		`arn:aws:s3:::bucket/$${aws:PrincipalTag/team}/*`,
		`/dev/xvdb$${x}`,
	} {
		if !strings.Contains(list, want) {
			t.Errorf("want %q, got:\n%s", want, list)
		}
	}

	// The heredoc must not be HTML-escaped either: a double-stache call site
	// would turn echo "hi" into echo &quot;hi&quot; inside main.tf.
	if strings.Contains(list, "&quot;") {
		t.Errorf("the heredoc body was HTML-escaped, which corrupts the script:\n%s", list)
	}
}

// The other half of the same hazard, and the one escaping cannot reach: a boot
// script that contains a line which is nothing but `EOT` terminates its own
// heredoc. With a fixed delimiter the pool block ends three lines into the
// user's script and everything after it -- `rm -rf /` here -- is handed to the
// HCL parser as configuration. Config content becoming syntax, again.
//
// So the assertion is end-to-end: the whole script survives inside the pool,
// the module call still closes, and no line of the body equals the delimiter
// that is actually emitted.
func TestComputeTemplate_UserDataCannotCloseItsOwnHeredoc(t *testing.T) {
	script := "#!/bin/bash\ncat <<EOT > /etc/motd\nplanted\nEOT\nrm -rf /\n"
	pool := computePool(map[string]interface{}{"user_data_extra": script})
	list := computePoolsList(t, computeOverlay(pool))

	open := regexp.MustCompile(`user_data_extra = <<-(\S+)`).FindStringSubmatch(list)
	if open == nil {
		t.Fatalf("no heredoc was emitted for user_data_extra:\n%s", list)
	}
	marker := open[1]
	if marker == "EOT" {
		t.Errorf("the delimiter is still EOT, which this script closes on line 4:\n%s", list)
	}

	// Every line of the script is still inside the pool, including the one
	// after the planted terminator.
	for _, line := range []string{"cat <<EOT > /etc/motd", "planted", "rm -rf /"} {
		if !strings.Contains(list, line) {
			t.Errorf("the script lost %q, so the heredoc closed early:\n%s", line, list)
		}
	}

	// The body -- everything between the opener and the emitted terminator --
	// must contain no line equal to the marker.
	body := list[strings.Index(list, open[0])+len(open[0]):]
	end := strings.Index(body, "\n"+marker)
	if end < 0 {
		t.Fatalf("heredoc %s is never closed:\n%s", marker, list)
	}
	for _, line := range strings.Split(body[:end], "\n") {
		if strings.TrimSpace(line) == marker {
			t.Errorf("body line %q terminates the heredoc early:\n%s", line, list)
		}
	}
}

// A `$` that is not opening an interpolation must survive untouched. Doubling
// every `$` would rewrite `echo $HOME` as `echo $$HOME` and break correct
// scripts -- the escape HCL defines is `$${`, not `$$`.
func TestComputeTemplate_LoneDollarIsNotDoubled(t *testing.T) {
	pool := computePool(map[string]interface{}{
		"user_data_extra": "echo $HOME && printf '100%% done' && cost=$5\n",
	})
	list := computePoolsList(t, computeOverlay(pool))

	if !strings.Contains(list, `echo $HOME && printf '100%% done' && cost=$5`) {
		t.Errorf("a lone $ or %% must pass through unchanged, got:\n%s", list)
	}
}
