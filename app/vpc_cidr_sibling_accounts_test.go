package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func cidrQA2Env(t *testing.T, project, declaredEnv, accountID, profile, region, cidr string, useDefaultVPC bool) Env {
	t.Helper()
	return cidrQAEnv(t, map[string]interface{}{
		"project":         project,
		"env":             declaredEnv,
		"account_id":      accountID,
		"aws_profile":     profile,
		"region":          region,
		"vpc_cidr":        cidr,
		"use_default_vpc": useDefaultVPC,
	})
}

func cidrQA2Sibling(t *testing.T, name, project, declaredEnv, accountID, profile, region string) vpcCIDRSiblingAccount {
	t.Helper()
	return vpcCIDRSiblingAccount{
		Name: name,
		Env:  cidrQA2Env(t, project, declaredEnv, accountID, profile, region, "", false),
	}
}

func cidrQA2Deps(uses []vpcCIDRUse, siblings []vpcCIDRSiblingAccount) vpcCIDRDeps {
	deps := cidrQADeps(uses)
	deps.siblingAccounts = func(_, _ string) ([]vpcCIDRSiblingAccount, error) {
		return append([]vpcCIDRSiblingAccount(nil), siblings...), nil
	}
	return deps
}

// cidrQA2SetFailingState assigns a synthetic failing function to readers.State
// using reflect so tests remain decoupled from the internal remoteStateLookup
// signature while still exercising §B's StateError path end-to-end.
func cidrQA2SetFailingState(readers *vpcCIDRAWS, stateErr error) bool {
	v := reflect.ValueOf(readers).Elem().FieldByName("State")
	if !v.IsValid() || !v.CanSet() || v.Kind() != reflect.Func {
		return false
	}
	ft := v.Type()
	if ft.NumOut() == 0 {
		return false
	}
	errIdx := ft.NumOut() - 1
	if !ft.Out(errIdx).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return false
	}
	fn := reflect.MakeFunc(ft, func(_ []reflect.Value) []reflect.Value {
		outs := make([]reflect.Value, ft.NumOut())
		for i := 0; i < errIdx; i++ {
			outs[i] = reflect.Zero(ft.Out(i))
		}
		outs[errIdx] = reflect.ValueOf(stateErr)
		return outs
	})
	v.Set(fn)
	return true
}

// TEST-40; REQ-A1 (Addendum 2 §A).
func TestVPCCIDRInputIncomplete_TrailingSlashAndSpaces(t *testing.T) {
	siblings := []vpcCIDRUse{
		siblingVPCCIDRUse("staging", "staging.yaml", netip.MustParsePrefix("10.42.0.0/16")),
	}
	tests := []struct {
		name       string
		input      string
		incomplete bool
		valid      bool
		code       string
	}{
		{"empty string", "", true, false, ""},
		{"only spaces", "   ", true, false, ""},
		{"no slash four octets", "10.77.0.0", true, false, ""},
		{"trailing slash", "10.77.0.0/", true, false, ""},
		{"trailing slash with spaces after", "10.77.0.0/   ", true, false, ""},
		{"surrounding spaces and trailing slash", "  10.77.0.0/ \t ", true, false, ""},
		{"three octets with trailing slash", "10.77.0/", true, false, ""},
		{"three octets with mask", "10.77.0/16", true, false, ""},
		{"empty fourth octet before slash", "10.77.0./16", true, false, ""},
		{"empty first octet before slash", ".77.0.0/16", true, false, ""},
		{"empty middle octet before slash", "10..0.0/16", true, false, ""},
		{"non-digit part before slash is a typo, not unfinished", "10.77.x.0/16", false, false, vpcCIDRCodeInvalidFormat},
		{"non-numeric mask after slash is complete", "10.77.0.0/x", false, false, vpcCIDRCodeInvalidFormat},
		{"too-small mask after slash is complete", "10.77.0.0/20", false, false, vpcCIDRCodeSize},
		{"valid complete range", "10.77.0.0/16", false, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := vpcCIDRInputIncomplete(tt.input); got != tt.incomplete {
				t.Errorf("vpcCIDRInputIncomplete(%q) = %v, want %v", tt.input, got, tt.incomplete)
			}
			res := inspectVPCCIDRCandidate(tt.input, siblings)
			if res == nil {
				t.Fatalf("inspectVPCCIDRCandidate(%q) returned nil", tt.input)
			}
			if res.Incomplete != tt.incomplete || res.Valid != tt.valid {
				t.Errorf("inspectVPCCIDRCandidate(%q) = {Incomplete:%v, Valid:%v}, want {Incomplete:%v, Valid:%v}",
					tt.input, res.Incomplete, res.Valid, tt.incomplete, tt.valid)
			}
			if tt.code == "" {
				if res.Error != nil {
					t.Errorf("inspectVPCCIDRCandidate(%q) unexpected error: %+v", tt.input, res.Error)
				}
			} else if res.Error == nil || res.Error.Code != tt.code {
				t.Errorf("inspectVPCCIDRCandidate(%q) error = %+v, want code %q", tt.input, res.Error, tt.code)
			}
		})
	}
}

