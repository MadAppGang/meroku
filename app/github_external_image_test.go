package main

import (
	"strings"
	"testing"
)

// REGRESSION: MTL-288 GitHub Actions role cannot push to an external backend image repository — Fixed in /dev:fix session dev-fix-20261028-002336-a4ff689a
//
// The deploy role is granted an in-account external backend repository only
// when workload.github_push_external_image is set (modules/github_policy). A
// user who sets backend_external_docker_image to such a repository and does not
// set the flag gets a CI pipeline that fails at the push with AccessDenied. The
// warning tested here tells them so at generation time.
//
// parseECRImageURI is the Go copy of the image grammar in
// modules/github_policy/main.tf. It only decides whether to WARN; the grant is
// decided in Terraform. The vectors are the ones checked in terraform console
// (root-cause.md, Fix Approach item 1). All account IDs are synthetic.

const testDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestParseECRImageURI(t *testing.T) {
	tests := []struct {
		name        string
		uri         string
		wantOK      bool
		wantAccount string
		wantRegion  string
		wantRepo    string
	}{
		// --- private ECR URIs that parse ---
		{
			name:        "repository with a tag",
			uri:         "000000000000.dkr.ecr.us-east-1.amazonaws.com/acme-ext:v1",
			wantOK:      true,
			wantAccount: "000000000000",
			wantRegion:  "us-east-1",
			wantRepo:    "acme-ext",
		},
		{
			name:        "repository with a digest",
			uri:         "000000000000.dkr.ecr.us-east-1.amazonaws.com/team/api@" + testDigest,
			wantOK:      true,
			wantAccount: "000000000000",
			wantRegion:  "us-east-1",
			wantRepo:    "team/api",
		},
		{
			name:        "repository with a tag and a digest",
			uri:         "000000000000.dkr.ecr.us-east-1.amazonaws.com/team/api:v1@" + testDigest,
			wantOK:      true,
			wantAccount: "000000000000",
			wantRegion:  "us-east-1",
			wantRepo:    "team/api",
		},
		{
			name:        "repository with a slash and no tag",
			uri:         "000000000000.dkr.ecr.us-east-1.amazonaws.com/team/legacy-api",
			wantOK:      true,
			wantAccount: "000000000000",
			wantRegion:  "us-east-1",
			wantRepo:    "team/legacy-api",
		},
		{
			name:        "another account still parses",
			uri:         "111111111111.dkr.ecr.eu-west-1.amazonaws.com/shared/acme-api:v1",
			wantOK:      true,
			wantAccount: "111111111111",
			wantRegion:  "eu-west-1",
			wantRepo:    "shared/acme-api",
		},

		// --- not a private ECR URI in the aws partition ---
		{name: "docker hub image", uri: "nginx:latest", wantOK: false},
		{name: "public ecr image", uri: "public.ecr.aws/nginx/nginx:latest", wantOK: false},
		{name: "empty string", uri: "", wantOK: false},
		{name: "aws-cn host", uri: "000000000000.dkr.ecr.cn-north-1.amazonaws.com.cn/shared/acme-api:v1", wantOK: false},
		{name: "wildcard in repository name", uri: "000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_*:x", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account, region, repo, ok := parseECRImageURI(tt.uri)
			if ok != tt.wantOK {
				t.Fatalf("parseECRImageURI(%q) ok = %v, want %v (got account=%q region=%q repo=%q)",
					tt.uri, ok, tt.wantOK, account, region, repo)
			}
			if !tt.wantOK {
				return
			}
			if account != tt.wantAccount || region != tt.wantRegion || repo != tt.wantRepo {
				t.Errorf("parseECRImageURI(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tt.uri, account, region, repo, tt.wantAccount, tt.wantRegion, tt.wantRepo)
			}
		})
	}
}

// externalImageEnvFixture is an environment that should warn: OIDC on, an
// in-account in-region ECR image, and no flag. Each case changes one thing.
func externalImageEnvFixture(workloadOverrides map[string]interface{}, topOverrides map[string]interface{}) map[string]interface{} {
	workload := map[string]interface{}{
		"enable_github_oidc":            true,
		"backend_external_docker_image": "000000000000.dkr.ecr.us-east-1.amazonaws.com/shared/acme-api:v1",
	}
	for k, v := range workloadOverrides {
		workload[k] = v
	}
	env := map[string]interface{}{
		"project":    "acme",
		"env":        "dev",
		"account_id": "000000000000",
		"region":     "us-east-1",
		"workload":   workload,
	}
	for k, v := range topOverrides {
		env[k] = v
	}
	return env
}

