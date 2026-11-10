package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

func TestV31IsRegisteredAtTheCurrentVersion(t *testing.T) {
	if CurrentSchemaVersion != 31 {
		t.Fatalf("CurrentSchemaVersion = %d, want 31", CurrentSchemaVersion)
	}
	last := AllMigrations[len(AllMigrations)-1]
	if last.Version != CurrentSchemaVersion || last.Apply == nil {
		t.Fatalf("last registered migration is v%d (Apply set: %v), want v%d",
			last.Version, last.Apply != nil, CurrentSchemaVersion)
	}
}

func TestPolicyForEnvName(t *testing.T) {
	cases := map[string]envPolicy{
		"dev":        {AutoDeploy: true},
		"prod":       {IsProd: true, RootDomain: true},
		"production": {IsProd: true, RootDomain: true},
		"staging":    {},
		"sandbox":    {},
	}
	for name, want := range cases {
		if got := policyForEnvName(name); got != want {
			t.Errorf("policyForEnvName(%q) = %+v, want %+v", name, got, want)
		}
	}
}

func TestCreateEnvWritesThePolicy(t *testing.T) {
	dev := createEnv("acme", "dev", "10.1.0.0/16")
	if dev.IsProd || dev.AutoDeploy == nil || !*dev.AutoDeploy || dev.Domain.AddEnvDomainPrefix != nil {
		t.Errorf("dev: is_prod=%v auto_deploy=%v prefix=%v", dev.IsProd, dev.AutoDeploy, dev.Domain.AddEnvDomainPrefix)
	}
	prod := createEnv("acme", "prod", "10.2.0.0/16")
	if !prod.IsProd || prod.AutoDeploy == nil || *prod.AutoDeploy {
		t.Errorf("prod: is_prod=%v auto_deploy=%v", prod.IsProd, prod.AutoDeploy)
	}
	sandbox := createEnv("acme", "sandbox", "10.3.0.0/16")
	if sandbox.IsProd || sandbox.AutoDeploy == nil || *sandbox.AutoDeploy {
		t.Errorf("sandbox: is_prod=%v auto_deploy=%v", sandbox.IsProd, sandbox.AutoDeploy)
	}
}

// An omitted false would render as the template default of true and give prod
// a prod.<domain> prefix.
func TestRootDomainSurvivesSaving(t *testing.T) {
	out, err := yaml.Marshal(createEnv("acme", "prod", "10.2.0.0/16"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "add_env_domain_prefix: false") {
		t.Errorf("explicit false was dropped on save:\n%s", out)
	}
}

func v31Fixture(env string) map[string]interface{} {
	return map[string]interface{}{
		"env":      env,
		"workload": map[interface{}]interface{}{"backend_auto_deploy": true},
		"domain":   map[interface{}]interface{}{"root_zone_id": "Z000", "add_env_domain_prefix": true},
		"services": []interface{}{
			map[interface{}]interface{}{"name": "api", "auto_deploy": true},
			map[interface{}]interface{}{"name": "off", "auto_deploy": false},
		},
		"scheduled_tasks": []interface{}{
			map[interface{}]interface{}{"name": "cleanup", "auto_deploy": true},
		},
	}
}

func TestMigrateToV31_NonDevTurnsEveryTargetOff(t *testing.T) {
	data := v31Fixture("staging")
	if err := migrateToV31(data); err != nil {
		t.Fatal(err)
	}
	if data["auto_deploy"] != false {
		t.Errorf("auto_deploy = %v, want false", data["auto_deploy"])
	}
	if data["workload"].(map[interface{}]interface{})["backend_auto_deploy"] != false {
		t.Error("backend stayed on")
	}
	for _, key := range []string{"services", "scheduled_tasks"} {
		for _, raw := range data[key].([]interface{}) {
			if raw.(map[interface{}]interface{})["auto_deploy"] != false {
				t.Errorf("%s entry stayed on: %v", key, raw)
			}
		}
	}
	domain := data["domain"].(map[interface{}]interface{})
	if domain["add_env_domain_prefix"] != true || domain["root_zone_id"] != "Z000" {
		t.Errorf("a non-prod domain must not change: %v", domain)
	}
}

func TestMigrateToV31_DevKeepsItsTargets(t *testing.T) {
	data := v31Fixture("dev")
	if err := migrateToV31(data); err != nil {
		t.Fatal(err)
	}
	if data["auto_deploy"] != true {
		t.Errorf("auto_deploy = %v, want true", data["auto_deploy"])
	}
	services := data["services"].([]interface{})
	if services[0].(map[interface{}]interface{})["auto_deploy"] != true ||
		services[1].(map[interface{}]interface{})["auto_deploy"] != false {
		t.Errorf("dev targets must keep their values: %v", services)
	}
}

func TestMigrateToV31_ProdDomainBecomesFlags(t *testing.T) {
	data := v31Fixture("prod")
	if err := migrateToV31(data); err != nil {
		t.Fatal(err)
	}
	if data["is_prod"] != true || data["auto_deploy"] != false {
		t.Errorf("is_prod=%v auto_deploy=%v", data["is_prod"], data["auto_deploy"])
	}
	domain := data["domain"].(map[interface{}]interface{})
	if domain["add_env_domain_prefix"] != false {
		t.Errorf("add_env_domain_prefix = %v, want false", domain["add_env_domain_prefix"])
	}
	if _, has := domain["root_zone_id"]; has {
		t.Error("root_zone_id would render the delegation branch for prod")
	}
}

// "production" flags as prod but keeps the domain it already renders.
func TestMigrateToV31_ProductionKeepsItsDomain(t *testing.T) {
	data := v31Fixture("production")
	if err := migrateToV31(data); err != nil {
		t.Fatal(err)
	}
	if data["is_prod"] != true {
		t.Error("is_prod not set")
	}
	domain := data["domain"].(map[interface{}]interface{})
	if domain["add_env_domain_prefix"] != true || domain["root_zone_id"] != "Z000" {
		t.Errorf("domain changed: %v", domain)
	}
}

func TestECRFlagsReachTheTemplate(t *testing.T) {
	out := renderMainHBS(t, map[string]interface{}{
		"ecr_strategy":     "cross_account",
		"ecr_image_events": true,
		"ecr_trusted_accounts": []interface{}{
			map[interface{}]interface{}{"account_id": "111111111111", "env": "sandbox", "region": "us-east-1", "forward_image_events": true},
		},
		"scheduled_tasks": []interface{}{
			map[interface{}]interface{}{"name": "cleanup", "schedule": "rate(1 day)"},
		},
	})
	for _, want := range []string{
		`ecr_image_events = true`,
		`forward_image_events = true`,
		`ecr_strategy = "cross_account"`,
		`"111111111111",`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered Terraform is missing %s", want)
		}
	}
}

