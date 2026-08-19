package recommend

import "strings"

// Rule R10: instance-family SUITABILITY, as an allowlist.
//
// ---------------------------------------------------------------------------
// The failure this exists to stop
// ---------------------------------------------------------------------------
//
// On the real 903-type ap-southeast-2 catalog, posture=cost-first returned
// inf1.24xlarge as the top recommendation for an ordinary web fleet. The
// arithmetic was not wrong: 96 vCPU / 192 GiB packs more task slots than
// anything else at that price, so it won cost-per-task-slot outright. The
// ANSWER was wrong, because inf1 is an AWS Inferentia ML-inference appliance
// and nobody sizing a web pool wants one.
//
// None of the existing rules could see it. Verified against the live API:
//
//	type            GpuInfo   AcceleratorInfo   InstanceStorageSupported
//	g5.xlarge       A10G      null              true
//	inf1.24xlarge   null      null              false
//	inf2.xlarge     null      null              false
//	i4i.xlarge      null      null              true
//
// Inferentia is invisible to DescribeInstanceTypes: it reports neither GpuInfo
// nor AcceleratorInfo, so FR-21.5's gpuCount == 0 test passes it through. The
// only signal the API gives is the family prefix in the type NAME.
//
// ---------------------------------------------------------------------------
// Why an allowlist and not a denylist
// ---------------------------------------------------------------------------
//
// A denylist fails OPEN. Every family AWS ships next -- and it ships several a
// year -- is silently eligible until somebody notices it being recommended,
// which is to say until it has already been recommended. A denylist is a list
// of mistakes already made.
//
// An allowlist fails CLOSED. A new family is ineligible until someone opts it
// in, and the cost of being wrong is a type that does not appear rather than a
// type that appears and should not. For a system whose failure mode is
// confident wrongness -- it always returns an answer, always with a
// plausible-sounding reason -- fail-closed is the only defensible direction.
//
// The keys are the family LETTER, not the full prefix, so a new generation of
// an allowed family (m8g, c8i, r9a) is eligible the day it lists, while a new
// CATEGORY is not. Generations are the thing that changes often and safely;
// categories are the thing that changes rarely and dangerously.
//
// ---------------------------------------------------------------------------
// What is eligible, and what is not
// ---------------------------------------------------------------------------
//
// This recommender sizes general container workloads onto an ECS pool. The
// families that do that job are the general-purpose, compute-optimised,
// memory-optimised and burstable ones:
//
//	m -- general purpose        (m7i, m7a, m7g, m8g, m7i-flex, m6id, ...)
//	c -- compute optimised      (c7i, c7g, c7a, c8g, c7gd, ...)
//	r -- memory optimised       (r7i, r7a, r7g, r8g, r7iz, r6idn, ...)
//	t -- burstable              (t3, t3a, t4g, ...)
//
// Everything else is excluded unless the operator names the exact type in the
// pool's instance_types. In particular:
//
//	inf, trn, dl, p, g, vt -- accelerators. The workload has to be written for
//	                          the accelerator; sizing a web pool onto one buys
//	                          silicon that will idle at full price.
//	i, d, im, is, h        -- storage optimised. Priced for the local NVMe,
//	                          which an ECS task gets no benefit from unless it
//	                          was built to use it.
//	x, z, u                -- AWS files these under "memory optimized" too, and
//	                          they are still excluded. They are memory
//	                          APPLIANCES: x2i/x2gd for in-memory databases,
//	                          u-*tb1 for SAP HANA, z1d for high-clock
//	                          licensing. The identical density arithmetic that
//	                          made inf1.24xlarge win a general fleet makes
//	                          x2gd.16xlarge win a memory-heavy one. r covers
//	                          memory-heavy containers at 8 GiB/vCPU, which is
//	                          the top of the range any container fleet observed
//	                          through this tool has ever asked for.
//	hpc                    -- HPC, no internet path by design, EFA-oriented.
//	f                      -- FPGA.
//	mac                    -- dedicated-host Mac, not schedulable by ECS/EC2 at
//	                          all in the way this pool assumes.
//
// An UNRECOGNISED prefix is ineligible, and that is the point of the whole
// mechanism rather than an oversight. The Miss it produces names
// RuleFamilyNotEligible, so a user who wonders where a type went can see that
// it was skipped for its family and not for a capacity reason.
//
// ---------------------------------------------------------------------------
// Hard constraint, not a weight
// ---------------------------------------------------------------------------
//
// R10 EXCLUDES. It is not a score adjustment, because a down-weighted inf1
// still wins as soon as the fleet is dense enough to pay for the down-weight
// -- and "dense enough" is a property of the user's workload, not of the
// rule. There is no weight that makes a wrong answer stop being available.
//
// It runs in preFilter, and it runs FIRST among the substantive rules, for two
// reasons that are not stylistic:
//
//  1. catalogRatioRange (pipeline step 3) is taken over the preFilter
//     survivors, and C-10 clamps R_eff into it on the argument that the range
//     is "the ratios that can actually be purchased". If ineligible families
//     were still in the range, R_eff could be clamped to a shape no eligible
//     candidate has, and C-10's guarantee -- that at least one candidate sits
//     near fit 1.0 -- would be false.
//  2. "not a machine this tool sizes" is a more honest exclusion reason than
//     "previous generation" or "unpriced", both of which inf1 would also trip
//     on some catalogs. The first rule to fire is the one the Miss reports.
var eligibleFamilies = map[string]bool{
	"m": true,
	"c": true,
	"r": true,
	"t": true,
}

