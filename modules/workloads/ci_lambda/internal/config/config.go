// Package config loads the Lambda's configuration from environment variables
// that Terraform populates.
//
// Every event -> identifier resolution in this Lambda is a map lookup against
// data Terraform emitted. Nothing here parses a repository name or a parameter
// path to invent an identifier: a key that is not in the map is not ours.
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Kind distinguishes a long-running ECS service from a scheduled (cron) task.
type Kind string

const (
	// KindService is a long-running ECS service, deployed with UpdateService.
	KindService Kind = "service"
	// KindScheduledTask is a scheduled task, deployed by registering a new
	// task-definition revision (there is no ECS service to update).
	KindScheduledTask Kind = "scheduled_task"
)

// Target is a deployable thing, keyed by identifier in ECS_SERVICE_MAP /
// SCHEDULED_TASK_MAP.
type Target struct {
	// ServiceName is the real ECS service name. Empty for scheduled tasks.
	ServiceName string `json:"service_name"`
	// TaskFamily is the real task-definition family.
	TaskFamily string `json:"task_family"`
	// Kind is "service" (default) or "scheduled_task".
	Kind Kind `json:"type"`
}

// S3File is one env file in S3, addressed exactly as the task definitions
// address it: the bucket name verbatim, no project/env decoration.
type S3File struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

// Flags are the per-source feature switches.
type Flags struct {
	ECR    bool
	SSM    bool
	S3     bool
	Manual bool
}

// Config is the fully resolved Lambda configuration.
type Config struct {
	Project string
	Env     string
	Region  string
	Cluster string

	LogLevel slog.Level

	// Targets is ECS_SERVICE_MAP merged with SCHEDULED_TASK_MAP.
	Targets map[string]Target
	// ECRRepos maps an ECR repository name to every identifier that deploys
	// from it. One repository can feed several targets (ecr_config
	// mode=use_existing), hence the slice.
	ECRRepos map[string][]string
	// SSMPrefixes maps an SSM parameter path prefix to an identifier.
	SSMPrefixes map[string]string
	// S3Files maps an identifier to the env files it consumes.
	S3Files map[string][]S3File
	// AutoDeploy maps an identifier to Terraform's answer to "may an event
	// redeploy this target on its own?".
	//
	// Its own map rather than a field on Target on purpose. Every other map
	// here answers one question: Targets says what ECS resources an identifier
	// names, ECRRepos says which push reaches it. Auto-deploy is a policy, it
	// changes for policy reasons, and folding it into a resource-identity map
	// would make a policy edit rewrite that map. Kept separate it is also
	// readable in one glance from `aws lambda get-function-configuration`, and
	// its key set can be asserted against the other maps' key set.
	//
	// A missing key means true — that is what a project that predates the
	// setting looks like, and such a project auto-deploys everything today.
	AutoDeploy map[string]bool

	SlackWebhookURL string

	MaxRetries     int
	RetryBaseDelay time.Duration
	DryRun         bool

	Enable Flags
}