// TEST-41; REQ-C1 (Addendum 2 §C).
func TestAWSVPCFromEC2_FiltersAssociationStatesMasksAndDedupes(t *testing.T) {
	input := ec2types.Vpc{
		VpcId:     aws.String("vpc-synthetic-c-states"),
		CidrBlock: aws.String("10.40.5.9/16"), // unmasked primary, always kept and masked
		CidrBlockAssociationSet: []ec2types.VpcCidrBlockAssociation{
			// Duplicate of primary (already masked)
			{
				CidrBlock:      aws.String("10.40.0.0/16"),
				CidrBlockState: &ec2types.VpcCidrBlockState{State: ec2types.VpcCidrBlockStateCodeAssociated},
			},
			// Missing CidrBlockState pointer -> kept and masked
			{
				CidrBlock:      aws.String("10.50.3.1/16"),
				CidrBlockState: nil,
			},
			// Present CidrBlockState struct with empty State value -> kept
			{
				CidrBlock:      aws.String("10.51.0.0/16"),
				CidrBlockState: &ec2types.VpcCidrBlockState{},
			},
			// Associated -> kept
			{
				CidrBlock:      aws.String("10.52.0.0/16"),
				CidrBlockState: &ec2types.VpcCidrBlockState{State: ec2types.VpcCidrBlockStateCodeAssociated},
			},
			// Associating -> kept and masked
			{
				CidrBlock:      aws.String("10.53.128.1/17"),
				CidrBlockState: &ec2types.VpcCidrBlockState{State: ec2types.VpcCidrBlockStateCodeAssociating},
			},
			// Duplicate of 10.52.0.0/16 with unmasked host bits -> deduped after masking
			{
				CidrBlock:      aws.String("10.52.9.9/16"),
				CidrBlockState: &ec2types.VpcCidrBlockState{State: ec2types.VpcCidrBlockStateCodeAssociating},
			},
			// Non-live states -> must all be excluded
			{
				CidrBlock:      aws.String("10.60.0.0/16"),
				CidrBlockState: &ec2types.VpcCidrBlockState{State: ec2types.VpcCidrBlockStateCodeDisassociating},
			},
			{
				CidrBlock:      aws.String("10.61.0.0/16"),
				CidrBlockState: &ec2types.VpcCidrBlockState{State: ec2types.VpcCidrBlockStateCodeDisassociated},
			},
			{
				CidrBlock:      aws.String("10.62.0.0/16"),
				CidrBlockState: &ec2types.VpcCidrBlockState{State: ec2types.VpcCidrBlockStateCodeFailing},
			},
			{
				CidrBlock:      aws.String("10.63.0.0/16"),
				CidrBlockState: &ec2types.VpcCidrBlockState{State: ec2types.VpcCidrBlockStateCodeFailed},
			},
		},
		Tags: []ec2types.Tag{
			{Key: aws.String("Project"), Value: aws.String("qa-project")},
			{Key: aws.String("Environment"), Value: aws.String("dev")},
		},
	}

	got := awsVPCFromEC2(input)
	if got.ID != "vpc-synthetic-c-states" {
		t.Errorf("got.ID = %q, want vpc-synthetic-c-states", got.ID)
	}
	wantList := []string{
		"10.40.0.0/16",
		"10.50.0.0/16",
		"10.51.0.0/16",
		"10.52.0.0/16",
		"10.53.128.0/17",
	}
	if len(got.CIDRs) != len(wantList) {
		t.Fatalf("got.CIDRs = %v (len %d), want %v (len %d) with duplicates and dead states removed",
			got.CIDRs, len(got.CIDRs), wantList, len(wantList))
	}
	seen := map[string]bool{}
	for _, p := range got.CIDRs {
		if p != p.Masked() {
			t.Errorf("prefix %s is not masked", p)
		}
		if seen[p.String()] {
			t.Errorf("duplicate prefix %s in got.CIDRs", p)
		}
		seen[p.String()] = true
	}
	wantSet := map[string]bool{
		"10.40.0.0/16":   true,
		"10.50.0.0/16":   true,
		"10.51.0.0/16":   true,
		"10.52.0.0/16":   true,
		"10.53.128.0/17": true,
	}
	if !reflect.DeepEqual(seen, wantSet) {
		t.Errorf("got CIDR set = %v, want %v", seen, wantSet)
	}
}

// TEST-42; REQ-D1 (Addendum 2 §D).
func TestSiblingVPCCIDRAccounts_ReadsIdentityAndIncludesDefaultVPCSiblings(t *testing.T) {
	dir := t.TempDir()

	// Self environment (must be excluded).
	cidrQAWriteEnv(t, dir, "dev.yaml", "yaml-dev", "000000000000",
		"aws_profile: fixture-profile-self\nuse_default_vpc: false\nvpc_cidr: 10.10.0.0/16\n")

	// Custom-VPC sibling with declared env different from file stem.
	cidrQAWriteEnv(t, dir, "zeta.yaml", "declared-zeta", "111111111111",
		"aws_profile: fixture-profile-zeta\nuse_default_vpc: false\nvpc_cidr: 10.20.0.0/16\n")

	// Explicit default-VPC sibling: excluded by siblingVPCCIDRUses, MUST be included by siblingVPCCIDRAccounts.
	cidrQAWriteEnv(t, dir, "defaultvpc.yaml", "declared-default", "222222222222",
		"aws_profile: fixture-profile-default\nuse_default_vpc: true\n")

	// Duplicate stem (.yml and .yaml): must be deduped to a single entry.
	for _, ext := range []string{".yml", ".yaml"} {
		cidrQAWriteEnv(t, dir, "alpha"+ext, "declared-alpha", "111111111111",
			"aws_profile: fixture-profile-alpha\nuse_default_vpc: false\nvpc_cidr: 10.30.0.0/16\n")
	}

	// Raw legacy (< v6) default-VPC sibling and non-environment files.
	rawFiles := map[string]string{
		"legacy.yaml": "# hand-written comment must survive\n" +
			"schema_version: 5\n" +
			"project: qa-legacy-proj\n" +
			"env: declared-legacy\n" +
			"account_id: \"222222222222\"\n" +
			"aws_profile: fixture-profile-legacy\n" +
			"region: eu-west-1\n" +
			"unmodeled_custom_key: preserve-me\n",
		"dns.yaml":      "root_domain: example.invalid\nroot_zone_id: ZSYNTHETIC\n",
		"taskfile.yaml": "version: '3'\ntasks: {}\n",
	}
	for name, content := range rawFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	before := cidrQASnapshot(t, dir)
	got, err := siblingVPCCIDRAccounts(dir, "dev")
	if err != nil {
		t.Fatalf("siblingVPCCIDRAccounts: %v", err)
	}

	wantNames := []string{"alpha", "defaultvpc", "legacy", "zeta"}
	var gotNames []string
	for _, s := range got {
		gotNames = append(gotNames, s.Name)
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("sibling account names = %v, want %v", gotNames, wantNames)
	}

	byName := map[string]Env{}
	for _, s := range got {
		byName[s.Name] = s.Env
	}

	if e := byName["defaultvpc"]; e.Project != "qa-project" || e.Env != "declared-default" ||
		e.AccountID != "222222222222" || e.AWSProfile != "fixture-profile-default" || e.Region != "us-east-1" {
		t.Errorf("defaultvpc Env = %+v, want populated Project/Env/AccountID/AWSProfile/Region", e)
	}
	if e := byName["legacy"]; e.Project != "qa-legacy-proj" || e.Env != "declared-legacy" ||
		e.AccountID != "222222222222" || e.AWSProfile != "fixture-profile-legacy" || e.Region != "eu-west-1" {
		t.Errorf("legacy Env = %+v, want populated Project/Env/AccountID/AWSProfile/Region", e)
	}
	if e := byName["zeta"]; e.Project != "qa-project" || e.Env != "declared-zeta" ||
		e.AccountID != "111111111111" || e.AWSProfile != "fixture-profile-zeta" || e.Region != "us-east-1" {
		t.Errorf("zeta Env = %+v, want populated Project/Env/AccountID/AWSProfile/Region", e)
	}

	cidrQAAssertUnchanged(t, dir, before)
}

