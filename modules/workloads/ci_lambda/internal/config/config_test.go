package config_test

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"madappgang.com/infrastructure/ci_lambda/internal/config"
	"madappgang.com/infrastructure/ci_lambda/internal/testsupport"
)

func TestLoadFromTerraformOutput(t *testing.T) {
	cfg := testsupport.Config(t, nil)

	require.Equal(t, "acme", cfg.Project)
	require.Equal(t, "dev", cfg.Env)
	require.Equal(t, "acme_cluster_dev", cfg.Cluster)
	require.Equal(t, slog.LevelInfo, cfg.LogLevel)
	require.Equal(t, 2, cfg.MaxRetries)
	require.Equal(t, time.Second, cfg.RetryBaseDelay)
	require.False(t, cfg.DryRun)
	require.Equal(t, config.Flags{ECR: true, SSM: true, S3: true, Manual: true}, cfg.Enable)
}

// TestBackendResolvesUnderItsRealName is the regression test for the defect
// that made every backend deploy a no-op: Terraform emitted the key "backend"
// and the Lambda looked up "".
func TestBackendResolvesUnderItsRealName(t *testing.T) {
	cfg := testsupport.Config(t, nil)

	target, ok := cfg.Target("backend")
	require.True(t, ok)
	require.Equal(t, "acme_service_dev", target.ServiceName)
	require.Equal(t, config.KindService, target.Kind)

	_, ok = cfg.Target("")
	require.False(t, ok, "the empty-string sentinel is gone")
}

func TestScheduledTasksAreTyped(t *testing.T) {
	cfg := testsupport.Config(t, nil)

	target, ok := cfg.Target("task:cleanup")
	require.True(t, ok)
	require.Equal(t, config.KindScheduledTask, target.Kind)
	require.Equal(t, "acme_task_cleanup_dev", target.TaskFamily)
	require.Empty(t, target.ServiceName, "a scheduled task has no ECS service")

	require.True(t, cfg.IsScheduledTask("task:cleanup"))
	require.False(t, cfg.IsScheduledTask("backend"))
	require.False(t, cfg.IsScheduledTask("nope"))
}

func TestIdentifiersForRepoFanOut(t *testing.T) {
	cfg := testsupport.Config(t, nil)

	require.Equal(t, []string{"backend"}, cfg.IdentifiersForRepo("acme_backend"))
	require.Equal(t, []string{"api", "reporting"}, cfg.IdentifiersForRepo("acme_service_api"))
	require.Empty(t, cfg.IdentifiersForRepo("otherproj_service_api"))
	require.Empty(t, cfg.IdentifiersForRepo(""))
}

func TestIdentifierForSSMPath(t *testing.T) {
	cfg := testsupport.Config(t, nil)

	cases := []struct {
		path string
		want string
		ok   bool
	}{
		{"/dev/acme/backend/env", "backend", true},
		{"dev/acme/backend/env", "backend", true}, // leading slash is optional
		{"/dev/acme/backend", "backend", true},
		{"/dev/acme/payment-worker/env", "payment-worker", true},
		{"/dev/acme/task/env", "task", true},
		{"/dev/acme/task/cleanup/env", "task:cleanup", true},
		{"/dev/acme/task/cleanup/nested/deep", "task:cleanup", true},
		{"/dev/acme/unknown/env", "", false},
		{"/dev/otherproj/backend/env", "", false},
		{"/prod/acme/backend/env", "", false},
		// A prefix must end on a segment boundary: "backendish" is not "backend".
		{"/dev/acme/backendish/env", "", false},
		{"", "", false},
	}

	for _, c := range cases {
		got, ok := cfg.IdentifierForSSMPath(c.path)
		require.Equalf(t, c.ok, ok, "path %q", c.path)
		require.Equalf(t, c.want, got, "path %q", c.path)
	}
}

func TestIdentifiersForS3(t *testing.T) {
	cfg := testsupport.Config(t, nil)

	require.Equal(t, []string{"backend"}, cfg.IdentifiersForS3("acme-config", "backend.env"))
	require.Equal(t, []string{"payment-worker", "reporting"}, cfg.IdentifiersForS3("acme-config", "shared.env"))
	require.Empty(t, cfg.IdentifiersForS3("acme-config", "nope.env"))
	require.Empty(t, cfg.IdentifiersForS3("other-bucket", "backend.env"))
}

func TestLoadRejectsMissingRequiredValues(t *testing.T) {
	for _, missing := range []string{"PROJECT_NAME", "PROJECT_ENV", "ECS_CLUSTER_NAME"} {
		env := testsupport.Env(t, map[string]string{missing: ""})
		_, err := config.Load(testsupport.Getenv(env))
		require.Errorf(t, err, "%s must be required", missing)
		require.Contains(t, err.Error(), missing)
	}
}

func TestLoadRejectsMalformedMaps(t *testing.T) {
	env := testsupport.Env(t, map[string]string{"ECR_REPO_MAP": "{not json"})
	_, err := config.Load(testsupport.Getenv(env))
	require.ErrorContains(t, err, "ECR_REPO_MAP")
}

