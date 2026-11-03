package main

// VPC address-range rules, the pool new environments draw from, and overlap
// detection. Pure: no I/O, no AWS, no YAML. Everything that reads the world
// lives in vpc_cidr_sources.go; everything that decides what to do with the
// answer lives in vpc_cidr_service.go.
//
// Why this exists: every custom-VPC environment meroku created used to get the
// literal 10.0.0.0/16. Two such environments can never be peered or attached to
// one Transit Gateway, because AWS refuses overlapping ranges — and a VPC's
// primary range cannot be changed afterwards, so the only fix is replacing the
// VPC and everything inside it. See ai_docs/VPC_CIDR_ALLOCATION.md.

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

const (
	// vpcCIDRMinBits and vpcCIDRMaxBits bound the VPC size meroku accepts.
	// /16 is the largest VPC AWS allows. /19 is the smallest the vpc module can
	// carve its subnets from (cidrsubnet(vpc, 8, n)) while each subnet stays
	// at /27 — the ALB minimum. The Terraform module accepts down to /20 so that
	// `plan` never locks out a VPC that already exists; Go is stricter because
	// it only judges values somebody is about to deploy.
	vpcCIDRMinBits = 16
	vpcCIDRMaxBits = 19

	// terraformDefaultVPCCIDR is what modules/vpc builds when vpc_cidr is not
	// set. It stays as it is: changing the module default would replace the VPC
	// of every environment that never wrote the key.
	terraformDefaultVPCCIDR = "10.0.0.0/16"
)

// Stable error codes. The web card switches on these, so they are part of the
// API contract (ai-docs api-contract.md) and must never be renamed.
const (
	vpcCIDRCodeInvalidFormat     = "invalid_format"
	vpcCIDRCodeNotNetworkAddress = "not_network_address"
	vpcCIDRCodeReservedRange     = "reserved_range"
	vpcCIDRCodeNotPrivate        = "not_private"
	vpcCIDRCodeSize              = "size"
)

// vpcCIDRError is a rule failure for one vpc_cidr value.
//
// It carries two texts because it has two audiences: Message is technical and
// goes to the CLI and logs; Hint is plain language for the web card, whose
// users may not know what a CIDR is. Fix, when set, is a corrected value the
// card can offer as a one-click "Use this".
type vpcCIDRError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
	Fix     string `json:"fix,omitempty"`
}

func (e *vpcCIDRError) Error() string { return e.Message }

// vpcCIDRUse is one range already taken by something meroku should not clash
// with: a sibling environment or a VPC in the AWS account.
type vpcCIDRUse struct {
	// Source identifies the owner for technical output and the API's
	// warning.source: `env "staging"`, or a generic label for AWS networks.
	Source string
	// Name is the plain-language owner shown in hints: "staging", or
	// "another network in this AWS account". Never a VPC id or name.
	Name string
	// File is the sibling's YAML file name, "" for AWS uses.
	File   string
	Prefix netip.Prefix
}

// vpcCIDRWarning is an overlap. Overlap is a warning, never an error: two
// environments that will never be connected may share a range on purpose.
type vpcCIDRWarning struct {
	Message string `json:"message"`
	Hint    string `json:"hint"`
	Source  string `json:"source"`
}

// errVPCCIDRPoolExhausted means every /16 in the pool overlaps something.
var errVPCCIDRPoolExhausted = errors.New("no free /16 left in the 10.0.0.0/8 pool: every candidate overlaps an existing environment or VPC")

// awsNetworkLabel is how an AWS VPC is named in warnings and the API. Generic
// on purpose: the web API is served with permissive CORS, so VPC ids and Name
// tags from the account stay out of it.
const awsNetworkLabel = "another network in this AWS account"

type reservedVPCRange struct {
	prefix netip.Prefix
	reason string // technical, for Message
	hint   string // plain, for Hint
}

var (
	rfc1918Ranges = []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
	}

	// reservedVPCRanges are refused outright. Checked before the private-range
	// rule so a value inside one gets the specific reason, not a generic one.
	reservedVPCRanges = []reservedVPCRange{
		{
			prefix: netip.MustParsePrefix("172.16.0.0/12"),
			reason: "172.16.0.0/12 is used by Docker's default bridge (172.17.0.0/16) and address pools, the AWS default VPC (172.31.0.0/16), and Cloud9/SageMaker",
			hint:   "This range is used by Docker and AWS internally — pick one of the suggestions",
		},
		{
			prefix: netip.MustParsePrefix("169.254.0.0/16"),
			reason: "169.254.0.0/16 is link-local; ECS serves task metadata and credentials from it",
			hint:   "This range is used by AWS internally — pick one of the suggestions",
		},
		{
			prefix: netip.MustParsePrefix("198.19.0.0/16"),
			reason: "198.19.0.0/16 is restricted by AWS for VPC ranges",
			hint:   "This range is reserved by AWS — pick one of the suggestions",
		},
	}

	// vpcCIDRPoolExclusions are valid ranges the random pick never offers.
	// 10.0.0.0/15 is the default of every comparable tool (and blocks secondary
	// ranges from 10.0.0.0/16 on AWS); 10.99.0.0/16 is Porter's default.
	vpcCIDRPoolExclusions = []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/15"),
		netip.MustParsePrefix("10.99.0.0/16"),
	}

	// vpcCIDRPool is every 10.N.0.0/16 not excluded above: 253 ranges.
	vpcCIDRPool = buildVPCCIDRPool()
)

