package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Every argument a module block in env/main.hbs passes must be a variable the
// module declares. Terraform refuses an undeclared one with "Unsupported
// argument", but only for a config that renders that line, so a render test
// that checks the text passes while every such config fails at plan. This
// reads the template rather than a rendered file, so an argument behind an
// {{#if}} that no fixture turns on is still checked.

var (
	hbsCommentPattern   = regexp.MustCompile(`(?s)\{\{!--.*?--\}\}|\{\{![^}]*\}\}`)
	mustachePattern     = regexp.MustCompile(`\{\{\{?[^}]*\}?\}\}`)
	stringPattern       = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
	moduleOpenPattern   = regexp.MustCompile(`^\s*module\s+"[^"]*"\s*\{`)
	moduleSourcePattern = regexp.MustCompile(`^\s*source\s*=\s*"\{\{\s*(?:@root\.)?modules\s*\}\}/([^"]+)"`)
	argumentPattern     = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_-]*)\s*=`)
	heredocPattern      = regexp.MustCompile(`<<-?([A-Za-z_][A-Za-z0-9_]*)\s*$`)
	variablePattern     = regexp.MustCompile(`(?m)^\s*variable\s+"([^"]+)"`)
)

// moduleMetaArguments are Terraform's own arguments, valid on any module call.
var moduleMetaArguments = map[string]bool{
	"source":     true,
	"version":    true,
	"count":      true,
	"for_each":   true,
	"depends_on": true,
	"providers":  true,
}

type moduleCall struct {
	line      int
	source    string
	arguments map[string]int // argument name -> template line
}

func TestMainHBSModuleArgumentsAreDeclared(t *testing.T) {
	calls := scanModuleCalls(t, readMainHBS(t))
	if len(calls) == 0 {
		t.Fatal("found no module blocks in env/main.hbs; the scanner is broken")
	}

	arguments := 0
	for _, call := range calls {
		arguments += len(call.arguments)
	}
	t.Logf("checked %d module blocks, %d arguments", len(calls), arguments)

	declared := map[string]map[string]bool{}
	for _, call := range calls {
		if call.source == "" {
			// A custom module the project supplies; its variables are not ours.
			continue
		}
		vars, ok := declared[call.source]
		if !ok {
			vars = moduleVariables(t, call.source)
			declared[call.source] = vars
		}

		names := make([]string, 0, len(call.arguments))
		for name := range call.arguments {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if moduleMetaArguments[name] || vars[name] {
				continue
			}
			t.Errorf("env/main.hbs:%d: the module block at line %d passes %q to modules/%s, which declares no such variable.\n"+
				"    Terraform refuses it with \"Unsupported argument\" for every config that renders this line.",
				call.arguments[name], call.line, name, call.source)
		}
	}
}

// scanModuleCalls returns each module block with the arguments set at its top
// level. Handlebars tags are blanked before braces are counted, so `{{#if}}`
// and `"{{name}}" = {` neither open a block nor read as an argument.
func scanModuleCalls(t *testing.T, template string) []moduleCall {
	t.Helper()

	// Blank comments but keep their newlines, so line numbers stay true.
	template = hbsCommentPattern.ReplaceAllStringFunc(template, func(comment string) string {
		return strings.Repeat("\n", strings.Count(comment, "\n"))
	})

	var (
		calls   []moduleCall
		current *moduleCall
		depth   int
		heredoc string
	)
	for index, line := range strings.Split(template, "\n") {
		lineNumber := index + 1

		if heredoc != "" {
			if strings.TrimSpace(line) == heredoc {
				heredoc = ""
			}
			continue
		}

		code := mustachePattern.ReplaceAllString(line, "x")
		code = stringPattern.ReplaceAllString(code, `""`)
		if hash := strings.Index(code, "#"); hash >= 0 {
			code = code[:hash]
		}
		if match := heredocPattern.FindStringSubmatch(code); match != nil {
			heredoc = match[1]
		}

		if current == nil {
			if moduleOpenPattern.MatchString(code) {
				calls = append(calls, moduleCall{line: lineNumber, arguments: map[string]int{}})
				current = &calls[len(calls)-1]
				depth = strings.Count(code, "{") - strings.Count(code, "}")
			}
			continue
		}

		if depth == 1 && moduleOpenPattern.MatchString(code) {
			// The {{else}} branch of a block whose {{#if}} branch opened the
			// same module with different arguments: a second call, one brace.
			calls = append(calls, moduleCall{line: lineNumber, arguments: map[string]int{}})
			current = &calls[len(calls)-1]
			continue
		}

		if depth == 1 {
			if match := moduleSourcePattern.FindStringSubmatch(line); match != nil {
				current.source = match[1]
			}
			if match := argumentPattern.FindStringSubmatch(code); match != nil {
				if _, seen := current.arguments[match[1]]; !seen {
					current.arguments[match[1]] = lineNumber
				}
			}
		}

		depth += strings.Count(code, "{") - strings.Count(code, "}")
		if depth <= 0 {
			current = nil
		}
	}
	if current != nil {
		t.Fatalf("env/main.hbs:%d: module block never closes; the scanner lost count of braces", current.line)
	}
	return calls
}

func moduleVariables(t *testing.T, source string) map[string]bool {
	t.Helper()

	files, err := filepath.Glob(filepath.Join("..", "modules", source, "*.tf"))
	if err != nil || len(files) == 0 {
		t.Fatalf("modules/%s: no .tf files found (%v)", source, err)
	}
	vars := map[string]bool{}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		for _, match := range variablePattern.FindAllStringSubmatch(string(raw), -1) {
			vars[match[1]] = true
		}
	}
	return vars
}
