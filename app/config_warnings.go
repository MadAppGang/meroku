package main

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// ConfigWarning is a configuration warning: something in the env YAML worth
// fixing that does not stop generation.
//
// Every presentation renders the same value: `meroku generate` prints it with
// printConfigWarnings, the web UI reads it from GET /api/config-warnings, and
// the TUI deploy shows it before anything runs.
type ConfigWarning struct {
	// Code is the stable rule id, snake_case.
	Code string `json:"code"`
	// Item is the part of the YAML the warning is about.
	Item WarningItem `json:"item"`
	// Title is one line, no newline.
	Title string `json:"title"`
	// Detail is the full text, including what to do. May contain "\n".
	Detail string `json:"detail"`
	// Docs is an absolute https URL, or "" when there is no link.
	Docs string `json:"docs,omitempty"`
}

// WarningItem is the part of the YAML a warning is about. Name is the YAML
// name verbatim, "#<1-based index>" for an entry without one, and always ""
// for the singleton kinds (env, backend, valkey, event_bus).
type WarningItem struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// The kinds of item a warning can belong to. The web UI maps each to a node on
// the canvas; WarningKindEnv has no node and is listed over the canvas.
const (
	WarningKindEnv           = "env"
	WarningKindBackend       = "backend"
	WarningKindService       = "service"
	WarningKindScheduledTask = "scheduled_task"
	WarningKindEventTask     = "event_task"
	WarningKindAmplifyApp    = "amplify_app"
	WarningKindComputePool   = "compute_pool"
	WarningKindValkey        = "valkey"
	WarningKindDataBucket    = "data_bucket"
	WarningKindEventBus      = "event_bus"
)

// warningKinds lists every WarningKind constant. It must match the
// TypeScript WarningKind union in web/src/types/configWarnings.ts.
var warningKinds = []string{
	WarningKindEnv,
	WarningKindBackend,
	WarningKindService,
	WarningKindScheduledTask,
	WarningKindEventTask,
	WarningKindAmplifyApp,
	WarningKindComputePool,
	WarningKindValkey,
	WarningKindDataBucket,
	WarningKindEventBus,
}

// Warning codes, one per rule. Stable: tests and the web UI key on them.
const (
	warnCodeRegionDrift                  = "region_drift"
	warnCodeVPCCIDROverlap               = "vpc_cidr_overlap"
	warnCodePolicyWildcardAction         = "policy_wildcard_action"
	warnCodeServiceNoHealthCheck         = "service_no_health_check"
	warnCodeAmplifyRuleDisabledWorkload  = "amplify_rule_disabled_workload"
	warnCodeAmplifyRuleUnreachable       = "amplify_rule_unreachable"
	warnCodeEventSubscriptionAllTypes    = "event_subscription_all_types"
	warnCodeEventProducesAny             = "event_produces_any"
	warnCodeEventQueueUnused             = "event_queue_unused"
	warnCodeEventBusToggle               = "event_bus_toggle"
	warnCodeOwnBackendBucketEnv          = "own_backend_bucket_env"
	warnCodeDataBucketDisabledWorkload   = "data_bucket_disabled_workload"
	warnCodeValkeyAccessDisabledWorkload = "valkey_access_disabled_workload"
	warnCodeValkeyNoAccess               = "valkey_no_access"
	warnCodeComputePoolUnused            = "compute_pool_unused"
	warnCodeComputePoolSpotWithoutBase   = "compute_pool_spot_without_base"
	warnCodeComputePoolBridgeSRV         = "compute_pool_bridge_srv"
	warnCodeBackendExternalImage         = "backend_external_image_not_pushable"
)

// ConfigWarningReport is what GET /api/config-warnings returns and the TUI
// deploy gate reads.
type ConfigWarningReport struct {
	Env string `json:"env"`
	// Warnings is never nil, so it encodes as [] when there are none.
	Warnings []ConfigWarning `json:"warnings"`
	// ConfigError is set when generate would refuse this YAML. Warnings then
	// holds what generate prints before refusing.
	ConfigError string `json:"configError,omitempty"`
}

// envItem is the item of a warning about the environment as a whole.
func envItem() WarningItem {
	return WarningItem{Kind: WarningKindEnv}
}

