package main

import (
	"strings"
	"testing"
)

// githubRunnersEnv is a NAT environment with one valid runner. Each test
// changes only the field it is about.
func githubRunnersEnv() map[string]interface{} {
	return map[string]interface{}{
		"use_default_vpc": false,
		"egress_strategy": "nat_gateway",
		"github_runners": map[string]interface{}{
			"enabled":      true,
			"token_secret": "meroku/github-runners/acme-dev",
			"vpc_access":   false,
			"runners": []interface{}{
				map[string]interface{}{
					"name":         "default",
					"repository":   "example-org/some-repo",
					"compute_type": "BUILD_GENERAL1_SMALL",
					"env_variables": []interface{}{
						map[string]interface{}{"name": "FOO", "value": "bar"},
					},
					"secrets": []interface{}{
						map[string]interface{}{"name": "DB_PASSWORD", "ssm": "/acme/dev/db_password"},
						map[string]interface{}{"name": "API_KEY", "secrets_manager": "acme/dev/api:key"},
					},
				},
			},
		},
	}
}

func githubRunnersBlock(env map[string]interface{}) map[string]interface{} {
	return env["github_runners"].(map[string]interface{})
}

func firstGitHubRunner(env map[string]interface{}) map[string]interface{} {
	return githubRunnersBlock(env)["runners"].([]interface{})[0].(map[string]interface{})
}

func requireGitHubRunnersError(t *testing.T, env map[string]interface{}, wants ...string) {
	t.Helper()
	err := validateGitHubRunnersConfigMap(env)
	if err == nil {
		t.Fatalf("expected an error mentioning %q, got none", wants)
	}
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q:\n%v", want, err)
		}
	}
}

func TestGitHubRunners_ValidConfigPasses(t *testing.T) {
	if err := validateGitHubRunnersConfigMap(githubRunnersEnv()); err != nil {
		t.Fatalf("valid config refused: %v", err)
	}
}

func TestGitHubRunners_AbsentOrDisabledIsNotChecked(t *testing.T) {
	if err := validateGitHubRunnersConfigMap(map[string]interface{}{}); err != nil {
		t.Errorf("absent block refused: %v", err)
	}

	// A disabled block renders nothing, so a half-written one must not block
	// generation.
	env := map[string]interface{}{
		"github_runners": map[string]interface{}{"enabled": false, "vpc_access": true},
	}
	if err := validateGitHubRunnersConfigMap(env); err != nil {
		t.Errorf("disabled block refused: %v", err)
	}
}

func TestGitHubRunners_EnabledRequiresTokenSecretAndRunners(t *testing.T) {
	env := map[string]interface{}{
		"github_runners": map[string]interface{}{"enabled": true},
	}
	requireGitHubRunnersError(t, env, "token_secret is required", "at least one runner")
}

func TestGitHubRunners_RunnerName(t *testing.T) {
	cases := map[string]string{
		"":                      "name is required",
		"Default":               "lower-case letters",
		"has_underscore":        "lower-case letters",
		"-leading":              "lower-case letters",
		"trailing-":             "lower-case letters",
		strings.Repeat("a", 41): "1-40 characters",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			env := githubRunnersEnv()
			firstGitHubRunner(env)["name"] = name
			requireGitHubRunnersError(t, env, want)
		})
	}
}

func TestGitHubRunners_RunnerNamesMustBeUnique(t *testing.T) {
	env := githubRunnersEnv()
	block := githubRunnersBlock(env)
	block["runners"] = append(block["runners"].([]interface{}),
		map[string]interface{}{"name": "default", "organization": "example-org"})
	requireGitHubRunnersError(t, env, `"default" is used by another runner`)
}

