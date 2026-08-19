package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// The twelve refusals of §4.6, one case each, plus the message-content
// assertions — because the whole point of validating here is that the error a
// user reads names the key and the fix. The errors these replace name neither:
// `Error: Invalid index` against a locals expression, an ASG scaling activity
// nobody looks at, or a task that sits in PROVISIONING with no event at all.
//
// Fixture builders mirror alb_validation_test.go:16-21 — small, positional, and
// obvious at the call site.

// computePoolFixture is a valid pool; each case breaks exactly one thing.
func computePoolFixture(name string, overrides map[string]interface{}) map[string]interface{} {
	pool := map[string]interface{}{
		"name":            name,
		"enabled":         true,
		"instance_types":  []interface{}{"m6i.large", "m6a.large"},
		"capacity_type":   "on_demand",
		"min_size":        1,
		"max_size":        6,
		"target_capacity": 100,
		"network_mode":    "bridge",
		"ami_family":      "al2023",
		"root_volume_gb":  30,
	}
	for k, v := range overrides {
		pool[k] = v
	}
	return pool
}

// computeConfigFixture assembles the three top-level keys the validator reads.
// A nil section is left out entirely, which is what an environment that never
// heard of compute pools looks like.
func computeConfigFixture(pools []map[string]interface{}, workload map[string]interface{}, services []map[string]interface{}) map[string]interface{} {
	cfg := map[string]interface{}{}

	if pools != nil {
		list := make([]interface{}, 0, len(pools))
		for _, p := range pools {
			list = append(list, p)
		}
		cfg["compute"] = map[string]interface{}{"pools": list}
	}
	if workload != nil {
		cfg["workload"] = workload
	}
	if services != nil {
		list := make([]interface{}, 0, len(services))
		for _, s := range services {
			list = append(list, s)
		}
		cfg["services"] = list
	}
	return cfg
}

func computeEC2Service(name, pool string, overrides map[string]interface{}) map[string]interface{} {
	svc := map[string]interface{}{
		"name":         name,
		"runtime":      "ec2",
		"compute_pool": pool,
	}
	for k, v := range overrides {
		svc[k] = v
	}
	return svc
}

func TestValidateComputeConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    map[string]interface{}
		wantError bool
	}{
		// --- the shapes that must keep working ---
		{
			name:      "no compute block at all is fine",
			config:    map[string]interface{}{},
			wantError: false,
		},
		{
			name: "a fargate environment with pools defined is fine",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				map[string]interface{}{"backend_runtime": "fargate"},
				[]map[string]interface{}{{"name": "api", "runtime": "fargate"}},
			),
			wantError: false,
		},
		{
			name: "an ec2 backend on an enabled pool is fine",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				map[string]interface{}{"backend_runtime": "ec2", "backend_compute_pool": "general"},
				nil,
			),
			wantError: false,
		},
		{
			name: "a service with no runtime key at all is fargate",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				nil,
				[]map[string]interface{}{{"name": "api", "compute_pool": "nowhere"}},
			),
			wantError: false,
		},

		// --- rule 1: a reference to a pool that does not exist (AC-43) ---
		{
			name: "an ec2 service naming a pool nothing defines is refused",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "gpu", nil)},
			),
			wantError: true,
		},

		// --- rule 2: min_size > max_size ---
		{
			name: "min_size above max_size is refused",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"min_size": 3, "max_size": 2,
				})},
				nil, nil,
			),
			wantError: true,
		},
		{
			name: "min_size equal to max_size is fine",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"min_size": 2, "max_size": 2,
				})},
				nil, nil,
			),
			wantError: false,
		},
		{
			name: "min_size zero is a value, not an absence (CON-7)",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"min_size": 0, "max_size": 6,
				})},
				nil, nil,
			),
			wantError: false,
		},

		// --- rule 3: on_demand together with on_demand_base ---
		{
			name: "on_demand with an on_demand_base is refused",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"capacity_type": "on_demand", "on_demand_base": 1,
				})},
				nil, nil,
			),
			wantError: true,
		},
		{
			// The dangerous half of rule 3: ec2_capacity.tf renders the base as
			// 0 under plain spot, so this config asks for one guaranteed
			// instance and gets a 100% spot pool — while the mere presence of
			// on_demand_base suppresses the pure-spot advisory that would have
			// said so.
			name: "spot with an on_demand_base is refused",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"capacity_type": "spot", "on_demand_base": 1,
				})},
				nil, nil,
			),
			wantError: true,
		},
		{
			name: "spot with no on_demand_base is fine",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"capacity_type": "spot",
				})},
				nil, nil,
			),
			wantError: false,
		},
		{
			name: "spot_with_base with an on_demand_base is fine",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"capacity_type": "spot_with_base", "on_demand_base": 1,
				})},
				nil, nil,
			),
			wantError: false,
		},

		// --- rule 4: a GPU AMI with nothing to drive ---
		{
			name: "the gpu ami family with no gpu instance type is refused",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("gpu", map[string]interface{}{
					"ami_family":     "al2023_gpu",
					"instance_types": []interface{}{"m6i.large", "c7i.large"},
				})},
				nil, nil,
			),
			wantError: true,
		},
		{
			name: "the gpu ami family with a gpu instance type is fine",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("gpu", map[string]interface{}{
					"ami_family":     "al2023_gpu",
					"instance_types": []interface{}{"g5.xlarge"},
				})},
				nil, nil,
			),
			wantError: false,
		},
		{
			// A family this parser has never seen must produce silence, not a
			// refusal — otherwise a type AWS ships next month breaks a config
			// that works.
			name: "an unrecognised family is never refused for lacking a gpu",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("ml", map[string]interface{}{
					"ami_family":     "al2023_gpu",
					"instance_types": []interface{}{"inf2.xlarge"},
				})},
				nil, nil,
			),
			wantError: false,
		},

		// --- rule 5: arm64 family, x86 instance ---
		{
			name: "the arm64 ami family with an x86 instance type is refused",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("arm", map[string]interface{}{
					"ami_family":     "al2023_arm64",
					"instance_types": []interface{}{"m7g.large", "m6i.large"},
				})},
				nil, nil,
			),
			wantError: true,
		},
		{
			name: "the arm64 ami family with arm64 instance types is fine",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("arm", map[string]interface{}{
					"ami_family":     "al2023_arm64",
					"instance_types": []interface{}{"m7g.large", "c7g.large", "t4g.medium"},
				})},
				nil, nil,
			),
			wantError: false,
		},

		// --- rule 6: a task no instance in the pool can hold ---
		{
			name: "a service asking for more memory than the largest instance is refused",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "general", map[string]interface{}{
					"memory": 16384,
				})},
			),
			wantError: true,
		},
		{
			name: "memory that fits the largest instance is fine",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "general", map[string]interface{}{
					"memory": 4096,
				})},
			),
			wantError: false,
		},
		{
			// The backend carries its memory as a string of MiB, unlike a
			// service's int.
			name: "the backend's string memory is read too",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				map[string]interface{}{
					"backend_runtime":      "ec2",
					"backend_compute_pool": "general",
					"backend_memory":       "16384",
				},
				nil,
			),
			wantError: true,
		},
		{
			name: "an instance type with no known memory silences the rule",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("odd", map[string]interface{}{
					"instance_types": []interface{}{"m6i.large", "i3.metal"},
				})},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "odd", map[string]interface{}{
					"memory": 999999,
				})},
			),
			wantError: false,
		},

		// --- rule 7: duplicate pool names (EC-13) ---
		{
			name: "two pools with the same name are refused",
			config: computeConfigFixture(
				[]map[string]interface{}{
					computePoolFixture("general", nil),
					computePoolFixture("batch", nil),
					computePoolFixture("general", map[string]interface{}{"max_size": 2}),
				},
				nil, nil,
			),
			wantError: true,
		},

		// --- rule 8: a pool that is defined but disabled (C-7) ---
		{
			name: "an ec2 service on a disabled pool is refused",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("batch", map[string]interface{}{
					"enabled": false,
				})},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "batch", nil)},
			),
			wantError: true,
		},
		{
			name: "a disabled pool nothing runs on is fine",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("batch", map[string]interface{}{
					"enabled": false,
				})},
				nil, nil,
			),
			wantError: false,
		},

		// --- rule 9: runtime ec2 naming no pool, after synthesis ---
		{
			name: "an ec2 service with an empty compute_pool is refused when pools exist",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "", nil)},
			),
			wantError: true,
		},
		{
			name: "an ec2 backend with no backend_compute_pool key at all is refused",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				map[string]interface{}{"backend_runtime": "ec2"},
				nil,
			),
			wantError: true,
		},

		// --- rule 10: x86 family, arm64 instance (C-11) ---
		{
			name: "the default ami family with an arm64 instance type is refused",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"instance_types": []interface{}{"m6i.large", "m7g.large"},
				})},
				nil, nil,
			),
			wantError: true,
		},
		{
			name: "a pool that omits ami_family is still x86, so arm64 is refused",
			config: computeConfigFixture(
				[]map[string]interface{}{{
					"name":           "general",
					"instance_types": []interface{}{"c7g.large"},
				}},
				nil, nil,
			),
			wantError: true,
		},

		// --- rule 11: awsvpc with no asserted egress (C-1 / D-6 / D-7) ---
		{
			name: "awsvpc without assume_egress is refused",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("batch", map[string]interface{}{
					"network_mode": "awsvpc",
				})},
				nil, nil,
			),
			wantError: true,
		},
		{
			name: "awsvpc with assume_egress false is still refused",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("batch", map[string]interface{}{
					"network_mode": "awsvpc", "assume_egress": false,
				})},
				nil, nil,
			),
			wantError: true,
		},
		{
			name: "awsvpc with assume_egress true is accepted (D-7)",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("batch", map[string]interface{}{
					"network_mode": "awsvpc", "assume_egress": true,
				})},
				nil, nil,
			),
			wantError: false,
		},
		{
			name: "a pool that omits network_mode is bridge, not awsvpc (D-6)",
			config: computeConfigFixture(
				[]map[string]interface{}{{
					"name":           "general",
					"instance_types": []interface{}{"m6i.large"},
				}},
				nil, nil,
			),
			wantError: false,
		},

		// --- rule 12: X-Ray under bridge (C-1 ripple, DEV-30) ---
		{
			name: "xray on a service running on a bridge pool is refused",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "general", map[string]interface{}{
					"xray_enabled": true,
				})},
			),
			wantError: true,
		},
		{
			name: "xray on the backend running on a bridge pool is refused",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				map[string]interface{}{
					"backend_runtime":      "ec2",
					"backend_compute_pool": "general",
					"xray_enabled":         true,
				},
				nil,
			),
			wantError: true,
		},
		{
			name: "xray on an awsvpc pool with an asserted egress path is fine",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"network_mode": "awsvpc", "assume_egress": true,
				})},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "general", map[string]interface{}{
					"xray_enabled": true,
				})},
			),
			wantError: false,
		},
		{
			name: "xray on a fargate service is untouched",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				nil,
				[]map[string]interface{}{{
					"name": "api", "runtime": "fargate", "xray_enabled": true,
				}},
			),
			wantError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateComputeConfigMap(tc.config)

			if tc.wantError && err == nil {
				t.Fatal("expected an error, got none")
			}
			if !tc.wantError && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}

