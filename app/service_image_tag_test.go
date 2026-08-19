package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Where a named service's container image tag comes from.
//
// modules/workloads/services.tf appended `:latest` OUTSIDE the ternary that
// chooses between the user's docker_image and the ECR URL meroku resolved:
//
//	image = "${each.value.docker_image != "" ? each.value.docker_image : local.service_ecr_urls[each.key]}:latest"
//
// so a service configured with `docker_image: public.ecr.aws/nginx/nginx:stable`
// was deployed as `public.ecr.aws/nginx/nginx:stable:latest` and every task died
// with `CannotPullImageManifestError: invalid reference format`. Observed live:
// the backend came up, the named service next to it never did. The three sibling
// paths (modules/workloads/variables.tf for the backend, modules/ecs_task/main.tf,
// modules/event_bridge_task/ecs.tf) all keep `:latest` inside the ECR branch;
// services.tf was the outlier.
//
// The tag is applied in Terraform, not in the template: main.hbs passes services
// through as opaque JSON (`services = {{{array services}}}`), so a render test can
// only prove the first half of the trip -- that a tagged docker_image reaches the
// module untouched. The second half is the HCL expression itself, which the rest
// of this file reads out of services.tf and evaluates for both branches.

// The template half: a docker_image with its own tag must arrive at the module
// call byte-for-byte, with nothing appended on the way.
func TestNamedServiceDockerImage_ReachesTheModuleVerbatim(t *testing.T) {
	const image = "public.ecr.aws/nginx/nginx:stable"

	overlay := map[string]interface{}{
		"services": []interface{}{
			map[string]interface{}{
				"name":           "worker",
				"enabled":        true,
				"docker_image":   image,
				"cpu":            256,
				"memory":         512,
				"desired_count":  1,
				"container_port": 3000,
				"host_port":      3000,
			},
		},
	}
	block := workloadsModuleBlock(t, renderMainHBS(t, overlay))

	if !strings.Contains(block, `"docker_image":"`+image+`"`) {
		t.Errorf("the module call does not carry docker_image %q verbatim:\n%s", image, block)
	}
	if strings.Contains(block, image+":latest") {
		t.Errorf("the template appended :latest to an already-tagged docker_image:\n%s", block)
	}
}

// serviceImageExprPath is the file and resource under test.
const serviceImageExprPath = "modules/workloads/services.tf"

// serviceContainerImageExpr returns the right-hand side of the `image =`
// argument in `resource "aws_ecs_task_definition" "services"`.
func serviceContainerImageExpr(t *testing.T) string {
	t.Helper()

	path := filepath.Join("..", "modules", "workloads", "services.tf")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", serviceImageExprPath, err)
	}

	src := string(body)
	start := strings.Index(src, `resource "aws_ecs_task_definition" "services" {`)
	if start < 0 {
		t.Fatalf(`%s no longer declares resource "aws_ecs_task_definition" "services"`, serviceImageExprPath)
	}
	rest := src[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatalf("%s: the services task definition block is unterminated", serviceImageExprPath)
	}
	block := rest[:end]

	// `^\s*image\s*=` cannot match the comment above it, which only mentions
	// docker_image in prose -- but pin the count anyway, so a second container
	// gaining its own image line makes this test say so instead of silently
	// testing whichever one came first.
	matches := regexp.MustCompile(`(?m)^[\t ]*image[\t ]*=[\t ]*(.+)$`).FindAllStringSubmatch(block, -1)
	if len(matches) != 1 {
		t.Fatalf("expected exactly one `image =` argument in the services task definition, found %d", len(matches))
	}
	return strings.TrimSpace(matches[0][1])
}

// Both branches of the image expression, evaluated the way Terraform would.
func TestNamedServiceImage_TagsOnlyTheResolvedECRURL(t *testing.T) {
	const ecrURL = "000000000000.dkr.ecr.us-east-1.amazonaws.com/myapp_service_worker"

	expr := serviceContainerImageExpr(t)

	tests := []struct {
		name        string
		dockerImage string
		want        string
	}{
		{
			// The regression. :stable:latest is the invalid reference that
			// killed the task.
			name:        "an explicitly tagged docker_image is used verbatim",
			dockerImage: "public.ecr.aws/nginx/nginx:stable",
			want:        "public.ecr.aws/nginx/nginx:stable",
		},
		{
			// Parity with modules/ecs_task and modules/event_bridge_task: a user
			// image is never retagged, tag or no tag. Docker's own :latest
			// default covers this one.
			name:        "an untagged docker_image is still used verbatim",
			dockerImage: "public.ecr.aws/nginx/nginx",
			want:        "public.ecr.aws/nginx/nginx",
		},
		{
			// The other branch, and the reason :latest is in the expression at
			// all: ecr.tf resolves a repository URL with no tag on it.
			name:        "no docker_image falls back to the resolved ECR URL, tagged :latest",
			dockerImage: "",
			want:        ecrURL + ":latest",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := evalServiceImageExpr(t, expr, map[string]string{
				"each.value.docker_image":          tc.dockerImage,
				"local.service_ecr_urls[each.key]": ecrURL,
			})
			if got != tc.want {
				t.Errorf("docker_image %q renders image %q, want %q\n  expression: %s",
					tc.dockerImage, got, tc.want, expr)
			}
		})
	}
}