// TEST-43; REQ-D2 (Addendum 2 §D).
func TestAWSAccountKeys_AndSiblingAccountLabels(t *testing.T) {
	t.Run("awsAccountKeys combinations", func(t *testing.T) {
		both := cidrQA2Env(t, "qa-project", "dev", "000000000000", "fixture-profile-a", "us-east-1", "", false)
		gotBoth := awsAccountKeys(both)
		wantBoth := map[string]bool{
			"account:000000000000|us-east-1":      true,
			"profile:fixture-profile-a|us-east-1": true,
		}
		gotSet := map[string]bool{}
		for _, k := range gotBoth {
			gotSet[k] = true
		}
		if len(gotBoth) != 2 || !reflect.DeepEqual(gotSet, wantBoth) {
			t.Errorf("awsAccountKeys(both) = %v, want %v", gotBoth, wantBoth)
		}

		accountOnly := cidrQA2Env(t, "qa-project", "dev", "111111111111", "", "eu-west-1", "", false)
		if got := awsAccountKeys(accountOnly); !reflect.DeepEqual(got, []string{"account:111111111111|eu-west-1"}) {
			t.Errorf("awsAccountKeys(accountOnly) = %v, want [account:111111111111|eu-west-1]", got)
		}

		profileOnly := cidrQA2Env(t, "qa-project", "dev", "", "fixture-profile-b", "ap-southeast-2", "", false)
		if got := awsAccountKeys(profileOnly); !reflect.DeepEqual(got, []string{"profile:fixture-profile-b|ap-southeast-2"}) {
			t.Errorf("awsAccountKeys(profileOnly) = %v, want [profile:fixture-profile-b|ap-southeast-2]", got)
		}

		neither := cidrQA2Env(t, "qa-project", "dev", "", "", "us-east-1", "", false)
		if got := awsAccountKeys(neither); len(got) != 0 {
			t.Errorf("awsAccountKeys(neither) = %v, want empty", got)
		}
	})

	t.Run("siblingAccountVPCCIDRUse and unreachable notes", func(t *testing.T) {
		p := netip.MustParsePrefix("10.88.0.0/16")
		u := siblingAccountVPCCIDRUse("staging", p)
		const wantLabel = "another network in staging's AWS account"
		if u.Source != wantLabel || u.Name != wantLabel || u.Prefix != p {
			t.Errorf("siblingAccountVPCCIDRUse = %+v, want Source=Name=%q Prefix=%s", u, wantLabel, p)
		}

		warnings := vpcCIDROverlapWarnings(netip.MustParsePrefix("10.88.0.0/16"), []vpcCIDRUse{u})
		if len(warnings) != 1 || warnings[0].Source != wantLabel {
			t.Fatalf("warnings = %+v, want 1 warning with Source %q", warnings, wantLabel)
		}
		cidrQAAssertPlainHint(t, warnings[0].Hint)

		const wantOwnNote = "Couldn't reach this environment's AWS account — these avoid your other environments and their AWS accounts only"
		if vpcCIDROwnAccountUnreachableNote != wantOwnNote {
			t.Errorf("vpcCIDROwnAccountUnreachableNote = %q, want %q", vpcCIDROwnAccountUnreachableNote, wantOwnNote)
		}

		gotSingle := vpcCIDRSiblingAccountUnreachableNote([]string{"staging"})
		wantSingle := "Couldn't reach the AWS account for staging — networks there were not checked"
		if gotSingle != wantSingle {
			t.Errorf("vpcCIDRSiblingAccountUnreachableNote([staging]) = %q, want %q", gotSingle, wantSingle)
		}

		gotMulti := vpcCIDRSiblingAccountUnreachableNote([]string{"alpha", "beta"})
		wantMulti := "Couldn't reach the AWS account for alpha, beta — networks there were not checked"
		if gotMulti != wantMulti {
			t.Errorf("vpcCIDRSiblingAccountUnreachableNote([alpha, beta]) = %q, want %q", gotMulti, wantMulti)
		}
	})
}

