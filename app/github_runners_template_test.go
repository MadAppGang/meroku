package main

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// The YAML -> HCL link for github_runners, rendered through the same
// preprocessEnvMap and env/main.hbs that applyTemplate uses.

// renderGitHubRunners renders the shipped project/dev.yaml with overlay on top,
// through preprocessEnvMap, and requires the result to parse.
func renderGitHubRunners(t *testing.T, overlay map[string]interface{}) string {
	t.Helper()
	out := renderMainHBS(t, overlay)
	if err := validateGeneratedHCL("main.tf", out); err != nil {
		t.Fatalf("rendered main.tf does not parse: %v", err)
	}
	return out
}

func githubRunnersModuleBlock(t *testing.T, rendered string) string {
	t.Helper()
	start := strings.Index(rendered, `module "github_runners" {`)
	if start < 0 {
		t.Fatal(`module "github_runners" is not in the rendered main.tf`)
	}
	end := strings.Index(rendered[start:], "\n}\n")
	if end < 0 {
		t.Fatal(`module "github_runners" block is not closed`)
	}
	return rendered[start : start+end+2]
}

// templateEnvWithRunners is an overlay with a custom VPC, so vpc_access has
// private subnets to use when the strategy is a NAT one.
func templateEnvWithRunners(strategy string, runners map[string]interface{}) map[string]interface{} {
	overlay := map[string]interface{}{
		"use_default_vpc": false,
		"github_runners":  runners,
	}
	if strategy != "" {
		overlay["egress_strategy"] = strategy
	}
	return overlay
}

func TestTemplate_GitHubRunners_AbsentOrDisabledRendersNothing(t *testing.T) {
	for label, block := range map[string]interface{}{
		"absent":   nil,
		"disabled": map[string]interface{}{"enabled": false, "token_secret": "x"},
	} {
		t.Run(label, func(t *testing.T) {
			env := map[string]interface{}{}
			if block != nil {
				env["github_runners"] = block
			}
			out := renderGitHubRunners(t, env)
			if strings.Contains(out, `module "github_runners"`) || strings.Contains(out, "github_runner_labels") {
				t.Error("a config without enabled github_runners must render no module and no output")
			}
			// The provider stays while token_secret is set, so removing runners
			// can still delete their GitHub webhooks.
			if got, want := strings.Contains(out, `provider "github"`), label == "disabled"; got != want {
				t.Errorf("github provider rendered = %v, want %v", got, want)
			}
		})
	}
}

