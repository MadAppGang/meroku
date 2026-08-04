package boundary_test

// The other half of the Terraform <-> Go boundary.
//
// boundary_test.go checks the *contents* of the maps Terraform ships. Nothing
// checked the *names* those maps arrive under, the field names inside them, the
// arguments the identifiers module is called with, or the event patterns that
// decide whether the Lambda is invoked at all. Every one of those is a channel
// where Terraform and Go can disagree in complete silence.
//
// Concretely, before these tests: renaming ECR_REPO_MAP in lambda.tf to
// anything at all left config.Load reading "", decodeMap substituting "{}",
// Validate ignoring an empty ECRRepos, SelfCheck iterating nothing, and every
// ECR push logged as "ECR repository is not mapped to any target in this
// project" with status=ignored and error=nil. Auto-deploy 100% dead, every log
// line reading "working as designed", every test green. That is D1 again, one
// layer up.

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"madappgang.com/infrastructure/ci_lambda/internal/config"
	"madappgang.com/infrastructure/ci_lambda/internal/handler"
)

// runtimeProvidedEnv are variables the Lambda runtime sets, so lambda.tf must
// NOT declare them (Lambda rejects reserved keys in a function's environment).
var runtimeProvidedEnv = map[string]bool{"AWS_REGION": true}

// ---------------------------------------------------------------- env names