// TEST-44; REQ-D3 (Addendum 2 §D Rules 3, 4).
func TestInspectVPCCIDR_SiblingAccountVPCsWarnAndAreAvoidedBySuggestions(t *testing.T) {
	env := cidrQA2Env(t, "qa-project", "dev", "000000000000", "fixture-profile-dev", "us-east-1", "10.77.0.0/16", false)
	siblings := []vpcCIDRSiblingAccount{
		cidrQA2Sibling(t, "staging", "qa-project", "staging", "111111111111", "fixture-profile-staging", "us-east-1"),
	}
	deps := cidrQA2Deps(nil, siblings)
	deps.aws = func(ctx context.Context, target Env) (vpcCIDRAWS, error) {
		switch target.AWSProfile {
		case "fixture-profile-dev":
			return cidrQAFakeAWS([]awsVPC{
				{
					ID:    "vpc-synthetic-own-other",
					CIDRs: []netip.Prefix{netip.MustParsePrefix("10.40.0.0/16")},
					Tags:  map[string]string{"Name": "synthetic-own-account-vpc"},
				},
			}, nil)(ctx, target)
		case "fixture-profile-staging":
			return cidrQAFakeAWS([]awsVPC{
				{
					ID:    "vpc-synthetic-staging-other",
					CIDRs: []netip.Prefix{netip.MustParsePrefix("10.77.0.0/16"), netip.MustParsePrefix("10.88.0.0/16")},
					Tags:  map[string]string{"Name": "synthetic-staging-account-vpc"},
				},
			}, nil)(ctx, target)
		default:
			return vpcCIDRAWS{}, errors.New("unexpected profile: " + target.AWSProfile)
		}
	}

	report := inspectVPCCIDR(context.Background(), deps, env,
		vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 10})

	if !report.AWSChecked || report.AWSNote != "" || report.Error != nil {
		t.Fatalf("unexpected report status: %+v", report)
	}
	const wantSource = "another network in staging's AWS account"
	if len(report.Warnings) != 1 || report.Warnings[0].Source != wantSource {
		t.Fatalf("report.Warnings = %+v, want 1 warning from %q", report.Warnings, wantSource)
	}
	cidrQAAssertPlainHint(t, report.Warnings[0].Hint)
	for _, secret := range []string{"vpc-synthetic", "synthetic-staging-account-vpc"} {
		if strings.Contains(report.Warnings[0].Message+report.Warnings[0].Hint+report.Warnings[0].Source, secret) {
			t.Errorf("sibling AWS warning leaked private VPC identifier %q: %+v", secret, report.Warnings[0])
		}
	}

	expectedAvoid := []vpcCIDRUse{
		{Source: "own account other", Prefix: netip.MustParsePrefix("10.40.0.0/16")},
		{Source: wantSource, Prefix: netip.MustParsePrefix("10.77.0.0/16")},
		{Source: wantSource, Prefix: netip.MustParsePrefix("10.88.0.0/16")},
	}
	if len(report.Suggestions) != 10 {
		t.Errorf("len(report.Suggestions) = %d, want 10", len(report.Suggestions))
	}
	cidrQAAssertSuggestions(t, report.Suggestions, expectedAvoid)
}

// TEST-45; REQ-D4 (Addendum 2 §D Rule 4).
func TestInspectVPCCIDR_ExcludesOwnAndAllSiblingOwnVPCsFromSiblingAccounts(t *testing.T) {
	// Configure dev with 10.77.0.0/16. In the sibling's AWS account, place:
	// 1. dev's own VPC (by Project/Environment tags) with 10.77.0.0/16
	// 2. staging's own VPC (by Name "<project>-vpc-<env>") with 10.77.0.0/16
	// 3. prod's own VPC (by Project/Environment tags using declared env "production") with 10.77.0.0/16
	// 4. an unrelated VPC in staging's account with 10.123.0.0/16
	// If any of (1)-(3) were NOT excluded, dev's 10.77.0.0/16 would get a spurious
	// "another network in staging's AWS account" overlap warning!
	env := cidrQA2Env(t, "qa-project", "yaml-dev", "000000000000", "fixture-profile-dev", "us-east-1", "10.77.0.0/16", false)
	siblings := []vpcCIDRSiblingAccount{
		cidrQA2Sibling(t, "prod", "qa-project", "production", "222222222222", "fixture-profile-prod", "us-east-1"),
		cidrQA2Sibling(t, "staging", "qa-project", "staging", "111111111111", "fixture-profile-staging", "us-east-1"),
	}
	deps := cidrQA2Deps(nil, siblings)
	deps.aws = func(ctx context.Context, target Env) (vpcCIDRAWS, error) {
		switch target.AWSProfile {
		case "fixture-profile-dev":
			return cidrQAFakeAWS(nil, nil)(ctx, target)
		case "fixture-profile-staging":
			return cidrQAFakeAWS([]awsVPC{
				{
					ID:    "vpc-synthetic-self-in-sibling",
					CIDRs: []netip.Prefix{netip.MustParsePrefix("10.77.0.0/16")},
					Tags:  map[string]string{"Project": "qa-project", "Environment": "yaml-dev"},
				},
				{
					ID:    "vpc-synthetic-staging-own-by-name",
					CIDRs: []netip.Prefix{netip.MustParsePrefix("10.77.0.0/16")},
					Tags:  map[string]string{"Name": "qa-project-vpc-staging"},
				},
				{
					ID:    "vpc-synthetic-prod-own-by-tags",
					CIDRs: []netip.Prefix{netip.MustParsePrefix("10.77.0.0/16")},
					Tags:  map[string]string{"Project": "qa-project", "Environment": "production"},
				},
				{
					ID:    "vpc-synthetic-unrelated-in-staging",
					CIDRs: []netip.Prefix{netip.MustParsePrefix("10.123.0.0/16")},
					Tags:  map[string]string{"Project": "other-project", "Environment": "staging"},
				},
			}, nil)(ctx, target)
		case "fixture-profile-prod":
			return cidrQAFakeAWS(nil, nil)(ctx, target)
		default:
			return vpcCIDRAWS{}, errors.New("unexpected profile: " + target.AWSProfile)
		}
	}

	report := inspectVPCCIDR(context.Background(), deps, env,
		vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 10})

	if len(report.Warnings) != 0 {
		t.Errorf("own/sibling-own VPCs in sibling account were not excluded; got warnings: %+v", report.Warnings)
	}
	// Now verify the unrelated 10.123.0.0/16 in staging's account WAS kept:
	envClash := cidrQA2Env(t, "qa-project", "yaml-dev", "000000000000", "fixture-profile-dev", "us-east-1", "10.123.0.0/16", false)
	reportClash := inspectVPCCIDR(context.Background(), deps, envClash,
		vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 10})
	if len(reportClash.Warnings) != 1 || reportClash.Warnings[0].Source != "another network in staging's AWS account" {
		t.Errorf("unrelated VPC in sibling account was not reported; warnings = %+v", reportClash.Warnings)
	}
}

