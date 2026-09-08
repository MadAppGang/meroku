package handler

import (
	"reflect"
	"strings"
)

// The event field values this package routes and filters on.
//
// They are constants rather than inline literals so the boundary test can
// assert that modules/workloads/lambda.tf's event patterns select exactly these
// values. A pattern that stops matching what these parsers read is otherwise
// invisible on both sides: EventBridge simply never invokes the Lambda, no code
// path runs, and no test notices.
const (
	SourceECR = "aws.ecr"
	SourceECS = "aws.ecs"
	SourceSSM = "aws.ssm"
	SourceS3  = "aws.s3"

	// ECRActionTypePush / ECRResultSuccess are re-checked in ecr.go even though
	// the rule filters on them, because an event can also arrive from a
	// hand-made rule.
	ECRActionTypePush = "PUSH"
	ECRResultSuccess  = "SUCCESS"

	// ECRMutableTag is the tag the ci_ecr_push rule must NOT act on.
	//
	// Every pipeline this repo generates pushes two tags per build — an
	// immutable one and this — so one build emits two ECR events. That was
	// harmless while a service deploy was an idempotent UpdateService(family),
	// and stopped being harmless when an ECR push began registering a revision
	// that PINS the pushed image: two events then register two revisions, and
	// the one carrying this tag pins a reference that can point somewhere else
	// tomorrow. A revision that cannot say which image it runs is not a
	// rollback point, which is the whole reason the pin exists.
	//
	// It is a constant here, and a literal inside lambda.tf's two ECR patterns,
	// for the same reason SSMOperationDelete is: a PatternContract can only
	// require that a value IS selected, so the exclusion is pinned separately
	// by internal/boundary.TestECRRuleExcludesTheMutableTag against this
	// constant. Change the tag here and that test names the Terraform.
	ECRMutableTag = "latest"

	// The Parameter Store operations, all three of them, because two of the
	// three are decisions rather than omissions.
	SSMOperationCreate = "Create"
	SSMOperationUpdate = "Update"
	SSMOperationDelete = "Delete"

	// DetailTypeDeploy / DetailTypeServiceDeploy are what the manual deploy
	// generators emit. Handle routes manual deploys on the detail-type because
	// the set of custom sources has changed over time.
	DetailTypeDeploy        = "DEPLOY"
	DetailTypeServiceDeploy = "SERVICE_DEPLOY"

	// SourceTerraformPrefix is what aws_lambda_invocation.{backend,services}_revision
	// puts in the `source` field. See TerraformInvocationSource.
	SourceTerraformPrefix = "terraform."
)

// TerraformInvocationSource is the event source that means "`terraform apply`
// invoked this Lambda directly", as opposed to EventBridge delivering somebody
// else's DEPLOY event.
//
// It is a discriminator, not decoration. manual() promotes a request carrying
// it to deploy.SourceTerraform, which is the only source allowed to poll
// through an IAM propagation race — see deploy.awaitingPropagation for why that
// permission cannot be given to the EventBridge-delivered sources.
//
// What makes it trustworthy is that no rule in lambda.tf accepts it:
// local.ci_manual_sources_scoped is action.{env} / github.actions.{env} (plus
// action.production in a production environment) and
// local.ci_manual_sources_global is action.deploy. An event on a "terraform.*"
// source therefore cannot arrive through EventBridge at all; it arrived by a
// direct RequestResponse Invoke, which in this module is aws_lambda_invocation
// and nothing else — a caller that is already blocked waiting for the answer,
// with no asynchronous redelivery behind it.
//
// One derivation, two readers: this function and the `source` argument of the
// two aws_lambda_invocation resources. internal/boundary asserts they agree,
// because a rename on either side would switch the poll off in silence — the
// deploy would simply go back to failing the first-apply race and reporting
// success.
func TerraformInvocationSource(env string) string { return SourceTerraformPrefix + env }

