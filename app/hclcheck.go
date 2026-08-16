package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// Generation used to write main.tf without ever looking at it.
//
// env/main.hbs has ~70 assignment sites of the bare form
//
//	setup_FCM_SNS = {{workload.setup_fcnsns}}
//
// and applyTemplate reads the YAML into a plain map, so it never picks up the
// zero values the Env struct would supply. A config missing any one of those
// keys renders the right-hand side as nothing at all, leaving `setup_FCM_SNS =`
// with no expression after it. raymond reports no error for that — a missing
// path is the empty string, which is the whole point of a template language —
// so generation printed its success message and exited 0.
//
// The first thing that noticed was `terraform init`, several steps and one
// context switch later, blaming a file the user is told never to edit by hand.
//
// Parsing the output before writing it turns that into an error at the moment
// and place the mistake was made. HCL's own parser is used rather than a
// hand-rolled scan for `= $`: the failure mode is "not valid Terraform", and the
// only honest test for that is the parser Terraform itself uses.

// maxReportedHCLDiagnostics caps the excerpt list.
//
// One malformed line usually produces a cascade — a missing expression makes the
// parser resynchronise at some later token and complain again — so printing
// every diagnostic buries the first one, which is nearly always the real cause.
const maxReportedHCLDiagnostics = 5

// validateGeneratedHCL parses rendered Terraform and returns an error describing
// every syntax problem in it, quoting the offending lines.
//
// filename is used only for the diagnostic headings; nothing is read from disk.
func validateGeneratedHCL(filename, source string) error {
	_, diags := hclsyntax.ParseConfig([]byte(source), filename, hcl.InitialPos)

	errors := make([]*hcl.Diagnostic, 0, len(diags))
	for _, diag := range diags {
		if diag.Severity == hcl.DiagError {
			errors = append(errors, diag)
		}
	}
	if len(errors) == 0 {
		return nil
	}

	// Source order, so the first thing printed is the first thing that went
	// wrong. hcl emits in order today; this does not depend on that.
	sort.SliceStable(errors, func(i, j int) bool {
		left, right := errors[i].Subject, errors[j].Subject
		if left == nil || right == nil {
			return right != nil
		}
		if left.Start.Line != right.Start.Line {
			return left.Start.Line < right.Start.Line
		}
		return left.Start.Column < right.Start.Column
	})

	lines := strings.Split(source, "\n")

	var report strings.Builder
	fmt.Fprintf(&report, "%s is not valid Terraform:\n", filename)
	for i, diag := range errors {
		if i == maxReportedHCLDiagnostics {
			fmt.Fprintf(&report, "\n  ...and %d more.\n", len(errors)-i)
			break
		}
		report.WriteString("\n")
		report.WriteString(describeHCLDiagnostic(diag, lines))
	}

	report.WriteString("\nThis is a defect in the generator or a value missing from the config —\n")
	report.WriteString("not something to fix by editing the generated file, which is rewritten\n")
	report.WriteString("on every run. Nothing was written.")

	return fmt.Errorf("%s", report.String())
}

// describeHCLDiagnostic renders one diagnostic with the source line it points
// at and a caret under the column.
func describeHCLDiagnostic(diag *hcl.Diagnostic, lines []string) string {
	var out strings.Builder

	if diag.Subject == nil {
		fmt.Fprintf(&out, "  %s: %s\n", diag.Summary, diag.Detail)
		return out.String()
	}

	fmt.Fprintf(&out, "  line %d: %s\n", diag.Subject.Start.Line, diag.Summary)
	if diag.Detail != "" {
		fmt.Fprintf(&out, "    %s\n", diag.Detail)
	}
	out.WriteString("\n")

	// One line of lead-in is enough to recognise the block without turning the
	// message into a page of Terraform.
	const context = 2
	first := max(diag.Subject.Start.Line-context, 1)
	last := min(diag.Subject.Start.Line, len(lines))

	width := len(fmt.Sprintf("%d", last))
	for number := first; number <= last; number++ {
		fmt.Fprintf(&out, "    %*d | %s\n", width, number, lines[number-1])
	}

	// hcl columns are 1-based and counted in unicode characters, which is what
	// the caret needs to line up under a line containing multi-byte text.
	caret := diag.Subject.Start.Column - 1
	if caret < 0 {
		caret = 0
	}
	fmt.Fprintf(&out, "    %*s | %s^\n", width, "", strings.Repeat(" ", caret))

	return out.String()
}