// TEST-46; REQ-D5 (Addendum 2 §D Rule 1).
func TestInspectVPCCIDR_SkipsSiblingsMissingProfileOrRegion(t *testing.T) {
	t.Setenv("AWS_PROFILE", "ambient-profile-must-never-be-used")

	env := cidrQA2Env(t, "qa-project", "dev", "000000000000", "fixture-profile-dev", "us-east-1", "10.77.0.0/16", false)
	siblings := []vpcCIDRSiblingAccount{
		cidrQA2Sibling(t, "no-profile", "qa-project", "no-profile", "111111111111", "", "us-east-1"),
		cidrQA2Sibling(t, "space-profile", "qa-project", "space-profile", "111111111111", "   ", "us-east-1"),
		cidrQA2Sibling(t, "no-region", "qa-project", "no-region", "222222222222", "fixture-profile-noregion", ""),
		cidrQA2Sibling(t, "space-region", "qa-project", "space-region", "222222222222", "fixture-profile-spaceregion", "  "),
		cidrQA2Sibling(t, "valid-sibling", "qa-project", "valid-sibling", "111111111111", "fixture-profile-valid", "us-east-1"),
	}

	var calledProfiles []string
	deps := cidrQA2Deps(nil, siblings)
	deps.aws = func(ctx context.Context, target Env) (vpcCIDRAWS, error) {
		calledProfiles = append(calledProfiles, target.AWSProfile)
		if strings.TrimSpace(target.AWSProfile) == "" || strings.TrimSpace(target.Region) == "" {
			t.Errorf("deps.aws called for sibling missing profile or region: %+v", target)
		}
		return cidrQAFakeAWS(nil, nil)(ctx, target)
	}

	report := inspectVPCCIDR(context.Background(), deps, env,
		vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 5})

	wantCalled := []string{"fixture-profile-dev", "fixture-profile-valid"}
	if !reflect.DeepEqual(calledProfiles, wantCalled) {
		t.Errorf("called profiles = %v, want %v (siblings missing profile/region must be skipped)", calledProfiles, wantCalled)
	}
	if !report.AWSChecked || report.AWSNote != "" || report.AWSError != "" {
		t.Errorf("skipped incomplete siblings should not trigger AWSNote or AWSError: %+v", report)
	}
}

// TEST-47; REQ-D6 (Addendum 2 §D Rule 2 & Rule 5).
func TestInspectVPCCIDR_DeduplicatesSharedAccountsAndSkipsOwnOnlyWhenListed(t *testing.T) {
	t.Run("own account listed succeeds and shared accounts are listed once", func(t *testing.T) {
		env := cidrQA2Env(t, "qa-project", "dev", "000000000000", "fixture-profile-a", "us-east-1", "10.77.0.0/16", false)
		siblings := []vpcCIDRSiblingAccount{
			// Shares account_id + region with dev (own account) -> must be skipped because dev succeeded.
			cidrQA2Sibling(t, "same-as-own-by-account", "qa-project", "s1", "000000000000", "fixture-profile-b", "us-east-1"),
			// Shares profile + region with dev (own account) -> must be skipped.
			cidrQA2Sibling(t, "same-as-own-by-profile", "qa-project", "s2", "", "fixture-profile-a", "us-east-1"),
			// First sibling in account 111111111111|us-east-1 with fixture-profile-c -> MUST be read once.
			cidrQA2Sibling(t, "sib-first", "qa-project", "s3", "111111111111", "fixture-profile-c", "us-east-1"),
			// Shares profile+region with sib-first (even without account_id) -> must be skipped.
			cidrQA2Sibling(t, "sib-dup-profile", "qa-project", "s4", "", "fixture-profile-c", "us-east-1"),
			// Shares account_id+region with sib-first (even with different profile) -> must be skipped.
			cidrQA2Sibling(t, "sib-dup-account", "qa-project", "s5", "111111111111", "fixture-profile-d", "us-east-1"),
			// Same account_id 111111111111 but DIFFERENT region eu-west-1 -> distinct key, MUST be read.
			cidrQA2Sibling(t, "sib-other-region", "qa-project", "s6", "111111111111", "fixture-profile-c", "eu-west-1"),
		}

		var calls []string
		deps := cidrQA2Deps(nil, siblings)
		deps.aws = func(ctx context.Context, target Env) (vpcCIDRAWS, error) {
			calls = append(calls, target.AWSProfile+"@"+target.Region)
			return cidrQAFakeAWS(nil, nil)(ctx, target)
		}

		report := inspectVPCCIDR(context.Background(), deps, env,
			vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 5})

		wantCalls := []string{
			"fixture-profile-a@us-east-1",
			"fixture-profile-c@us-east-1",
			"fixture-profile-c@eu-west-1",
		}
		if !reflect.DeepEqual(calls, wantCalls) {
			t.Errorf("AWS reader calls = %v, want %v", calls, wantCalls)
		}
		if !report.AWSChecked || report.AWSNote != "" {
			t.Errorf("unexpected AWSChecked/AWSNote: %+v", report)
		}
	})

	t.Run("own account listing fails so sibling sharing account IS read", func(t *testing.T) {
		env := cidrQA2Env(t, "qa-project", "dev", "000000000000", "fixture-profile-broken", "us-east-1", "10.77.0.0/16", false)
		siblings := []vpcCIDRSiblingAccount{
			cidrQA2Sibling(t, "staging", "qa-project", "staging", "000000000000", "fixture-profile-working", "us-east-1"),
			cidrQA2Sibling(t, "prod", "qa-project", "prod", "000000000000", "fixture-profile-working-2", "us-east-1"),
		}

		var calls []string
		deps := cidrQA2Deps(nil, siblings)
		deps.aws = func(ctx context.Context, target Env) (vpcCIDRAWS, error) {
			calls = append(calls, target.AWSProfile)
			if target.AWSProfile == "fixture-profile-broken" {
				return cidrQAFakeAWS(nil, errors.New("synthetic own listing failure"))(ctx, target)
			}
			return cidrQAFakeAWS([]awsVPC{
				{
					ID:    "vpc-synthetic-shared",
					CIDRs: []netip.Prefix{netip.MustParsePrefix("10.77.0.0/16")},
				},
			}, nil)(ctx, target)
		}

		report := inspectVPCCIDR(context.Background(), deps, env,
			vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 5})

		// Own account failed, so staging (sharing 000000000000|us-east-1) is NOT skipped,
		// and once staging succeeds, prod (also sharing 000000000000|us-east-1) IS deduped.
		wantCalls := []string{"fixture-profile-broken", "fixture-profile-working"}
		if !reflect.DeepEqual(calls, wantCalls) {
			t.Errorf("AWS calls when own fails = %v, want %v", calls, wantCalls)
		}
		if report.AWSChecked {
			t.Errorf("AWSChecked = true, want false because own account listing failed (Rule 5)")
		}
		if report.AWSNote != vpcCIDROwnAccountUnreachableNote {
			t.Errorf("AWSNote = %q, want %q", report.AWSNote, vpcCIDROwnAccountUnreachableNote)
		}
		if len(report.Warnings) != 1 || report.Warnings[0].Source != "another network in staging's AWS account" {
			t.Errorf("expected overlap warning from staging's fallback listing, got %+v", report.Warnings)
		}
	})
}