// Load reads configuration through the supplied getenv function so tests never
// have to touch the process environment.
func Load(getenv func(string) string) (*Config, error) {
	if getenv == nil {
		return nil, fmt.Errorf("config: getenv must not be nil")
	}

	cfg := &Config{
		Project:         getenv("PROJECT_NAME"),
		Env:             getenv("PROJECT_ENV"),
		Region:          stringOr(getenv("AWS_REGION"), "us-east-1"),
		Cluster:         getenv("ECS_CLUSTER_NAME"),
		SlackWebhookURL: getenv("SLACK_WEBHOOK_URL"),
		MaxRetries:      intOr(getenv("MAX_DEPLOYMENT_RETRIES"), 2),
		RetryBaseDelay:  time.Duration(intOr(getenv("RETRY_BASE_DELAY_MS"), 1000)) * time.Millisecond,
		DryRun:          boolOr(getenv("DRY_RUN"), false),
		Enable: Flags{
			ECR:    boolOr(getenv("ENABLE_ECR_MONITORING"), true),
			SSM:    boolOr(getenv("ENABLE_SSM_MONITORING"), true),
			S3:     boolOr(getenv("ENABLE_S3_MONITORING"), true),
			Manual: boolOr(getenv("ENABLE_MANUAL_DEPLOY"), true),
		},
	}

	level, err := ParseLevel(getenv("LOG_LEVEL"))
	if err != nil {
		return nil, err
	}
	cfg.LogLevel = level

	services := map[string]Target{}
	if err := decodeMap(getenv("ECS_SERVICE_MAP"), &services, "ECS_SERVICE_MAP"); err != nil {
		return nil, err
	}
	tasks := map[string]Target{}
	if err := decodeMap(getenv("SCHEDULED_TASK_MAP"), &tasks, "SCHEDULED_TASK_MAP"); err != nil {
		return nil, err
	}

	cfg.Targets = make(map[string]Target, len(services)+len(tasks))
	for id, t := range services {
		if t.Kind == "" {
			t.Kind = KindService
		}
		cfg.Targets[id] = t
	}
	for id, t := range tasks {
		t.Kind = KindScheduledTask
		cfg.Targets[id] = t
	}

	if err := decodeMap(getenv("ECR_REPO_MAP"), &cfg.ECRRepos, "ECR_REPO_MAP"); err != nil {
		return nil, err
	}
	if err := decodeMap(getenv("SSM_SERVICE_MAP"), &cfg.SSMPrefixes, "SSM_SERVICE_MAP"); err != nil {
		return nil, err
	}
	if err := decodeMap(getenv("S3_SERVICE_MAP"), &cfg.S3Files, "S3_SERVICE_MAP"); err != nil {
		return nil, err
	}
	if err := decodeMap(getenv("AUTO_DEPLOY_MAP"), &cfg.AutoDeploy, "AUTO_DEPLOY_MAP"); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate reports configuration that makes the Lambda unable to do its job at
// all. It deliberately does not check the *contents* of the lookup maps
// against one another; that is SelfCheck, which reports rather than fails.
func (c *Config) Validate() error {
	var problems []string

	if c.Project == "" {
		problems = append(problems, "PROJECT_NAME is required")
	}
	if c.Env == "" {
		problems = append(problems, "PROJECT_ENV is required")
	}
	if c.Cluster == "" {
		problems = append(problems, "ECS_CLUSTER_NAME is required")
	}
	if len(c.Targets) == 0 {
		problems = append(problems, "ECS_SERVICE_MAP or SCHEDULED_TASK_MAP must contain at least one entry")
	}

	for _, id := range sortedKeys(c.Targets) {
		t := c.Targets[id]
		if t.TaskFamily == "" {
			problems = append(problems, fmt.Sprintf("target %q: task_family is required", id))
		}
		if t.Kind != KindScheduledTask && t.ServiceName == "" {
			problems = append(problems, fmt.Sprintf("target %q: service_name is required for services", id))
		}
	}

	if c.MaxRetries < 0 {
		problems = append(problems, "MAX_DEPLOYMENT_RETRIES must be non-negative")
	}
	if c.RetryBaseDelay < 0 {
		problems = append(problems, "RETRY_BASE_DELAY_MS must be non-negative")
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid configuration:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

// Target returns the deployment target for an identifier.
func (c *Config) Target(id string) (Target, bool) {
	t, ok := c.Targets[id]
	return t, ok
}

// IsScheduledTask reports whether an identifier names a scheduled task.
func (c *Config) IsScheduledTask(id string) bool {
	t, ok := c.Targets[id]
	return ok && t.Kind == KindScheduledTask
}

// AutoDeployEnabled reports whether an event may redeploy an identifier.
//
// An identifier absent from AUTO_DEPLOY_MAP answers true. That is the
// fail-open direction on purpose: a Terraform version that has not started
// emitting the map yet, or an entry that was forgotten, must not disable every
// deployment in the project at once and in silence. SelfCheck reports a
// disagreement between this map and the target map loudly at startup instead.
func (c *Config) AutoDeployEnabled(id string) bool {
	enabled, ok := c.AutoDeploy[id]
	return !ok || enabled
}

// PartitionByAutoDeploy splits identifiers into those an event may redeploy and
// those Terraform marked auto_deploy = false. Order is preserved in both.
func (c *Config) PartitionByAutoDeploy(ids []string) (enabled, disabled []string) {
	for _, id := range ids {
		if c.AutoDeployEnabled(id) {
			enabled = append(enabled, id)
			continue
		}
		disabled = append(disabled, id)
	}
	return enabled, disabled
}

// IdentifiersForRepo returns every identifier that deploys from an ECR
// repository. A repository this project does not own returns nothing — that is
// how a second project's push in the same account is ignored rather than
// guessed at.
func (c *Config) IdentifiersForRepo(repo string) []string {
	ids := append([]string(nil), c.ECRRepos[repo]...)
	sort.Strings(ids)
	return ids
}

// IdentifierForSSMPath resolves a changed SSM parameter to an identifier by
// longest path-segment prefix.
//
// Longest match matters: /{env}/{project}/task/{name} must win over a service
// literally named "task", and /{env}/{project}/task/{name}/env must resolve at
// all (four segments, which the old anchored regex could not express).
func (c *Config) IdentifierForSSMPath(path string) (string, bool) {
	path = normalisePath(path)

	best, bestID := "", ""
	for prefix, id := range c.SSMPrefixes {
		p := normalisePath(prefix)
		if path != p && !strings.HasPrefix(path, p+"/") {
			continue
		}
		if len(p) > len(best) {
			best, bestID = p, id
		}
	}
	if best == "" {
		return "", false
	}
	return bestID, true
}

// IdentifiersForS3 returns every identifier bound to this exact bucket and key.
func (c *Config) IdentifiersForS3(bucket, key string) []string {
	var ids []string
	for id, files := range c.S3Files {
		for _, f := range files {
			if f.Bucket == bucket && f.Key == key {
				ids = append(ids, id)
				break
			}
		}
	}
	sort.Strings(ids)
	return ids
}

// SelfCheck compares the maps Terraform shipped against one another and
// returns a human-readable problem per inconsistency.
//
// It never fails initialization. A Lambda that refuses to start turns one bad
// map entry into "every event in the account is retried by the async invoke
// path", which is the failure mode this rewrite exists to remove. The caller
// logs the problems loudly and carries on; unresolvable events are ignored
// individually.
func (c *Config) SelfCheck(backendID string) []string {
	var problems []string

	if _, ok := c.Targets[backendID]; !ok {
		problems = append(problems, fmt.Sprintf(
			"target map has no %q entry (available: %s)", backendID, strings.Join(sortedKeys(c.Targets), ", ")))
	}

	for _, repo := range sortedKeys(c.ECRRepos) {
		for _, id := range c.ECRRepos[repo] {
			if _, ok := c.Targets[id]; !ok {
				problems = append(problems, fmt.Sprintf(
					"ECR_REPO_MAP[%q] references unknown target %q", repo, id))
			}
		}
	}

	for _, prefix := range sortedKeys(c.SSMPrefixes) {
		id := c.SSMPrefixes[prefix]
		if _, ok := c.Targets[id]; !ok {
			problems = append(problems, fmt.Sprintf(
				"SSM_SERVICE_MAP[%q] references unknown target %q", prefix, id))
		}
	}

	for _, id := range sortedKeys(c.S3Files) {
		if _, ok := c.Targets[id]; !ok {
			problems = append(problems, fmt.Sprintf(
				"S3_SERVICE_MAP has entry for unknown target %q", id))
		}
	}

	// AUTO_DEPLOY_MAP must describe exactly the set of things that can be
	// deployed. An entry for a target that does not exist is dead policy; a
	// target with no entry falls back to true, which is the safe direction but
	// also the one where "I turned it off in YAML" quietly did not arrive.
	for _, id := range sortedKeys(c.AutoDeploy) {
		if _, ok := c.Targets[id]; !ok {
			problems = append(problems, fmt.Sprintf(
				"AUTO_DEPLOY_MAP has entry for unknown target %q", id))
		}
	}

	// Skipped when the map is absent entirely: that is a Lambda still running
	// against a Terraform state from before this setting existed, where every
	// target auto-deploying is correct, not a problem to report on every
	// invocation.
	if len(c.AutoDeploy) > 0 {
		for _, id := range sortedKeys(c.Targets) {
			if _, ok := c.AutoDeploy[id]; !ok {
				problems = append(problems, fmt.Sprintf(
					"AUTO_DEPLOY_MAP has no entry for target %q (defaulting to enabled)", id))
			}
		}
	}

	return problems
}

// ParseLevel maps the LOG_LEVEL variable onto a slog level.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error (got %q)", s)
	}
}

func decodeMap(raw string, dst any, name string) error {
	s := raw
	if strings.TrimSpace(s) == "" {
		s = "{}"
	}
	if err := json.Unmarshal([]byte(s), dst); err != nil {
		return fmt.Errorf("failed to parse %s: %w", name, err)
	}
	return nil
}

func normalisePath(p string) string {
	p = strings.TrimRight(p, "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func stringOr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func intOr(v string, fallback int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		return n
	}
	return fallback
}

func boolOr(v string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on", "enabled":
		return true
	case "false", "0", "no", "off", "disabled":
		return false
	default:
		return fallback
	}
}