// SSMDeployOperations are the Parameter Store operations that can mean an
// operator changed this project's configuration.
//
// Create and Update are the SAME action. Both are PutParameter — with and
// without Overwrite — and Parameter Store picks between them purely on whether
// the name already existed. meroku writes parameters that way itself
// (app/api_ssm.go), so ADDING a variable emits Create and only ever Create, and
// routing the two differently meant the one operation that adds configuration
// was the one nothing listened for.
//
// The cost of admitting Create, stated rather than left to be discovered:
// adding a parameter now causes TWO rolling restarts where there used to be
// none. The Create arrives at PutParameter time, before any apply, so the
// deployment it triggers lands on the revision the service is ALREADY running —
// a restart that changes nothing. The apply that follows registers revision N+1
// carrying the new `secrets` entry, and aws_lambda_invocation.*_revision
// (services.tf, backend.tf) deploys that one. Only the second restart delivers
// the parameter.
//
// Create earns its place anyway, on the case the second restart cannot cover.
// Delete-then-recreate: the revision the service is running still lists the
// parameter in `secrets`, so its tasks cannot resolve a secret and are failing
// right now, and the Create that restores the value is exactly the moment a
// restart is warranted. No apply need follow that sequence — the rendered
// content is unchanged, so Terraform registers no revision and the A1 edge never
// fires. Without Create the service stays broken until somebody notices.
//
// SSMOperationDelete is deliberately absent, and its absence is load-bearing. A
// deployment cannot restore a deleted parameter; it makes the loss fatal. The
// revision the service is running still lists the parameter in `secrets`, so
// every task launched after the delete fails on ResourceInitializationError —
// redeploying replaces the tasks that were still working with ones that cannot
// start. Terraform's next apply removes the entry from the list, and it is the
// only thing that can. Because a PatternContract can only assert the PRESENCE
// of a value, that absence is pinned separately by
// internal/boundary.TestSSMRuleExcludesDelete.
//
// Two readers, one slice. ssm() ranges over it to decide whether an event
// deploys, and PatternContracts() below publishes it as what lambda.tf's
// ci_ssm_change rule must select. Deriving both from one value is what makes
// eventpattern.go, ssm.go and lambda.tf impossible to change independently: a
// rule that stops matching what the handler reads is otherwise invisible from
// both sides — EventBridge simply never invokes the Lambda, no code path runs,
// and nothing anywhere reports a problem.
var SSMDeployOperations = []string{SSMOperationCreate, SSMOperationUpdate}

// PatternContract is one EventBridge rule as this package needs it to be.
type PatternContract struct {
	// Rule is the aws_cloudwatch_event_rule resource name in lambda.tf.
	Rule string
	// Source is the event source the router in Handle switches on.
	Source string
	// DetailFields are detail fields the rule's pattern names, mapped to the
	// values the pattern must select. An empty value list means the pattern
	// filters on the field by prefix or by value list without this package
	// caring which values — only that the field name is the one parsed here.
	//
	// The field names come from the struct tags of the types that parse these
	// events, so a renamed tag and a renamed pattern key cannot drift apart.
	DetailFields map[string][]string
}

// PatternContracts is what lambda.tf's event patterns must express for the
// parsers in this package to see anything at all.
//
// It is asserted against the real lambda.tf by
// internal/boundary.TestLambdaTFEventPatternsMatchTheHandler.
func PatternContracts() []PatternContract {
	ecr := jsonTags(ecrDetail{})
	ssm := jsonTags(ssmDetail{})
	s3 := jsonTags(s3Detail{})
	s3req := jsonTags(s3Detail{}.RequestParameters)

	return []PatternContract{
		{
			Rule:   "ci_ecr_push",
			Source: SourceECR,
			DetailFields: map[string][]string{
				ecr["RepositoryName"]: nil, // the project's repository allow-list
				ecr["ActionType"]:     {ECRActionTypePush},
				ecr["Result"]:         {ECRResultSuccess},
				// The tag is filtered by exclusion, so no value can be required
				// here — only that the rule filters on the field this package
				// parses at all. What must be excluded is ECRMutableTag, pinned
				// by internal/boundary.TestECRRuleExcludesTheMutableTag.
				ecr["Tag"]: nil,
			},
		},
		{
			Rule:         "ci_ecs_state",
			Source:       SourceECS,
			DetailFields: nil, // scoped by the `resources` ARN prefix, not by detail
		},
		{
			Rule:   "ci_ssm_change",
			Source: SourceSSM,
			DetailFields: map[string][]string{
				ssm["Name"]:      nil, // the project's parameter path prefix
				ssm["Operation"]: SSMDeployOperations,
			},
		},
		{
			Rule:   "s3_env_file_change_rule",
			Source: SourceS3,
			DetailFields: map[string][]string{
				s3["EventName"]:         nil, // PutObject / DeleteObject
				s3["RequestParameters"]: nil,
				s3req["BucketName"]:     nil,
				s3req["Key"]:            nil,
			},
		},
	}
}

// jsonTags maps a struct's field names to their JSON tag names.
func jsonTags(v any) map[string]string {
	t := reflect.TypeOf(v)
	out := make(map[string]string, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" {
			tag = f.Name
		}
		if comma := strings.IndexByte(tag, ','); comma >= 0 {
			tag = tag[:comma]
		}
		out[f.Name] = tag
	}
	return out
}
