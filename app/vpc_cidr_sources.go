package main

// Where the ranges already in use come from: sibling environment YAMLs in the
// project directory, and the VPCs in an environment's AWS account.

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gopkg.in/yaml.v2"
)

// vpcCIDRProbe is the handful of fields the sibling scan needs.
//
// Read raw, never through loadEnv: that path migrates, writes a backup and
// rewrites the file (see readEnvIdentity in github_oidc_ownroles.go). Scanning
// siblings must not touch files belonging to environments nobody asked about.
type vpcCIDRProbe struct {
	Project       string `yaml:"project"`
	Env           string `yaml:"env"`
	SchemaVersion int    `yaml:"schema_version"`
	UseDefaultVPC *bool  `yaml:"use_default_vpc"`
	VPCCIDR       string `yaml:"vpc_cidr"`
	AWSProfile    string `yaml:"aws_profile"`
	Region        string `yaml:"region"`
	AccountID     string `yaml:"account_id"`
}

// usesDefaultVPC resolves a missing use_default_vpc the way the config's own
// schema history does: before schema v6 there was no custom VPC at all, so a
// file below v6 (or with no version) that does not say otherwise is on the
// default VPC. From v6 the migration writes the key, so a missing key on a
// current file means false.
func (p vpcCIDRProbe) usesDefaultVPC() bool {
	if p.UseDefaultVPC != nil {
		return *p.UseDefaultVPC
	}
	return p.SchemaVersion < 6
}

// siblingProbe is one sibling environment YAML, read raw.
type siblingProbe struct {
	Stem, File string
	Probe      vpcCIDRProbe
}

// scanSiblingProbes reads every other environment YAML in dir. self is the
// file stem of the environment being checked (its own file is skipped). The
// filter matches listEnvironmentNames: *.yaml/*.yml, not dns, and declaring
// both project and env. A stem present as both .yaml and .yml counts once.
func scanSiblingProbes(dir, self string) ([]siblingProbe, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", dir, err)
	}

	var out []siblingProbe
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if !strings.EqualFold(ext, ".yaml") && !strings.EqualFold(ext, ".yml") {
			continue
		}
		stem := strings.TrimSuffix(entry.Name(), ext)
		if stem == "dns" || stem == self || seen[stem] {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var probe vpcCIDRProbe
		if err := yaml.Unmarshal(data, &probe); err != nil {
			continue
		}
		if probe.Project == "" || probe.Env == "" {
			continue
		}
		seen[stem] = true
		out = append(out, siblingProbe{Stem: stem, File: entry.Name(), Probe: probe})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Stem < out[j].Stem })
	return out, nil
}

// siblingVPCCIDRUses returns the effective range of every other environment
// YAML in dir, sorted by name. Default-VPC environments and values that do
// not parse are not counted.
func siblingVPCCIDRUses(dir, self string) ([]vpcCIDRUse, error) {
	probes, err := scanSiblingProbes(dir, self)
	if err != nil {
		return nil, err
	}
	var uses []vpcCIDRUse
	for _, sp := range probes {
		effective := effectiveVPCCIDR(sp.Probe.usesDefaultVPC(), sp.Probe.VPCCIDR)
		if effective == "" {
			continue
		}
		p, err := netip.ParsePrefix(effective)
		if err != nil || !p.Addr().Is4() {
			continue
		}
		uses = append(uses, siblingVPCCIDRUse(sp.Stem, sp.File, p.Masked()))
	}
	return uses, nil
}

// vpcCIDRSiblingAccount is a sibling environment as far as its AWS account
// goes: enough of an Env to build pinned AWS readers and to recognise the
// sibling's own VPC in a listing.
type vpcCIDRSiblingAccount struct {
	Name string // file stem
	Env  Env
}

// siblingVPCCIDRAccounts returns every other environment in dir, with its
// profile, region and account. Default-VPC siblings are included: their
// account can still hold networks this environment must avoid.
func siblingVPCCIDRAccounts(dir, self string) ([]vpcCIDRSiblingAccount, error) {
	probes, err := scanSiblingProbes(dir, self)
	if err != nil {
		return nil, err
	}
	out := make([]vpcCIDRSiblingAccount, 0, len(probes))
	for _, sp := range probes {
		out = append(out, vpcCIDRSiblingAccount{
			Name: sp.Stem,
			Env: Env{
				Project:    sp.Probe.Project,
				Env:        sp.Probe.Env,
				AWSProfile: sp.Probe.AWSProfile,
				Region:     sp.Probe.Region,
				AccountID:  sp.Probe.AccountID,
			},
		})
	}
	return out, nil
}

// awsAccountKeys identifies the account and region an env's readers reach.
// The account ID (when known) and the profile are both keys, so two envs are
// the same target if either matches in the same region.
func awsAccountKeys(env Env) []string {
	region := strings.TrimSpace(env.Region)
	var keys []string
	if id := strings.TrimSpace(env.AccountID); id != "" {
		keys = append(keys, "account:"+id+"|"+region)
	}
	if profile := strings.TrimSpace(env.AWSProfile); profile != "" {
		keys = append(keys, "profile:"+profile+"|"+region)
	}
	return keys
}

