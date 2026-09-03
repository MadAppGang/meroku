package main

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// Every fixture below is synthetic. Account IDs are 000000000000 and the
// organisations are acme / billing / example.com — this repository is public and
// its CLAUDE.md forbids real infrastructure data in any committed file. Nothing
// here was copied out of an AWS console.

const (
	testGitHubProviderARN = "arn:aws:iam::000000000000:oidc-provider/token.actions.githubusercontent.com"
	testSAMLProviderARN   = "arn:aws:iam::000000000000:saml-provider/example.com"
	testGoogleProviderARN = "accounts.google.com"
)

// githubTrustPolicy renders a canonical single-statement GitHub trust policy
// with the Condition block supplied verbatim, so each case varies exactly one
// thing and the rest of the document is above suspicion.
func githubTrustPolicy(condition string) string {
	return `{
	  "Version": "2012-10-17",
	  "Statement": [{
	    "Effect": "Allow",
	    "Principal": { "Federated": "` + testGitHubProviderARN + `" },
	    "Action": ["sts:AssumeRoleWithWebIdentity"]` + condition + `
	  }]
	}`
}

// TestParseGitHubTrustPolicy is the whole parser contract. Each case is named
// for the misreading it rules out, because the shapes below differ by a few
// bytes and a bare JSON blob two years from now says nothing about why it was
// worth writing down.
func TestParseGitHubTrustPolicy(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want githubTrustGrant
	}{
		{
			name: "plain JSON: the canonical role meroku generates",
			doc: githubTrustPolicy(`,
	    "Condition": { "StringLike": {
	      "token.actions.githubusercontent.com:sub": ["repo:acme/api:*"]
	    }}`),
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			name: "url-encoded: GetRole returns the document percent-encoded",
			doc: url.QueryEscape(githubTrustPolicy(`,
	    "Condition": { "StringLike": {
	      "token.actions.githubusercontent.com:sub": ["repo:acme/api:*"]
	    }}`)),
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			name: "StringLike value as a bare string, not a list",
			doc: githubTrustPolicy(`,
	    "Condition": { "StringLike": {
	      "token.actions.githubusercontent.com:sub": "repo:acme/api:*"
	    }}`),
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			name: "StringLike value as a list of several subjects",
			doc: githubTrustPolicy(`,
	    "Condition": { "StringLike": {
	      "token.actions.githubusercontent.com:sub": [
	        "repo:acme/api:ref:refs/heads/main",
	        "repo:acme/api:pull_request"
	      ]
	    }}`),
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{
				"repo:acme/api:ref:refs/heads/main",
				"repo:acme/api:pull_request",
			}},
		},
		{
			// A literal is a glob with no metacharacters, so it needs no
			// translation and no escaping to reach the intersection engine.
			name: "StringEquals counts as a subject restriction",
			doc: githubTrustPolicy(`,
	    "Condition": { "StringEquals": {
	      "token.actions.githubusercontent.com:sub": "repo:acme/api:ref:refs/heads/main"
	    }}`),
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:ref:refs/heads/main"}},
		},
		{
			name: "ForAnyValue: prefix is stripped, not treated as a different operator",
			doc: githubTrustPolicy(`,
	    "Condition": { "ForAnyValue:StringLike": {
	      "token.actions.githubusercontent.com:sub": ["repo:acme/api:*"]
	    }}`),
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			name: "ForAllValues: prefix is stripped too",
			doc: githubTrustPolicy(`,
	    "Condition": { "ForAllValues:StringLike": {
	      "token.actions.githubusercontent.com:sub": ["repo:acme/api:*"]
	    }}`),
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			// The highest-severity parser bug available: an exact operator
			// lookup finds nothing, reads as "no subject condition", and
			// reports Unrestricted on a correctly fenced role.
			name: "mixed-case operator: stringlike restricts exactly as StringLike does",
			doc: githubTrustPolicy(`,
	    "Condition": { "stringlike": {
	      "token.actions.githubusercontent.com:sub": ["repo:acme/api:*"]
	    }}`),
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			name: "mixed-case condition key: IAM compares it case-insensitively",
			doc: githubTrustPolicy(`,
	    "Condition": { "StringLike": {
	      "Token.Actions.GitHubUserContent.com:Sub": ["repo:acme/api:*"]
	    }}`),
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			name: "IfExists suffix: a GitHub token always carries sub, so it restricts",
			doc: githubTrustPolicy(`,
	    "Condition": { "StringLikeIfExists": {
	      "token.actions.githubusercontent.com:sub": ["repo:acme/api:*"]
	    }}`),
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			// The classic misconfiguration and the most valuable thing the scan
			// can find: no GitHub claim pinned means every repository on GitHub.
			name: "no Condition block at all means unrestricted",
			doc:  githubTrustPolicy(``),
			want: githubTrustGrant{IsGitHub: true, Unrestricted: true},
		},
		{
			// :aud is in every trust policy AWS documents and pins the audience,
			// not the caller. Counting it as a fence would mean this parser
			// never reports the misconfiguration it exists to find.
			name: "a Condition on aud alone is the genuine misconfiguration",
			doc: githubTrustPolicy(`,
	    "Condition": { "StringEquals": {
	      "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
	    }}`),
			want: githubTrustGrant{IsGitHub: true, Unrestricted: true},
		},
		{
			// A non-GitHub condition key says nothing about which repository may
			// assume the role, so it does not make the role evaluable either.
			name: "a Condition on a non-GitHub key alone is still unrestricted",
			doc: githubTrustPolicy(`,
	    "Condition": { "IpAddress": { "aws:SourceIp": "203.0.113.0/24" }}`),
			want: githubTrustGrant{IsGitHub: true, Unrestricted: true},
		},
		{
			// Hardened, and not by a fence this scan can intersect. Reporting
			// Unrestricted here would fire the loudest alert in the feature at a
			// role that trusts exactly one organisation — and a check that cries
			// wolf on correct configuration is a check people click past.
			name: "repository_owner beside aud, with no sub: fenced but not evaluable",
			doc: githubTrustPolicy(`,
	    "Condition": {
	      "StringEquals": { "token.actions.githubusercontent.com:aud": "sts.amazonaws.com" },
	      "StringLike": { "token.actions.githubusercontent.com:repository_owner": "acme" }
	    }`),
			want: githubTrustGrant{
				IsGitHub:                true,
				RestrictedByOtherClaims: true,
				OtherClaimKeys:          []string{"repository_owner"},
			},
		},
		{
			name: "job_workflow_ref with no sub: fenced but not evaluable",
			doc: githubTrustPolicy(`,
	    "Condition": { "StringLike": {
	      "token.actions.githubusercontent.com:job_workflow_ref": "acme/api/.github/workflows/deploy.yml@*"
	    }}`),
			want: githubTrustGrant{
				IsGitHub:                true,
				RestrictedByOtherClaims: true,
				OtherClaimKeys:          []string{"job_workflow_ref"},
			},
		},
		{
			name: "repository and environment with no sub: both claims are named",
			doc: githubTrustPolicy(`,
	    "Condition": { "StringEquals": {
	      "token.actions.githubusercontent.com:repository": "acme/api",
	      "token.actions.githubusercontent.com:environment": "prod"
	    }}`),
			want: githubTrustGrant{
				IsGitHub:                true,
				RestrictedByOtherClaims: true,
				// Sorted, because Go randomises map iteration and a warning a
				// user re-reads must not reshuffle itself.
				OtherClaimKeys: []string{"environment", "repository"},
			},
		},
		{
			// StringNotLike carves a hole rather than drawing a boundary: there
			// is no pattern to intersect. It is a fence all the same, so the
			// role is neither unrestricted nor cleared.
			name: "StringNotLike on sub is a fence this scan cannot intersect",
			doc: githubTrustPolicy(`,
	    "Condition": { "StringNotLike": {
	      "token.actions.githubusercontent.com:sub": "repo:acme/forbidden:*"
	    }}`),
			want: githubTrustGrant{
				IsGitHub:                true,
				RestrictedByOtherClaims: true,
				OtherClaimKeys:          []string{"sub"},
			},
		},
		{
			// When sub IS pinned, it is what the scan evaluates. Any other claim
			// only narrows it further, so it changes no verdict and reporting
			// "cannot evaluate" alongside subjects we can evaluate is noise.
			name: "sub beside repository_owner: the sub condition is what we evaluate",
			doc: githubTrustPolicy(`,
	    "Condition": { "StringLike": {
	      "token.actions.githubusercontent.com:sub": "repo:acme/api:*",
	      "token.actions.githubusercontent.com:repository_owner": "acme"
	    }}`),
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			// One statement is evaluable and its neighbour is not. Neither
			// answer may swallow the other.
			name: "a sub statement beside a repository_owner statement reports both",
			doc: `{
			  "Statement": [
			    {
			      "Effect": "Allow",
			      "Principal": { "Federated": "` + testGitHubProviderARN + `" },
			      "Action": "sts:AssumeRoleWithWebIdentity",
			      "Condition": { "StringLike": {
			        "token.actions.githubusercontent.com:sub": "repo:acme/api:*"
			      }}
			    },
			    {
			      "Effect": "Allow",
			      "Principal": { "Federated": "` + testGitHubProviderARN + `" },
			      "Action": "sts:AssumeRoleWithWebIdentity",
			      "Condition": { "StringEquals": {
			        "token.actions.githubusercontent.com:repository_owner": "billing"
			      }}
			    }
			  ]
			}`,
			want: githubTrustGrant{
				IsGitHub:                true,
				Subjects:                []string{"repo:acme/api:*"},
				RestrictedByOtherClaims: true,
				OtherClaimKeys:          []string{"repository_owner"},
			},
		},
		{
			name: "aud and sub in one block: sub is found beside an ignored key",
			doc: githubTrustPolicy(`,
	    "Condition": {
	      "StringEquals": { "token.actions.githubusercontent.com:aud": "sts.amazonaws.com" },
	      "StringLike": { "token.actions.githubusercontent.com:sub": "repo:acme/api:*" }
	    }`),
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			name: "singleton Statement written as an object rather than an array",
			doc: `{
			  "Version": "2012-10-17",
			  "Statement": {
			    "Effect": "Allow",
			    "Principal": { "Federated": "` + testGitHubProviderARN + `" },
			    "Action": "sts:AssumeRoleWithWebIdentity",
			    "Condition": { "StringLike": {
			      "token.actions.githubusercontent.com:sub": "repo:acme/api:*"
			    }}
			  }
			}`,
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			name: "singleton Action written as a string rather than an array",
			doc: `{
			  "Statement": [{
			    "Effect": "Allow",
			    "Principal": { "Federated": "` + testGitHubProviderARN + `" },
			    "Action": "sts:AssumeRoleWithWebIdentity",
			    "Condition": { "StringLike": {
			      "token.actions.githubusercontent.com:sub": "repo:acme/api:*"
			    }}
			  }]
			}`,
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			name: "Principal.Federated as a single-element list",
			doc: `{
			  "Statement": [{
			    "Effect": "Allow",
			    "Principal": { "Federated": ["` + testGitHubProviderARN + `"] },
			    "Action": ["sts:AssumeRoleWithWebIdentity"],
			    "Condition": { "StringLike": {
			      "token.actions.githubusercontent.com:sub": "repo:acme/api:*"
			    }}
			  }]
			}`,
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			name: "Principal.Federated list mixing a SAML provider with GitHub",
			doc: `{
			  "Statement": [{
			    "Effect": "Allow",
			    "Principal": { "Federated": ["` + testSAMLProviderARN + `", "` + testGitHubProviderARN + `"] },
			    "Action": ["sts:AssumeRoleWithWebIdentity"],
			    "Condition": { "StringLike": {
			      "token.actions.githubusercontent.com:sub": "repo:acme/api:*"
			    }}
			  }]
			}`,
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			name: "a wildcard Action still grants assume-role-with-web-identity",
			doc: `{
			  "Statement": [{
			    "Effect": "Allow",
			    "Principal": { "Federated": "` + testGitHubProviderARN + `" },
			    "Action": "sts:*",
			    "Condition": { "StringLike": {
			      "token.actions.githubusercontent.com:sub": "repo:acme/api:*"
			    }}
			  }]
			}`,
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			// Policies routinely split this out with the same Federated
			// principal and no Condition. Counted as a grant it would report
			// Unrestricted on a role that is correctly fenced.
			name: "a GitHub statement granting only sts:TagSession is not a grant",
			doc: `{
			  "Statement": [{
			    "Effect": "Allow",
			    "Principal": { "Federated": "` + testGitHubProviderARN + `" },
			    "Action": "sts:TagSession"
			  }]
			}`,
			want: githubTrustGrant{},
		},
		{
			name: "a Deny statement carrying a sub is ignored, not unioned",
			doc: `{
			  "Statement": [
			    {
			      "Effect": "Allow",
			      "Principal": { "Federated": "` + testGitHubProviderARN + `" },
			      "Action": "sts:AssumeRoleWithWebIdentity",
			      "Condition": { "StringLike": {
			        "token.actions.githubusercontent.com:sub": "repo:acme/api:*"
			      }}
			    },
			    {
			      "Effect": "Deny",
			      "Principal": { "Federated": "` + testGitHubProviderARN + `" },
			      "Action": "sts:AssumeRoleWithWebIdentity",
			      "Condition": { "StringLike": {
			        "token.actions.githubusercontent.com:sub": "repo:acme/forbidden:*"
			      }}
			    }
			  ]
			}`,
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			// A lone Deny would otherwise be read as a GitHub role with no sub
			// condition, i.e. the loudest verdict the parser has.
			name: "a lone Deny statement is not a GitHub grant",
			doc: `{
			  "Statement": [{
			    "Effect": "Deny",
			    "Principal": { "Federated": "` + testGitHubProviderARN + `" },
			    "Action": "sts:AssumeRoleWithWebIdentity"
			  }]
			}`,
			want: githubTrustGrant{},
		},
		{
			name: "a statement with NotPrincipal is skipped rather than inverted",
			doc: `{
			  "Statement": [{
			    "Effect": "Allow",
			    "NotPrincipal": { "Federated": "` + testGitHubProviderARN + `" },
			    "Action": "sts:AssumeRoleWithWebIdentity"
			  }]
			}`,
			want: githubTrustGrant{},
		},
		{
			name: "a statement with NotAction is skipped rather than inverted",
			doc: `{
			  "Statement": [{
			    "Effect": "Allow",
			    "Principal": { "Federated": "` + testGitHubProviderARN + `" },
			    "NotAction": "sts:AssumeRole"
			  }]
			}`,
			want: githubTrustGrant{},
		},
		{
			name: "a SAML federated principal is not GitHub",
			doc: `{
			  "Statement": [{
			    "Effect": "Allow",
			    "Principal": { "Federated": "` + testSAMLProviderARN + `" },
			    "Action": "sts:AssumeRoleWithSAML"
			  }]
			}`,
			want: githubTrustGrant{},
		},
		{
			name: "a Google federated principal is not GitHub",
			doc: `{
			  "Statement": [{
			    "Effect": "Allow",
			    "Principal": { "Federated": "` + testGoogleProviderARN + `" },
			    "Action": "sts:AssumeRoleWithWebIdentity",
			    "Condition": { "StringEquals": {
			      "accounts.google.com:sub": "000000000000000000000"
			    }}
			  }]
			}`,
			want: githubTrustGrant{},
		},
		{
			name: "an sts:AssumeRole statement for an AWS principal is not GitHub",
			doc: `{
			  "Statement": [{
			    "Effect": "Allow",
			    "Principal": { "AWS": "arn:aws:iam::000000000000:root" },
			    "Action": "sts:AssumeRole"
			  }]
			}`,
			want: githubTrustGrant{},
		},
		{
			name: "an ECS service principal is not GitHub",
			doc: `{
			  "Statement": [{
			    "Effect": "Allow",
			    "Principal": { "Service": "ecs-tasks.amazonaws.com" },
			    "Action": "sts:AssumeRole"
			  }]
			}`,
			want: githubTrustGrant{},
		},
		{
			name: "a bare wildcard Principal is understood and is not GitHub",
			doc: `{
			  "Statement": [{
			    "Effect": "Allow",
			    "Principal": "*",
			    "Action": "sts:AssumeRole"
			  }]
			}`,
			want: githubTrustGrant{},
		},
		{
			name: "subjects are unioned across several Allow statements",
			doc: `{
			  "Statement": [
			    {
			      "Effect": "Allow",
			      "Principal": { "Federated": "` + testGitHubProviderARN + `" },
			      "Action": "sts:AssumeRoleWithWebIdentity",
			      "Condition": { "StringLike": {
			        "token.actions.githubusercontent.com:sub": ["repo:acme/api:*"]
			      }}
			    },
			    {
			      "Effect": "Allow",
			      "Principal": { "Federated": "` + testGitHubProviderARN + `" },
			      "Action": "sts:AssumeRoleWithWebIdentity",
			      "Condition": { "StringEquals": {
			        "token.actions.githubusercontent.com:sub": "repo:billing/jobs:ref:refs/heads/main"
			      }}
			    }
			  ]
			}`,
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{
				"repo:acme/api:*",
				"repo:billing/jobs:ref:refs/heads/main",
			}},
		},
		{
			name: "a repeated subject is unioned once, not twice",
			doc: `{
			  "Statement": [
			    {
			      "Effect": "Allow",
			      "Principal": { "Federated": "` + testGitHubProviderARN + `" },
			      "Action": "sts:AssumeRoleWithWebIdentity",
			      "Condition": { "StringLike": {
			        "token.actions.githubusercontent.com:sub": ["repo:acme/api:*", "repo:acme/api:*"]
			      }}
			    },
			    {
			      "Effect": "Allow",
			      "Principal": { "Federated": "` + testGitHubProviderARN + `" },
			      "Action": "sts:AssumeRoleWithWebIdentity",
			      "Condition": { "StringLike": {
			        "token.actions.githubusercontent.com:sub": "repo:acme/api:*"
			      }}
			    }
			  ]
			}`,
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			// One unfenced statement makes the role unfenced, however tight its
			// neighbour is: a token matching neither subject still assumes it.
			name: "an unrestricted statement beside a restricted one still reports unrestricted",
			doc: `{
			  "Statement": [
			    {
			      "Effect": "Allow",
			      "Principal": { "Federated": "` + testGitHubProviderARN + `" },
			      "Action": "sts:AssumeRoleWithWebIdentity",
			      "Condition": { "StringLike": {
			        "token.actions.githubusercontent.com:sub": "repo:acme/api:*"
			      }}
			    },
			    {
			      "Effect": "Allow",
			      "Principal": { "Federated": "` + testGitHubProviderARN + `" },
			      "Action": "sts:AssumeRoleWithWebIdentity"
			    }
			  ]
			}`,
			want: githubTrustGrant{IsGitHub: true, Unrestricted: true, Subjects: []string{"repo:acme/api:*"}},
		},
		{
			// GovCloud and China use different partitions. Matching the ARN
			// suffix rather than the whole string is what makes this work.
			name: "a GovCloud partition provider ARN is still GitHub",
			doc: `{
			  "Statement": [{
			    "Effect": "Allow",
			    "Principal": { "Federated": "arn:aws-us-gov:iam::000000000000:oidc-provider/token.actions.githubusercontent.com" },
			    "Action": "sts:AssumeRoleWithWebIdentity",
			    "Condition": { "StringLike": {
			      "token.actions.githubusercontent.com:sub": "repo:acme/api:*"
			    }}
			  }]
			}`,
			want: githubTrustGrant{IsGitHub: true, Subjects: []string{"repo:acme/api:*"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseGitHubTrustPolicy(c.doc)
			if err != nil {
				t.Fatalf("parseGitHubTrustPolicy returned an error for a document it must understand: %v", err)
			}
			if got.IsGitHub != c.want.IsGitHub {
				t.Errorf("IsGitHub = %t, want %t", got.IsGitHub, c.want.IsGitHub)
			}
			if got.Unrestricted != c.want.Unrestricted {
				t.Errorf("Unrestricted = %t, want %t", got.Unrestricted, c.want.Unrestricted)
			}
			if got.RestrictedByOtherClaims != c.want.RestrictedByOtherClaims {
				t.Errorf("RestrictedByOtherClaims = %t, want %t", got.RestrictedByOtherClaims, c.want.RestrictedByOtherClaims)
			}
			if !reflect.DeepEqual(got.Subjects, c.want.Subjects) {
				t.Errorf("Subjects = %#v, want %#v", got.Subjects, c.want.Subjects)
			}
			if !reflect.DeepEqual(got.OtherClaimKeys, c.want.OtherClaimKeys) {
				t.Errorf("OtherClaimKeys = %#v, want %#v", got.OtherClaimKeys, c.want.OtherClaimKeys)
			}
		})
	}
}