// lambdaTFEnvNames returns the keys of the
// `environment { variables { ... } }` block of aws_lambda_function.lambda_deploy.
func lambdaTFEnvNames(t *testing.T) map[string]bool {
	t.Helper()

	src := lambdaTF(t)
	block := regexp.MustCompile(`(?s)environment\s*\{\s*variables\s*=\s*\{(.*?)\n    \}`).FindStringSubmatch(src)
	require.Len(t, block, 2, "the environment { variables { ... } } block was not found in lambda.tf")

	names := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s*([A-Z][A-Z0-9_]*)\s*=`).FindAllStringSubmatch(block[1], -1) {
		names[m[1]] = true
	}
	require.NotEmpty(t, names, "no environment variable names parsed out of lambda.tf")
	return names
}

// configGetenvNames returns every name config.Load reads, and the subset it
// reads through decodeMap.
//
// Read out of the AST of the real source file rather than from a list kept
// alongside it: a list is one more thing that can be updated in one place and
// not the other, which is the failure mode under test.
func configGetenvNames(t *testing.T) (all, maps map[string]bool) {
	t.Helper()

	path := filepath.Join("..", "config", "config.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	require.NoError(t, err)

	all, maps = map[string]bool{}, map[string]bool{}

	literal := func(e ast.Expr) (string, bool) {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			return "", false
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "getenv" || len(call.Args) != 1 {
			return "", false
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			return "", false
		}
		return s, true
	}

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if name, ok := literal(call); ok {
			all[name] = true
			return true
		}
		// decodeMap(getenv("X"), &dst, "X")
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "decodeMap" && len(call.Args) > 0 {
			if name, ok := literal(call.Args[0]); ok {
				all[name] = true
				maps[name] = true
			}
		}
		return true
	})

	require.NotEmpty(t, all, "no getenv(...) calls found in config.go")
	require.NotEmpty(t, maps, "no decodeMap(getenv(...)) calls found in config.go")
	return all, maps
}

// TestEnvironmentVariableNamesAgree is the name-level boundary gate.
//
// Renaming a variable on either side is a silent, total loss of one event
// source: Terraform stops emitting the name Go reads, Go reads "", decodeMap
// substitutes "{}", and the map is empty. Only ECS_SERVICE_MAP, PROJECT_NAME,
// PROJECT_ENV and ECS_CLUSTER_NAME fail closed through Validate; every other
// variable fails open and silent.
func TestEnvironmentVariableNamesAgree(t *testing.T) {
	inTerraform := lambdaTFEnvNames(t)
	read, _ := configGetenvNames(t)

	for name := range read {
		if runtimeProvidedEnv[name] {
			require.Falsef(t, inTerraform[name],
				"lambda.tf declares %s, which the Lambda runtime provides and reserves", name)
			continue
		}
		require.Truef(t, inTerraform[name],
			"config.Load reads %s but lambda.tf does not emit it: the Lambda will read \"\" and "+
				"either fail closed at startup or, for a map, silently resolve nothing forever", name)
	}

	for name := range inTerraform {
		require.Truef(t, read[name],
			"lambda.tf emits %s but config.Load never reads it: either it was renamed on one side "+
				"only, or it is dead configuration", name)
	}
}

// TestGoldenFixtureCoversEveryFailOpenMap keeps the golden capture honest.
//
// The maps are the variables that fail open — an empty one is indistinguishable
// from "nothing is configured for this source". A fixture that stops carrying
// one silently stops testing that whole event source.
func TestGoldenFixtureCoversEveryFailOpenMap(t *testing.T) {
	g, _ := loadGolden(t)
	inTerraform := lambdaTFEnvNames(t)
	_, mapVars := configGetenvNames(t)

	for name := range mapVars {
		require.Containsf(t, g.Env, name,
			"the golden capture does not carry %s, so nothing exercises that event source", name)
		require.NotEmptyf(t, g.Env[name], "%s is empty in the golden capture", name)
	}

	for name := range g.Env {
		require.Truef(t, inTerraform[name],
			"the golden capture carries %s, which lambda.tf does not emit: the fixture has stopped "+
				"mirroring the real file", name)
	}
}

// ---------------------------------------------------------------- map fields

// TestMapFieldNamesAgree covers the JSON field names *inside* the maps.
//
// service_name and task_family fail closed through Validate. bucket, key and
// type do not: renaming `bucket` makes every S3 trigger resolve to nothing, and
// renaming `type` makes every scheduled task look like a service and get
// deployed with UpdateService against a service that does not exist.
func TestMapFieldNamesAgree(t *testing.T) {
	g, _ := loadGolden(t)
	src := lambdaTF(t)

	targetTags := jsonTagsOf(t, config.Target{})
	fileTags := jsonTagsOf(t, config.S3File{})

	// lambda.tf writes the target attributes by hand.
	for _, tag := range targetTags {
		require.Regexpf(t, `(?m)^\s*`+regexp.QuoteMeta(tag)+`\s*=`, src,
			"config.Target reads JSON field %q but lambda.tf never writes an attribute of that name", tag)
	}

	// The S3 attributes come from local.all_env_files_s3, so lambda.tf reads
	// them rather than writing them.
	for _, tag := range fileTags {
		requirePresent(t, src, "each.value."+tag,
			"config.S3File reads JSON field "+tag+", so the env-file objects must carry it")
	}

	// And the shipped maps really are shaped that way.
	requireObjectFields(t, g.Env["ECS_SERVICE_MAP"], targetTags, "ECS_SERVICE_MAP")
	requireObjectFields(t, g.Env["SCHEDULED_TASK_MAP"], targetTags, "SCHEDULED_TASK_MAP")

	var s3 map[string][]map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(g.Env["S3_SERVICE_MAP"]), &s3))
	require.NotEmpty(t, s3)
	for id, files := range s3 {
		for _, f := range files {
			require.ElementsMatchf(t, fileTags, keysOfRaw(f),
				"S3_SERVICE_MAP[%q] entry does not match the JSON tags of config.S3File", id)
		}
	}
}

// requireObjectFields asserts every field of every object in a JSON map is one
// the Go struct reads. An unknown field is silently dropped by encoding/json,
// which is exactly how a rename disappears.
func requireObjectFields(t *testing.T, raw string, allowed []string, name string) {
	t.Helper()

	var m map[string]map[string]json.RawMessage
	require.NoErrorf(t, json.Unmarshal([]byte(raw), &m), "%s is not a JSON object map", name)
	require.NotEmptyf(t, m, "%s is empty", name)

	for id, obj := range m {
		for field := range obj {
			require.Containsf(t, allowed, field,
				"%s[%q] carries field %q, which no field of config.Target reads; encoding/json "+
					"drops it without a word", name, id, field)
		}
	}
}

// ---------------------------------------------------------------- module call

// Two spaces exactly: a module block's own arguments, never the keys of a map
// passed to one (which terraform fmt indents by four).
var moduleArg = regexp.MustCompile(`(?m)^  ([a-z][a-z0-9_]*)\s*=`)

// moduleArgs returns the argument names of a module block.
func moduleArgs(t *testing.T, src, name string) []string {
	t.Helper()

	block := regexp.MustCompile(`(?s)module "` + name + `" \{(.*?)\n\}`).FindStringSubmatch(src)
	require.Lenf(t, block, 2, "module %q not found", name)

	var out []string
	for _, m := range moduleArg.FindAllStringSubmatch(block[1], -1) {
		if m[1] == "source" {
			continue
		}
		out = append(out, m[1])
	}
	sort.Strings(out)
	require.NotEmptyf(t, out, "module %q has no arguments", name)
	return out
}

// TestIdentifierModuleInputsAgree covers the inputs to module.ci_identifiers.
//
// Those locals are what the whole identifier set is computed from, and every
// one of them is try(..., "")-guarded while tf_identifiers drops empty entries.
// A dropped or renamed input therefore removes a repository from ECR_REPO_MAP
// *and* from the ECR rule's allow-list at the same time, consistently, so
// SelfCheck stays clean and the boundary test still passes with fewer entries.
//
// The golden fixture calls the same module by hand, so it can silently stop
// exercising an input. Requiring the three declarations — the module's own
// variables, the real call, and the fixture's call — to name the same set is
// what keeps them together.
func TestIdentifierModuleInputsAgree(t *testing.T) {
	realCall := moduleArgs(t, lambdaTF(t), "ci_identifiers")

	fixture, err := os.ReadFile(filepath.Join("testdata", "tfgolden", "main.tf"))
	require.NoError(t, err)
	fixtureCall := moduleArgs(t, commentLine.ReplaceAllString(string(fixture), ""), "ids")

	require.ElementsMatch(t, realCall, fixtureCall,
		"lambda.tf and the golden fixture do not call tf_identifiers with the same arguments; "+
			"the fixture has stopped mirroring the real call and can no longer prove anything about it")

	declared := declaredVariables(t, filepath.Join("..", "..", "tf_identifiers", "variables.tf"))
	require.ElementsMatch(t, declared, realCall,
		"lambda.tf does not pass every variable tf_identifiers declares; an omitted input falls back "+
			"to its default and silently drops entries from ECR_REPO_MAP and from the ECR event pattern")
}

func declaredVariables(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)

	var out []string
	for _, m := range regexp.MustCompile(`(?m)^variable "([a-z0-9_]+)"`).FindAllStringSubmatch(string(b), -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	require.NotEmpty(t, out)
	return out
}

// ---------------------------------------------------------------- patterns

// TestEventPatternsMatchWhatTheHandlerParses covers the event patterns.
//
// Nothing evaluated one before. The ECR action-type/result values, the SSM
// operation, the S3 requestParameters field names and every source string are
// asserted in handler_test.go only against events the test itself writes, so a
// pattern that stops matching what the handler parses is invisible from both
// sides: EventBridge simply never invokes the Lambda, no code path runs, no
// error is logged, and no test fails.
//
// handler.PatternContracts() derives its field names from the struct tags of
// the types that parse these events, so this compares lambda.tf against the
// parsers themselves rather than against a restatement of them.
func TestEventPatternsMatchWhatTheHandlerParses(t *testing.T) {
	src := lambdaTF(t)

	// Where each rule's pattern actually lives. The ECR rule picks between two
	// prebuilt locals at 2,048 characters, and BOTH have to satisfy the
	// contract — the fallback is the one that only appears on large projects,
	// which is exactly where nobody is watching.
	patternText := map[string][]string{
		"ci_ecr_push": {
			jsonencodeLocal(t, src, "ci_ecr_pattern_explicit"),
			jsonencodeLocal(t, src, "ci_ecr_pattern_prefix"),
		},
		"ci_ecs_state":            {inlinePattern(t, src, "ci_ecs_state")},
		"ci_ssm_change":           {inlinePattern(t, src, "ci_ssm_change")},
		"s3_env_file_change_rule": {inlinePattern(t, src, "s3_env_file_change_rule")},
	}

	for _, c := range handler.PatternContracts() {
		t.Run(c.Rule, func(t *testing.T) {
			bodies, ok := patternText[c.Rule]
			require.Truef(t, ok, "handler.PatternContracts names rule %q, which this test cannot locate "+
				"in lambda.tf; add it rather than dropping the assertion", c.Rule)

			for i, body := range bodies {
				require.Containsf(t, body, `"`+c.Source+`"`,
					"rule %q (pattern %d) must select source %q, which is what handler.Handle routes on",
					c.Rule, i, c.Source)

				for field, values := range c.DetailFields {
					require.Regexpf(t, `"?`+regexp.QuoteMeta(field)+`"?\s*[:=]`, body,
						"rule %q (pattern %d) does not filter on detail field %q, which is the JSON tag "+
							"the handler parses", c.Rule, i, field)
					for _, v := range values {
						require.Regexpf(t,
							`"?`+regexp.QuoteMeta(field)+`"?\s*[:=]\s*\[[^]]*"`+regexp.QuoteMeta(v)+`"`, body,
							"rule %q (pattern %d) must select %s = %q; the handler ignores anything else, "+
								"so a pattern that excludes this value means the Lambda is never invoked "+
								"and nothing anywhere reports a problem", c.Rule, i, field, v)
					}
				}
			}
		})
	}
}

// inlinePattern returns the jsonencode({...}) body of a rule's event_pattern.
func inlinePattern(t *testing.T, src, rule string) string {
	t.Helper()
	block := ruleBlock(t, src, rule)
	body, ok := balanced(block, "event_pattern = jsonencode({")
	require.Truef(t, ok, "rule %q has no inline event_pattern = jsonencode({ ... })", rule)
	return body
}

// jsonencodeLocal returns the jsonencode({...}) body assigned to a local.
func jsonencodeLocal(t *testing.T, src, name string) string {
	t.Helper()
	body, ok := balanced(src, name+" = jsonencode({")
	require.Truef(t, ok, "local.%s is not a jsonencode({ ... }) in lambda.tf", name)
	return body
}

// balanced returns the text between the brace that opens at the end of `after`
// and its matching close.
func balanced(src, after string) (string, bool) {
	i := strings.Index(src, after)
	if i < 0 {
		return "", false
	}
	start := i + len(after)
	depth := 1
	for j := start; j < len(src); j++ {
		switch src[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start:j], true
			}
		}
	}
	return "", false
}

// TestManualDetailTypesAgree pins the manual path, which routes on the
// detail-type rather than the source.
func TestManualDetailTypesAgree(t *testing.T) {
	src := lambdaTF(t)

	for _, rule := range []string{"ci_manual_deploy", "ci_manual_deploy_global"} {
		block := ruleBlock(t, src, rule)
		for _, dt := range []string{handler.DetailTypeDeploy, handler.DetailTypeServiceDeploy} {
			require.Containsf(t, block, `"`+dt+`"`,
				"rule %q must accept detail-type %q: handler.Handle routes manual deploys on it", rule, dt)
		}
	}
}

// ---------------------------------------------------------------- helpers

// jsonTagsOf returns the JSON field names a struct actually decodes. Taking
// them from the type means these tests compare Terraform against the Go code
// that reads it, not against a second description of the Go code.
func jsonTagsOf(t *testing.T, v any) []string {
	t.Helper()

	typ := reflect.TypeOf(v)
	out := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		tag := f.Tag.Get("json")
		require.NotEmptyf(t, tag, "%s.%s has no json tag, so its wire name is its Go name by accident",
			typ.Name(), f.Name)
		if comma := strings.IndexByte(tag, ','); comma >= 0 {
			tag = tag[:comma]
		}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func keysOfRaw(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
