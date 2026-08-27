package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
)

// The Go half of the naming rule that modules/naming implements in Terraform.
//
// Two halves exist because name construction happens in two places that cannot
// share code. Most AWS names are built inside modules/**.tf, which only ever
// see var.project and var.env, so the algorithm has to run in Terraform. But
// env/main.hbs builds SQS and SNS names from the environment YAML, and that is
// rendered by this process before Terraform is ever invoked — no Terraform
// module can reach those.
//
// The two implementations must agree. They are pinned together by a shared
// vector table: app/testdata/aws_name_vectors.json is read by TestAWSName here
// and by modules/naming/tests/cascade.tftest.hcl, so a change to one half that
// the other does not follow fails a test rather than silently producing two
// different names for the same input.
//
// The cascade, identical to modules/naming/main.tf:
//
//	1. legacy, verbatim, when it fits          -- never rename live infrastructure
//	2. project + parts + env                   -- drop decoration, keep identity
//	3. parts + 8-char digest                   -- collapse project and env
//
// suffix (".fifo", an S3 bucket postfix) is counted against the budget and
// never truncated: a FIFO queue whose name loses ".fifo" is rejected by AWS.

const awsNameDigestLen = 8

// awsName renders one AWS resource name that is guaranteed to fit limit.
//
// legacy is the exact string this resource is named today, supplied by the
// caller and not derived here. That matches modules/naming, and it has to:
// there is no single historical template to derive. env/main.hbs has always
// rendered "project-env-identity" for queues, while every Terraform module
// renders "project-identity-env", and several put env in the middle. The only
// way to leave a deployed resource's name alone is for whoever knows what it
// was called to say so. Pass "" for a resource that does not exist yet, and the
// cascade starts at form 2.
//
// parts is the identity, most significant first, without project or env.
func awsName(project, env string, parts []string, limit int, separator, suffix, legacy string) string {
	if separator == "" {
		separator = "-"
	}

	// Form 1: whatever this resource is already called.
	if legacy != "" && len(legacy) <= limit {
		return legacy
	}

	// Form 2: the same fields with the decoration gone.
	full := strings.Join(append(append([]string{project}, parts...), env), separator) + suffix
	if len(full) <= limit {
		return full
	}

	// Form 3: identity plus a digest standing in for project and env. "|" cannot
	// appear in an AWS name, so ["a-b"] and ["a","b"] hash differently.
	sum := md5.Sum([]byte(strings.Join(append([]string{project, env}, parts...), "|")))
	digest := hex.EncodeToString(sum[:])[:awsNameDigestLen]

	head := strings.Join(parts, separator)
	budget := limit - awsNameDigestLen - len(separator) - len(suffix)
	if budget < 1 {
		// Nothing readable fits. The digest and the suffix still have to.
		return digest + suffix
	}
	if len(head) > budget {
		head = head[:budget]
		// Never end the readable head on the separator: AWS rejects a name with
		// a doubled or trailing hyphen in several services.
		head = strings.TrimRight(head, separator)
	}
	return head + separator + digest + suffix
}

// legacyName reproduces the form env/main.hbs has always rendered for these
// resources: project, then env, THEN the identity.
//
// Note the ordering. Every Terraform module puts env last, and modules/naming's
// form 2 follows them; this template put env second and always has. That is
// exactly why legacy is a separate form rather than something derived: the two
// halves of this algorithm agree on the cascade, not on a single template, and
// the only way to leave a deployed queue's name alone is to reproduce whatever
// its own generator used to emit.
func legacyName(project, env string, parts []string, separator, suffix string) string {
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(append([]string{project, env}, parts...), separator) + suffix
}

// awsNameSuffix returns the suffix a queue or topic needs, given whether it is
// FIFO. Kept separate so the template does not have to spell ".fifo" out.
func awsNameSuffix(fifo interface{}) string {
	if isTruthy(fifo) {
		return ".fifo"
	}
	return ""
}

// awsNameParts drops empty segments so a template can pass a fixed arity and
// still express "no second part".
func awsNameParts(in ...string) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

// awsNameLimits is the table the Handlebars helper validates against, so a
// template cannot quietly ask for a limit AWS does not actually enforce.
var awsNameLimits = map[string]int{
	"lb":              32,
	"lb_target_group": 32,
	"apprunner":       40,
	"s3_bucket":       63,
	"rds":             63,
	"iam_role":        64,
	"lambda":          64,
	"event_rule":      64,
	"scheduler":       64,
	"sqs_queue":       80,
	"iam_policy":      128,
	"sns_topic":       256,
}

func awsNameLimitFor(kind string) (int, error) {
	if l, ok := awsNameLimits[kind]; ok {
		return l, nil
	}
	return 0, fmt.Errorf("unknown AWS resource kind %q; add it to awsNameLimits in app/aws_name.go", kind)
}
