package boundary_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"madappgang.com/infrastructure/ci_lambda/contract"
	"madappgang.com/infrastructure/ci_lambda/internal/boundary"
	"madappgang.com/infrastructure/ci_lambda/internal/config"
)

// loadGolden reads the captured Terraform output and the Config the Lambda
// would build from it.
func loadGolden(t *testing.T) (*boundary.Golden, *config.Config) {
	t.Helper()

	g, err := boundary.LoadGolden(boundary.GoldenPath)
	require.NoError(t, err)

	cfg, err := config.Load(func(k string) string { return g.Env[k] })
	require.NoError(t, err, "the real config loader must accept Terraform's real output")
	return g, cfg
}

// TestEveryTerraformIdentifierResolves is the boundary gate. It always runs:
// no build tag, no terraform binary, no network, no credentials.
func TestEveryTerraformIdentifierResolves(t *testing.T) {
	g, cfg := loadGolden(t)

	require.NotEmpty(t, g.Identifiers.ECRRepoIDs)
	require.NotEmpty(t, g.Identifiers.SSMPrefixIDs)

	// 1. Nothing Terraform emitted is unknown to the Lambda.
	require.Empty(t, cfg.SelfCheck(contract.Load().BackendID()))

	// 2. The backend identifier agrees on both sides. If either side ever
	//    changes it, this line fails.
	require.Equal(t, contract.Load().BackendID(), g.Identifiers.BackendID)
	_, ok := cfg.Target(g.Identifiers.BackendID)
	require.True(t, ok, "the backend identifier must resolve to a target")

	// 3. Every ECR push this project can emit resolves to the same set of
	//    targets Terraform expects, and every one of them is deployable.
	for repo, wantIDs := range g.Identifiers.ECRRepoIDs {
		require.ElementsMatch(t, wantIDs, cfg.IdentifiersForRepo(repo), "repository %q", repo)
		for _, id := range wantIDs {
			_, ok := cfg.Target(id)
			require.Truef(t, ok, "repository %q maps to unknown target %q", repo, id)
		}
	}

	// 4. Every SSM parameter this project can emit resolves.
	for prefix, wantID := range g.Identifiers.SSMPrefixIDs {
		got, ok := cfg.IdentifierForSSMPath(prefix + "/env")
		require.Truef(t, ok, "parameter %s/env resolved to nothing", prefix)
		require.Equalf(t, wantID, got, "parameter %s/env", prefix)

		_, ok = cfg.Target(got)
		require.Truef(t, ok, "prefix %q maps to unknown target %q", prefix, got)
	}

	// 5. Every S3 env file resolves to at least one target.
	for id, files := range cfg.S3Files {
		_, ok := cfg.Target(id)
		require.Truef(t, ok, "S3 map references unknown target %q", id)
		for _, f := range files {
			require.Contains(t, cfg.IdentifiersForS3(f.Bucket, f.Key), id)
		}
	}
}

// TestScheduledTaskIdentifiersUseTheContractPrefix pins the second half of the
// contract: Terraform builds "task:{name}" keys from task_id_prefix, Go reads
// the same value out of the same file.
func TestScheduledTaskIdentifiersUseTheContractPrefix(t *testing.T) {
	g, cfg := loadGolden(t)
	spec := contract.Load()

	require.NotEmpty(t, g.Identifiers.TaskIDs)
	for name, id := range g.Identifiers.TaskIDs {
		require.Equal(t, spec.TaskID(name), id)
		require.True(t, spec.IsTaskID(id))
		require.True(t, cfg.IsScheduledTask(id), "%q must be typed as a scheduled task", id)
	}
}

// TestForeignProjectRepositoryIsNotOurs is the cross-project isolation check.
// A repository belonging to another project in the same account resolves to
// nothing, because resolution is a lookup and the map contains our repositories
// only. There is no parser left to be tricked.
func TestForeignProjectRepositoryIsNotOurs(t *testing.T) {
	_, cfg := loadGolden(t)

	for _, repo := range []string{
		"otherproj_service_api",
		"acmetwo_backend",
		"acme_service_api_extra",
		"unrelated",
	} {
		require.Empty(t, cfg.IdentifiersForRepo(repo), "repository %q must not resolve", repo)
	}
}