// awsVPC is one VPC in the account: its active ranges and its tags.
type awsVPC struct {
	ID    string
	CIDRs []netip.Prefix
	Tags  map[string]string
}

// vpcLister lists every VPC in one account and region. A function type so
// tests substitute a fake and never reach AWS.
type vpcLister func(ctx context.Context) ([]awsVPC, error)

// ec2VPCLister lists VPCs with DescribeVpcs, following every page. Each VPC
// carries all of its IPv4 ranges in the associated or associating state —
// secondary ranges included, because peering refuses an overlap on any of them.
func ec2VPCLister(client ec2.DescribeVpcsAPIClient) vpcLister {
	return func(ctx context.Context) ([]awsVPC, error) {
		var vpcs []awsVPC
		pager := ec2.NewDescribeVpcsPaginator(client, &ec2.DescribeVpcsInput{})
		for pager.HasMorePages() {
			page, err := pager.NextPage(ctx)
			if err != nil {
				return nil, fmt.Errorf("DescribeVpcs: %w", err)
			}
			for _, v := range page.Vpcs {
				vpcs = append(vpcs, awsVPCFromEC2(v))
			}
		}
		return vpcs, nil
	}
}

func awsVPCFromEC2(v ec2types.Vpc) awsVPC {
	out := awsVPC{Tags: map[string]string{}}
	if v.VpcId != nil {
		out.ID = *v.VpcId
	}
	for _, t := range v.Tags {
		if t.Key != nil && t.Value != nil {
			out.Tags[*t.Key] = *t.Value
		}
	}
	// A range we leave out is a range we might suggest, so err toward keeping:
	// the primary block always counts, and an association counts unless AWS
	// says it is being removed or failed to attach. A missing state counts.
	seen := map[netip.Prefix]bool{}
	add := func(s *string) {
		if s == nil {
			return
		}
		p, err := netip.ParsePrefix(*s)
		if err != nil {
			return
		}
		p = p.Masked()
		if !seen[p] {
			seen[p] = true
			out.CIDRs = append(out.CIDRs, p)
		}
	}
	add(v.CidrBlock)
	for _, a := range v.CidrBlockAssociationSet {
		if a.CidrBlockState != nil {
			switch a.CidrBlockState.State {
			case ec2types.VpcCidrBlockStateCodeDisassociating,
				ec2types.VpcCidrBlockStateCodeDisassociated,
				ec2types.VpcCidrBlockStateCodeFailing,
				ec2types.VpcCidrBlockStateCodeFailed:
				continue
			}
		}
		add(a.CidrBlock)
	}
	return out
}

// vpcCIDRAWS is what the VPC range checks read from AWS, both halves bound to
// the same pinned configuration.
type vpcCIDRAWS struct {
	ListVPCs vpcLister
	State    remoteStateLookup
}

// errVPCCIDRNoAWSTarget is returned when the env does not name both a profile
// and a region. Falling back to the ambient $AWS_PROFILE could list another
// account entirely and report a confident, wrong "no overlap".
var errVPCCIDRNoAWSTarget = errors.New("the environment has no aws_profile and region set, so there is no AWS account to check")

// newVPCCIDRAWSForEnv builds the AWS readers for env, mirroring
// newAWSClientsForEnv (github_oidc.go): ONE aws.Config pinned to env.Region and
// env.AWSProfile, so the VPC listing and the state read are about the same
// account.
func newVPCCIDRAWSForEnv(ctx context.Context, env Env) (vpcCIDRAWS, error) {
	if strings.TrimSpace(env.AWSProfile) == "" || strings.TrimSpace(env.Region) == "" {
		return vpcCIDRAWS{}, errVPCCIDRNoAWSTarget
	}
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(env.Region),
		config.WithSharedConfigProfile(env.AWSProfile),
	)
	if err != nil {
		return vpcCIDRAWS{}, fmt.Errorf("failed to load AWS configuration for profile %s: %w", env.AWSProfile, err)
	}
	s3Client := s3.NewFromConfig(cfg)
	return vpcCIDRAWS{
		ListVPCs: ec2VPCLister(ec2.NewFromConfig(cfg)),
		State: func(ctx context.Context, env Env) (remoteStateSummary, error) {
			return readRemoteStateSummary(ctx, s3Client, env)
		},
	}, nil
}

// isOwnVPC reports whether v is the VPC modules/vpc builds for env: tagged
// Project/Environment, or named "<project>-vpc-<env>" (modules/vpc/main.tf).
func isOwnVPC(v awsVPC, env Env) bool {
	if env.Project == "" || env.Env == "" {
		return false
	}
	if v.Tags["Project"] == env.Project && v.Tags["Environment"] == env.Env {
		return true
	}
	return v.Tags["Name"] == fmt.Sprintf("%s-vpc-%s", env.Project, env.Env)
}

// awsVPCUses splits the account's VPCs into the environment's own VPC(s) and
// the ranges of every other VPC. The others are labelled generically.
func awsVPCUses(vpcs []awsVPC, env Env) (own []awsVPC, others []vpcCIDRUse) {
	for _, v := range vpcs {
		if isOwnVPC(v, env) {
			own = append(own, v)
			continue
		}
		for _, p := range v.CIDRs {
			others = append(others, awsVPCCIDRUse(p))
		}
	}
	return own, others
}
