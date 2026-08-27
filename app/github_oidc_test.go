package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// fakeIAM answers GetOpenIDConnectProvider from a script rather than AWS.
type fakeIAM struct {
	out *iam.GetOpenIDConnectProviderOutput
	err error

	gotARN string
}

func (f *fakeIAM) GetOpenIDConnectProvider(ctx context.Context, in *iam.GetOpenIDConnectProviderInput, _ ...func(*iam.Options)) (*iam.GetOpenIDConnectProviderOutput, error) {
	if in.OpenIDConnectProviderArn != nil {
		f.gotARN = *in.OpenIDConnectProviderArn
	}
	return f.out, f.err
}

func providerFound(tags ...iamtypes.Tag) *fakeIAM {
	return &fakeIAM{out: &iam.GetOpenIDConnectProviderOutput{Tags: tags}}
}

func providerAbsent() *fakeIAM {
	return &fakeIAM{err: &iamtypes.NoSuchEntityException{}}
}

func tag(k, v string) iamtypes.Tag {
	return iamtypes.Tag{Key: aws.String(k), Value: aws.String(v)}
}

// stateOwning builds a lookup whose state manages the given resource types.
func stateOwning(types ...string) remoteStateLookup {
	managed := map[string]bool{}
	for _, t := range types {
		managed[t] = true
	}
	return func(context.Context, Env) (remoteStateSummary, error) {
		return remoteStateSummary{ManagedTypes: managed, ManagedCount: len(managed)}, nil
	}
}

func stateFailing(err error) remoteStateLookup {
	return func(context.Context, Env) (remoteStateSummary, error) {
		return remoteStateSummary{}, err
	}
}

// oidcTestEnv is named apart from state_reconnect_test.go's testEnv, which
// builds a different shape for a different subject.
func oidcTestEnv() Env {
	return Env{AccountID: "285253872242", Region: "ap-southeast-2"}
}

// The four rows of the ownership table. The first is the one that matters: it
// is the case a naive "does it exist in AWS" check inverts, and inverting it
// destroys the provider every project in the account federates against.
func TestResolveGithubOIDCStatus_OwnershipTable(t *testing.T) {
	tests := []struct {
		name       string
		iamClient  *fakeIAM
		lookup     remoteStateLookup
		wantExists bool
		wantOwned  bool
		wantCreate bool
	}{
		{
			name:       "owns it and it exists: keeps creating it",
			iamClient:  providerFound(),
			lookup:     stateOwning(githubOIDCResourceType),
			wantExists: true,
			wantOwned:  true,
			wantCreate: true,
		},
		{
			name:       "exists but another project owns it: federates",
			iamClient:  providerFound(tag("Project", "circl")),
			lookup:     stateOwning("aws_ecs_cluster"),
			wantExists: true,
			wantOwned:  false,
			wantCreate: false,
		},
		{
			name:       "does not exist: first project creates it",
			iamClient:  providerAbsent(),
			lookup:     stateOwning("aws_ecs_cluster"),
			wantExists: false,
			wantOwned:  false,
			wantCreate: true,
		},
		{
			name:       "state owns it but it is gone: recreates it",
			iamClient:  providerAbsent(),
			lookup:     stateOwning(githubOIDCResourceType),
			wantExists: false,
			wantOwned:  false,
			wantCreate: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveGithubOIDCStatus(context.Background(), oidcTestEnv(), tc.iamClient, tc.lookup)
			if err != nil {
				t.Fatalf("resolveGithubOIDCStatus: %v", err)
			}
			if got.Exists != tc.wantExists {
				t.Errorf("Exists = %v, want %v", got.Exists, tc.wantExists)
			}
			if got.OwnedByThisEnv != tc.wantOwned {
				t.Errorf("OwnedByThisEnv = %v, want %v", got.OwnedByThisEnv, tc.wantOwned)
			}
			if got.ShouldCreateProvider() != tc.wantCreate {
				t.Errorf("ShouldCreateProvider() = %v, want %v", got.ShouldCreateProvider(), tc.wantCreate)
			}
		})
	}
}