// Every refusal has to name the key that is wrong and the edit that fixes it.
// A message that says only "invalid compute configuration" would pass the table
// above and be worth nothing at the terminal.
func TestValidateComputeConfigErrorNamesTheKeysAndTheFix(t *testing.T) {
	tests := []struct {
		rule   string
		config map[string]interface{}
		want   []string
	}{
		{
			rule: "1 — missing pool (AC-43)",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "gpu", nil)},
			),
			want: []string{`services["api"].compute_pool`, `"gpu"`, `"general"`, "compute.pools"},
		},
		{
			rule: "2 — min_size above max_size",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"min_size": 3, "max_size": 2,
				})},
				nil, nil,
			),
			want: []string{`compute.pools["general"].min_size`, `compute.pools["general"].max_size`, "3", "2"},
		},
		{
			rule: "3 — on_demand with a base",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"capacity_type": "on_demand", "on_demand_base": 1,
				})},
				nil, nil,
			),
			want: []string{`compute.pools["general"].capacity_type`, `compute.pools["general"].on_demand_base`, "spot_with_base"},
		},
		{
			// The message must name the capacity type actually written, and say
			// what plain spot does with the base — "ignored" would be a lie
			// here, and the lie is the one that costs an outage.
			rule: "3 — spot with a base",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"capacity_type": "spot", "on_demand_base": 1,
				})},
				nil, nil,
			),
			want: []string{
				`compute.pools["general"].capacity_type is "spot"`,
				`compute.pools["general"].on_demand_base`,
				"100% spot",
				"spot_with_base",
			},
		},
		{
			rule: "4 — gpu family, no gpu",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("gpu", map[string]interface{}{
					"ami_family":     "al2023_gpu",
					"instance_types": []interface{}{"m6i.large", "c7i.large"},
				})},
				nil, nil,
			),
			want: []string{`compute.pools["gpu"].ami_family`, "al2023_gpu", "m6i.large, c7i.large", "g5.xlarge"},
		},
		{
			rule: "5 — arm64 family, x86 instance",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("arm", map[string]interface{}{
					"ami_family":     "al2023_arm64",
					"instance_types": []interface{}{"m7g.large", "m6i.large"},
				})},
				nil, nil,
			),
			want: []string{`compute.pools["arm"].ami_family`, `"m6i.large"`, "x86_64", "al2023"},
		},
		{
			rule: "6 — memory beyond the largest instance",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "general", map[string]interface{}{
					"memory": 16384,
				})},
			),
			want: []string{`services["api"].memory`, "16384", "m6i.large", "8192", "instance_types"},
		},
		{
			rule: "7 — duplicate names (EC-13)",
			config: computeConfigFixture(
				[]map[string]interface{}{
					computePoolFixture("general", nil),
					computePoolFixture("batch", nil),
					computePoolFixture("general", nil),
				},
				nil, nil,
			),
			want: []string{"compute.pools", `"general"`, "for_each", "1", "3"},
		},
		{
			rule: "8 — disabled pool (C-7)",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("batch", map[string]interface{}{
					"enabled": false,
				})},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "batch", nil)},
			),
			want: []string{
				`services["api"].compute_pool`,
				`compute.pools["batch"].enabled: true`,
				`aws_ecs_capacity_provider.pool["batch"]`,
				`services["api"].runtime: fargate`,
			},
		},
		{
			rule: "9 — ec2 with no pool named",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				map[string]interface{}{"backend_runtime": "ec2"},
				nil,
			),
			want: []string{
				"workload.backend_runtime",
				"workload.backend_compute_pool",
				`"general"`,
				"workload.backend_runtime: fargate",
			},
		},
		{
			rule: "10 — x86 family, arm64 instance (C-11)",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"instance_types": []interface{}{"m6i.large", "m7g.large"},
				})},
				nil, nil,
			),
			want: []string{`compute.pools["general"].ami_family`, `"m7g.large"`, "arm64", "al2023_arm64"},
		},
		{
			rule: "11 — awsvpc with no egress (D-6 / D-7)",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("batch", map[string]interface{}{
					"network_mode": "awsvpc",
				})},
				nil, nil,
			),
			want: []string{
				`compute.pools["batch"].network_mode`,
				`compute.pools["batch"].network_mode: bridge`,
				`compute.pools["batch"].assume_egress: true`,
				"NAT gateway",
				"meroku does not create one",
			},
		},
		{
			rule: "12 — xray under bridge (DEV-30)",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "general", map[string]interface{}{
					"xray_enabled": true,
				})},
			),
			want: []string{
				`services["api"].xray_enabled`,
				`compute.pools["general"]`,
				"bridge",
				`services["api"].xray_enabled: false`,
				"awsvpc",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.rule, func(t *testing.T) {
			err := validateComputeConfigMap(tc.config)
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error message does not mention %q:\n%s", want, err)
				}
			}
		})
	}
}

