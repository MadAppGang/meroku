package main

// Decisions about VPC ranges: the generate-time check, the read-only report
// behind `meroku vpc-cidr check|suggest` and GET /api/vpc-cidr, the guarded
// write behind `meroku vpc-cidr assign`, and the random range for a new env.

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
)

// vpcCIDRDeps is everything the service reads from the outside world, so
// tests can replace each part.
type vpcCIDRDeps struct {
	// siblings returns the ranges of the other environment YAMLs in dir.
	siblings func(dir, self string) ([]vpcCIDRUse, error)
	// aws builds the AWS readers for an environment.
	aws func(ctx context.Context, env Env) (vpcCIDRAWS, error)
	// siblingAccounts returns the other environments with their AWS
	// accounts; with AWS asked for, each distinct account is listed too.
	siblingAccounts func(dir, self string) ([]vpcCIDRSiblingAccount, error)
	// intn returns a uniform int in [0, n).
	intn func(int) int
}

// defaultVPCCIDRDeps is the production wiring.
func defaultVPCCIDRDeps() vpcCIDRDeps {
	return vpcCIDRDeps{
		siblings:        siblingVPCCIDRUses,
		aws:             newVPCCIDRAWSForEnv,
		siblingAccounts: siblingVPCCIDRAccounts,
		intn:            rand.IntN,
	}
}

// Values of vpcCIDRReport.Deployed.
const (
	vpcCIDRDeployedNo        = "no"
	vpcCIDRDeployedYes       = "yes"
	vpcCIDRDeployedUnknown   = "unknown"
	vpcCIDRDeployedUnchecked = "unchecked"
)

// vpcCIDRAWSUnreachableNote is the plain sentence shown when AWS was asked for
// and could not be read.
const vpcCIDRAWSUnreachableNote = "Couldn't reach AWS — these avoid your other meroku environments only"

// vpcCIDROwnAccountUnreachableNote is shown when this environment's account
// could not be read but at least one sibling's account was.
const vpcCIDROwnAccountUnreachableNote = "Couldn't reach this environment's AWS account — these avoid your other environments and their AWS accounts only"

// The notes for an environment with no aws_profile/region yet. Nothing failed,
// so they must not read like an error to someone setting up a new environment.
const (
	vpcCIDRNoAWSAccountNote             = "This environment isn't connected to an AWS account yet — these avoid your other meroku environments only"
	vpcCIDRNoAWSAccountSiblingsReadNote = "This environment isn't connected to an AWS account yet — these avoid your other environments and their AWS accounts"
)

// vpcCIDRSiblingAccountUnreachableNote names the environments whose AWS
// accounts could not be read.
func vpcCIDRSiblingAccountUnreachableNote(names []string) string {
	return fmt.Sprintf("Couldn't reach the AWS account for %s — networks there were not checked", strings.Join(names, ", "))
}

// vpcCIDRReplaceWarning is what changing vpc_cidr on a deployed env costs.
const vpcCIDRReplaceWarning = "Changing vpc_cidr on a deployed environment replaces the VPC and everything in it: " +
	"subnets, security groups, the ALB and its target groups, the Aurora subnet group, and every ECS service."

// vpcCIDROverrideGuidance ends every generate-time rule error.
func vpcCIDROverrideGuidance(envName string) string {
	return "If this environment is already deployed, do NOT change vpc_cidr — add " +
		"`allow_vpc_cidr_override: true` to its YAML. If it is not deployed yet, run " +
		fmt.Sprintf("`meroku vpc-cidr assign %s`.", envName)
}

// vpcCIDRCandidateResult is the verdict on a value the user is typing.
type vpcCIDRCandidateResult struct {
	Value      string           `json:"value"`
	Valid      bool             `json:"valid"`
	Incomplete bool             `json:"incomplete"`
	Error      *vpcCIDRError    `json:"error,omitempty"`
	Warnings   []vpcCIDRWarning `json:"warnings"`
}

