package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Static checks over modules/**/*.tf that keep events:PutEvents where
// modules/events puts it.
//
// modules/events grants PutEvents per events.produces entry, conditioned on
// detail type and source. IAM allows add up, so any broader grant on the same
// role defeats it without a single error: the workload can still publish
// anything to any bus. The AWS-managed policies CloudWatchFullAccess and
// AmazonEventBridgeFullAccess both carry events:*, which is how the task roles
// had it before.

// putEventsAllowed lists the files outside modules/events that may name
// events:PutEvents, and why. Neither grant is on a role a container assumes.
var putEventsAllowed = map[string]string{
	"workloads/ecr_events.tf": "the role EventBridge assumes to forward ECR push events to another account's default bus",
	"github_policy/main.tf":   "the GitHub Actions deploy role",
}

type tfAttachment struct {
	file, name, role, policyARN string
}

func moduleTFFiles(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "modules")
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".terraform" || d.Name() == "tests") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".tf") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking modules/: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no .tf files found under modules/")
	}
	return files
}

var (
	attachmentStart = regexp.MustCompile(`^resource "aws_iam_role_policy_attachment" "([^"]+)"`)
	attributeLine   = regexp.MustCompile(`^\s*(role|policy_arn)\s*=\s*(.+)$`)
)

// roleAttachments returns every aws_iam_role_policy_attachment in the modules
// with its role and policy_arn expressions. Blocks end at a `}` in column zero.
func roleAttachments(t *testing.T) []tfAttachment {
	t.Helper()
	var out []tfAttachment
	for _, path := range moduleTFFiles(t) {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(f)
		var current *tfAttachment
		for scanner.Scan() {
			line := scanner.Text()
			if m := attachmentStart.FindStringSubmatch(line); m != nil {
				current = &tfAttachment{file: path, name: m[1]}
				continue
			}
			if current == nil {
				continue
			}
			if line == "}" {
				out = append(out, *current)
				current = nil
				continue
			}
			if m := attributeLine.FindStringSubmatch(line); m != nil {
				if m[1] == "role" {
					current.role = strings.TrimSpace(m[2])
				} else {
					current.policyARN = strings.TrimSpace(m[2])
				}
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestModules_NoCloudWatchFullAccessOnATaskRole(t *testing.T) {
	for _, a := range roleAttachments(t) {
		if !strings.Contains(a.policyARN, "CloudWatchFullAccess") {
			continue
		}
		// An execution role is ECS's own; a container never sees its
		// credentials, so events:* there reaches no code of the user's.
		if !strings.Contains(a.role, "execution") {
			t.Errorf("%s: aws_iam_role_policy_attachment.%s attaches CloudWatchFullAccess to %s.\n"+
				"    It carries events:*, which defeats the per-entry PutEvents grant of modules/events.\n"+
				"    Grant the CloudWatch actions the container needs in an inline policy instead.",
				a.file, a.name, a.role)
		}
	}
}

func TestModules_EventBridgeFullAccessOnlyOnTheScheduler(t *testing.T) {
	for _, a := range roleAttachments(t) {
		if !strings.Contains(a.policyARN, "AmazonEventBridgeFullAccess") {
			continue
		}
		if !strings.Contains(a.role, "scheduler") {
			t.Errorf("%s: aws_iam_role_policy_attachment.%s attaches AmazonEventBridgeFullAccess to %s, which is not the scheduler's role.",
				a.file, a.name, a.role)
		}
	}
}

func TestModules_PutEventsOnlyFromModuleEvents(t *testing.T) {
	grant := regexp.MustCompile(`"events:(PutEvents|\*)"`)
	for _, path := range moduleTFFiles(t) {
		rel, err := filepath.Rel(filepath.Join("..", "modules"), path)
		if err != nil {
			t.Fatal(err)
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "events/") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
				continue
			}
			if !grant.MatchString(line) {
				continue
			}
			if _, ok := putEventsAllowed[rel]; ok {
				continue
			}
			t.Errorf("modules/%s:%d grants %s outside modules/events.\n"+
				"    A workload's PutEvents comes only from its events.produces entries.",
				rel, i+1, grant.FindString(line))
		}
	}
}