func TestGitHubRunners_ExactlyOneOfRepositoryOrOrganization(t *testing.T) {
	t.Run("neither", func(t *testing.T) {
		env := githubRunnersEnv()
		delete(firstGitHubRunner(env), "repository")
		requireGitHubRunnersError(t, env, "runner 'default'", "exactly one of repository")
	})
	t.Run("both", func(t *testing.T) {
		env := githubRunnersEnv()
		firstGitHubRunner(env)["organization"] = "example-org"
		requireGitHubRunnersError(t, env, "only one of repository or organization")
	})
	t.Run("organization alone", func(t *testing.T) {
		env := githubRunnersEnv()
		delete(firstGitHubRunner(env), "repository")
		firstGitHubRunner(env)["organization"] = "example-org"
		if err := validateGitHubRunnersConfigMap(env); err != nil {
			t.Fatalf("organization runner refused: %v", err)
		}
	})
}

func TestGitHubRunners_OneOwnerPerEnvironment(t *testing.T) {
	t.Run("repository and organization under the same owner, any case", func(t *testing.T) {
		env := githubRunnersEnv()
		block := githubRunnersBlock(env)
		block["runners"] = append(block["runners"].([]interface{}),
			map[string]interface{}{"name": "org", "organization": "Example-Org"})
		if err := validateGitHubRunnersConfigMap(env); err != nil {
			t.Fatalf("same owner refused: %v", err)
		}
	})
	t.Run("two owners", func(t *testing.T) {
		env := githubRunnersEnv()
		block := githubRunnersBlock(env)
		block["runners"] = append(block["runners"].([]interface{}),
			map[string]interface{}{"name": "other", "repository": "another-org/repo"})
		requireGitHubRunnersError(t, env, "runner 'other'", `"another-org"`, "same GitHub user or organization")
	})
	t.Run("explicit owner that no runner matches", func(t *testing.T) {
		env := githubRunnersEnv()
		githubRunnersBlock(env)["owner"] = "another-org"
		requireGitHubRunnersError(t, env, `"example-org"`)
	})
}

func TestGitHubRunners_AssignOwner(t *testing.T) {
	env := githubRunnersEnv()
	assignGitHubRunnersOwner(env)
	if got := githubRunnersBlock(env)["owner"]; got != "example-org" {
		t.Errorf("owner = %v, want example-org", got)
	}

	// Disabled runners still get an owner: the provider deletes their webhooks.
	env = githubRunnersEnv()
	githubRunnersBlock(env)["enabled"] = false
	assignGitHubRunnersOwner(env)
	if got := githubRunnersBlock(env)["owner"]; got != "example-org" {
		t.Errorf("disabled block: owner = %v, want example-org", got)
	}

	env = githubRunnersEnv()
	githubRunnersBlock(env)["owner"] = "kept-org"
	assignGitHubRunnersOwner(env)
	if got := githubRunnersBlock(env)["owner"]; got != "kept-org" {
		t.Errorf("explicit owner overwritten: %v", got)
	}
}

func TestGitHubRunners_RepositoryMustBeOwnerSlashRepo(t *testing.T) {
	for _, repo := range []string{"some-repo", "https://github.com/example-org/some-repo", "a/b/c", "example-org/"} {
		t.Run(repo, func(t *testing.T) {
			env := githubRunnersEnv()
			firstGitHubRunner(env)["repository"] = repo
			requireGitHubRunnersError(t, env, "must be owner/repo")
		})
	}
}

func TestGitHubRunners_ComputeType(t *testing.T) {
	for _, ok := range gitHubRunnerComputeTypes {
		env := githubRunnersEnv()
		firstGitHubRunner(env)["compute_type"] = ok
		if err := validateGitHubRunnersConfigMap(env); err != nil {
			t.Errorf("%s refused: %v", ok, err)
		}
	}

	env := githubRunnersEnv()
	firstGitHubRunner(env)["compute_type"] = "BUILD_LAMBDA_1GB"
	requireGitHubRunnersError(t, env, `compute_type "BUILD_LAMBDA_1GB"`, "BUILD_GENERAL1_2XLARGE")
}

func TestGitHubRunners_TimeoutRange(t *testing.T) {
	for _, bad := range []int{4, 2161} {
		env := githubRunnersEnv()
		firstGitHubRunner(env)["timeout_minutes"] = bad
		requireGitHubRunnersError(t, env, "timeout_minutes")
	}
}

