package main

import (
	"regexp"
	"strings"
	"testing"
)

// These render env/main.hbs and check what the postgres module receives. The
// module decides the DB subnet group from default_vpc and private_subnets, so a
// template that forgets to pass them leaves a standard instance in the account's
// default VPC again.

// postgresModuleBlock returns the body of `module "postgres" { ... }`.
func postgresModuleBlock(t *testing.T, rendered string) string {
	t.Helper()

	start := strings.Index(rendered, `module "postgres" {`)
	if start < 0 {
		t.Fatal(`module "postgres" not found in the rendered template`)
	}
	rest := rendered[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatal(`module "postgres" block is unterminated`)
	}
	return rest[:end]
}

// postgresPlacementOverlay is a standard (non-Aurora) postgres section with the
// given extra keys, in an env whose VPC choice is useDefaultVPC.
func postgresPlacementOverlay(useDefaultVPC bool, egress string, extra map[string]interface{}) map[string]interface{} {
	pg := map[string]interface{}{
		"enabled":                             true,
		"aurora":                              false,
		"dbname":                              "myapp",
		"username":                            "dbadmin",
		"engine_version":                      "17",
		"public_access":                       false,
		"iam_database_authentication_enabled": false,
		"multi_az":                            false,
		"storage_encrypted":                   true,
		"deletion_protection":                 false,
		"skip_final_snapshot":                 true,
	}
	for k, v := range extra {
		pg[k] = v
	}
	overlay := map[string]interface{}{
		"use_default_vpc": useDefaultVPC,
		"postgres":        pg,
	}
	if egress != "" {
		overlay["egress_strategy"] = egress
	}
	return overlay
}

var (
	defaultVPCTrueLine     = regexp.MustCompile(`(?m)^\s*default_vpc\s*=\s*true\s*$`)
	defaultVPCFalseLine    = regexp.MustCompile(`(?m)^\s*default_vpc\s*=\s*false\s*$`)
	privateSubnetsTrueLine = regexp.MustCompile(`(?m)^\s*private_subnets\s*=\s*true\s*$`)
	privateSubnetIDsLine   = regexp.MustCompile(`(?m)^\s*private_subnet_ids\s*=\s*module\.vpc\.private_subnet_ids\s*$`)
	anyPrivateSubnetIDs    = regexp.MustCompile(`(?m)^\s*private_subnet_ids\s*=`)
)

// The bug's shape: own VPC, standard instance. The module must be told it is
// not in the default VPC so it builds a subnet group from the env's subnets.
func TestTemplate_PostgresOwnVPCPassesDefaultVPCFalse(t *testing.T) {
	block := postgresModuleBlock(t, renderMainHBS(t, postgresPlacementOverlay(false, "public_ip", nil)))

	if !defaultVPCFalseLine.MatchString(block) {
		t.Errorf("an own-VPC env must render `default_vpc = false` into module \"postgres\"; got:\n%s", block)
	}
	if anyPrivateSubnetIDs.MatchString(block) {
		t.Errorf("without postgres.private_subnets no private_subnet_ids line may be rendered; got:\n%s", block)
	}
}

// A default-VPC env keeps the account's default subnet group, so an existing
// database there sees no plan change.
func TestTemplate_PostgresDefaultVPCPassesDefaultVPCTrue(t *testing.T) {
	block := postgresModuleBlock(t, renderMainHBS(t, postgresPlacementOverlay(true, "", nil)))

	if !defaultVPCTrueLine.MatchString(block) {
		t.Errorf("a default-VPC env must render `default_vpc = true` into module \"postgres\"; got:\n%s", block)
	}
	if anyPrivateSubnetIDs.MatchString(block) {
		t.Errorf("a default-VPC env has no private subnets to pass; got:\n%s", block)
	}
}

func TestTemplate_PostgresPrivateSubnetsPassesPrivateSubnetIDs(t *testing.T) {
	overlay := postgresPlacementOverlay(false, "nat_gateway", map[string]interface{}{"private_subnets": true})
	block := postgresModuleBlock(t, renderMainHBS(t, overlay))

	if !privateSubnetsTrueLine.MatchString(block) {
		t.Errorf("postgres.private_subnets: true must render `private_subnets = true`; got:\n%s", block)
	}
	if !privateSubnetIDsLine.MatchString(block) {
		t.Errorf("postgres.private_subnets: true must render `private_subnet_ids = module.vpc.private_subnet_ids`; got:\n%s", block)
	}
	if !defaultVPCFalseLine.MatchString(block) {
		t.Errorf("an own-VPC env must render `default_vpc = false`; got:\n%s", block)
	}
}

func TestTemplate_PostgresPrivateSubnetsFalseRendersNoPrivateLines(t *testing.T) {
	overlay := postgresPlacementOverlay(false, "nat_gateway", map[string]interface{}{"private_subnets": false})
	block := postgresModuleBlock(t, renderMainHBS(t, overlay))

	if anyPrivateSubnetIDs.MatchString(block) || privateSubnetsTrueLine.MatchString(block) {
		t.Errorf("postgres.private_subnets: false must render no private placement lines; got:\n%s", block)
	}
}
