package markdownlint_test

import (
	"context"
	engine "github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	rules "github.com/portpowered/markdown-linter/pkg/rules"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocsCheckFormattingRuleReportsTrailingWhitespaceAndUnclosedFence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docs", "broken.md")
	writeFile(t, path, "# Broken  \n\n```go\nfmt.Println(\"open\")\n")

	linter := engine.New(engine.WithRules(rules.NewDocsCheckFormattingRule()))
	violations, err := linter.RunFile(context.Background(), path)
	if err != nil {
		t.Fatalf("RunFile returned error: %v", err)
	}

	got := renderViolations(violations)
	for _, expected := range []string{
		"broken.md:1: markdown.docs-check-formatting: ",
		"has trailing whitespace.",
		"broken.md: markdown.docs-check-formatting: ",
		"has an unclosed fenced code block.",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("violations missing %q:\n%s", expected, got)
		}
	}
}

func TestDocsCheckFormattingRuleAllowsClosedLongerFence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docs", "valid.md")
	writeFile(t, path, "````markdown\n```go\nfmt.Println(\"nested\")\n```\n````\n")

	linter := engine.New(engine.WithRules(rules.NewDocsCheckFormattingRule()))
	violations, err := linter.RunFile(context.Background(), path)
	if err != nil {
		t.Fatalf("RunFile returned error: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
}

func renderViolations(violations []interfaces.Violation) string {
	lines := make([]string, 0, len(violations))
	for _, violation := range violations {
		lines = append(lines, violation.String())
	}
	return strings.Join(lines, "\n")
}