// vpcCIDRReport is the read-only picture of one environment's range. Its JSON
// is the body of GET /api/vpc-cidr (api-contract.md), field for field.
type vpcCIDRReport struct {
	Env            string                  `json:"env"`
	UseDefaultVPC  bool                    `json:"useDefaultVpc"`
	Override       bool                    `json:"override"`
	Configured     string                  `json:"configured"`
	Effective      string                  `json:"effective"`
	Error          *vpcCIDRError           `json:"error,omitempty"`
	Warnings       []vpcCIDRWarning        `json:"warnings"`
	Suggestions    []string                `json:"suggestions"`
	AWSChecked     bool                    `json:"awsChecked"`
	AWSNote        string                  `json:"awsNote,omitempty"`
	Deployed       string                  `json:"deployed"`
	DeployedReason string                  `json:"deployedReason"`
	Candidate      *vpcCIDRCandidateResult `json:"candidate,omitempty"`

	// AWSError is the technical reason AWS could not be read, for the CLI.
	AWSError string `json:"-"`
	// StateError is the technical reason remote state could not be read,
	// for the CLI. The JSON reason stays plain.
	StateError string `json:"-"`
}

// vpcCIDRInspectOptions selects what inspectVPCCIDR looks at.
type vpcCIDRInspectOptions struct {
	// EnvName is the file stem (<name>.yaml); it can differ from env.Env.
	EnvName string
	// Dir is the project directory holding the environment YAMLs.
	Dir string
	// WithAWS also lists VPCs and reads remote state in the env's account,
	// and lists the VPCs in every sibling environment's account.
	WithAWS bool
	// Count is how many suggestions to return (0 for none).
	Count int
	// Candidate, when non-nil, is validated as typed. A candidate request
	// never calls AWS, whatever WithAWS says: it runs once per keystroke.
	Candidate *string
}

