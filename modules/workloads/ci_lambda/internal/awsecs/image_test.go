package awsecs_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"madappgang.com/infrastructure/ci_lambda/internal/awsecs"
)

func TestSplitImageRef(t *testing.T) {
	cases := []struct {
		ref      string
		wantRepo string
		wantVer  string
	}{
		{
			"000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_backend:abc123",
			"000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_backend", "abc123",
		},
		{
			"000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_backend@sha256:deadbeef",
			"000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_backend", "sha256:deadbeef",
		},
		// A colon in the registry host is a port, not a tag. LastIndex(":")
		// used to cut here and produce a repository that matched nothing.
		{"registry.example.com:5000/team/api", "registry.example.com:5000/team/api", ""},
		{"registry.example.com:5000/team/api:v1", "registry.example.com:5000/team/api", "v1"},
		{"amazon/aws-xray-daemon", "amazon/aws-xray-daemon", ""},
		{"postgres:16", "postgres", "16"},
		{"", "", ""},
	}

	for _, c := range cases {
		repo, ver := awsecs.SplitImageRef(c.ref)
		require.Equalf(t, c.wantRepo, repo, "repo of %q", c.ref)
		require.Equalf(t, c.wantVer, ver, "version of %q", c.ref)
	}
}