// yaml.v2 hands nested nodes back as map[interface{}]interface{} while the
// generate path converts them to map[string]interface{}. A check that only
// understood one of the two could be bypassed by whichever loader a caller
// happened to use (CON-4).
func TestValidateComputeConfig_AcceptsBothMapShapes(t *testing.T) {
	const doc = `
compute:
  pools:
    - name: batch
      instance_types: [m6i.large]
      network_mode: awsvpc
workload:
  backend_runtime: ec2
  backend_compute_pool: batch
`

	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(doc), &raw); err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	if _, isNested := raw["compute"].(map[interface{}]interface{}); !isNested {
		t.Fatalf("fixture is not the yaml.v2 shape this test exists for: %T", raw["compute"])
	}

	rawErr := validateComputeConfigMap(raw)
	if rawErr == nil {
		t.Fatal("the yaml.v2 shape bypassed the awsvpc refusal")
	}

	converted, ok := convertToJSONCompatible(raw).(map[string]interface{})
	if !ok {
		t.Fatal("converted fixture is not a mapping")
	}
	convertedErr := validateComputeConfigMap(converted)
	if convertedErr == nil {
		t.Fatal("the converted shape bypassed the awsvpc refusal")
	}

	if rawErr.Error() != convertedErr.Error() {
		t.Errorf("the two shapes produce different messages:\n%s\n---\n%s", rawErr, convertedErr)
	}
}