// TEST-48; REQ-D7 (Addendum 2 §D Rules 5, 6, 7 & §B).
func TestInspectVPCCIDR_AWSNoteCombinationsAndDeployedIndependence(t *testing.T) {
	t.Run("all accounts reachable produces empty AWSNote and AWSError", func(t *testing.T) {
		env := cidrQA2Env(t, "qa-project", "dev", "000000000000", "fixture-profile-dev", "us-east-1", "10.77.0.0/16", false)
		siblings := []vpcCIDRSiblingAccount{
			cidrQA2Sibling(t, "alpha", "qa-project", "alpha", "111111111111", "fixture-profile-alpha", "us-east-1"),
		}
		deps := cidrQA2Deps(nil, siblings)
		deps.aws = cidrQAFakeAWS(nil, nil)

		report := inspectVPCCIDR(context.Background(), deps, env,
			vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 5})
		if !report.AWSChecked || report.AWSNote != "" || report.AWSError != "" {
			t.Errorf("report = %+v, want AWSChecked=true, empty AWSNote and AWSError", report)
		}
	})

	t.Run("own account not listed and no sibling account reached", func(t *testing.T) {
		env := cidrQA2Env(t, "qa-project", "dev", "000000000000", "fixture-profile-dev", "us-east-1", "10.77.0.0/16", false)
		deps := cidrQA2Deps(nil, nil)
		deps.aws = func(context.Context, Env) (vpcCIDRAWS, error) {
			return vpcCIDRAWS{}, errors.New("synthetic-own-err")
		}

		report := inspectVPCCIDR(context.Background(), deps, env,
			vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 5})
		if report.AWSChecked {
			t.Error("AWSChecked = true, want false")
		}
		if report.AWSNote != vpcCIDRAWSUnreachableNote {
			t.Errorf("AWSNote = %q, want %q", report.AWSNote, vpcCIDRAWSUnreachableNote)
		}
		if !strings.Contains(report.AWSError, "synthetic-own-err") {
			t.Errorf("AWSError = %q, want synthetic-own-err", report.AWSError)
		}
	})

	t.Run("own account not listed and all sibling accounts also fail", func(t *testing.T) {
		env := cidrQA2Env(t, "qa-project", "dev", "000000000000", "fixture-profile-dev", "us-east-1", "10.77.0.0/16", false)
		siblings := []vpcCIDRSiblingAccount{
			cidrQA2Sibling(t, "alpha", "qa-project", "alpha", "111111111111", "fixture-profile-alpha", "us-east-1"),
			cidrQA2Sibling(t, "beta", "qa-project", "beta", "222222222222", "fixture-profile-beta", "us-east-1"),
		}
		deps := cidrQA2Deps(nil, siblings)
		deps.aws = func(ctx context.Context, target Env) (vpcCIDRAWS, error) {
			if target.AWSProfile == "fixture-profile-alpha" {
				return vpcCIDRAWS{}, errors.New("synthetic-alpha-factory-err")
			}
			return cidrQAFakeAWS(nil, errors.New("synthetic-"+target.AWSProfile+"-list-err"))(ctx, target)
		}

		report := inspectVPCCIDR(context.Background(), deps, env,
			vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 5})
		if report.AWSChecked {
			t.Error("AWSChecked = true, want false")
		}
		wantSibNote := vpcCIDRSiblingAccountUnreachableNote([]string{"alpha", "beta"})
		wantJoined := vpcCIDRAWSUnreachableNote + ". " + wantSibNote
		if report.AWSNote != wantJoined && report.AWSNote != vpcCIDRAWSUnreachableNote {
			t.Errorf("AWSNote = %q, want %q", report.AWSNote, wantJoined)
		}
		for _, diag := range []string{"synthetic-fixture-profile-dev-list-err", "synthetic-alpha-factory-err", "synthetic-fixture-profile-beta-list-err"} {
			if !strings.Contains(report.AWSError, diag) {
				t.Errorf("AWSError = %q, missing diagnostic %q", report.AWSError, diag)
			}
		}
	})

	t.Run("own account not listed, one sibling reached, two siblings failed in order", func(t *testing.T) {
		env := cidrQA2Env(t, "qa-project", "dev", "000000000000", "fixture-profile-dev", "us-east-1", "10.77.0.0/16", false)
		siblings := []vpcCIDRSiblingAccount{
			cidrQA2Sibling(t, "alpha", "qa-project", "alpha", "111111111111", "fixture-profile-alpha", "us-east-1"),
			cidrQA2Sibling(t, "beta", "qa-project", "beta", "222222222222", "fixture-profile-beta", "us-east-1"),
			cidrQA2Sibling(t, "zeta", "qa-project", "zeta", "222222222222", "fixture-profile-zeta", "eu-west-1"),
		}
		deps := cidrQA2Deps(nil, siblings)
		deps.aws = func(ctx context.Context, target Env) (vpcCIDRAWS, error) {
			switch target.AWSProfile {
			case "fixture-profile-dev":
				return vpcCIDRAWS{}, errors.New("synthetic-own-factory-err")
			case "fixture-profile-alpha":
				return cidrQAFakeAWS(nil, nil)(ctx, target)
			case "fixture-profile-beta":
				return vpcCIDRAWS{}, errors.New("synthetic-beta-factory-err")
			case "fixture-profile-zeta":
				return cidrQAFakeAWS(nil, errors.New("synthetic-zeta-list-err"))(ctx, target)
			default:
				return vpcCIDRAWS{}, errors.New("unexpected profile")
			}
		}

		report := inspectVPCCIDR(context.Background(), deps, env,
			vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 5})
		if report.AWSChecked {
			t.Error("AWSChecked = true, want false")
		}
		wantNote := vpcCIDROwnAccountUnreachableNote + ". " + vpcCIDRSiblingAccountUnreachableNote([]string{"beta", "zeta"})
		if report.AWSNote != wantNote {
			t.Errorf("AWSNote = %q, want %q", report.AWSNote, wantNote)
		}
		for _, diag := range []string{"synthetic-own-factory-err", "synthetic-beta-factory-err", "synthetic-zeta-list-err"} {
			if !strings.Contains(report.AWSError, diag) {
				t.Errorf("AWSError = %q, missing %q", report.AWSError, diag)
			}
		}
	})

	t.Run("own account listed with own VPC deployed=yes while sibling accounts fail", func(t *testing.T) {
		env := cidrQA2Env(t, "qa-project", "dev", "000000000000", "fixture-profile-dev", "us-east-1", "10.77.0.0/16", false)
		siblings := []vpcCIDRSiblingAccount{
			cidrQA2Sibling(t, "alpha", "qa-project", "alpha", "111111111111", "fixture-profile-alpha", "us-east-1"),
			cidrQA2Sibling(t, "zeta", "qa-project", "zeta", "222222222222", "fixture-profile-zeta", "us-east-1"),
		}
		deps := cidrQA2Deps(nil, siblings)
		deps.aws = func(ctx context.Context, target Env) (vpcCIDRAWS, error) {
			switch target.AWSProfile {
			case "fixture-profile-dev":
				return cidrQAFakeAWS([]awsVPC{
					{
						ID:    "vpc-synthetic-own-dev",
						CIDRs: []netip.Prefix{netip.MustParsePrefix("10.77.0.0/16")},
						Tags:  map[string]string{"Project": "qa-project", "Environment": "dev"},
					},
				}, nil)(ctx, target)
			case "fixture-profile-alpha":
				return vpcCIDRAWS{}, errors.New("synthetic-alpha-factory-err")
			case "fixture-profile-zeta":
				return cidrQAFakeAWS(nil, errors.New("synthetic-zeta-list-err"))(ctx, target)
			default:
				return vpcCIDRAWS{}, errors.New("unexpected")
			}
		}

		report := inspectVPCCIDR(context.Background(), deps, env,
			vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 5})
		if !report.AWSChecked {
			t.Error("AWSChecked = false, want true when own account succeeded")
		}
		if report.Deployed != vpcCIDRDeployedYes {
			t.Errorf("Deployed = %q, want %q (Rule 7: deployed is unaffected by sibling failures)", report.Deployed, vpcCIDRDeployedYes)
		}
		wantNote := vpcCIDRSiblingAccountUnreachableNote([]string{"alpha", "zeta"})
		if report.AWSNote != wantNote {
			t.Errorf("AWSNote = %q, want %q", report.AWSNote, wantNote)
		}
		for _, diag := range []string{"synthetic-alpha-factory-err", "synthetic-zeta-list-err"} {
			if !strings.Contains(report.AWSError, diag) {
				t.Errorf("AWSError = %q, missing %q", report.AWSError, diag)
			}
			if strings.Contains(report.DeployedReason, diag) || strings.Contains(report.AWSNote, diag) {
				t.Errorf("raw sibling error %q leaked into DeployedReason or AWSNote: %+v", diag, report)
			}
		}
	})
}