// TestSharedRepositoryFansOut pins the ecr_config mode=use_existing case: one
// push, every consumer deployed. A repo-name parser structurally cannot express
// this — it yields exactly one identifier.
func TestSharedRepositoryFansOut(t *testing.T) {
	g, cfg := loadGolden(t)

	shared := ""
	for repo, ids := range g.Identifiers.ECRRepoIDs {
		if len(ids) > 1 {
			shared = repo
			break
		}
	}
	require.NotEmpty(t, shared, "the synthetic project must contain a shared repository")
	require.Greater(t, len(cfg.IdentifiersForRepo(shared)), 1)
}

// TestLongestPrefixBeatsAShorterOne pins the scheduled-task SSM path. The
// synthetic project contains a service literally named "task", so
// /dev/acme/task/cleanup/env must resolve to the task and not to the service.
func TestLongestPrefixBeatsAShorterOne(t *testing.T) {
	g, cfg := loadGolden(t)

	require.Contains(t, g.Identifiers.ServiceIDs, "task",
		"the fixture must keep a service named \"task\" for this test to mean anything")

	id, ok := cfg.IdentifierForSSMPath("/dev/acme/task/cleanup/env")
	require.True(t, ok)
	require.Equal(t, "task:cleanup", id)

	id, ok = cfg.IdentifierForSSMPath("/dev/acme/task/env")
	require.True(t, ok)
	require.Equal(t, "task", id)

	// Hyphenated service names were unreachable through the old \w+ regex.
	id, ok = cfg.IdentifierForSSMPath("/dev/acme/payment-worker/env")
	require.True(t, ok)
	require.Equal(t, "payment-worker", id)

	// Another project's parameters are not ours.
	_, ok = cfg.IdentifierForSSMPath("/dev/otherproj/backend/env")
	require.False(t, ok)
}

// TestAutoDeployMapCoversExactlyTheTargets is the key-set gate on the new map.
//
// AUTO_DEPLOY_MAP is a sibling of the other maps rather than a field inside
// them, which buys a clean assertion and costs one: nothing structural stops the
// two key sets drifting. A target with no entry falls back to enabled — the safe
// direction, and therefore the silent one — so "I disabled it in YAML and it
// deployed anyway" would otherwise look exactly like a Lambda working correctly.
func TestAutoDeployMapCoversExactlyTheTargets(t *testing.T) {
	g, cfg := loadGolden(t)

	require.NotEmpty(t, cfg.AutoDeploy, "AUTO_DEPLOY_MAP is empty: nothing below proves anything")

	targets := make([]string, 0, len(cfg.Targets))
	for id := range cfg.Targets {
		targets = append(targets, id)
	}
	policies := make([]string, 0, len(cfg.AutoDeploy))
	for id := range cfg.AutoDeploy {
		policies = append(policies, id)
	}

	require.ElementsMatch(t, targets, policies,
		"AUTO_DEPLOY_MAP and the target maps describe different sets of identifiers; a target "+
			"missing here silently defaults to auto-deploy enabled")

	// The Terraform-side view has to agree with what the Lambda decoded.
	require.Equal(t, g.Identifiers.AutoDeploy, cfg.AutoDeploy)

	// SelfCheck enforces the same agreement at runtime, and boundary_test's
	// first case requires it to be clean — so this cannot pass while that
	// silently stops checking.
	require.Empty(t, cfg.SelfCheck(contract.Load().BackendID()))
}