func TestResolveGithubOIDCStatus_AddressesTheDerivedARN(t *testing.T) {
	client := providerFound()

	if _, err := resolveGithubOIDCStatus(context.Background(), oidcTestEnv(), client, stateOwning()); err != nil {
		t.Fatalf("resolveGithubOIDCStatus: %v", err)
	}

	want := "arn:aws:iam::285253872242:oidc-provider/token.actions.githubusercontent.com"
	if client.gotARN != want {
		t.Errorf("read ARN %q, want %q", client.gotARN, want)
	}
}

func TestResolveGithubOIDCStatus_NeverDeployedEnvOwnsNothing(t *testing.T) {
	// An absent state is a fact about a project that has never deployed, not a
	// failed read. It must resolve, not error.
	got, err := resolveGithubOIDCStatus(context.Background(), oidcTestEnv(),
		providerFound(tag("Project", "circl")), stateFailing(errRemoteStateAbsent))
	if err != nil {
		t.Fatalf("resolveGithubOIDCStatus: %v", err)
	}
	if got.OwnedByThisEnv {
		t.Error("OwnedByThisEnv = true for an environment with no state at all")
	}
	if got.ShouldCreateProvider() {
		t.Error("ShouldCreateProvider() = true, want false — the provider exists and this env does not own it")
	}
}

func TestResolveGithubOIDCStatus_UnreadableStateIsNotOwnership(t *testing.T) {
	// AccessDenied on the state bucket says nothing about ownership. Reporting
	// "not owned" here would tell the owning project to stop creating the
	// provider, and the next apply would destroy it.
	got, err := resolveGithubOIDCStatus(context.Background(), oidcTestEnv(),
		providerFound(), stateFailing(errors.New("AccessDenied")))
	if err == nil {
		t.Fatal("resolveGithubOIDCStatus returned no error for an unreadable state")
	}
	if !got.StateUnknown {
		t.Error("StateUnknown = false; callers cannot tell this answer is a guess")
	}
}

func TestResolveGithubOIDCStatus_DeniedIAMReadIsNotAbsence(t *testing.T) {
	// Only NoSuchEntity means "no provider". A refusal to answer must not be
	// read as absence, which would tell a second project to create a duplicate.
	_, err := resolveGithubOIDCStatus(context.Background(), oidcTestEnv(),
		&fakeIAM{err: errors.New("AccessDenied")}, stateOwning())
	if err == nil {
		t.Fatal("resolveGithubOIDCStatus treated a denied IAM read as a definite answer")
	}
}

func TestResolveGithubOIDCStatus_TagsOnlyNameTheOwner(t *testing.T) {
	// A provider created outside meroku carries no tags. It must still resolve.
	got, err := resolveGithubOIDCStatus(context.Background(), oidcTestEnv(),
		providerFound(), stateOwning("aws_ecs_cluster"))
	if err != nil {
		t.Fatalf("resolveGithubOIDCStatus: %v", err)
	}
	if got.OwnerProject != "" || got.OwnerEnv != "" {
		t.Errorf("owner = %q/%q, want empty for an untagged provider", got.OwnerProject, got.OwnerEnv)
	}
	if got.ShouldCreateProvider() {
		t.Error("ShouldCreateProvider() = true; missing tags must not change the decision")
	}
	if !strings.Contains(got.Summary(), "already exists") {
		t.Errorf("Summary() = %q, want it to still describe the provider", got.Summary())
	}
}

func TestResolveGithubOIDCStatus_RequiresAccountID(t *testing.T) {
	env := oidcTestEnv()
	env.AccountID = ""

	if _, err := resolveGithubOIDCStatus(context.Background(), env, providerFound(), stateOwning()); err == nil {
		t.Fatal("resolveGithubOIDCStatus accepted a config with no account_id")
	}
}

// ---------------------------------------------------------------------------
// setWorkloadBool
// ---------------------------------------------------------------------------