// TEST-49; REQ-D8 (Addendum 2 §D Rule 8) & REQ-A1 (§A).
func TestInspectVPCCIDR_CandidateCallsNoOwnOrSiblingAWS(t *testing.T) {
	env := cidrQA2Env(t, "qa-project", "dev", "000000000000", "fixture-profile-dev", "us-east-1", "10.77.0.0/16", false)
	siblings := []vpcCIDRSiblingAccount{
		cidrQA2Sibling(t, "staging", "qa-project", "staging", "111111111111", "fixture-profile-staging", "us-east-1"),
		cidrQA2Sibling(t, "prod", "qa-project", "prod", "222222222222", "fixture-profile-prod", "us-east-1"),
	}

	for _, cand := range []string{"10.77.0.0/", "10.77.0.0/16"} {
		t.Run("candidate="+cand, func(t *testing.T) {
			awsCalls := 0
			deps := cidrQA2Deps(nil, siblings)
			deps.aws = func(context.Context, Env) (vpcCIDRAWS, error) {
				awsCalls++
				return cidrQAFakeAWS(nil, nil)(context.Background(), Env{})
			}
			candidateValue := cand
			report := inspectVPCCIDR(context.Background(), deps, env,
				vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 5, Candidate: &candidateValue})
			if awsCalls != 0 {
				t.Errorf("deps.aws called %d times on candidate request, want 0 (Rule 8)", awsCalls)
			}
			if report.AWSChecked || report.Deployed != vpcCIDRDeployedUnchecked {
				t.Errorf("candidate report = %+v, want AWSChecked=false and Deployed=unchecked", report)
			}
			if report.Candidate == nil || report.Candidate.Value != cand {
				t.Fatalf("report.Candidate = %+v, want Value=%q", report.Candidate, cand)
			}
		})
	}
}