// inspectVPCCIDR reports on env's range without changing anything. An AWS
// failure becomes AWSNote and Deployed "unknown"; it is never an error.
func inspectVPCCIDR(ctx context.Context, deps vpcCIDRDeps, env Env, opts vpcCIDRInspectOptions) vpcCIDRReport {
	withAWS := opts.WithAWS && opts.Candidate == nil

	report := vpcCIDRReport{
		Env:           opts.EnvName,
		UseDefaultVPC: env.UseDefaultVPC,
		Override:      env.AllowVPCCIDROverride,
		Configured:    env.VPCCIDR,
		Effective:     effectiveVPCCIDR(env.UseDefaultVPC, env.VPCCIDR),
		Warnings:      []vpcCIDRWarning{},
		Suggestions:   []string{},
	}

	if !env.UseDefaultVPC && strings.TrimSpace(env.VPCCIDR) != "" && !env.AllowVPCCIDROverride {
		if _, err := validateVPCCIDR(env.VPCCIDR); err != nil {
			var verr *vpcCIDRError
			if errors.As(err, &verr) {
				report.Error = verr
			}
		}
	}

	var siblings []vpcCIDRUse
	if deps.siblings != nil {
		// A directory that cannot be read means no siblings to compare with;
		// the report is still useful without them.
		siblings, _ = deps.siblings(opts.Dir, opts.EnvName)
	}

	// AWS: own VPC (for the deployed rule) and every other VPC's ranges.
	var awsOthers []vpcCIDRUse
	var ownVPCs []awsVPC
	var awsReaders vpcCIDRAWS
	awsReady, listed := false, false
	ownAWSError := ""
	ownNoTarget := false
	if withAWS && deps.aws != nil {
		readers, err := deps.aws(ctx, env)
		if err != nil {
			report.AWSError = err.Error()
			ownAWSError = report.AWSError
			ownNoTarget = errors.Is(err, errVPCCIDRNoAWSTarget)
		} else {
			awsReaders, awsReady = readers, true
			if readers.ListVPCs != nil {
				vpcs, err := readers.ListVPCs(ctx)
				if err != nil {
					report.AWSError = err.Error()
					ownAWSError = report.AWSError
				} else {
					ownVPCs, awsOthers = awsVPCUses(vpcs, env)
					listed = true
				}
			}
		}
		report.AWSChecked = listed

		siblingOthers, reached, unreached, errs := listSiblingAccountVPCs(ctx, deps, env, opts, listed)
		awsOthers = append(awsOthers, siblingOthers...)

		var notes []string
		switch {
		case !listed && ownNoTarget && reached > 0:
			notes = append(notes, vpcCIDRNoAWSAccountSiblingsReadNote)
		case !listed && ownNoTarget:
			notes = append(notes, vpcCIDRNoAWSAccountNote)
		case !listed && reached > 0:
			notes = append(notes, vpcCIDROwnAccountUnreachableNote)
		case !listed:
			notes = append(notes, vpcCIDRAWSUnreachableNote)
		}
		if len(unreached) > 0 {
			notes = append(notes, vpcCIDRSiblingAccountUnreachableNote(unreached))
		}
		report.AWSNote = strings.Join(notes, ". ")
		if len(errs) > 0 {
			all := append([]string{}, errs...)
			if report.AWSError != "" {
				all = append([]string{report.AWSError}, errs...)
			}
			report.AWSError = strings.Join(all, "; ")
		}
	}

	// Keep the remote-state failure for the CLI; the JSON reason is plain.
	if awsReaders.State != nil {
		state := awsReaders.State
		awsReaders.State = func(ctx context.Context, e Env) (remoteStateSummary, error) {
			summary, err := state(ctx, e)
			if err != nil && !errors.Is(err, errRemoteStateAbsent) {
				report.StateError = err.Error()
			}
			return summary, err
		}
	}

	// Overlap warnings for the effective range.
	if p, ok := parseVPCCIDRForOverlap(report.Effective); ok {
		report.Warnings = vpcCIDROverlapWarnings(p, append(append([]vpcCIDRUse{}, siblings...), awsOthers...))
	}

	// Suggestions avoid siblings and, when checked, every other VPC.
	if opts.Count > 0 && deps.intn != nil {
		used := append(append([]vpcCIDRUse{}, siblings...), awsOthers...)
		if picks, err := pickVPCCIDRs(deps.intn, used, opts.Count); err == nil {
			for _, p := range picks {
				report.Suggestions = append(report.Suggestions, p.String())
			}
		}
	}

	report.Deployed, report.DeployedReason = vpcCIDRDeployed(ctx, env, withAWS, awsReady, awsReaders, listed, ownVPCs, ownAWSError)

	if opts.Candidate != nil {
		report.Candidate = inspectVPCCIDRCandidate(*opts.Candidate, siblings)
	}
	return report
}

// listSiblingAccountVPCs lists the VPCs in every distinct AWS account named by
// a sibling environment, skipping the account this env's readers already
// covered (when that listing worked). Each sibling's own VPC is left out (its range is counted from its
// YAML), and so is this env's own VPC, in case two profiles reach one account.
// It returns the other ranges, how many accounts were read, the names of the
// envs whose account could not be read, and the technical errors.
func listSiblingAccountVPCs(ctx context.Context, deps vpcCIDRDeps, env Env, opts vpcCIDRInspectOptions, ownListed bool) (uses []vpcCIDRUse, reached int, unreached []string, errs []string) {
	if deps.siblingAccounts == nil || deps.aws == nil {
		return nil, 0, nil, nil
	}
	accounts, err := deps.siblingAccounts(opts.Dir, opts.EnvName)
	if err != nil || len(accounts) == 0 {
		return nil, 0, nil, nil
	}

	done := map[string]bool{}
	if ownListed {
		for _, k := range awsAccountKeys(env) {
			done[k] = true
		}
	}
	for _, acct := range accounts {
		keys := awsAccountKeys(acct.Env)
		if strings.TrimSpace(acct.Env.AWSProfile) == "" || strings.TrimSpace(acct.Env.Region) == "" {
			continue // no account to reach; never fall back to $AWS_PROFILE
		}
		seen := false
		for _, k := range keys {
			if done[k] {
				seen = true
			}
		}
		for _, k := range keys {
			done[k] = true
		}
		if seen {
			continue
		}

		readers, err := deps.aws(ctx, acct.Env)
		if err == nil && readers.ListVPCs == nil {
			err = errors.New("no VPC lister")
		}
		var vpcs []awsVPC
		if err == nil {
			vpcs, err = readers.ListVPCs(ctx)
		}
		if err != nil {
			unreached = append(unreached, acct.Name)
			errs = append(errs, fmt.Sprintf("%s: %v", acct.Name, err))
			continue
		}
		reached++
		for _, v := range vpcs {
			if isOwnVPC(v, env) || isOwnVPCOfAny(v, accounts) {
				continue
			}
			for _, p := range v.CIDRs {
				uses = append(uses, siblingAccountVPCCIDRUse(acct.Name, p))
			}
		}
	}
	return uses, reached, unreached, errs
}