func TestLoadRejectsAnUnknownLogLevel(t *testing.T) {
	env := testsupport.Env(t, map[string]string{"LOG_LEVEL": "chatty"})
	_, err := config.Load(testsupport.Getenv(env))
	require.ErrorContains(t, err, "LOG_LEVEL")
}

func TestSelfCheckIsCleanForRealTerraformOutput(t *testing.T) {
	cfg := testsupport.Config(t, nil)
	require.Empty(t, cfg.SelfCheck("backend"))
}

func TestSelfCheckReportsEveryKindOfDrift(t *testing.T) {
	t.Run("ECR map points at a target that does not exist", func(t *testing.T) {
		cfg := testsupport.Config(t, map[string]string{
			"ECR_REPO_MAP": `{"acme_backend":["backend"],"acme_service_ghost":["ghost"]}`,
		})
		problems := cfg.SelfCheck("backend")
		require.Len(t, problems, 1)
		require.Contains(t, problems[0], "ghost")
	})

	t.Run("SSM map points at a target that does not exist", func(t *testing.T) {
		cfg := testsupport.Config(t, map[string]string{
			"SSM_SERVICE_MAP": `{"/dev/acme/ghost":"ghost"}`,
		})
		problems := cfg.SelfCheck("backend")
		require.Len(t, problems, 1)
		require.Contains(t, problems[0], "ghost")
	})

	t.Run("S3 map points at a target that does not exist", func(t *testing.T) {
		cfg := testsupport.Config(t, map[string]string{
			"S3_SERVICE_MAP": `{"ghost":[{"bucket":"b","key":"k"}]}`,
		})
		problems := cfg.SelfCheck("backend")
		require.Len(t, problems, 1)
		require.Contains(t, problems[0], "ghost")
	})

	// The exact shape of the original defect: Terraform renames the backend key
	// (or Go starts expecting a different one) and nothing notices.
	t.Run("the backend identifier is missing", func(t *testing.T) {
		cfg := testsupport.Config(t, map[string]string{
			"ECS_SERVICE_MAP":    `{"api":{"service_name":"acme_service_api_dev","task_family":"acme_service_api_dev"}}`,
			"SCHEDULED_TASK_MAP": `{}`,
			"ECR_REPO_MAP":       `{"acme_service_api":["api"]}`,
			"SSM_SERVICE_MAP":    `{"/dev/acme/api":"api"}`,
			"S3_SERVICE_MAP":     `{"api":[{"bucket":"acme-config","key":"api.env"}]}`,
			"AUTO_DEPLOY_MAP":    `{"api":true}`,
		})
		problems := cfg.SelfCheck("backend")
		require.Len(t, problems, 1)
		require.Contains(t, problems[0], `no "backend" entry`)
	})

	// Both directions of the AUTO_DEPLOY_MAP key set. It is a separate map, so
	// nothing structural keeps it aligned with the targets; this is the check
	// that does.
	t.Run("auto-deploy map points at a target that does not exist", func(t *testing.T) {
		cfg := testsupport.Config(t, map[string]string{
			"AUTO_DEPLOY_MAP": autoDeployMapPlus(t, map[string]bool{"ghost": false}),
		})
		problems := cfg.SelfCheck("backend")
		require.Len(t, problems, 1)
		require.Contains(t, problems[0], "ghost")
	})

	t.Run("auto-deploy map is missing a target", func(t *testing.T) {
		cfg := testsupport.Config(t, map[string]string{
			"AUTO_DEPLOY_MAP": `{"backend":true}`,
		})
		problems := cfg.SelfCheck("backend")
		require.NotEmpty(t, problems)
		for _, p := range problems {
			require.Contains(t, p, "AUTO_DEPLOY_MAP has no entry for target")
		}
		require.Contains(t, strings.Join(problems, "\n"), `"api"`)
	})

	// A Lambda still running against a Terraform state from before the setting
	// existed. Every target auto-deploying is correct there, so reporting it on
	// every invocation would be noise, not a finding.
	t.Run("an absent auto-deploy map is not drift", func(t *testing.T) {
		cfg := testsupport.Config(t, map[string]string{"AUTO_DEPLOY_MAP": ""})
		require.Empty(t, cfg.SelfCheck("backend"))
		require.True(t, cfg.AutoDeployEnabled("api"))
	})
}

// autoDeployMapPlus re-encodes the captured AUTO_DEPLOY_MAP with extra entries,
// so a test that adds one bad key does not also have to restate the good ones.
func autoDeployMapPlus(t *testing.T, extra map[string]bool) string {
	t.Helper()

	var m map[string]bool
	require.NoError(t, json.Unmarshal([]byte(testsupport.Env(t, nil)["AUTO_DEPLOY_MAP"]), &m))
	for k, v := range extra {
		m[k] = v
	}
	raw, err := json.Marshal(m)
	require.NoError(t, err)
	return string(raw)
}

func TestValidateRejectsIncompleteTargets(t *testing.T) {
	env := testsupport.Env(t, map[string]string{
		"ECS_SERVICE_MAP": `{"api":{"task_family":"acme_service_api_dev"}}`,
	})
	_, err := config.Load(testsupport.Getenv(env))
	require.ErrorContains(t, err, "service_name is required")
}
