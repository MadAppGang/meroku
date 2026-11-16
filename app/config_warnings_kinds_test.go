package main

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const warningKindsTSFile = "../web/src/types/configWarnings.ts"

// tsWarningKinds returns the string literals of the WarningKind union in the
// web types: everything from `export type WarningKind =` to the closing `;`.
func tsWarningKinds(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile(warningKindsTSFile)
	if err != nil {
		t.Fatalf("reading %s: %v", warningKindsTSFile, err)
	}
	decl := regexp.MustCompile(`(?s)export\s+type\s+WarningKind\s*=(.*?);`).FindSubmatch(src)
	if decl == nil {
		t.Fatalf("%s has no `export type WarningKind = ...;` declaration", warningKindsTSFile)
	}
	body := string(decl[1])

	literal := regexp.MustCompile(`"([^"]*)"|'([^']*)'`)
	var kinds []string
	for _, m := range literal.FindAllStringSubmatch(body, -1) {
		kinds = append(kinds, m[1]+m[2])
	}
	// Anything left after removing the literals and the union bars is a type
	// this parser does not understand, such as a reference to another alias.
	rest := strings.TrimSpace(strings.NewReplacer("|", "", "\n", "", "\t", "", " ", "").
		Replace(literal.ReplaceAllString(body, "")))
	if rest != "" {
		t.Fatalf("WarningKind in %s holds something other than string literals: %q", warningKindsTSFile, rest)
	}
	if len(kinds) == 0 {
		t.Fatalf("WarningKind in %s has no string literals", warningKindsTSFile)
	}
	return kinds
}

func sortedCopy(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

// The web canvas maps each warning to a node by its kind, so a kind the Go side
// sends and the TypeScript union lacks is a warning the UI cannot place.
func TestWarningKindsMatchWebUnion(t *testing.T) {
	goKinds := sortedCopy(warningKinds)
	tsKinds := sortedCopy(tsWarningKinds(t))

	inGo := map[string]bool{}
	for _, k := range goKinds {
		if inGo[k] {
			t.Errorf("warningKinds lists %q twice", k)
		}
		inGo[k] = true
	}
	inTS := map[string]bool{}
	for _, k := range tsKinds {
		if inTS[k] {
			t.Errorf("WarningKind in %s lists %q twice", warningKindsTSFile, k)
		}
		inTS[k] = true
	}
	for _, k := range goKinds {
		if !inTS[k] {
			t.Errorf("Go warning kind %q is missing from WarningKind in %s", k, warningKindsTSFile)
		}
	}
	for _, k := range tsKinds {
		if !inGo[k] {
			t.Errorf("WarningKind %q in %s has no Go constant in warningKinds (app/config_warnings.go)", k, warningKindsTSFile)
		}
	}
}