func isOwnVPCOfAny(v awsVPC, accounts []vpcCIDRSiblingAccount) bool {
	for _, a := range accounts {
		if isOwnVPC(v, a.Env) {
			return true
		}
	}
	return false
}

// vpcCIDRDeployed applies the deployed rule:
//
//	no account and no profile          → no (nothing can exist yet)
//	AWS not asked                      → unchecked
//	own VPC found by tags              → yes
//	remote state owns aws_vpc          → yes
//	remote state absent / has no VPC   → no
//	anything unreadable                → unknown
func vpcCIDRDeployed(ctx context.Context, env Env, withAWS, awsReady bool, readers vpcCIDRAWS, listed bool, own []awsVPC, awsErr string) (string, string) {
	if strings.TrimSpace(env.AccountID) == "" && strings.TrimSpace(env.AWSProfile) == "" {
		return vpcCIDRDeployedNo, "no AWS account is configured for this environment yet"
	}
	if !withAWS {
		return vpcCIDRDeployedUnchecked, "AWS was not checked — rerun with --aws to find out whether this environment is deployed"
	}
	if listed && len(own) > 0 {
		return vpcCIDRDeployedYes, "a VPC tagged for this environment exists in the AWS account"
	}
	if !awsReady || readers.State == nil {
		return vpcCIDRDeployedUnknown, "could not read AWS — the reason is shown by `meroku vpc-cidr check <env> --aws`"
	}
	if strings.TrimSpace(env.StateBucket) == "" {
		if listed {
			return vpcCIDRDeployedNo, "no VPC for this environment in the account and no state backend configured"
		}
		return vpcCIDRDeployedUnknown, "could not list VPCs and no state backend is configured"
	}
	summary, err := readers.State(ctx, env)
	switch {
	case errors.Is(err, errRemoteStateAbsent):
		return vpcCIDRDeployedNo, "remote state not found"
	case err != nil:
		return vpcCIDRDeployedUnknown, "remote state could not be read"
	case summary.owns("aws_vpc"):
		return vpcCIDRDeployedYes, "remote state manages a VPC"
	default:
		return vpcCIDRDeployedNo, "remote state has no VPC"
	}
}

// inspectVPCCIDRCandidate validates a typed value and checks it against
// siblings. It ignores allow_vpc_cidr_override: it judges what is being typed.
func inspectVPCCIDRCandidate(value string, siblings []vpcCIDRUse) *vpcCIDRCandidateResult {
	result := &vpcCIDRCandidateResult{Value: value, Warnings: []vpcCIDRWarning{}}
	if vpcCIDRInputIncomplete(value) {
		result.Incomplete = true
		return result
	}
	p, err := validateVPCCIDR(value)
	if err != nil {
		var verr *vpcCIDRError
		if errors.As(err, &verr) {
			result.Error = verr
		}
		return result
	}
	result.Valid = true
	result.Warnings = vpcCIDROverlapWarnings(p, siblings)
	return result
}