// A tiny evaluator for the one expression shape services.tf uses. It exists so
// the assertions above are about the image string a task would actually be given,
// not about where the parentheses sit -- the buggy form evaluates cleanly here
// too, and produces ".../nginx:stable:latest".
//
// Grammar understood, which is all Terraform needs for this argument:
//
//	expr    := string | ternary | ref
//	ternary := expr "?" expr ":" expr
//	string  := '"' ( text | "${" expr "}" )* '"'
//	cond    := expr "!=" expr        (the only operator in play)
//
// Anything outside it fails the test with the offending fragment named, which is
// the correct outcome: an expression this cannot read is one nobody has proven.
func evalServiceImageExpr(t *testing.T, expr string, refs map[string]string) string {
	t.Helper()

	expr = strings.TrimSpace(expr)
	switch {
	case strings.HasPrefix(expr, `"`):
		return evalHCLString(t, expr, refs)
	case strings.Contains(expr, "?"):
		return evalHCLTernary(t, expr, refs)
	default:
		return lookupHCLRef(t, expr, refs)
	}
}

// evalHCLTernary evaluates `cond ? a : b`. The split points are the first
// top-level "?" and the first top-level ":" after it -- top-level meaning outside
// any quoted string, which is what keeps the ":latest" in a branch from being
// mistaken for the branch separator.
func evalHCLTernary(t *testing.T, expr string, refs map[string]string) string {
	t.Helper()

	q := indexTopLevel(expr, '?')
	if q < 0 {
		t.Fatalf("not a ternary: %s", expr)
	}
	cond, rest := expr[:q], expr[q+1:]

	c := indexTopLevel(rest, ':')
	if c < 0 {
		t.Fatalf("ternary has no branch separator: %s", expr)
	}
	if evalHCLCondition(t, cond, refs) {
		return evalServiceImageExpr(t, rest[:c], refs)
	}
	return evalServiceImageExpr(t, rest[c+1:], refs)
}

// evalHCLCondition handles `x != y`, the only comparison in this expression.
func evalHCLCondition(t *testing.T, cond string, refs map[string]string) bool {
	t.Helper()

	parts := strings.SplitN(cond, "!=", 2)
	if len(parts) != 2 {
		t.Fatalf("unsupported condition %q -- this test only models `x != y`", strings.TrimSpace(cond))
	}
	return evalServiceImageExpr(t, parts[0], refs) != evalServiceImageExpr(t, parts[1], refs)
}

// evalHCLString unquotes a string literal and expands its ${...} interpolations.
func evalHCLString(t *testing.T, expr string, refs map[string]string) string {
	t.Helper()

	if len(expr) < 2 || !strings.HasSuffix(expr, `"`) {
		t.Fatalf("unterminated string literal: %s", expr)
	}
	body := expr[1 : len(expr)-1]

	var out strings.Builder
	for {
		open := strings.Index(body, "${")
		if open < 0 {
			out.WriteString(body)
			return out.String()
		}
		out.WriteString(body[:open])

		depth, closeIdx := 0, -1
		for i := open + 1; i < len(body); i++ {
			switch body[i] {
			case '{':
				depth++
			case '}':
				if depth--; depth == 0 {
					closeIdx = i
				}
			}
			if closeIdx >= 0 {
				break
			}
		}
		if closeIdx < 0 {
			t.Fatalf("unterminated interpolation in %s", expr)
		}
		out.WriteString(evalServiceImageExpr(t, body[open+2:closeIdx], refs))
		body = body[closeIdx+1:]
	}
}

// lookupHCLRef resolves a bare reference. An unknown one is fatal rather than
// empty: services.tf rewritten to read a different local means this test is
// evaluating something it was never checked against.
func lookupHCLRef(t *testing.T, ref string, refs map[string]string) string {
	t.Helper()

	ref = strings.TrimSpace(ref)
	if v, ok := refs[ref]; ok {
		return v
	}
	t.Fatalf("%s references %q, which this test does not model -- teach evalServiceImageExpr about it",
		serviceImageExprPath, ref)
	return ""
}

// indexTopLevel finds b outside of any quoted string. No escape handling: the
// expression has no backslashes, and one appearing would show up as an
// unsupported-fragment failure rather than a wrong pass.
func indexTopLevel(s string, b byte) int {
	inString := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inString = !inString
		case b:
			if !inString {
				return i
			}
		}
	}
	return -1
}