func TestSetWorkloadBool_FlipsAnExistingKey(t *testing.T) {
	doc := strings.Join([]string{
		"project: demo",
		"workload:",
		"  enable_github_oidc: true",
		"  github_oidc_create_provider: true",
		"  backend_image_port: 8080",
		"",
	}, "\n")

	got, ok := setWorkloadBool(doc, "github_oidc_create_provider", false)
	if !ok {
		t.Fatal("setWorkloadBool reported no workload block")
	}
	if !strings.Contains(got, "  github_oidc_create_provider: false") {
		t.Errorf("key was not flipped:\n%s", got)
	}
	if strings.Contains(got, "github_oidc_create_provider: true") {
		t.Errorf("the old value survived:\n%s", got)
	}
	if !strings.Contains(got, "  backend_image_port: 8080") {
		t.Errorf("an unrelated line was disturbed:\n%s", got)
	}
}

func TestSetWorkloadBool_InsertsWhenAbsent(t *testing.T) {
	// v28 leaves the key out of configs with OIDC disabled, so a project that
	// enables it later arrives here with nothing to flip.
	doc := strings.Join([]string{
		"project: demo",
		"workload:",
		"  enable_github_oidc: true",
		"postgres:",
		"  enabled: true",
		"",
	}, "\n")

	got, ok := setWorkloadBool(doc, "github_oidc_create_provider", false)
	if !ok {
		t.Fatal("setWorkloadBool reported no workload block")
	}
	if !strings.Contains(got, "  github_oidc_create_provider: false") {
		t.Errorf("key was not inserted:\n%s", got)
	}

	// It has to land inside workload, not after the next top-level block.
	lines := strings.Split(got, "\n")
	var keyAt, postgresAt int
	for i, l := range lines {
		if strings.Contains(l, "github_oidc_create_provider") {
			keyAt = i
		}
		if strings.TrimSpace(l) == "postgres:" {
			postgresAt = i
		}
	}
	if keyAt > postgresAt {
		t.Errorf("key was inserted after the workload block ended:\n%s", got)
	}
}

func TestSetWorkloadBool_LeavesOtherBlocksAlone(t *testing.T) {
	// The same key name under another block must not be touched. This is the
	// bug setDomainEnabledFalse documents for domain.enabled.
	doc := strings.Join([]string{
		"cognito:",
		"  github_oidc_create_provider: true",
		"workload:",
		"  enable_github_oidc: true",
		"  github_oidc_create_provider: true",
		"",
	}, "\n")

	got, _ := setWorkloadBool(doc, "github_oidc_create_provider", false)

	lines := strings.Split(got, "\n")
	if strings.TrimSpace(lines[1]) != "github_oidc_create_provider: true" {
		t.Errorf("the cognito block was edited:\n%s", got)
	}
	if strings.TrimSpace(lines[4]) != "github_oidc_create_provider: false" {
		t.Errorf("the workload block was not edited:\n%s", got)
	}
}

func TestSetWorkloadBool_NoWorkloadBlock(t *testing.T) {
	doc := "project: demo\npostgres:\n  enabled: true\n"

	if _, ok := setWorkloadBool(doc, "github_oidc_create_provider", false); ok {
		t.Error("setWorkloadBool claimed to edit a document with no workload block")
	}
}

func TestSetWorkloadBool_AlreadyCorrectIsUnchanged(t *testing.T) {
	// writeGithubOIDCCreateProvider skips the backup and the write on this, so
	// a re-run must produce a byte-identical document.
	doc := "workload:\n  github_oidc_create_provider: false\n"

	got, ok := setWorkloadBool(doc, "github_oidc_create_provider", false)
	if !ok {
		t.Fatal("setWorkloadBool reported no workload block")
	}
	if got != doc {
		t.Errorf("document changed when the value was already correct:\n%q", got)
	}
}

func TestSetWorkloadBool_PreservesIndentStyle(t *testing.T) {
	doc := "workload:\n    enable_github_oidc: true\n"

	got, ok := setWorkloadBool(doc, "github_oidc_create_provider", false)
	if !ok {
		t.Fatal("setWorkloadBool reported no workload block")
	}
	if !strings.Contains(got, "\n    github_oidc_create_provider: false") {
		t.Errorf("insert did not match the block's four-space indent:\n%q", got)
	}
}
