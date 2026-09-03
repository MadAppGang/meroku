package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// Own-role exclusion for the GitHub OIDC subject overlap scan.
//
// The scan lists every IAM role in an account and flags the GitHub Actions roles
// whose OIDC subject patterns overlap this project's. Roles belonging to this
// project's own environments have to come out first: dev and prod of one project
// legitimately share a repository, so a scan that flagged them would report a
// conflict on every single-project account and be worth nothing.
//
// The exclusion set costs zero AWS calls. ListRoles returns no tags, so reading a
// Project tag off each role would cost one call per role; instead the names are
// computed with the same cascade Terraform uses.

// githubActionsRoleParts is the identity half of the github_actions_role request
// in modules/workloads/naming.tf. Kept as one value because the digest in cascade
// form 3 is taken over these exact strings — ["github","actions","role"] and
// ["github-actions-role"] hash differently.
var githubActionsRoleParts = []string{"github", "actions", "role"}

// ownGitHubRoleName returns the IAM role name modules/workloads would give this
// project+env, computed rather than guessed.
//
// It reproduces the github_actions_role request in modules/workloads/naming.tf
// byte for byte: the same legacy template, the same parts, limit 64, and the
// default "-" separator. awsName is the Go half of modules/naming, pinned to the
// Terraform half by app/testdata/aws_name_vectors.json, so the two agree.
//
// Do not be tempted to assemble "project-env-github-actions-role" directly. That
// is only cascade form 1. Once the legacy name passes 64 characters the cascade
// falls through to form 3, "github-actions-role-<8 hex>", which contains neither
// the project nor the environment — a suffix or prefix filter misses it entirely
// and the project's own role gets reported as somebody else's conflict. Running
// the cascade reproduces the digest exactly.
func ownGitHubRoleName(project, env string) string {
	return awsName(project, env, githubActionsRoleParts, 64, "", "",
		project+"-"+env+"-github-actions-role")
}

// ownRoleNames returns the set of GitHub Actions role names belonging to this
// project across every sibling environment deployed into the same AWS account.
//
// Siblings in a different account cannot own a role in the account being
// scanned, and a same-named environment of a different project is exactly the
// conflict this feature is looking for, so both are left in.
func ownRoleNames(thisProject, thisAccountID string) (map[string]bool, error) {
	envNames, err := listEnvironmentNames()
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	own := make(map[string]bool, len(envNames))
	for _, name := range envNames {
		probe, ok := readEnvIdentity(name)
		if !ok {
			continue
		}
		if probe.Project != thisProject || probe.AccountID != thisAccountID {
			continue
		}
		// probe.Env, never name: staging.yaml may declare `env: stage`, and
		// Terraform builds the role name from the declared value.
		own[ownGitHubRoleName(probe.Project, probe.Env)] = true
	}
	return own, nil
}

// envIdentity is the three fields the exclusion set needs. They have been stable
// since schema v5, so they can be read at any schema version without migrating.
type envIdentity struct {
	Project   string `yaml:"project"`
	Env       string `yaml:"env"`
	AccountID string `yaml:"account_id"`
}

// readEnvIdentity reads one sibling environment's identity RAW.
//
// It must never call loadEnv, loadEnvFromPath, loadEnvWithMigration or
// loadEnvToMap. All of them route through loadEnvWithMigration
// (app/migrations.go:1562), which for any file below CurrentSchemaVersion prints
// a migration banner, writes a <file>.backup_<timestamp>, and rewrites the
// environment file through a full marshal cycle (app/migrations.go:1616-1623) —
// losing every comment, reordering every key, and writing in whatever new
// defaults the intervening migrations add.
//
// This feature's central rule is "report and refuse, never rewrite". A read-only
// conflict scan that silently migrates half the user's configs the first time it
// runs violates that outright, and it would do so on files belonging to
// environments the operator did not ask about. TestOwnRoleNamesDoesNotRewrite-
// Siblings is the guard.
//
// So: os.ReadFile plus yaml.Unmarshal into a probe struct, following
// looksLikeEnvironmentConfig (app/env_selector.go:63), including its rule that a
// file which cannot be read or parsed is simply not an environment here.
func readEnvIdentity(name string) (envIdentity, bool) {
	for _, ext := range []string{".yaml", ".yml"} {
		data, err := os.ReadFile(name + ext)
		if err != nil {
			continue
		}

		var probe envIdentity
		if err := yaml.Unmarshal(data, &probe); err != nil {
			continue
		}
		if probe.Project != "" && probe.Env != "" {
			return probe, true
		}
	}
	return envIdentity{}, false
}