// parseVPCCIDRForOverlap parses a range for overlap checks only, tolerating
// host bits (an invalid value still overlaps what it overlaps).
func parseVPCCIDRForOverlap(s string) (netip.Prefix, bool) {
	if s == "" {
		return netip.Prefix{}, false
	}
	p, err := netip.ParsePrefix(strings.TrimSpace(s))
	if err != nil || !p.Addr().Is4() {
		return netip.Prefix{}, false
	}
	return p.Masked(), true
}

// errVPCCIDRAssignRefused wraps every refusal to write.
var errVPCCIDRAssignRefused = errors.New("vpc_cidr not changed")

// vpcCIDRAssignOptions selects what assignVPCCIDR does.
type vpcCIDRAssignOptions struct {
	EnvName string
	Dir     string
	WithAWS bool
	// Force writes even when the environment is or may be deployed.
	Force bool
}

// vpcCIDRAssignResult is what assignVPCCIDR did.
type vpcCIDRAssignResult struct {
	Previous   string        // vpc_cidr before ("" when unset)
	CIDR       string        // vpc_cidr after (or the refused suggestion)
	Changed    bool          // the YAML was rewritten
	BackupPath string        // backup written before the change
	Report     vpcCIDRReport // the report the decision was made on
}

// assignVPCCIDR gives env a valid, non-overlapping range and writes it.
//
// Idempotent: a range that is set, valid and free is kept. Otherwise the first
// suggestion is written — but only when the environment is known not to be
// deployed, or Force is set. A refusal wraps errVPCCIDRAssignRefused and the
// result still carries the suggestion.
func assignVPCCIDR(ctx context.Context, deps vpcCIDRDeps, env Env, opts vpcCIDRAssignOptions) (vpcCIDRAssignResult, error) {
	result := vpcCIDRAssignResult{Previous: env.VPCCIDR}
	if env.UseDefaultVPC {
		return result, fmt.Errorf("%w: %s uses the AWS default VPC (use_default_vpc: true), so vpc_cidr is not used", errVPCCIDRAssignRefused, opts.EnvName)
	}

	report := inspectVPCCIDR(ctx, deps, env, vpcCIDRInspectOptions{
		EnvName: opts.EnvName,
		Dir:     opts.Dir,
		WithAWS: opts.WithAWS,
		Count:   1,
	})
	result.Report = report

	current := strings.TrimSpace(env.VPCCIDR)
	if current != "" && len(report.Warnings) == 0 {
		if _, err := validateVPCCIDR(current); err == nil {
			result.CIDR = current
			return result, nil
		}
	}

	if len(report.Suggestions) == 0 {
		return result, errVPCCIDRPoolExhausted
	}
	result.CIDR = report.Suggestions[0]

	if !opts.Force {
		switch report.Deployed {
		case vpcCIDRDeployedNo:
		case vpcCIDRDeployedUnchecked:
			return result, fmt.Errorf("%w: meroku has not checked whether %s is deployed — rerun with --aws (or --force). %s",
				errVPCCIDRAssignRefused, opts.EnvName, vpcCIDRReplaceWarning)
		default:
			return result, fmt.Errorf("%w: %s may already be deployed (%s, %s). %s Rerun with --force only if you mean it.",
				errVPCCIDRAssignRefused, opts.EnvName, report.Deployed, report.DeployedReason, vpcCIDRReplaceWarning)
		}
	}

	path, err := findEnvYAMLInDir(opts.Dir, opts.EnvName)
	if err != nil {
		return result, err
	}
	backup, err := CreateProjectBackup(path)
	if err != nil {
		return result, fmt.Errorf("failed to back up %s: %w", path, err)
	}
	result.BackupPath = backup

	fresh, err := loadEnv(strings.TrimSuffix(path, filepath.Ext(path)))
	if err != nil {
		return result, fmt.Errorf("failed to reload %s: %w", path, err)
	}
	fresh.VPCCIDR = result.CIDR
	if err := saveEnvToFile(fresh, path); err != nil {
		return result, fmt.Errorf("failed to write %s: %w", path, err)
	}
	result.Changed = true
	return result, nil
}