func buildVPCCIDRPool() []netip.Prefix {
	pool := make([]netip.Prefix, 0, 256)
	for n := 0; n < 256; n++ {
		p := netip.PrefixFrom(netip.AddrFrom4([4]byte{10, byte(n), 0, 0}), 16)
		excluded := false
		for _, ex := range vpcCIDRPoolExclusions {
			if ex.Overlaps(p) {
				excluded = true
				break
			}
		}
		if !excluded {
			pool = append(pool, p)
		}
	}
	return pool
}

// validateVPCCIDR checks one vpc_cidr value against meroku's rules, in order:
// IPv4 format, reserved ranges, private ranges, size, network address.
//
// The range rules run on the masked value first, so the network-address error
// (the only one carrying a Fix) is reported only when the corrected value
// passes every other rule. A Fix is therefore always a value that validates:
// 172.20.1.0/16 reports reserved_range, never "did you mean 172.20.0.0/16".
//
// It returns the parsed prefix on success. Every failure is a *vpcCIDRError.
func validateVPCCIDR(s string) (netip.Prefix, error) {
	value := strings.TrimSpace(s)

	p, err := netip.ParsePrefix(value)
	if err != nil || !p.Addr().Is4() {
		return netip.Prefix{}, &vpcCIDRError{
			Code:    vpcCIDRCodeInvalidFormat,
			Message: fmt.Sprintf("vpc_cidr %q is not an IPv4 CIDR block (expected something like 10.42.0.0/16)", value),
			Hint:    "This isn't a network range meroku understands. It should look like 10.42.0.0/16 — or pick one of the suggestions",
		}
	}

	masked := p.Masked()

	for _, r := range reservedVPCRanges {
		if r.prefix.Overlaps(masked) {
			return netip.Prefix{}, &vpcCIDRError{
				Code:    vpcCIDRCodeReservedRange,
				Message: fmt.Sprintf("vpc_cidr %s overlaps a reserved range: %s", value, r.reason),
				Hint:    r.hint,
			}
		}
	}

	if !isPrivateVPCRange(masked) {
		return netip.Prefix{}, &vpcCIDRError{
			Code:    vpcCIDRCodeNotPrivate,
			Message: fmt.Sprintf("vpc_cidr %s is not inside a private range (RFC 1918: 10.0.0.0/8 or 192.168.0.0/16; 172.16.0.0/12 is reserved)", value),
			Hint:    "This is a public internet address, not a private one — pick one of the suggestions",
		}
	}

	if masked.Bits() < vpcCIDRMinBits || masked.Bits() > vpcCIDRMaxBits {
		return netip.Prefix{}, &vpcCIDRError{
			Code: vpcCIDRCodeSize,
			Message: fmt.Sprintf("vpc_cidr %s has prefix length /%d; it must be between /%d and /%d (/16 is the AWS maximum, /19 keeps every subnet at /27 or larger for the ALB)",
				value, masked.Bits(), vpcCIDRMinBits, vpcCIDRMaxBits),
			Hint: "This range is the wrong size for your services — pick one of the suggestions",
		}
	}

	// Every rule above passed for masked, so it is safe to offer as the fix.
	if masked != p {
		return netip.Prefix{}, &vpcCIDRError{
			Code: vpcCIDRCodeNotNetworkAddress,
			Message: fmt.Sprintf("vpc_cidr %s is not a network address: the host bits must be zero (did you mean %s?)",
				value, masked),
			Hint: fmt.Sprintf("Use %s instead — a range has to start at its first address", masked),
			Fix:  masked.String(),
		}
	}

	return p, nil
}

// isPrivateVPCRange reports whether p lies entirely inside one RFC 1918 range.
func isPrivateVPCRange(p netip.Prefix) bool {
	for _, r := range rfc1918Ranges {
		if r.Bits() <= p.Bits() && r.Contains(p.Addr()) {
			return true
		}
	}
	return false
}