// Generated Terraform reads flags. A comparison against the environment's name
// belongs in policyForEnvName.
func TestTerraformDoesNotBranchOnTheEnvName(t *testing.T) {
	tfCheck := regexp.MustCompile(`var\.env\s*[!=]=|contains\([^)]*,\s*var\.env\)`)
	err := filepath.WalkDir(filepath.Join("..", "modules"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".terraform" || d.Name() == "tests" || d.Name() == "testdata") {
			return filepath.SkipDir
		}
		if d.IsDir() || filepath.Ext(path) != ".tf" {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(src), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
			if tfCheck.MatchString(line) {
				t.Errorf("%s branches on the environment name: %s", path, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	hbs, err := os.ReadFile(filepath.Join("..", "env", "main.hbs"))
	if err != nil {
		t.Fatal(err)
	}
	if m := regexp.MustCompile(`\{\{#compare (\.\./)*env\b[^}]*\}\}`).Find(hbs); m != nil {
		t.Errorf("env/main.hbs branches on the environment name: %s", m)
	}
}

// Only the policy code compares against the production names; everything else
// reads is_prod or the domain flags.
func TestOnlyThePolicyCodeReadsProductionNames(t *testing.T) {
	allowed := map[string]bool{"env_policy.go": true, "migrations.go": true}
	cmp := regexp.MustCompile(`(==|!=|===|!==)\s*"(prod|production)"|"(prod|production)"\s*(==|!=|===|!==)`)
	check := func(root string, exts ...string) {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "webapp") {
				return filepath.SkipDir
			}
			name := d.Name()
			if d.IsDir() || allowed[name] || strings.HasSuffix(name, "_test.go") || strings.Contains(name, ".stories.") {
				return nil
			}
			for _, ext := range exts {
				if strings.HasSuffix(name, ext) {
					src, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					for _, line := range strings.Split(string(src), "\n") {
						// NODE_ENV is the build mode, not an environment name.
						if m := cmp.FindString(line); m != "" && !strings.Contains(line, "NODE_ENV") {
							t.Errorf("%s compares against a production name: %s", path, m)
						}
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	check(".", ".go")
	check(filepath.Join("..", "web", "src"), ".ts", ".tsx")
}
