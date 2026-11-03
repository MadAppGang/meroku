package main

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// TEST-10; REQ-15/M1.
func TestVPCCIDRProbe_RespectsLegacyDefaultVPC(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name    string
		version int
		flag    *bool
		want    bool
	}{
		{"missing schema and flag means legacy default", 0, nil, true},
		{"before v6 with no flag", 5, nil, true},
		{"at v6 with no flag", 6, nil, false},
		{"current with no flag", CurrentSchemaVersion, nil, false},
		{"old explicit custom wins", 5, &no, false},
		{"unversioned explicit custom wins", 0, &no, false},
		{"current explicit default wins", CurrentSchemaVersion, &yes, true},
		{"current explicit custom", CurrentSchemaVersion, &no, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := vpcCIDRProbe{SchemaVersion: tt.version, UseDefaultVPC: tt.flag}
			if got := p.usesDefaultVPC(); got != tt.want {
				t.Errorf("usesDefaultVPC() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TEST-11; REQ-5/R1.5, REQ-8/R2, REQ-15/M1, REQ-30.
func TestSiblingVPCCIDRUses_ReadsWithoutChangingFiles(t *testing.T) {
	dir := t.TempDir()
	cidrQAWriteEnv(t, dir, "dev.yaml", "not-the-file-stem", "000000000000",
		"use_default_vpc: false\nvpc_cidr: 10.11.0.0/16\n")
	cidrQAWriteEnv(t, dir, "zeta.yaml", "zeta", "000000000000",
		"use_default_vpc: false\nvpc_cidr: 10.7.1.3/16\n")
	for _, file := range []string{"alpha.yml", "alpha.yaml"} {
		cidrQAWriteEnv(t, dir, file, "alpha", "000000000000",
			"use_default_vpc: false\nvpc_cidr: 10.5.33.0/19\n")
	}
	cidrQAWriteEnv(t, dir, "implicit.yaml", "implicit", "000000000000",
		"use_default_vpc: false\n")
	cidrQAWriteEnv(t, dir, "default.yaml", "default", "000000000000",
		"use_default_vpc: true\nvpc_cidr: 10.88.0.0/16\n")

	raw := map[string]string{
		"legacy.yaml":    "# do not migrate this sibling\nschema_version: 5\nproject: qa-project\nenv: legacy\nvpc_cidr: 10.6.0.0/16\nunknown_key: keep-me\n",
		"ancient.yaml":   "project: qa-project\nenv: ancient\nvpc_cidr: 10.8.0.0/16\n",
		"oldcustom.yaml": "schema_version: 5\nproject: qa-project\nenv: oldcustom\nuse_default_vpc: false\nvpc_cidr: 10.9.7.0/16\n",
		"dns.yaml":       "root_domain: example.invalid\nroot_zone_id: synthetic-zone\n",
		"taskfile.yaml":  "version: '3'\ntasks: {}\n",
	}
	for file, body := range raw {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o600); err != nil {
			t.Fatalf("write synthetic %s: %v", file, err)
		}
	}
	before := cidrQASnapshot(t, dir)
	got, err := siblingVPCCIDRUses(dir, "dev")
	if err != nil {
		t.Fatalf("read siblings: %v", err)
	}
	wantNames := []string{"alpha", "implicit", "oldcustom", "zeta"}
	wantPrefixes := []string{"10.5.32.0/19", "10.0.0.0/16", "10.9.0.0/16", "10.7.0.0/16"}
	var names []string
	for i, u := range got {
		names = append(names, u.Name)
		if i >= len(wantNames) {
			continue
		}
		if u.Prefix.String() != wantPrefixes[i] {
			t.Errorf("%s prefix = %s, want %s", u.Name, u.Prefix, wantPrefixes[i])
		}
		if u.Source != "env \""+u.Name+"\"" {
			t.Errorf("source = %q, want plain environment identity for %s", u.Source, u.Name)
		}
		if u.Prefix != u.Prefix.Masked() {
			t.Errorf("sibling %s was not masked: %s", u.Name, u.Prefix)
		}
		if stem := strings.TrimSuffix(filepath.Base(u.File), filepath.Ext(u.File)); stem != u.Name {
			t.Errorf("file %q does not identify sibling %s", u.File, u.Name)
		}
	}
	if !reflect.DeepEqual(names, wantNames) {
		t.Errorf("sorted sibling names = %v, want %v (self/default/legacy excluded, extensions deduped)", names, wantNames)
	}
	cidrQAAssertUnchanged(t, dir, before)
}

// TEST-12; REQ-6/R1.6, REQ-9/R3, REQ-30.
func TestAWSVPCFromEC2_IncludesSecondaryNetworks(t *testing.T) {
	input := ec2types.Vpc{
		VpcId:     aws.String("vpc-synthetic-primary"),
		CidrBlock: aws.String("10.40.0.0/16"),
		CidrBlockAssociationSet: []ec2types.VpcCidrBlockAssociation{
			{CidrBlock: aws.String("10.40.0.0/16")},
			{CidrBlock: aws.String("10.80.0.0/16")},
		},
		Tags: []ec2types.Tag{{Key: aws.String("Name"), Value: aws.String("synthetic-network-name")}},
	}
	got := awsVPCFromEC2(input)
	have := map[string]bool{}
	for _, p := range got.CIDRs {
		have[p.String()] = true
	}
	want := map[string]bool{"10.40.0.0/16": true, "10.80.0.0/16": true}
	if !reflect.DeepEqual(have, want) {
		t.Errorf("converted networks = %v, want primary and secondary %v", have, want)
	}
	if got.ID != "vpc-synthetic-primary" || got.Tags["Name"] != "synthetic-network-name" {
		t.Errorf("converted identity needed for own-VPC matching was lost: %+v", got)
	}
}

// Fake the SDK's public request/response boundary, not any implementation helper.
// No invocation assertions: the returned VPC set is the pagination oracle.
type cidrQAEC2Pages struct {
	pages map[string]*ec2.DescribeVpcsOutput
	err   error
}

func (f cidrQAEC2Pages) DescribeVpcs(_ context.Context, input *ec2.DescribeVpcsInput, _ ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	token := ""
	if input != nil {
		token = aws.ToString(input.NextToken)
	}
	if page, ok := f.pages[token]; ok {
		return page, nil
	}
	return nil, errors.New("unexpected synthetic pagination token")
}

// TEST-13; REQ-6/R1.6, REQ-9/R3, REQ-30.
func TestEC2VPCLister_ReturnsAllPages(t *testing.T) {
	pages := cidrQAEC2Pages{pages: map[string]*ec2.DescribeVpcsOutput{
		"": {
			Vpcs:      []ec2types.Vpc{{VpcId: aws.String("vpc-synthetic-page-one"), CidrBlock: aws.String("10.40.0.0/16")}},
			NextToken: aws.String("synthetic-page-two"),
		},
		"synthetic-page-two": {
			Vpcs: []ec2types.Vpc{{
				VpcId:                   aws.String("vpc-synthetic-page-two"),
				CidrBlock:               aws.String("10.77.0.0/16"),
				CidrBlockAssociationSet: []ec2types.VpcCidrBlockAssociation{{CidrBlock: aws.String("10.120.0.0/16")}},
			}},
		},
	}}
	got, err := ec2VPCLister(pages)(context.Background())
	if err != nil {
		t.Fatalf("list synthetic pages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("VPCs = %+v, want both pages", got)
	}
	networks := map[string]bool{}
	for _, v := range got {
		for _, p := range v.CIDRs {
			networks[p.String()] = true
		}
	}
	want := map[string]bool{"10.40.0.0/16": true, "10.77.0.0/16": true, "10.120.0.0/16": true}
	if !reflect.DeepEqual(networks, want) {
		t.Errorf("listed networks = %v, want %v", networks, want)
	}
}

// TEST-14; REQ-6/R1.6, REQ-20/M6, REQ-30.
func TestAWSVPCUses_UsesGenericNetworkLabels(t *testing.T) {
	env := cidrQAEnv(t, map[string]interface{}{"account_id": "000000000000", "aws_profile": "fixture-profile"})
	vpcs := []awsVPC{
		{
			ID:    "vpc-synthetic-private-one",
			Tags:  map[string]string{"Name": "synthetic-private-name-one"},
			CIDRs: []netip.Prefix{netip.MustParsePrefix("10.42.0.0/16"), netip.MustParsePrefix("10.77.0.0/16")},
		},
		{
			ID:    "vpc-synthetic-private-two",
			Tags:  map[string]string{"Name": "synthetic-private-name-two"},
			CIDRs: []netip.Prefix{netip.MustParsePrefix("10.88.0.0/16")},
		},
	}
	own, others := awsVPCUses(vpcs, env)
	if len(own) != 0 {
		t.Errorf("untagged synthetic VPCs classified as ours: %+v", own)
	}
	got := map[string]bool{}
	for _, u := range others {
		got[u.Prefix.String()] = true
		if u.Source != "another network in this AWS account" || u.Name != "another network in this AWS account" {
			t.Errorf("AWS identity was not generic: %+v", u)
		}
	}
	want := map[string]bool{"10.42.0.0/16": true, "10.77.0.0/16": true, "10.88.0.0/16": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("other networks = %v, want all associations %v", got, want)
	}
	warnings := vpcCIDROverlapWarnings(netip.MustParsePrefix("10.42.0.0/16"), others)
	if len(warnings) != 1 {
		t.Fatalf("warnings = %+v, want one", warnings)
	}
	w := warnings[0]
	cidrQAAssertPlainHint(t, w.Hint)
	for _, secretIdentity := range []string{"vpc-synthetic", "synthetic-private-name"} {
		if strings.Contains(w.Message+w.Hint+w.Source, secretIdentity) {
			t.Errorf("warning leaked a synthetic AWS network identity: %+v", w)
		}
	}
}