// workloadItem is the item of a warning about one entry of the YAML. The
// singleton kinds never carry a name.
func workloadItem(kind, name string) WarningItem {
	switch kind {
	case WarningKindEnv, WarningKindBackend, WarningKindValkey, WarningKindEventBus:
		name = ""
	}
	return WarningItem{Kind: kind, Name: name}
}

// docsBaseURL is where the repository's docs are published.
const docsBaseURL = "https://github.com/MadAppGang/meroku/blob/main/"

// docsLink turns a repository-relative docs path into an absolute URL a
// browser can open.
func docsLink(path string) string {
	return docsBaseURL + path
}

// sortConfigWarnings orders warnings env-wide first, then by item kind, item
// name, code and detail, so every presentation shows the same order.
func sortConfigWarnings(ws []ConfigWarning) {
	sort.SliceStable(ws, func(i, j int) bool {
		a, b := ws[i], ws[j]
		if (a.Item.Kind == WarningKindEnv) != (b.Item.Kind == WarningKindEnv) {
			return a.Item.Kind == WarningKindEnv
		}
		if a.Item.Kind != b.Item.Kind {
			return a.Item.Kind < b.Item.Kind
		}
		if a.Item.Name != b.Item.Name {
			return a.Item.Name < b.Item.Name
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Detail < b.Detail
	})
}

// dedupeConfigWarnings drops a warning with the same code, item and detail as
// an earlier one, keeping order.
func dedupeConfigWarnings(ws []ConfigWarning) []ConfigWarning {
	type key struct {
		code   string
		item   WarningItem
		detail string
	}
	seen := make(map[key]bool, len(ws))
	out := make([]ConfigWarning, 0, len(ws))
	for _, w := range ws {
		k := key{w.Code, w.Item, w.Detail}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, w)
	}
	return out
}

// normalizeConfigWarnings returns a sorted, deduplicated, never-nil copy.
func normalizeConfigWarnings(ws []ConfigWarning) []ConfigWarning {
	out := dedupeConfigWarnings(ws)
	sortConfigWarnings(out)
	return out
}

// printConfigWarnings is the one plain-text renderer of configuration
// warnings: a "⚠️  <title>" line, the detail indented three spaces, a Docs
// line when there is a link, and a blank line after each. No colour: the
// output may be a CI log.
func printConfigWarnings(w io.Writer, ws []ConfigWarning) {
	for _, warning := range ws {
		fmt.Fprintf(w, "⚠️  %s\n", warning.Title)
		if warning.Detail != "" {
			for _, line := range strings.Split(warning.Detail, "\n") {
				if strings.TrimSpace(line) == "" {
					fmt.Fprintln(w)
					continue
				}
				fmt.Fprintf(w, "   %s\n", line)
			}
		}
		if warning.Docs != "" {
			fmt.Fprintf(w, "   Docs: %s\n", warning.Docs)
		}
		fmt.Fprintln(w)
	}
}

// printGenerateNotices prints the notices preprocessing recorded. A notice
// says what generate did on the user's behalf; it is not a warning.
func printGenerateNotices(w io.Writer, notices []string) {
	for _, n := range notices {
		fmt.Fprint(w, n)
		if !strings.HasSuffix(n, "\n") {
			fmt.Fprintln(w)
		}
	}
}

// warningInputs is what the env-level warning sources read from outside the
// env map.
type warningInputs struct {
	// EnvName is the env file stem.
	EnvName string
	// EnvFile is the path the YAML was read from; its directory is where the
	// sibling environments are looked for.
	EnvFile string
	// AmbientRegion resolves the shell's region for the region_drift check.
	// nil skips region_drift: the web server's environment is not the shell
	// any deploy runs in.
	AmbientRegion func(profile string) (region, source string)
	// Siblings lists the VPC ranges of the other environments in dir.
	Siblings func(dir, self string) ([]vpcCIDRUse, error)
}

// defaultWarningInputs is the inputs for the process that deploys: the shell's
// region and the sibling YAMLs next to the env file.
func defaultWarningInputs(env string) warningInputs {
	envFile, err := resolveEnvFilePath(env)
	if err != nil {
		envFile = env + ".yaml"
	}
	return warningInputs{
		EnvName:       env,
		EnvFile:       envFile,
		AmbientRegion: resolveAmbientRegion,
		Siblings:      siblingVPCCIDRUses,
	}
}