// Where the validator runs is as much of the contract as what it checks
// (DEV-9). Before filterDisabledItems it would refuse a disabled service's
// dangling reference; before synthesis it would refuse the one case FR-58
// exists to make work.
func TestValidateComputeConfig_RunsAfterFilterAndSynthesis(t *testing.T) {
	t.Run("a disabled service's dangling pool reference is not refused", func(t *testing.T) {
		cfg := computeConfigFixture(
			[]map[string]interface{}{computePoolFixture("general", nil)},
			nil,
			[]map[string]interface{}{computeEC2Service("api", "deleted-pool", map[string]interface{}{
				"enabled": false,
			})},
		)
		if err := preprocessEnvMap(cfg); err != nil {
			t.Fatalf("a disabled service was validated anyway: %v", err)
		}
	})

	t.Run("the same reference on an enabled service is refused", func(t *testing.T) {
		cfg := computeConfigFixture(
			[]map[string]interface{}{computePoolFixture("general", nil)},
			nil,
			[]map[string]interface{}{computeEC2Service("api", "deleted-pool", nil)},
		)
		if err := preprocessEnvMap(cfg); err == nil {
			t.Fatal("expected the dangling reference to be refused")
		}
	})

	t.Run("runtime ec2 with no pools anywhere is accepted, because synthesis ran first", func(t *testing.T) {
		cfg := computeConfigFixture(
			nil,
			map[string]interface{}{"backend_runtime": "ec2"},
			nil,
		)
		if err := preprocessEnvMap(cfg); err != nil {
			t.Fatalf("the FR-58 zero-config path was refused: %v", err)
		}
	})
}

// FR-58: `runtime: ec2` alone is a working one-line change. The injection and
// the reference to it are one operation — a pool nothing points at plans as
// `aws_ecs_capacity_provider.pool[""]`, which is not a convenience.
func TestSynthesizeDefaultPool_RewritesReferences(t *testing.T) {
	cfg := computeConfigFixture(
		nil,
		map[string]interface{}{"backend_runtime": "ec2"},
		[]map[string]interface{}{
			{"name": "api", "runtime": "ec2"},
			{"name": "web", "runtime": "fargate"},
		},
	)

	synthesizeDefaultComputePool(cfg)

	pools := computePoolViews(cfg)
	if len(pools) != 1 {
		t.Fatalf("expected exactly one synthesized pool, got %d", len(pools))
	}
	pool := pools[0]
	if pool.name != "default" {
		t.Errorf("synthesized pool name = %q, want \"default\"", pool.name)
	}
	if pool.networkMode != "bridge" {
		t.Errorf("synthesized network_mode = %q, want \"bridge\" (D-6)", pool.networkMode)
	}
	if pool.capacityType != "on_demand" {
		t.Errorf("synthesized capacity_type = %q, want \"on_demand\" (DEV-13)", pool.capacityType)
	}
	if !pool.enabled {
		t.Error("the synthesized pool is disabled")
	}
	if pool.minSize == nil || *pool.minSize != 1 || pool.maxSize == nil || *pool.maxSize != 6 {
		t.Errorf("synthesized size range = %v..%v, want 1..6", pool.minSize, pool.maxSize)
	}
	if len(pool.instanceTypes) != 3 {
		t.Errorf("synthesized instance_types = %v, want three types", pool.instanceTypes)
	}

	units := computeUnitViews(cfg)
	for _, u := range units {
		switch u.label {
		case "the backend", `services["api"]`:
			if u.pool != "default" {
				t.Errorf("%s.compute_pool = %q, want \"default\"", u.label, u.pool)
			}
		case `services["web"]`:
			if u.pool != "" {
				t.Errorf("a fargate service was rewritten to %q", u.pool)
			}
		}
	}

	// And the whole point: what it produced passes its own validator.
	if err := validateComputeConfigMap(cfg); err != nil {
		t.Fatalf("the synthesized configuration does not validate: %v", err)
	}
}

