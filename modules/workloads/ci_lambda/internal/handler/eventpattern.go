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

	// SSMOperationUpdate: Create is Terraform's own parameter creation and
	// Delete removes the configuration a service needs. Neither is a deploy.
	SSMOperationUpdate = "Update"

	// DetailTypeDeploy / DetailTypeServiceDeploy are what the manual deploy
	// generators emit. Handle routes manual deploys on the detail-type because
	// the set of custom sources has changed over time.
	DetailTypeDeploy        = "DEPLOY"
	DetailTypeServiceDeploy = "SERVICE_DEPLOY"
)

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
				ssm["Operation"]: {SSMOperationUpdate},
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
