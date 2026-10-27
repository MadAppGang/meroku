package main

import (
	"fmt"
	"regexp"
	"strings"
)

// ecrImageURIRegex is a copy of local.ecr_image_uri_regex in
// modules/github_policy/main.tf. The two must stay identical: edit both.
//
// Terraform decides the grant from its copy; this one only decides whether to
// warn. Captures: account, region, repo. "*" and "?" cannot match the repo part,
// and the aws-cn host (amazonaws.com.cn) does not match at all.
var ecrImageURIRegex = regexp.MustCompile(`^([0-9]{12})\.dkr\.ecr\.([a-z0-9-]+)\.amazonaws\.com/([a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*)(?::[A-Za-z0-9_][A-Za-z0-9_.-]{0,127})?(?:@sha256:[a-f0-9]+)?$`)

// parseECRImageURI splits a private ECR image URI into its account, region and
// repository name, dropping any tag and digest. ok is false for anything that is
// not a private ECR URI in the aws partition.
func parseECRImageURI(uri string) (account, region, repo string, ok bool) {
	m := ecrImageURIRegex.FindStringSubmatch(uri)
	if m == nil {
		return "", "", "", false
	}
	return m[1], m[2], m[3], true
}

// githubExternalImageWarnings reports a backend_external_docker_image that the
// GitHub Actions role could be granted but is not: an ECR repository in this
// account and region, with OIDC on and github_push_external_image unset. CI
// pushing to it would fail with AccessDenied.
//
// Images in another account or region, and non-ECR images, get no grant and no
// warning. Nor does a repository the project's own prefix grant already covers
// (<project>_backend, <project>_service_*, <project>_task_*). Without OIDC no
// role is created, so there is nothing to warn about.
// Returned rather than printed so that a test can read them, like
// computeWarnings.
func githubExternalImageWarnings(envMap map[string]interface{}) []string {
	workload, ok := yamlSubMap(envMap, "workload")
	if !ok {
		return nil
	}
	if oidc, _ := workload["enable_github_oidc"].(bool); !oidc {
		return nil
	}
	if push, _ := workload["github_push_external_image"].(bool); push {
		return nil
	}

	account, region, repo, ok := parseECRImageURI(stringFromMap(workload, "backend_external_docker_image"))
	if !ok {
		return nil
	}
	if account != stringFromMap(envMap, "account_id") || region != stringFromMap(envMap, "region") {
		return nil
	}
	// The prefixes local.github_ecr_local_repository_arns in
	// modules/github_policy/main.tf already grants, flag or no flag.
	if project := stringFromMap(envMap, "project"); project != "" &&
		(repo == project+"_backend" ||
			strings.HasPrefix(repo, project+"_service_") ||
			strings.HasPrefix(repo, project+"_task_")) {
		return nil
	}

	return []string{fmt.Sprintf(
		"workload.backend_external_docker_image: CI cannot push to %s; set github_push_external_image if CI builds this image",
		repo,
	)}
}