func TestSynthesizeDefaultPool_LeavesEverythingElseAlone(t *testing.T) {
	t.Run("a fargate-only environment gains no compute block", func(t *testing.T) {
		cfg := computeConfigFixture(
			nil,
			map[string]interface{}{"backend_runtime": "fargate"},
			[]map[string]interface{}{{"name": "api", "runtime": "fargate"}},
		)
		synthesizeDefaultComputePool(cfg)
		if _, exists := cfg["compute"]; exists {
			t.Fatalf("a fargate environment gained a compute block: %v", cfg["compute"])
		}
	})

	t.Run("an environment that defines pools keeps exactly those", func(t *testing.T) {
		cfg := computeConfigFixture(
			[]map[string]interface{}{computePoolFixture("general", nil)},
			map[string]interface{}{"backend_runtime": "ec2"},
			nil,
		)
		synthesizeDefaultComputePool(cfg)

		pools := computePoolViews(cfg)
		if len(pools) != 1 || pools[0].name != "general" {
			t.Fatalf("the user's pools were replaced: %v", pools)
		}
		// And the empty reference is rule 9's refusal, not a guess about which
		// of the user's pools was meant.
		if err := validateComputeConfigMap(cfg); err == nil {
			t.Fatal("an empty backend_compute_pool was silently filled in")
		}
	})

	t.Run("a unit that already names a pool is never rewritten", func(t *testing.T) {
		cfg := computeConfigFixture(
			nil,
			map[string]interface{}{"backend_runtime": "ec2", "backend_compute_pool": "gpu"},
			nil,
		)
		synthesizeDefaultComputePool(cfg)

		units := computeUnitViews(cfg)
		if len(units) != 1 || units[0].pool != "gpu" {
			t.Fatalf("backend_compute_pool was rewritten: %v", units)
		}
		// Rule 1 then refuses it, naming the pool that does not exist.
		if err := validateComputeConfigMap(cfg); err == nil {
			t.Fatal("a reference to a non-existent pool survived synthesis unrefused")
		}
	})
}

// FR-53: things worth saying that are not worth refusing. All three return a
// nil error — the assertion is that they are said at all.
func TestValidateComputeConfig_Warnings(t *testing.T) {
	t.Run("spot with no base behind an ALB", func(t *testing.T) {
		cfg := computeConfigFixture(
			[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
				"capacity_type": "spot",
			})},
			nil,
			[]map[string]interface{}{computeEC2Service("api", "general", nil)},
		)
		cfg["alb"] = map[string]interface{}{"enabled": true}

		if err := validateComputeConfigMap(cfg); err != nil {
			t.Fatalf("a warning was raised as an error: %v", err)
		}
		assertComputeWarns(t, computeWarnings(cfg), "spot_with_base")
	})

	t.Run("a pool nothing references", func(t *testing.T) {
		cfg := computeConfigFixture(
			[]map[string]interface{}{computePoolFixture("batch", nil)},
			map[string]interface{}{"backend_runtime": "fargate"},
			nil,
		)
		if err := validateComputeConfigMap(cfg); err != nil {
			t.Fatalf("a warning was raised as an error: %v", err)
		}
		assertComputeWarns(t, computeWarnings(cfg), "referenced by no workload")
	})

	t.Run("a bridge pool consumed through API Gateway", func(t *testing.T) {
		cfg := computeConfigFixture(
			[]map[string]interface{}{computePoolFixture("general", nil)},
			nil,
			[]map[string]interface{}{computeEC2Service("api", "general", nil)},
		)
		cfg["alb"] = map[string]interface{}{"enabled": false}

		if err := validateComputeConfigMap(cfg); err != nil {
			t.Fatalf("a warning was raised as an error: %v", err)
		}
		assertComputeWarns(t, computeWarnings(cfg), "Cloud Map SRV")
	})

	t.Run("a fargate environment says nothing at all", func(t *testing.T) {
		cfg := computeConfigFixture(
			nil,
			map[string]interface{}{"backend_runtime": "fargate"},
			[]map[string]interface{}{{"name": "api", "runtime": "fargate"}},
		)
		if warnings := computeWarnings(cfg); len(warnings) != 0 {
			t.Fatalf("an environment with no pools produced warnings: %v", warnings)
		}
	})
}