// effectiveVPCCIDR is the range Terraform will actually build. An empty value
// means the module default; the default VPC means no meroku range at all ("").
func effectiveVPCCIDR(useDefaultVPC bool, cidr string) string {
	if useDefaultVPC {
		return ""
	}
	if v := strings.TrimSpace(cidr); v != "" {
		return v
	}
	return terraformDefaultVPCCIDR
}

// pickVPCCIDR picks one free /16 uniformly from the pool. intn is
// math/rand/v2's IntN in production and a seeded source in tests.
func pickVPCCIDR(intn func(int) int, used []vpcCIDRUse) (netip.Prefix, error) {
	picks, err := pickVPCCIDRs(intn, used, 1)
	if err != nil {
		return netip.Prefix{}, err
	}
	return picks[0], nil
}

// pickVPCCIDRs draws up to n distinct free /16s from the pool without
// replacement (partial Fisher–Yates). It returns fewer than n only when fewer
// are free, and errVPCCIDRPoolExhausted when none are. n < 1 returns nothing.
func pickVPCCIDRs(intn func(int) int, used []vpcCIDRUse, n int) ([]netip.Prefix, error) {
	if n < 1 {
		return []netip.Prefix{}, nil
	}

	free := make([]netip.Prefix, 0, len(vpcCIDRPool))
	for _, p := range vpcCIDRPool {
		if len(overlappingUses(p, used)) == 0 {
			free = append(free, p)
		}
	}
	if len(free) == 0 {
		return nil, errVPCCIDRPoolExhausted
	}

	k := min(n, len(free))
	for i := 0; i < k; i++ {
		j := i + intn(len(free)-i)
		free[i], free[j] = free[j], free[i]
	}
	return free[:k], nil
}

// overlappingUses returns the uses whose range overlaps p.
func overlappingUses(p netip.Prefix, uses []vpcCIDRUse) []vpcCIDRUse {
	var out []vpcCIDRUse
	for _, u := range uses {
		if u.Prefix.IsValid() && p.Overlaps(u.Prefix) {
			out = append(out, u)
		}
	}
	return out
}

// vpcCIDROverlapWarnings turns every use overlapping p into a warning. Always
// non-nil, so it encodes as [] rather than null.
func vpcCIDROverlapWarnings(p netip.Prefix, uses []vpcCIDRUse) []vpcCIDRWarning {
	warnings := []vpcCIDRWarning{}
	for _, u := range overlappingUses(p, uses) {
		where := u.Source
		if u.File != "" {
			where = fmt.Sprintf("%s (%s)", u.Source, u.File)
		}
		hint := fmt.Sprintf("Clashes with %s — you won't be able to connect these two later", u.Name)
		if u.File == "" {
			hint = fmt.Sprintf("Clashes with %s — you won't be able to connect them later", u.Name)
		}
		warnings = append(warnings, vpcCIDRWarning{
			Message: fmt.Sprintf("%s overlaps %s %s", p, where, u.Prefix),
			Hint:    hint,
			Source:  u.Source,
		})
	}
	return warnings
}

// siblingVPCCIDRUse builds the use for another environment's YAML.
func siblingVPCCIDRUse(envName, file string, p netip.Prefix) vpcCIDRUse {
	return vpcCIDRUse{
		Source: fmt.Sprintf("env %q", envName),
		Name:   envName,
		File:   file,
		Prefix: p,
	}
}

// awsVPCCIDRUse builds the use for a VPC found in the AWS account.
func awsVPCCIDRUse(p netip.Prefix) vpcCIDRUse {
	return vpcCIDRUse{Source: awsNetworkLabel, Name: awsNetworkLabel, Prefix: p}
}

// siblingAccountVPCCIDRUse builds the use for a VPC found in the AWS account
// of another environment (named by its file stem).
func siblingAccountVPCCIDRUse(envName string, p netip.Prefix) vpcCIDRUse {
	label := fmt.Sprintf("another network in %s's AWS account", envName)
	return vpcCIDRUse{Source: label, Name: label, Prefix: p}
}

// vpcCIDRInputIncomplete reports whether a value being typed is not yet a
// whole range: no "/" yet, nothing after it, or fewer than four dot-separated
// numbers. The web card shows "keep typing" for these instead of an error.
func vpcCIDRInputIncomplete(s string) bool {
	value := strings.TrimSpace(s)
	addr, bits, found := strings.Cut(value, "/")
	if !found || strings.TrimSpace(bits) == "" {
		return true
	}
	parts := strings.Split(addr, ".")
	if len(parts) < 4 {
		return true
	}
	for _, part := range parts {
		if part == "" {
			return true
		}
	}
	return false
}