func TestGithubExternalImageWarnings(t *testing.T) {
	tests := []struct {
		name     string
		envMap   map[string]interface{}
		wantWarn bool
		wantRepo string // repository named in the warning; "" means the fixture's shared/acme-api
	}{
		{
			name:     "same account and region without the flag warns",
			envMap:   externalImageEnvFixture(nil, nil),
			wantWarn: true,
		},
		{
			name:     "an explicit false flag warns",
			envMap:   externalImageEnvFixture(map[string]interface{}{"github_push_external_image": false}, nil),
			wantWarn: true,
		},
		{
			name: "a workload section as raw yaml.v2 map still warns",
			envMap: map[string]interface{}{
				"account_id": "000000000000",
				"region":     "us-east-1",
				"workload": map[interface{}]interface{}{
					"enable_github_oidc":            true,
					"backend_external_docker_image": "000000000000.dkr.ecr.us-east-1.amazonaws.com/shared/acme-api:v1",
				},
			},
			wantWarn: true,
		},
		{
			name:     "the flag set gives no warning",
			envMap:   externalImageEnvFixture(map[string]interface{}{"github_push_external_image": true}, nil),
			wantWarn: false,
		},
		{
			name: "an image in another account gives no warning",
			envMap: externalImageEnvFixture(map[string]interface{}{
				"backend_external_docker_image": "111111111111.dkr.ecr.us-east-1.amazonaws.com/shared/acme-api:v1",
			}, nil),
			wantWarn: false,
		},
		{
			name: "an image in another region gives no warning",
			envMap: externalImageEnvFixture(map[string]interface{}{
				"backend_external_docker_image": "000000000000.dkr.ecr.eu-west-1.amazonaws.com/shared/acme-api:v1",
			}, nil),
			wantWarn: false,
		},
		{
			name: "the project's own backend repository gives no warning, because the prefix grant covers it",
			envMap: externalImageEnvFixture(map[string]interface{}{
				"backend_external_docker_image": "000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_backend:v1",
			}, nil),
			wantWarn: false,
		},
		{
			name: "a project service repository gives no warning, because the prefix grant covers it",
			envMap: externalImageEnvFixture(map[string]interface{}{
				"backend_external_docker_image": "000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_service_api:v1",
			}, nil),
			wantWarn: false,
		},
		{
			name: "a project task repository gives no warning, because the prefix grant covers it",
			envMap: externalImageEnvFixture(map[string]interface{}{
				"backend_external_docker_image": "000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_task_cleanup:v1",
			}, nil),
			wantWarn: false,
		},
		{
			name: "another project whose name starts with this one still warns",
			envMap: externalImageEnvFixture(map[string]interface{}{
				"backend_external_docker_image": "000000000000.dkr.ecr.us-east-1.amazonaws.com/acmecorp_backend:v1",
			}, nil),
			wantWarn: true,
			wantRepo: "acmecorp_backend",
		},
		{
			name:     "a non-ECR image gives no warning",
			envMap:   externalImageEnvFixture(map[string]interface{}{"backend_external_docker_image": "nginx:latest"}, nil),
			wantWarn: false,
		},
		{
			name:     "an empty image gives no warning",
			envMap:   externalImageEnvFixture(map[string]interface{}{"backend_external_docker_image": ""}, nil),
			wantWarn: false,
		},
		{
			name:     "OIDC disabled gives no warning, because no role is created",
			envMap:   externalImageEnvFixture(map[string]interface{}{"enable_github_oidc": false}, nil),
			wantWarn: false,
		},
		{
			name: "OIDC key absent gives no warning",
			envMap: map[string]interface{}{
				"account_id": "000000000000",
				"region":     "us-east-1",
				"workload": map[string]interface{}{
					"backend_external_docker_image": "000000000000.dkr.ecr.us-east-1.amazonaws.com/shared/acme-api:v1",
				},
			},
			wantWarn: false,
		},
		{
			name:     "no workload section gives no warning",
			envMap:   map[string]interface{}{"account_id": "000000000000", "region": "us-east-1"},
			wantWarn: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := githubExternalImageWarnings(tt.envMap)

			if !tt.wantWarn {
				if len(warnings) != 0 {
					t.Fatalf("expected no warnings, got %q", warnings)
				}
				return
			}

			if len(warnings) != 1 {
				t.Fatalf("expected exactly one warning, got %d: %q", len(warnings), warnings)
			}
			wantRepo := tt.wantRepo
			if wantRepo == "" {
				wantRepo = "shared/acme-api"
			}
			w := warnings[0]
			for _, want := range []string{
				"CI cannot push to " + wantRepo,
				"set github_push_external_image if CI builds this image",
			} {
				if !strings.Contains(w, want) {
					t.Errorf("warning %q does not contain %q", w, want)
				}
			}
		})
	}
}