func assertComputeWarns(t *testing.T, warnings []string, want string) {
	t.Helper()
	for _, w := range warnings {
		if strings.Contains(w, want) {
			return
		}
	}
	t.Errorf("no warning mentions %q; got %v", want, warnings)
}

// The name parser is what three refusals rest on, and it has to be sure of
// itself in exactly one direction: an unrecognised family must produce silence,
// never a wrong answer.
func TestComputeInstanceShape(t *testing.T) {
	tests := []struct {
		name     string
		arch     string
		gpu      bool
		gpuKnown bool
	}{
		{name: "m6i.large", arch: "x86_64", gpuKnown: true},
		{name: "m7i-flex.large", arch: "x86_64", gpuKnown: true},
		{name: "m6a.large", arch: "x86_64", gpuKnown: true},
		{name: "c6in.xlarge", arch: "x86_64", gpuKnown: true},
		{name: "t3.medium", arch: "x86_64", gpuKnown: true},
		{name: "m7g.large", arch: "arm64", gpuKnown: true},
		{name: "c7gn.large", arch: "arm64", gpuKnown: true},
		{name: "t4g.medium", arch: "arm64", gpuKnown: true},
		{name: "x2gd.large", arch: "arm64", gpuKnown: true},
		{name: "im4gn.large", arch: "arm64", gpuKnown: true},
		{name: "a1.large", arch: "arm64", gpuKnown: true},
		{name: "g5.xlarge", arch: "x86_64", gpu: true, gpuKnown: true},
		{name: "g4dn.xlarge", arch: "x86_64", gpu: true, gpuKnown: true},
		{name: "g5g.xlarge", arch: "arm64", gpu: true, gpuKnown: true},
		{name: "p4d.24xlarge", arch: "x86_64", gpu: true, gpuKnown: true},
		// Neither answer is safe to refuse on.
		{name: "inf2.xlarge", arch: "x86_64"},
		{name: "trn1.2xlarge", arch: "x86_64"},
		{name: "mac2.metal"},
		{name: "nonsense"},
		{name: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeInstanceShape(tc.name)
			if got.arch != tc.arch {
				t.Errorf("arch = %q, want %q", got.arch, tc.arch)
			}
			if got.gpu != tc.gpu || got.gpuKnown != tc.gpuKnown {
				t.Errorf("gpu = %v (known %v), want %v (known %v)", got.gpu, got.gpuKnown, tc.gpu, tc.gpuKnown)
			}
		})
	}
}

// Rule 6's memory figures may over-state a size — never under-state one — so
// the rule can only fail to catch a real problem, not invent one.
func TestComputeInstanceMemoryMiB(t *testing.T) {
	tests := []struct {
		name  string
		want  int
		known bool
	}{
		{"m6i.large", 8192, true},    // from the fallback catalogue
		{"m6i.4xlarge", 65536, true}, // derived
		{"c7i.large", 4096, true},
		{"r7i.2xlarge", 65536, true},
		{"t3.small", 2048, true},
		{"t4g.medium", 4096, true},
		{"m7g.large", 8192, true},
		{"i3.large", 0, false},     // 15.25 GiB does not follow the ladder
		{"m6i.metal", 0, false},    // off the size ladder
		{"p4d.24xlarge", 0, false}, // no clean derivation, so no rule 6
		{"nonsense", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, known := computeInstanceMemoryMiB(tc.name)
			if known != tc.known {
				t.Fatalf("known = %v, want %v (got %d MiB)", known, tc.known, got)
			}
			if known && got != tc.want {
				t.Errorf("memory = %d MiB, want %d", got, tc.want)
			}
		})
	}
}