// findEnvYAMLInDir returns dir/<name>.yaml or dir/<name>.yml.
func findEnvYAMLInDir(dir, name string) (string, error) {
	for _, ext := range []string{".yaml", ".yml"} {
		path := filepath.Join(dir, name+ext)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("environment file %s.yaml not found in %s", name, dir)
}

// newEnvVPCCIDR picks a random free /16 for a new environment, avoiding the
// other environment YAMLs in the current directory. Offline.
func newEnvVPCCIDR(envName string) (string, error) {
	return newEnvVPCCIDRWith(defaultVPCCIDRDeps(), ".", envName)
}

// newEnvVPCCIDRWith is newEnvVPCCIDR with injectable dependencies.
func newEnvVPCCIDRWith(deps vpcCIDRDeps, dir, envName string) (string, error) {
	var siblings []vpcCIDRUse
	if deps.siblings != nil {
		siblings, _ = deps.siblings(dir, envName)
	}
	p, err := pickVPCCIDR(deps.intn, siblings)
	if err != nil {
		return "", err
	}
	return p.String(), nil
}

// validateVPCCIDRMap is the generate-time rule check on the raw config map.
// Skipped for the default VPC, an unset vpc_cidr, or
// allow_vpc_cidr_override: true. envName is the file stem, used in guidance.
func validateVPCCIDRMap(envName string, envMap map[string]interface{}) error {
	if useDefault, _ := envMap["use_default_vpc"].(bool); useDefault {
		return nil
	}
	if override, _ := envMap["allow_vpc_cidr_override"].(bool); override {
		return nil
	}
	raw, present := envMap["vpc_cidr"]
	if !present || raw == nil {
		return nil
	}
	value, ok := raw.(string)
	if !ok {
		return fmt.Errorf("vpc_cidr must be a string like 10.42.0.0/16, got %T.\n\n%s", raw, vpcCIDROverrideGuidance(envName))
	}
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if _, err := validateVPCCIDR(value); err != nil {
		return fmt.Errorf("%s.\n\n%s", err.Error(), vpcCIDROverrideGuidance(envName))
	}
	return nil
}

// vpcCIDROverlapWarningText is the generate-time overlap warning for envMap
// against siblings, with the AWS tip. "" when there is nothing to say.
func vpcCIDROverlapWarningText(envName string, envMap map[string]interface{}, siblings []vpcCIDRUse) string {
	useDefault, _ := envMap["use_default_vpc"].(bool)
	p, ok := parseVPCCIDRForOverlap(effectiveVPCCIDR(useDefault, stringFromMap(envMap, "vpc_cidr")))
	if !ok {
		return ""
	}
	warnings := vpcCIDROverlapWarnings(p, siblings)
	if len(warnings) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n⚠️  VPC range overlap\n\n")
	for _, w := range warnings {
		fmt.Fprintf(&b, "    %s\n", w.Message)
	}
	fmt.Fprintf(&b, "\nOverlapping VPCs can never be peered or share a Transit Gateway. Fine if\n")
	fmt.Fprintf(&b, "these environments will never be connected; otherwise give one of them a\n")
	fmt.Fprintf(&b, "different range before it is deployed (`meroku vpc-cidr assign %s`).\n", envName)
	fmt.Fprintf(&b, "Tip: `meroku vpc-cidr check %s --aws` also checks VPCs already in your AWS account.\n", envName)
	return b.String()
}

// warnOnVPCCIDROverlap prints the sibling overlap warning during generation.
// Offline: generation never calls AWS for this.
func warnOnVPCCIDROverlap(envFile string, envMap map[string]interface{}) {
	dir := filepath.Dir(envFile)
	self := strings.TrimSuffix(filepath.Base(envFile), filepath.Ext(envFile))
	siblings, err := siblingVPCCIDRUses(dir, self)
	if err != nil {
		return
	}
	if w := vpcCIDROverlapWarningText(self, envMap, siblings); w != "" {
		fmt.Print(w)
	}
}