// TestParseGitHubTrustPolicyErrors is the half of the contract that matters
// most. getTrustedAccountsFromRole answers every one of these with an empty
// slice and a nil error, which makes "I could not read this role" identical to
// "this role trusts nobody" — and a scan built on that reports no conflicts for
// a document it never understood. Each case asserts an error AND asserts that
// the returned grant is not a quietly empty one that a caller could believe.
func TestParseGitHubTrustPolicyErrors(t *testing.T) {
	cases := []struct {
		name string
		doc  string
	}{
		{"malformed JSON: a truncated document", `{"Statement": [{"Effect": "Allow"`},
		{"malformed JSON: not JSON at all", `AccessDenied`},
		{"empty document", ``},
		{"whitespace-only document", "  \n\t "},
		{"a JSON array where a policy object belongs", `["Statement"]`},
		{"a policy with no Statement", `{"Version": "2012-10-17"}`},
		{"Statement as a string", `{"Statement": "Allow everything"}`},
		{"a statement that is not an object", `{"Statement": [42]}`},
		{"a statement with no Effect", `{"Statement": [{
			"Principal": { "Federated": "` + testGitHubProviderARN + `" },
			"Action": "sts:AssumeRoleWithWebIdentity"
		}]}`},
		{"an Effect that is neither Allow nor Deny", `{"Statement": [{
			"Effect": "Permit",
			"Principal": { "Federated": "` + testGitHubProviderARN + `" },
			"Action": "sts:AssumeRoleWithWebIdentity"
		}]}`},
		{"a GitHub statement with no Action", `{"Statement": [{
			"Effect": "Allow",
			"Principal": { "Federated": "` + testGitHubProviderARN + `" }
		}]}`},
		{"an Action that is a number", `{"Statement": [{
			"Effect": "Allow",
			"Principal": { "Federated": "` + testGitHubProviderARN + `" },
			"Action": 1
		}]}`},
		{"a Condition that is not an object", `{"Statement": [{
			"Effect": "Allow",
			"Principal": { "Federated": "` + testGitHubProviderARN + `" },
			"Action": "sts:AssumeRoleWithWebIdentity",
			"Condition": "StringLike"
		}]}`},
		{"a sub condition value that is not a string", `{"Statement": [{
			"Effect": "Allow",
			"Principal": { "Federated": "` + testGitHubProviderARN + `" },
			"Action": "sts:AssumeRoleWithWebIdentity",
			"Condition": { "StringLike": { "token.actions.githubusercontent.com:sub": 7 } }
		}]}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseGitHubTrustPolicy(c.doc)
			if err == nil {
				t.Fatalf("want an error, got grant %#v with nil error: a failed read must never look like a role that trusts nobody", got)
			}
			if !reflect.DeepEqual(got, githubTrustGrant{}) {
				t.Errorf("a failing parse must return the zero grant, got %#v", got)
			}
		})
	}
}

// TestParseGitHubTrustPolicyVerdictsAreExclusive pins the invariant the caller
// depends on: Unrestricted and RestrictedByOtherClaims are different answers to
// different questions, and a role that pins a claim other than :aud is never
// reported as trusting all of GitHub. That confusion is what makes a security
// check fire its loudest alert at correctly hardened roles, which is how one
// gets ignored.
func TestParseGitHubTrustPolicyVerdictsAreExclusive(t *testing.T) {
	// Every GitHub claim a trust policy can realistically pin, one per case,
	// with no sub anywhere. None of these roles trusts all of GitHub.
	for _, claim := range []string{"repository", "repository_owner", "job_workflow_ref", "environment", "actor", "workflow", "ref"} {
		t.Run(claim, func(t *testing.T) {
			grant, err := parseGitHubTrustPolicy(githubTrustPolicy(`,
			  "Condition": {
			    "StringEquals": { "token.actions.githubusercontent.com:aud": "sts.amazonaws.com" },
			    "StringLike": { "token.actions.githubusercontent.com:` + claim + `": "acme" }
			  }`))
			if err != nil {
				t.Fatalf("parseGitHubTrustPolicy: %v", err)
			}
			if grant.Unrestricted {
				t.Errorf("a role fenced on :%s does not trust all of GitHub, but Unrestricted is true", claim)
			}
			if !grant.RestrictedByOtherClaims {
				t.Errorf("a role fenced on :%s is not evaluable by this scan, but RestrictedByOtherClaims is false", claim)
			}
			if len(grant.Subjects) != 0 {
				t.Errorf("Subjects must be empty when there is no sub condition, got %#v", grant.Subjects)
			}
			if !reflect.DeepEqual(grant.OtherClaimKeys, []string{claim}) {
				t.Errorf("OtherClaimKeys = %#v, want [%q]", grant.OtherClaimKeys, claim)
			}
		})
	}
}

// TestParseGitHubTrustPolicyURLEncodedIsIdentical pins the sniff itself: the
// same document read plain and percent-encoded must give the same answer, or
// the scan's verdict depends on which IAM call it happened to come from.
func TestParseGitHubTrustPolicyURLEncodedIsIdentical(t *testing.T) {
	plain := githubTrustPolicy(`,
	  "Condition": { "StringLike": {
	    "token.actions.githubusercontent.com:sub": ["repo:acme/api:*", "repo:billing/jobs:*"]
	  }}`)

	fromPlain, err := parseGitHubTrustPolicy(plain)
	if err != nil {
		t.Fatalf("plain document: %v", err)
	}

	encoded := url.QueryEscape(plain)
	if !strings.Contains(encoded, "%7B") {
		t.Fatalf("the fixture must actually be percent-encoded for this test to prove anything")
	}
	fromEncoded, err := parseGitHubTrustPolicy(encoded)
	if err != nil {
		t.Fatalf("percent-encoded document: %v", err)
	}

	if !reflect.DeepEqual(fromPlain, fromEncoded) {
		t.Errorf("encoding changed the verdict: plain %#v, encoded %#v", fromPlain, fromEncoded)
	}
}

// TestParseGitHubTrustPolicySubjectsFeedTheGlobEngine closes the loop with P0:
// the strings this parser hands back are IAM globs, and a StringEquals literal
// has to work in the intersection engine untranslated and unescaped. If it ever
// needed quoting, this is where that would surface.
func TestParseGitHubTrustPolicySubjectsFeedTheGlobEngine(t *testing.T) {
	wildcard, err := parseGitHubTrustPolicy(githubTrustPolicy(`,
	  "Condition": { "StringLike": {
	    "token.actions.githubusercontent.com:sub": "repo:acme/*"
	  }}`))
	if err != nil {
		t.Fatalf("wildcard policy: %v", err)
	}

	literal, err := parseGitHubTrustPolicy(githubTrustPolicy(`,
	  "Condition": { "StringEquals": {
	    "token.actions.githubusercontent.com:sub": "repo:acme/api:ref:refs/heads/main"
	  }}`))
	if err != nil {
		t.Fatalf("literal policy: %v", err)
	}

	if len(wildcard.Subjects) != 1 || len(literal.Subjects) != 1 {
		t.Fatalf("fixtures must yield one subject each, got %#v and %#v", wildcard.Subjects, literal.Subjects)
	}
	if !globsIntersect([]byte(wildcard.Subjects[0]), []byte(literal.Subjects[0])) {
		t.Errorf("%q and %q must overlap: an org-wide StringLike swallows a StringEquals literal in it",
			wildcard.Subjects[0], literal.Subjects[0])
	}
}
