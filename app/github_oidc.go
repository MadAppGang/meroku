package main

// Sharing one account's GitHub Actions OIDC provider between meroku projects.
//
// AWS keys an OIDC provider on its issuer URL and embeds that URL in the ARN as
// the resource path, so an account holds exactly one
// token.actions.githubusercontent.com provider and there is no field left over
// to tell two of them apart. Every meroku project before schema v28 tried to
// create its own, which meant the second project in an account failed its apply
// with EntityAlreadyExists and had no configuration to fall back on.
//
// The fix is workload.github_oidc_create_provider. Deciding its value is the
// interesting part, and the rule is narrower than it first looks: the question
// is not whether the provider exists, it is whether *this environment's state*
// owns it.
//
//	owns it  exists  create_provider
//	yes      yes     true   — keep owning it
//	no       yes     false  — federate against the existing one
//	no       no      true   — first project in this account
//	yes      no      true   — drift; recreate it
//
// Deciding on existence alone inverts the first row. The owning project would
// see "it exists, so do not create it", count would fall from 1 to 0, and
// Terraform would destroy the provider that every project in the account
// federates against.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const (
	// githubOIDCIssuerHost is the issuer as it appears in an OIDC provider ARN,
	// which is the URL with the scheme removed. GitHub issues tokens with this
	// exact iss claim, so it is not configurable: a provider registered under
	// any other URL would never match a GitHub token.
	githubOIDCIssuerHost = "token.actions.githubusercontent.com"

	githubOIDCIssuerURL = "https://" + githubOIDCIssuerHost

	// githubOIDCResourceType is the Terraform type as it appears in state.
	githubOIDCResourceType = "aws_iam_openid_connect_provider"
)

// githubOIDCProviderARN builds the provider's ARN, which is fully determined by
// the account and the issuer host. Used to address the provider for a read.
// Terraform must not build the ARN this way — see the locals block in
// modules/workloads/github.tf for why.
func githubOIDCProviderARN(accountID string) string {
	return fmt.Sprintf("arn:aws:iam::%s:oidc-provider/%s", accountID, githubOIDCIssuerHost)
}

// githubOIDCStatus is what meroku learned about the account's provider.
type githubOIDCStatus struct {
	// Exists reports whether the account already has the provider.
	Exists bool
	// ARN is set when Exists is true.
	ARN string
	// OwnedByThisEnv reports whether this environment's Terraform state manages
	// it. This is the field the decision turns on.
	OwnedByThisEnv bool
	// OwnerProject and OwnerEnv come from the provider's meroku tags and exist
	// to name the other project in a message. They are empty for a provider
	// created outside meroku, which is why nothing may branch on them.
	OwnerProject string
	OwnerEnv     string
	// StateUnknown reports that the state could not be read, so OwnedByThisEnv
	// is a guess rather than a fact. Callers must not write config on it.
	StateUnknown bool
}

// ShouldCreateProvider returns the correct value of
// workload.github_oidc_create_provider for this environment.
func (s githubOIDCStatus) ShouldCreateProvider() bool {
	return !s.Exists || s.OwnedByThisEnv
}

// Summary describes the account's provider in one line, for a UI or a preflight
// message.
func (s githubOIDCStatus) Summary() string {
	switch {
	case !s.Exists:
		return "No GitHub OIDC provider in this account yet; this project will create it."
	case s.OwnedByThisEnv:
		return "This environment already owns the account's GitHub OIDC provider."
	case s.OwnerProject != "":
		return fmt.Sprintf("The GitHub OIDC provider is owned by project %q (%s); this project will federate against it.",
			s.OwnerProject, s.OwnerEnv)
	default:
		return "The GitHub OIDC provider already exists in this account; this project will federate against it."
	}
}

// iamOIDCReader is the slice of the IAM API this file uses. An interface so the
// tests never reach AWS.
type iamOIDCReader interface {
	GetOpenIDConnectProvider(ctx context.Context, in *iam.GetOpenIDConnectProviderInput, optFns ...func(*iam.Options)) (*iam.GetOpenIDConnectProviderOutput, error)
}