func TestGitHubRunners_SecretNeedsExactlyOneSource(t *testing.T) {
	for label, secret := range map[string]map[string]interface{}{
		"neither": {"name": "X"},
		"both":    {"name": "X", "ssm": "/a", "secrets_manager": "b"},
	} {
		t.Run(label, func(t *testing.T) {
			env := githubRunnersEnv()
			firstGitHubRunner(env)["secrets"] = []interface{}{secret}
			requireGitHubRunnersError(t, env, `secret "X" must set exactly one of ssm`)
		})
	}
}

func TestGitHubRunners_VariableNamesUniqueAcrossEnvAndSecrets(t *testing.T) {
	env := githubRunnersEnv()
	firstGitHubRunner(env)["secrets"] = []interface{}{
		map[string]interface{}{"name": "FOO", "ssm": "/acme/dev/foo"},
	}
	requireGitHubRunnersError(t, env, `"FOO" is defined in env_variables and again in secrets`)
}

func TestGitHubRunners_CodeBuildPrefixIsReserved(t *testing.T) {
	env := githubRunnersEnv()
	firstGitHubRunner(env)["env_variables"] = []interface{}{
		map[string]interface{}{"name": "CODEBUILD_SOURCE_VERSION", "value": "x"},
	}
	requireGitHubRunnersError(t, env, "CODEBUILD_", "reserves")

	env = githubRunnersEnv()
	firstGitHubRunner(env)["secrets"] = []interface{}{
		map[string]interface{}{"name": "codebuild_token", "ssm": "/x"},
	}
	requireGitHubRunnersError(t, env, "CODEBUILD_")
}

func TestGitHubRunners_VPCAccessNeedsNAT(t *testing.T) {
	for _, strategy := range []string{"nat_gateway", "nat_gateway_ha"} {
		env := githubRunnersEnv()
		env["egress_strategy"] = strategy
		githubRunnersBlock(env)["vpc_access"] = true
		if err := validateGitHubRunnersConfigMap(env); err != nil {
			t.Errorf("vpc_access with %s refused: %v", strategy, err)
		}
	}

	// public_ip explicitly, and absent, which means public_ip. The second is
	// the "switched egress_strategy back" case: the same check refuses it.
	for label, strategy := range map[string]interface{}{"public_ip": "public_ip", "absent": nil} {
		t.Run(label, func(t *testing.T) {
			env := githubRunnersEnv()
			if strategy == nil {
				delete(env, "egress_strategy")
			} else {
				env["egress_strategy"] = strategy
			}
			githubRunnersBlock(env)["vpc_access"] = true
			requireGitHubRunnersError(t, env,
				`egress_strategy is "public_ip"`,
				"gets no public IP",
				"no internet access",
				"egress_strategy: nat_gateway",
				"vpc_access: false")
		})
	}
}

func TestGitHubRunners_VPCAccessRefusesDefaultVPC(t *testing.T) {
	env := githubRunnersEnv()
	env["use_default_vpc"] = true
	githubRunnersBlock(env)["vpc_access"] = true
	requireGitHubRunnersError(t, env, "use_default_vpc is true", "gets no public IP")
}

// The check sits in preprocessEnvMap, the one pipeline every generate runs, so
// a later edit elsewhere in the file is caught on the next generate.
func TestGitHubRunners_RunsOnEveryGenerate(t *testing.T) {
	env := githubRunnersEnv()
	env["egress_strategy"] = "public_ip"
	githubRunnersBlock(env)["vpc_access"] = true

	err := preprocessEnvMap(env)
	if err == nil || !strings.Contains(err.Error(), "github_runners") {
		t.Fatalf("preprocessEnvMap must refuse vpc_access without a NAT, got %v", err)
	}
}

// The YAML loader yields map[interface{}]interface{} before conversion; the
// check must not be bypassed by that shape.
func TestGitHubRunners_AcceptsRawYAMLMaps(t *testing.T) {
	env := map[string]interface{}{
		"github_runners": map[interface{}]interface{}{
			"enabled": true,
			"runners": []interface{}{
				map[interface{}]interface{}{"name": "default", "repository": "example-org/some-repo"},
			},
		},
	}
	requireGitHubRunnersError(t, env, "token_secret is required")
}
