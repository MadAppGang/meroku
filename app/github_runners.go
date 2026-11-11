package main

import (
	"fmt"
	"regexp"
	"strings"
)

// CodeBuild compute types a GitHub Actions runner may use. These are the
// on-demand EC2 sizes; Lambda compute and reserved fleets are not offered.
var gitHubRunnerComputeTypes = []string{
	"BUILD_GENERAL1_SMALL",
	"BUILD_GENERAL1_MEDIUM",
	"BUILD_GENERAL1_LARGE",
	"BUILD_GENERAL1_XLARGE",
	"BUILD_GENERAL1_2XLARGE",
}

const (
	// CodeBuild's own limits on build_timeout, in minutes.
	gitHubRunnerMinTimeout = 5
	gitHubRunnerMaxTimeout = 2160
	// The runner name is part of the CodeBuild project name, the log group and
	// the IAM role parts, so it is kept short.
	gitHubRunnerMaxNameLen = 40
)

var (
	gitHubRunnerNamePattern    = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
	gitHubRepositoryPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)
	gitHubOrganizationPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)
	gitHubRunnerEnvNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// validateGitHubRunnersConfigMap checks the github_runners block at generate
// time, on the map applyTemplate renders from. It runs on every generate, so a
// later change elsewhere in the file (egress_strategy back to public_ip,
// use_default_vpc turned on) is refused by the same check that accepted the
// block in the first place.
//
// Absent or `enabled: false` is not checked: neither renders anything.
func validateGitHubRunnersConfigMap(envMap map[string]interface{}) error {
	block, ok := yamlSubMap(envMap, "github_runners")
	if !ok {
		return nil
	}
	if enabled, _ := computeBoolField(block, "enabled"); !enabled {
		return nil
	}

	var problems []string
	if computeStringField(block, "token_secret") == "" {
		problems = append(problems,
			"token_secret is required: the name of the Secrets Manager secret holding the GitHub token CodeBuild uses (see ai_docs/GITHUB_RUNNERS.md for its format)")
	}

	runners := computeMapList(block, "runners")
	if len(runners) == 0 {
		problems = append(problems, "runners must list at least one runner")
	}

	seen := map[string]bool{}
	for i, runner := range runners {
		problems = append(problems, gitHubRunnerProblems(i, runner, seen)...)
	}

	if vpcAccess, _ := computeBoolField(block, "vpc_access"); vpcAccess {
		if problem := gitHubRunnerVPCProblem(envMap); problem != "" {
			problems = append(problems, problem)
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid github_runners settings:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}

// gitHubRunnerProblems checks one runner. seen carries the names of the
// runners before it, for the uniqueness check.
func gitHubRunnerProblems(index int, runner map[string]interface{}, seen map[string]bool) []string {
	var problems []string

	name := computeStringField(runner, "name")
	label := fmt.Sprintf("runner #%d", index+1)
	switch {
	case name == "":
		problems = append(problems, label+": name is required")
	case len(name) > gitHubRunnerMaxNameLen || !gitHubRunnerNamePattern.MatchString(name):
		problems = append(problems, fmt.Sprintf(
			"%s: name %q must be 1-%d characters of lower-case letters, digits and hyphens, starting and ending with a letter or digit",
			label, name, gitHubRunnerMaxNameLen))
	case seen[name]:
		problems = append(problems, fmt.Sprintf("%s: name %q is used by another runner; runner names must be unique", label, name))
	}
	if name != "" {
		seen[name] = true
		label = fmt.Sprintf("runner '%s'", name)
	}

	repository := computeStringField(runner, "repository")
	organization := computeStringField(runner, "organization")
	switch {
	case repository == "" && organization == "":
		problems = append(problems, label+": set exactly one of repository (owner/repo) or organization")
	case repository != "" && organization != "":
		problems = append(problems, label+": set only one of repository or organization, not both")
	case repository != "" && !gitHubRepositoryPattern.MatchString(repository):
		problems = append(problems, fmt.Sprintf("%s: repository %q must be owner/repo, e.g. madappgang/some-repo", label, repository))
	case organization != "" && !gitHubOrganizationPattern.MatchString(organization):
		problems = append(problems, fmt.Sprintf("%s: organization %q must be a GitHub organization name, e.g. madappgang", label, organization))
	}

	if computeType := computeStringField(runner, "compute_type"); computeType != "" && !containsString(gitHubRunnerComputeTypes, computeType) {
		problems = append(problems, fmt.Sprintf("%s: compute_type %q must be one of: %s",
			label, computeType, strings.Join(gitHubRunnerComputeTypes, ", ")))
	}

	if timeout := computeIntField(runner, "timeout_minutes"); timeout != nil && (*timeout < gitHubRunnerMinTimeout || *timeout > gitHubRunnerMaxTimeout) {
		problems = append(problems, fmt.Sprintf("%s: timeout_minutes %d must be between %d and %d",
			label, *timeout, gitHubRunnerMinTimeout, gitHubRunnerMaxTimeout))
	}

	return append(problems, gitHubRunnerVariableProblems(label, runner)...)
}

// gitHubRunnerVariableProblems checks env_variables and secrets together: they
// become one list of environment variables on the CodeBuild project, so a name
// may appear once across both.
func gitHubRunnerVariableProblems(label string, runner map[string]interface{}) []string {
	var problems []string
	names := map[string]string{} // name -> where it was first seen

	checkName := func(where, name string) {
		switch {
		case name == "":
			problems = append(problems, fmt.Sprintf("%s: %s entry has no name", label, where))
			return
		case !gitHubRunnerEnvNamePattern.MatchString(name):
			problems = append(problems, fmt.Sprintf("%s: %s name %q must be letters, digits and underscores, not starting with a digit", label, where, name))
		case strings.HasPrefix(strings.ToUpper(name), "CODEBUILD_"):
			problems = append(problems, fmt.Sprintf("%s: %s name %q starts with CODEBUILD_, which CodeBuild reserves for its own variables", label, where, name))
		}
		if first, dup := names[name]; dup {
			problems = append(problems, fmt.Sprintf("%s: %q is defined in %s and again in %s; each variable name may appear once across env_variables and secrets", label, name, first, where))
			return
		}
		names[name] = where
	}

	for _, variable := range computeMapList(runner, "env_variables") {
		checkName("env_variables", computeStringField(variable, "name"))
	}
	for _, secret := range computeMapList(runner, "secrets") {
		name := computeStringField(secret, "name")
		checkName("secrets", name)
		ssm := computeStringField(secret, "ssm")
		secretsManager := computeStringField(secret, "secrets_manager")
		if (ssm == "") == (secretsManager == "") {
			problems = append(problems, fmt.Sprintf(
				"%s: secret %q must set exactly one of ssm (a Parameter Store name) or secrets_manager (name-or-arn[:json-key])",
				label, name))
		}
	}
	return problems
}

// gitHubRunnerVPCProblem explains why vpc_access cannot work in this
// environment, or returns "" when it can.
//
// CodeBuild never gives a build in a VPC a public IP address, so the build
// reaches the internet — GitHub, package registries, the runner's own job
// queue — only through a NAT Gateway. Without one, every job sits queued until
// it times out. The default VPC has no private subnets and no NAT at all.
func gitHubRunnerVPCProblem(envMap map[string]interface{}) string {
	if useDefaultVPC, _ := envMap["use_default_vpc"].(bool); useDefaultVPC {
		return "vpc_access is true but use_default_vpc is true.\n" +
			"    CodeBuild in a VPC gets no public IP, so it needs private subnets behind a NAT Gateway\n" +
			"    to reach GitHub. The AWS default VPC has neither.\n" +
			"    Fix: set use_default_vpc: false and egress_strategy: nat_gateway (or nat_gateway_ha),\n" +
			"    or set github_runners.vpc_access: false."
	}

	strategy, _ := envMap["egress_strategy"].(string)
	if strategy == "" {
		strategy = string(EgressPublicIP)
	}
	if EgressStrategy(strategy) == EgressNATGateway || EgressStrategy(strategy) == EgressNATGatewayHA {
		return ""
	}
	return fmt.Sprintf("vpc_access is true but egress_strategy is %q.\n"+
		"    CodeBuild in a VPC gets no public IP, so without a NAT Gateway it has no internet access:\n"+
		"    it cannot reach GitHub, and every job stays queued until it times out.\n"+
		"    Fix: set egress_strategy: nat_gateway (or nat_gateway_ha), or set github_runners.vpc_access: false.",
		strategy)
}