// resolveGithubOIDCStatus answers both halves of the question: the state for
// ownership, IAM for existence.
//
// Both reads are allowed to fail. A failure to read is not evidence, so the
// result carries StateUnknown and callers decline to act rather than guessing.
func resolveGithubOIDCStatus(ctx context.Context, env Env, iamClient iamOIDCReader, lookup remoteStateLookup) (githubOIDCStatus, error) {
	if env.AccountID == "" {
		return githubOIDCStatus{}, errors.New("account_id is not set in this environment's config")
	}

	status := githubOIDCStatus{ARN: githubOIDCProviderARN(env.AccountID)}

	out, err := iamClient.GetOpenIDConnectProvider(ctx, &iam.GetOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: aws.String(status.ARN),
	})
	switch {
	case err == nil:
		status.Exists = true
		status.OwnerProject, status.OwnerEnv = merokuTagsOf(out.Tags)
	case isIAMNotFound(err):
		// A definite answer: the account has no provider. Nothing else to ask.
		status.Exists = false
		status.ARN = ""
		return status, nil
	default:
		return githubOIDCStatus{}, fmt.Errorf("could not read the account's GitHub OIDC provider: %w", err)
	}

	// The provider exists, so ownership now decides everything.
	summary, err := lookup(ctx, env)
	switch {
	case err == nil:
		status.OwnedByThisEnv = summary.owns(githubOIDCResourceType)
	case errors.Is(err, errRemoteStateAbsent):
		// This environment has never been deployed, so it owns nothing. That is
		// a fact, not a failed read.
		status.OwnedByThisEnv = false
	default:
		status.StateUnknown = true
		return status, fmt.Errorf("could not read this environment's terraform state: %w", err)
	}

	return status, nil
}

// merokuTagsOf pulls the Project and Environment tags the module writes
// (modules/workloads/github.tf). Absent tags are normal for a provider created
// outside meroku and are reported as empty strings.
func merokuTagsOf(tags []iamtypes.Tag) (project, env string) {
	for _, t := range tags {
		if t.Key == nil || t.Value == nil {
			continue
		}
		switch *t.Key {
		case "Project":
			project = *t.Value
		case "Environment":
			env = *t.Value
		}
	}
	return project, env
}

// isIAMNotFound reports whether IAM said the entity does not exist, as opposed
// to refusing to answer. AccessDenied must never be read as absence.
func isIAMNotFound(err error) bool {
	var notFound *iamtypes.NoSuchEntityException
	return errors.As(err, &notFound)
}

// newIAMClient builds an IAM client for an environment's region.
func newIAMClient(ctx context.Context, region string) (*iam.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS configuration: %w", err)
	}
	return iam.NewFromConfig(cfg), nil
}