// TestDisabledTargetStaysReachableAndExplainable is the disabled-target case.
//
// The point of the whole setting is the diagnostic. A disabled target that had
// been *removed* from the maps would make the Lambda answer "no target uses
// repository acme_service_payment-worker" — which is false, reads like a naming
// bug, and cannot be told apart from a real one. So the requirement is the
// opposite of an exclusion: everything still resolves, and only the policy flag
// says no.
//
// Expectations are derived from the fixture's own inputs, so this keeps testing
// a disabled target only for as long as the fixture contains one — and says so
// if it stops.
func TestDisabledTargetStaysReachableAndExplainable(t *testing.T) {
	g, cfg := loadGolden(t)
	spec := contract.Load()

	var disabledServices, disabledTasks []string
	for name, enabled := range g.Inputs.ServiceAutoDeploy {
		if !enabled {
			disabledServices = append(disabledServices, g.Identifiers.ServiceIDs[name])
		}
	}
	for name, enabled := range g.Inputs.TaskAutoDeploy {
		if !enabled {
			disabledTasks = append(disabledTasks, spec.TaskID(name))
		}
	}

	require.NotEmpty(t, disabledServices,
		"the fixture must keep a service with auto_deploy = false for this test to mean anything")
	require.NotEmpty(t, disabledTasks,
		"the fixture must keep a scheduled task with auto_deploy = false for this test to mean anything")

	for _, id := range append(append([]string{}, disabledServices...), disabledTasks...) {
		t.Run(id, func(t *testing.T) {
			// Still a real target: a manual DEPLOY still works.
			_, ok := cfg.Target(id)
			require.Truef(t, ok, "%q must stay in the target map: manual deploys are not governed "+
				"by auto_deploy, and a missing target cannot be reported by name", id)

			// Still reachable by every automatic source it was reachable by,
			// so the invocation happens and the log line gets written.
			var reachedByRepo bool
			for _, ids := range g.Identifiers.ECRRepoIDs {
				for _, got := range ids {
					if got == id {
						reachedByRepo = true
					}
				}
			}
			require.Truef(t, reachedByRepo,
				"%q must stay in ECR_REPO_MAP; dropping it turns 'auto_deploy is disabled' into "+
					"'no target uses repository ...', which is a different and untrue statement", id)

			var reachedBySSM bool
			for prefix, got := range g.Identifiers.SSMPrefixIDs {
				if got != id {
					continue
				}
				reachedBySSM = true
				resolved, ok := cfg.IdentifierForSSMPath(prefix + "/env")
				require.True(t, ok)
				require.Equal(t, id, resolved)
			}
			require.Truef(t, reachedBySSM, "%q must stay in SSM_SERVICE_MAP", id)

			// And the only thing that says no is the policy.
			require.Falsef(t, cfg.AutoDeployEnabled(id), "%q must read as auto_deploy = false", id)
		})
	}

	// The enabled ones are unaffected.
	for name, enabled := range g.Inputs.ServiceAutoDeploy {
		if enabled {
			require.True(t, cfg.AutoDeployEnabled(g.Identifiers.ServiceIDs[name]), name)
		}
	}
	require.Equal(t, g.Inputs.BackendAutoDeploy, cfg.AutoDeployEnabled(g.Identifiers.BackendID))
}

// TestUnknownIdentifierIsNotTreatedAsDisabled pins the fail-open direction.
//
// An identifier with no entry answers true, and that has to stay true for a
// Lambda still running against a Terraform state from before this setting
// existed: the alternative is every deployment in the project stopping at once,
// silently, the moment a map arrives empty.
func TestUnknownIdentifierIsNotTreatedAsDisabled(t *testing.T) {
	_, cfg := loadGolden(t)
	require.True(t, cfg.AutoDeployEnabled("a-service-that-does-not-exist"))
}

// TestGoldenUsesSyntheticDataOnly keeps the public repository clean.
func TestGoldenUsesSyntheticDataOnly(t *testing.T) {
	g, _ := loadGolden(t)

	flat := strings.Join(append(keysOf(g.Env), valuesOf(g.Env)...), " ")
	for _, forbidden := range []string{"amazonaws.com/", "arn:aws:", "hooks.slack.com"} {
		require.NotContains(t, flat, forbidden)
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func valuesOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
