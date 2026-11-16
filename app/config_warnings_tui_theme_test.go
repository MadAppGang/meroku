package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// The warning screen and the deploy gate take every colour from the theme
// tokens in theme.go, which adapt to a light or dark terminal. A raw
// lipgloss.Color fixes one colour for both backgrounds. The check walks the
// AST, so a comment that names lipgloss.Color does not trip it.
func TestConfigWarningsTUIUsesThemeColoursOnly(t *testing.T) {
	for _, file := range []string{"config_warnings_tui.go", "deploy_warning_gate.go"} {
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		lipglossName := importName(parsed, "github.com/charmbracelet/lipgloss")
		if lipglossName == "" {
			continue
		}

		var calls []string
		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Color" {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == lipglossName {
				calls = append(calls, fset.Position(call.Pos()).String()+": lipgloss.Color(...)")
			}
			return true
		})
		if len(calls) > 0 {
			t.Errorf("%s builds a colour directly; use the theme tokens (theme.*, adapt()) from theme.go:\n  %s",
				file, strings.Join(calls, "\n  "))
		}
	}
}