// webWarningInputs is defaultWarningInputs without region_drift. The web
// server's AWS_REGION is whatever environment was last selected, so it says
// nothing about the shell a deploy will run in.
func webWarningInputs(env string) warningInputs {
	in := defaultWarningInputs(env)
	in.AmbientRegion = nil
	return in
}

// envLevelWarnings are the warnings computed from the raw env map before
// preprocessing: region drift and VPC range overlap with a sibling. Reads
// only the map and the inputs; prints nothing.
func envLevelWarnings(envMap map[string]interface{}, in warningInputs) []ConfigWarning {
	var out []ConfigWarning

	if in.AmbientRegion != nil {
		ambient, source := in.AmbientRegion(stringFromMap(envMap, "aws_profile"))
		if w, ok := regionDriftWarning(stringFromMap(envMap, "region"), ambient, source); ok {
			out = append(out, w)
		}
	}

	if in.Siblings != nil && in.EnvFile != "" {
		self := in.EnvName
		if self == "" {
			self = strings.TrimSuffix(filepath.Base(in.EnvFile), filepath.Ext(in.EnvFile))
		}
		if siblings, err := in.Siblings(filepath.Dir(in.EnvFile), self); err == nil {
			if w, ok := vpcCIDROverlapWarning(self, envMap, siblings); ok {
				out = append(out, w)
			}
		}
	}
	return out
}

// configWarnings is every warning for a map preprocessEnvMap has already run
// on: the env-level ones passed in, those preprocessing recorded, and the two
// sources that read the preprocessed map. Sorted, deduplicated, never nil.
// Prints nothing.
func configWarnings(envMap map[string]interface{}, envLevel []ConfigWarning) []ConfigWarning {
	all := make([]ConfigWarning, 0, len(envLevel))
	all = append(all, envLevel...)
	all = append(all, generateWarnings(envMap)...)
	all = append(all, githubExternalImageWarnings(envMap)...)
	all = append(all, amplifyRewriteWarnings(envMap)...)
	return normalizeConfigWarnings(all)
}

// collectEnvConfigWarnings loads <env>.yaml without writing it, runs the same
// checks and preprocessing generate runs, in memory, and returns the report.
// No AWS or network call, no file write, no output.
//
// ConfigError is set where generate would refuse the YAML, and Warnings then
// holds exactly what generate prints before that refusal.
func collectEnvConfigWarnings(env string, in warningInputs) (report ConfigWarningReport) {
	report = ConfigWarningReport{Env: env, Warnings: []ConfigWarning{}}

	// Preprocessing a hand-edited YAML runs type assertions on whatever the
	// user wrote. A panic in one must not take down the web server or the TUI
	// before the deploy that would report the same input properly.
	defer func() {
		if r := recover(); r != nil {
			report.ConfigError = fmt.Sprintf("checking the configuration failed: %v", r)
		}
	}()

	envFile, envMap, err := loadEnvMapReadOnly(env)
	if err != nil {
		report.ConfigError = err.Error()
		return report
	}
	in.EnvFile = envFile
	if in.EnvName == "" {
		in.EnvName = env
	}

	if err := validateConfigBeforePreprocess(env, envMap); err != nil {
		report.ConfigError = invalidConfigurationError(envFile, err).Error()
		return report
	}

	envLevel := envLevelWarnings(envMap, in)

	if previous, ok := previousEventBusState(env); ok {
		envMap[eventBusPreviousKey] = previous
	}

	if err := preprocessEnvMap(envMap); err != nil {
		report.Warnings = normalizeConfigWarnings(envLevel)
		report.ConfigError = invalidConfigurationError(envFile, err).Error()
		return report
	}

	report.Warnings = configWarnings(envMap, envLevel)

	if err := validateCustomTerraformRootNames(env); err != nil {
		report.ConfigError = err.Error()
	}
	return report
}

// invalidConfigurationError wraps a refusal the way generate reports it.
func invalidConfigurationError(envFile string, err error) error {
	return fmt.Errorf("invalid configuration in %s:\n\n%w", envFile, err)
}