// familyOf is the family letter R10 tests, parsed from the type name.
//
// It delegates to ParseFamilyGeneration, which stops at the first character
// that is not a lowercase letter. That handles every shape the catalog
// actually carries:
//
//	"m7i-flex.large"    -> "m"    (stops at '7'; the "-flex" suffix is past it)
//	"c7g.medium"        -> "c"
//	"r7iz.large"        -> "r"
//	"t4g.small"         -> "t"
//	"inf1.24xlarge"     -> "inf"
//	"im4gn.large"       -> "im"
//	"u-6tb1.56xlarge"   -> "u"    (stops at '-', so the family is "u" and not
//	                               "u-6tb1"; generation parses as 0)
//	"mac2-m2.metal"     -> "mac"
//	"hpc7a.48xlarge"    -> "hpc"
//	""                  -> ""     (ineligible, which is fail-closed)
//
// Note the collisions that do NOT happen, because the parse consumes the whole
// alphabetic run rather than one character: "mac" never reads as "m", "trn"
// never as "t", "inf" and "im" and "is" never as "i" being eligible (i is not
// eligible either), "c" is never confused with anything.
func familyOf(instanceType string) string {
	family, _ := ParseFamilyGeneration(instanceType)
	return family
}

// pinnedTypeSet is the operator's explicit opt-in: the exact type names listed
// in the pool's instance_types.
//
// The opt-in is by TYPE and not by family on purpose. Writing
// instance_types: [inf1.24xlarge] says "size my pool against this machine,
// I know what it is"; it does not say "and also consider inf1.6xlarge and
// inf2.48xlarge", which the operator never named and may not want. The
// narrower reading is the one that cannot surprise anybody.
//
// Names are lowercased on both sides: EC2 type names are lowercase, but this
// list is hand-edited YAML and "M7i.large" is a typo that should widen the
// search rather than silently fail to.
func pinnedTypeSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	set := make(map[string]bool, len(names))
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" {
			continue
		}
		set[n] = true
	}
	return set
}

// familyEligible is rule R10 itself: the type's family is on the allowlist, or
// the operator named this exact type in the pool's instance_types.
func familyEligible(instanceType string, pinned map[string]bool) bool {
	if pinned[strings.ToLower(instanceType)] {
		return true
	}
	return eligibleFamilies[familyOf(instanceType)]
}