// TEST-50; REQ-B1 (Addendum 2 §B) & REQ-D7 (Addendum 2 §D Rule 6).
func TestGetVPCCIDRReport_HidesSiblingAndOwnAWSErrorsAndStateErrorFromJSON(t *testing.T) {
	fields := map[string]interface{}{
		"account_id":   "000000000000",
		"aws_profile":  "fixture-profile-dev",
		"region":       "us-east-1",
		"state_bucket": "synthetic-state-bucket",
		"state_file":   "dev.tfstate",
		"vpc_cidr":     "10.77.0.0/16",
	}
	cidrQASetupAPI(t, fields, nil)

	const ownSecret = "synthetic-own-secret-diag vpc-synthetic-own-err"
	const sibSecret = "synthetic-sibling-secret-diag vpc-synthetic-sib-err"
	const stateSecret = "synthetic-state-secret-diag s3://synthetic-bucket-err"

	siblings := []vpcCIDRSiblingAccount{
		cidrQA2Sibling(t, "staging", "qa-project", "staging", "111111111111", "fixture-profile-staging", "us-east-1"),
	}

	t.Run("own and sibling AWS errors stay in AWSError and never appear in API JSON", func(t *testing.T) {
		vpcCIDRAPIDeps = func() vpcCIDRDeps {
			deps := cidrQA2Deps(nil, siblings)
			deps.aws = func(ctx context.Context, target Env) (vpcCIDRAWS, error) {
				if target.AWSProfile == "fixture-profile-dev" {
					return vpcCIDRAWS{}, errors.New(ownSecret)
				}
				return cidrQAFakeAWS(nil, errors.New(sibSecret))(ctx, target)
			}
			return deps
		}

		// Verify the service report itself carries the technical errors in AWSError.
		svcReport := inspectVPCCIDR(context.Background(), vpcCIDRAPIDeps(), cidrQAEnv(t, fields),
			vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 5})
		if !strings.Contains(svcReport.AWSError, ownSecret) || !strings.Contains(svcReport.AWSError, sibSecret) {
			t.Errorf("service report AWSError = %q, want both %q and %q", svcReport.AWSError, ownSecret, sibSecret)
		}

		w := cidrQAAPIRequest(http.MethodGet, url.Values{"env": {"dev"}, "aws": {"1"}})
		wire, raw := cidrQAReadAPIReport(t, w)
		body := w.Body.String()

		for _, forbidden := range []string{ownSecret, sibSecret, "vpc-synthetic-own-err", "vpc-synthetic-sib-err", "AWSError", "awsError", "StateError", "stateError"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("API JSON leaked %q in body: %s", forbidden, body)
			}
		}
		for key := range raw {
			if strings.EqualFold(key, "AWSError") || strings.EqualFold(key, "StateError") {
				t.Errorf("API JSON exposed hidden field %q", key)
			}
		}
		if wire.AWSChecked || wire.AWSNote == "" || wire.DeployedReason == "" {
			t.Errorf("wire report missing expected plain fields: %+v", wire)
		}
	})

	t.Run("remote state error sets StateError and never leaks into DeployedReason or API JSON", func(t *testing.T) {
		vpcCIDRAPIDeps = func() vpcCIDRDeps {
			deps := cidrQA2Deps(nil, siblings)
			deps.aws = func(ctx context.Context, target Env) (vpcCIDRAWS, error) {
				readers, err := cidrQAFakeAWS(nil, nil)(ctx, target)
				if err != nil {
					return readers, err
				}
				if target.AWSProfile == "fixture-profile-dev" {
					cidrQA2SetFailingState(&readers, errors.New(stateSecret))
				}
				return readers, nil
			}
			return deps
		}

		svcReport := inspectVPCCIDR(context.Background(), vpcCIDRAPIDeps(), cidrQAEnv(t, fields),
			vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 5})
		if !strings.Contains(svcReport.StateError, stateSecret) {
			t.Errorf("svcReport.StateError = %q, want %q", svcReport.StateError, stateSecret)
		}
		if strings.Contains(svcReport.DeployedReason, stateSecret) || strings.Contains(svcReport.DeployedReason, "synthetic-bucket-err") {
			t.Errorf("DeployedReason leaked raw StateError text: %q", svcReport.DeployedReason)
		}

		w := cidrQAAPIRequest(http.MethodGet, url.Values{"env": {"dev"}, "aws": {"1"}})
		wire, raw := cidrQAReadAPIReport(t, w)
		body := w.Body.String()
		for _, forbidden := range []string{stateSecret, "synthetic-bucket-err", "StateError", "stateError"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("API JSON leaked state diagnostic %q in body: %s", forbidden, body)
			}
		}
		for key := range raw {
			if strings.EqualFold(key, "StateError") {
				t.Errorf("API JSON exposed hidden field %q", key)
			}
		}
		if wire.Deployed != vpcCIDRDeployedUnknown || strings.TrimSpace(wire.DeployedReason) == "" {
			t.Errorf("wire state failure report = %+v, want deployed=unknown and plain deployedReason", wire)
		}
	})
}

// TEST-51; REQ-B1 (Addendum 2 §B).
func TestInspectVPCCIDR_StateErrorAndDeployedReasonSanitization(t *testing.T) {
	// Verify struct tags on vpcCIDRReport for AWSError and StateError.
	rt := reflect.TypeOf(vpcCIDRReport{})
	for _, fieldName := range []string{"AWSError", "StateError"} {
		sf, ok := rt.FieldByName(fieldName)
		if !ok {
			t.Fatalf("vpcCIDRReport is missing required field %s", fieldName)
		}
		if tag := sf.Tag.Get("json"); tag != "-" {
			t.Errorf("vpcCIDRReport.%s json tag = %q, want \"-\"", fieldName, tag)
		}
	}

	// Verify direct JSON marshaling of a populated vpcCIDRReport never includes AWSError or StateError.
	sample := vpcCIDRReport{
		Env:            "dev",
		Deployed:       vpcCIDRDeployedUnknown,
		DeployedReason: "could not verify remote state",
		AWSError:       "raw-aws-error-secret",
		StateError:     "raw-state-error-secret",
	}
	encoded, err := json.Marshal(sample)
	if err != nil {
		t.Fatalf("marshal vpcCIDRReport: %v", err)
	}
	if strings.Contains(string(encoded), "raw-aws-error-secret") || strings.Contains(string(encoded), "raw-state-error-secret") {
		t.Errorf("json.Marshal(vpcCIDRReport) leaked hidden error fields: %s", string(encoded))
	}
}