// newAWSClientsForEnv builds the IAM and STS clients the subject overlap scan
// uses, pinned to one environment's region and profile.
//
// Two things about it are load-bearing.
//
// It honours env.AWSProfile, which newIAMClient does not. A scan that runs
// against whatever AWS_PROFILE the server happens to hold can list a different
// account entirely and return a confident, meaningless "no conflicts".
//
// And both clients come from ONE aws.Config. Constructing them separately lets
// STS assert account A while IAM lists account B, which defeats the assertion
// completely — the assertion would then be checking the credentials of a client
// that never makes the call whose answer is being trusted.
//
// The empty-profile case is not handled here: WithSharedConfigProfile("") is a
// documented no-op, so passing it unconditionally would silently do nothing.
// The refusal that covers a fresh environment lives in
// githubOIDCConflictRefusal, where an empty account_id stops the scan outright.
func newAWSClientsForEnv(ctx context.Context, env Env) (*iam.Client, *sts.Client, error) {
	opts := []func(*config.LoadOptions) error{config.WithRegion(env.Region)}
	if env.AWSProfile != "" {
		opts = append(opts, config.WithSharedConfigProfile(env.AWSProfile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load AWS configuration: %w", err)
	}
	return iam.NewFromConfig(cfg), sts.NewFromConfig(cfg), nil
}

// githubOIDCOutcome is what the pre-deploy GitHub OIDC checks concluded.
//
// It exists because those checks now have two independent things to say and a
// bool can only carry one. Widening the return type is what makes refusal
// expressible at all: the old value meant "the YAML changed", and the deploy
// path continued on every value that was not true, so "a confirmed subject
// conflict the operator did not accept" had no channel to travel down.
type githubOIDCOutcome struct {
	// Regenerate is the existing bool's meaning, unchanged: <env>.yaml was
	// rewritten and the generated terraform is now stale.
	Regenerate bool
	// Block is a confirmed subject conflict the operator did not accept —
	// declined at the prompt, or found with no terminal to ask on. A scan that
	// merely failed never sets it.
	Block bool
}

// newGithubOIDCReader builds the IAM client the provider check reads with. A
// variable so a test can answer it without reaching AWS, following
// startSyncScreen in app/state_reconnect.go.
var newGithubOIDCReader = func(ctx context.Context, region string) (iamOIDCReader, error) {
	return newIAMClient(ctx, region)
}

// lookupGithubOIDCRemoteState is the same seam for the ownership half.
var lookupGithubOIDCRemoteState remoteStateLookup = lookupRemoteStateFromS3

// runGithubOIDCConflictCheck is the same seam for the subject overlap scan,
// whose decision logic is tested directly in github_oidc_cli_test.go.
var runGithubOIDCConflictCheck = checkGithubOIDCSubjectConflicts

// resolveGithubOIDCForEnv settles how this environment relates to the account's
// GitHub OIDC provider, records the answer in <env>.yaml, and then looks for
// another project in the same account whose GitHub Actions role trusts the same
// subjects.
//
// It runs after the AWS pre-flight, which has already validated credentials.
// The two halves have deliberately different authority.
//
// The provider half is advisory in the same sense the compute pool checks are:
// a read it could not make is reported and skipped. Refusing to deploy because
// a diagnostic failed would be worse than deploying without it, and the apply
// still fails loudly on a real collision. That contract also covers a subject
// scan that could not finish — it prints an unmistakable "could not verify"
// line and never blocks.
//
// A subject conflict the scan actually FOUND is not a failed diagnostic. It is
// a privilege boundary that is not there, evidenced by a concrete sub claim
// that assumes both roles, so it blocks unless a human accepts it at a prompt —
// and unconditionally when there is nobody to ask.
//
// It mutates e alongside the file, the way applyDNSOutcome does, so the caller
// is not left holding a stale config.
func resolveGithubOIDCForEnv(ctx context.Context, envName string, e *Env) githubOIDCOutcome {
	if !e.Workload.EnableGithubOIDC {
		return githubOIDCOutcome{}
	}

	// Sequenced rather than written as one composite literal: the provider half
	// may rewrite <env>.yaml and mutate e, and the scan is handed a copy of e
	// after it has done so.
	out := githubOIDCOutcome{Regenerate: resolveGithubOIDCProvider(ctx, envName, e)}
	out.Block = runGithubOIDCConflictCheck(ctx, *e)
	return out
}

// resolveGithubOIDCProvider is the provider half, byte-for-byte the behaviour
// that used to be the whole of resolveGithubOIDCForEnv. It reports whether
// <env>.yaml was rewritten and so needs regenerating.
func resolveGithubOIDCProvider(ctx context.Context, envName string, e *Env) bool {
	fmt.Println("\n🔑 Checking the account's GitHub OIDC provider...")

	iamClient, err := newGithubOIDCReader(ctx, e.Region)
	if err != nil {
		fmt.Printf("   ⚠️  Skipped: %v\n", err)
		return false
	}

	status, err := resolveGithubOIDCStatus(ctx, *e, iamClient, lookupGithubOIDCRemoteState)
	if err != nil {
		fmt.Printf("   ⚠️  Skipped: %v\n", err)
		fmt.Println("      If the apply fails with EntityAlreadyExists, another project in this")
		fmt.Println("      account owns the provider: set workload.github_oidc_create_provider to false.")
		return false
	}

	want := status.ShouldCreateProvider()
	// nil means the key is absent, which the template renders as true.
	current := e.Workload.GithubOIDCCreateProvider == nil || *e.Workload.GithubOIDCCreateProvider

	if want == current {
		fmt.Printf("   ✅ %s\n", status.Summary())
		return false
	}

	backup, err := writeGithubOIDCCreateProvider(envName, want)
	if err != nil {
		fmt.Printf("   ⚠️  Could not record the change: %v\n", err)
		fmt.Printf("      Set workload.github_oidc_create_provider to %t in %s.yaml by hand.\n", want, envName)
		return false
	}

	e.Workload.GithubOIDCCreateProvider = &want
	fmt.Printf("   ℹ️  %s\n", status.Summary())
	if status.ARN != "" {
		fmt.Printf("      %s\n", status.ARN)
	}
	fmt.Printf("   ✏️  Set github_oidc_create_provider: %t in %s.yaml", want, envName)
	if backup != "" {
		fmt.Printf(" (backup: %s)", backup)
	}
	fmt.Println()
	return true
}

// ---------------------------------------------------------------------------
// Web UI
// ---------------------------------------------------------------------------

// githubOIDCStatusResponse is what the GitHub node reads to decide what to show.
type githubOIDCStatusResponse struct {
	Exists         bool   `json:"exists"`
	ARN            string `json:"arn,omitempty"`
	OwnedByThisEnv bool   `json:"owned_by_this_env"`
	OwnerProject   string `json:"owner_project,omitempty"`
	OwnerEnv       string `json:"owner_env,omitempty"`
	// CreateProvider is the value workload.github_oidc_create_provider must hold.
	CreateProvider bool `json:"create_provider"`
	// Changed and Backup describe a write this request performed.
	Changed bool   `json:"changed"`
	Backup  string `json:"backup,omitempty"`
	Summary string `json:"summary"`
}

// getGithubOIDCStatus resolves the account's provider for one environment and
// records the answer in <env>.yaml when it differs from what is on disk.
//
// The write is the point. Reporting the collision and leaving the user to edit
// YAML by hand puts the constraint in front of them at the moment they enable
// the feature, then asks them to act on it; resolving it means the deploy that
// used to fail with EntityAlreadyExists now succeeds untouched.
func getGithubOIDCStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Env string `json:"env"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}
	defer r.Body.Close()

	if req.Env == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env is required"})
		return
	}

	env, err := loadEnv(req.Env)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("environment not found: %v", err)})
		return
	}

	ctx := r.Context()
	iamClient, err := newIAMClient(ctx, env.Region)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}

	status, err := resolveGithubOIDCStatus(ctx, env, iamClient, lookupRemoteStateFromS3)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}

	resp := githubOIDCStatusResponse{
		Exists:         status.Exists,
		ARN:            status.ARN,
		OwnedByThisEnv: status.OwnedByThisEnv,
		OwnerProject:   status.OwnerProject,
		OwnerEnv:       status.OwnerEnv,
		CreateProvider: status.ShouldCreateProvider(),
		Summary:        status.Summary(),
	}

	// Only write when OIDC is on. Recording a provider decision for a config
	// that creates no role would be describing a resource it never builds.
	current := env.Workload.GithubOIDCCreateProvider == nil || *env.Workload.GithubOIDCCreateProvider
	if env.Workload.EnableGithubOIDC && resp.CreateProvider != current {
		backup, err := writeGithubOIDCCreateProvider(req.Env, resp.CreateProvider)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("could not record the change: %v", err)})
			return
		}
		resp.Changed = true
		resp.Backup = backup
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ---------------------------------------------------------------------------
// Writing the resolution back to <env>.yaml
// ---------------------------------------------------------------------------

// writeGithubOIDCCreateProvider records the resolution in the environment file
// and returns the path of the backup it wrote.
//
// It edits the one line rather than round-tripping through Env, for the reason
// disableCustomDomain gives (app/dns_disable_domain.go): a marshal cycle drops
// every key the Go type does not model and reformats the rest, and this file is
// the one thing in a project that cannot be regenerated.
func writeGithubOIDCCreateProvider(envName string, value bool) (string, error) {
	path, err := resolveEnvFilePath(envName)
	if err != nil {
		return "", err
	}

	original, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("could not read %s: %w", path, err)
	}

	updated, changed := setWorkloadBool(string(original), "github_oidc_create_provider", value)
	if !changed {
		return "", fmt.Errorf("could not find a workload block in %s", path)
	}
	if updated == string(original) {
		return "", nil // already correct; no backup, no write
	}

	backup := fmt.Sprintf("%s.backup_%s", path, time.Now().Format("20060102_150405"))
	if err := os.WriteFile(backup, original, 0o644); err != nil {
		return "", fmt.Errorf("could not back up %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return backup, fmt.Errorf("could not write %s: %w", path, err)
	}
	return backup, nil
}

// setWorkloadBool sets a boolean key inside the top-level `workload` block,
// leaving every other byte of the document alone.
//
// It inserts the key when it is absent, which setDomainEnabledFalse never has
// to do. v28 only writes github_oidc_create_provider into configs that already
// enable OIDC, so a project that turns OIDC on afterwards reaches here with no
// key to flip — as does any hand-written config, which no migration ever sees.
//
// Scoping to the block matters as much as it does for domain.enabled: these key
// names are not unique across the document.
//
// Reports false only when there is no workload block to edit.
func setWorkloadBool(doc, key string, value bool) (string, bool) {
	lines := strings.Split(doc, "\n")

	inWorkload := false
	blockIndent := ""
	lastBlockLine := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// A top-level key is unindented. Reaching one ends the workload block.
		if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if inWorkload {
				break // the block ended at the previous remembered line
			}
			inWorkload = trimmed == "workload:"
			continue
		}

		if !inWorkload {
			continue
		}

		// Blank lines and comments belong to the block but are not the place to
		// insert after, so they do not move lastBlockLine.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		lastBlockLine = i
		if blockIndent == "" {
			blockIndent = line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		}

		if k, _, found := strings.Cut(trimmed, ":"); found && strings.TrimSpace(k) == key {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = fmt.Sprintf("%s%s: %t", indent, key, value)
			return strings.Join(lines, "\n"), true
		}
	}

	if lastBlockLine < 0 {
		return doc, false // no workload block, or an empty one
	}

	// Not present: insert after the block's last real line.
	inserted := append([]string{}, lines[:lastBlockLine+1]...)
	inserted = append(inserted, fmt.Sprintf("%s%s: %t", blockIndent, key, value))
	inserted = append(inserted, lines[lastBlockLine+1:]...)
	return strings.Join(inserted, "\n"), true
}
