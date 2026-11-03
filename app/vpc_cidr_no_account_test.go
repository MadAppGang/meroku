package main

// Orchestrator-written (not blind): the note for an environment that has no
// aws_profile/region yet. Nothing failed, so it must not say "couldn't reach".

import (
	"context"
	"strings"
	"testing"
)

func TestInspectVPCCIDR_NoAWSAccountYetIsNotAFailure(t *testing.T) {
	noTarget := func(_ context.Context, e Env) (vpcCIDRAWS, error) {
		if strings.TrimSpace(e.AWSProfile) == "" || strings.TrimSpace(e.Region) == "" {
			return vpcCIDRAWS{}, errVPCCIDRNoAWSTarget
		}
		return cidrQAFakeAWS([]awsVPC{{ID: "vpc-synthetic-sib", CIDRs: nil}}, nil)(context.Background(), e)
	}
	own := cidrQA2Env(t, "demo", "dev", "", "", "us-east-1", "10.30.0.0/16", false)
	opts := vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 1}

	t.Run("no siblings with accounts", func(t *testing.T) {
		deps := cidrQA2Deps(nil, nil)
		deps.aws = noTarget
		report := inspectVPCCIDR(context.Background(), deps, own, opts)
		if report.AWSNote != vpcCIDRNoAWSAccountNote {
			t.Errorf("AWSNote = %q, want %q", report.AWSNote, vpcCIDRNoAWSAccountNote)
		}
		if strings.Contains(strings.ToLower(report.AWSNote), "couldn't reach") {
			t.Errorf("AWSNote reads like a failure: %q", report.AWSNote)
		}
		if report.Deployed != vpcCIDRDeployedNo {
			t.Errorf("Deployed = %q, want no", report.Deployed)
		}
	})

	t.Run("a sibling account was read", func(t *testing.T) {
		sib := cidrQA2Sibling(t, "staging", "demo", "staging", "111111111111", "fixture-profile-b", "us-east-1")
		deps := cidrQA2Deps(nil, []vpcCIDRSiblingAccount{sib})
		deps.aws = noTarget
		report := inspectVPCCIDR(context.Background(), deps, own, opts)
		if report.AWSNote != vpcCIDRNoAWSAccountSiblingsReadNote {
			t.Errorf("AWSNote = %q, want %q", report.AWSNote, vpcCIDRNoAWSAccountSiblingsReadNote)
		}
	})
}