func TestTemplate_GitHubRunners_ProviderTokenIsEphemeral(t *testing.T) {
	env := templateEnvWithRunners("", map[string]interface{}{
		"enabled":      true,
		"token_secret": "meroku/github-runners/acme-dev",
		"runners": []interface{}{
			map[string]interface{}{"name": "default", "repository": "example-org/some-repo"},
		},
	})
	out := renderGitHubRunners(t, env)
	for _, want := range []string{
		`ephemeral "aws_secretsmanager_secret_version" "github_runners_token"`,
		`secret_id = "meroku/github-runners/acme-dev"`,
		`owner = "example-org"`,
		`token = jsondecode(ephemeral.aws_secretsmanager_secret_version.github_runners_token.secret_string).Token`,
		`source  = "integrations/github"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered template lacks %q", want)
		}
	}
	if strings.Contains(out, `data "aws_secretsmanager_secret_version"`) {
		t.Error("the token must be read ephemerally, never through a data source that stores it in state")
	}
}

func TestTemplate_GitHubRunners_RendersRunnersWithDefaults(t *testing.T) {
	env := templateEnvWithRunners("", map[string]interface{}{
		"enabled":      true,
		"token_secret": "meroku/github-runners/instagram-dev",
		"runners": []interface{}{
			map[string]interface{}{
				"name":       "default",
				"repository": "example-org/some-repo",
				"env_variables": []interface{}{
					map[string]interface{}{"name": "FOO", "value": `say "hi" ${HOME}`},
				},
				"secrets": []interface{}{
					map[string]interface{}{"name": "DB_PASSWORD", "ssm": "/instagram/dev/db_password"},
					map[string]interface{}{"name": "API_KEY", "secrets_manager": "instagram/dev/api:key"},
				},
			},
			map[string]interface{}{
				"name":            "org",
				"organization":    "example-org",
				"compute_type":    "BUILD_GENERAL1_LARGE",
				"image":           "aws/codebuild/amazonlinux-aarch64-standard:3.0",
				"docker":          true,
				"timeout_minutes": 120,
			},
		},
	})

	out := renderGitHubRunners(t, env)
	block := githubRunnersModuleBlock(t, out)

	for _, want := range []string{
		`source       = "../../infrastructure/modules/github_runners"`,
		`token_secret = "meroku/github-runners/instagram-dev"`,
		`vpc_access   = false`,
		// Defaults filled in for the first runner.
		`name            = "default"`,
		`repository      = "example-org/some-repo"`,
		`compute_type    = "BUILD_GENERAL1_SMALL"`,
		`image           = "aws/codebuild/amazonlinux-x86_64-standard:5.0"`,
		`docker          = false`,
		`timeout_minutes = 60`,
		// Escaped: the quote cannot end the string, ${ is not interpolated.
		`"value":"say \"hi\" $${HOME}"`,
		`"ssm":"/instagram/dev/db_password"`,
		`"secrets_manager":"instagram/dev/api:key"`,
		// Explicit values for the second.
		`organization    = "example-org"`,
		`compute_type    = "BUILD_GENERAL1_LARGE"`,
		`docker          = true`,
		`timeout_minutes = 120`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("module block is missing %s\n%s", want, block)
		}
	}

	if strings.Contains(block, "subnet_ids") || strings.Contains(block, "vpc_id") {
		t.Error("without vpc_access the module must not receive subnets")
	}
	if !strings.Contains(out, `output "github_runner_labels"`) {
		t.Error("the runs-on labels must be an environment output")
	}
}

func TestTemplate_GitHubRunners_VPCAccessPassesPrivateSubnets(t *testing.T) {
	env := templateEnvWithRunners("nat_gateway", map[string]interface{}{
		"enabled":      true,
		"token_secret": "meroku/github-runners/instagram-dev",
		"vpc_access":   true,
		"runners": []interface{}{
			map[string]interface{}{"name": "default", "repository": "example-org/some-repo"},
		},
	})

	block := githubRunnersModuleBlock(t, renderGitHubRunners(t, env))
	for _, want := range []string{
		"vpc_access   = true",
		"vpc_id       = local.vpc_id",
		"subnet_ids   = module.vpc.private_subnet_ids",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("module block is missing %s\n%s", want, block)
		}
	}
}

// A TUI or API save goes through the Env struct. A block with no struct field
// would be dropped by the next unrelated save, and the runners destroyed on the
// apply after that.
func TestGitHubRunners_RoundTripsThroughEnvStruct(t *testing.T) {
	const doc = `
project: acme
env: dev
github_runners:
  enabled: true
  token_secret: meroku/github-runners/acme-dev
  vpc_access: true
  runners:
    - name: default
      repository: example-org/some-repo
      compute_type: BUILD_GENERAL1_MEDIUM
      image: aws/codebuild/amazonlinux-x86_64-standard:5.0
      docker: true
      timeout_minutes: 90
      env_variables:
        - name: FOO
          value: bar
      secrets:
        - name: DB_PASSWORD
          ssm: /acme/dev/db_password
        - name: API_KEY
          secrets_manager: acme/dev/api:key
    - name: org
      organization: example-org
`
	var first Env
	if err := yaml.Unmarshal([]byte(doc), &first); err != nil {
		t.Fatal(err)
	}
	if first.GitHubRunners == nil || len(first.GitHubRunners.Runners) != 2 {
		t.Fatalf("github_runners not loaded: %+v", first.GitHubRunners)
	}

	saved, err := yaml.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var second Env
	if err := yaml.Unmarshal(saved, &second); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.GitHubRunners, second.GitHubRunners) {
		t.Errorf("github_runners changed across save:\nbefore %+v\nafter  %+v", first.GitHubRunners, second.GitHubRunners)
	}

	// The raw maps of the original and the saved file must agree on the block,
	// so nothing the user wrote is lost or invented.
	var rawOriginal, rawSaved map[string]interface{}
	if err := yaml.Unmarshal([]byte(doc), &rawOriginal); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(saved, &rawSaved); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rawOriginal["github_runners"], rawSaved["github_runners"]) {
		t.Errorf("saved YAML differs:\nbefore %#v\nafter  %#v", rawOriginal["github_runners"], rawSaved["github_runners"])
	}
}

func TestGitHubRunners_AbsentStaysAbsentOnSave(t *testing.T) {
	saved, err := yaml.Marshal(Env{Project: "acme", Env: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "github_runners") {
		t.Errorf("an environment without runners must save without the key:\n%s", saved)
	}
}
