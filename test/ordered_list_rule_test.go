package markdownlint_test

import (
	"context"
	engine "github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	rules "github.com/portpowered/markdown-linter/pkg/rules"
	"testing"
)

func TestOrderedListRule_SequentialLists_AllowsValidNumbering(t *testing.T) {
	path := writeTempMarkdown(t, "# Steps\n\n1. First\n2. Second\n3. Third\n")

	violations := runOrderedListRule(t, path)

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}

func TestOrderedListRule_NestedLists_EvaluatesNestedListsIndependently(t *testing.T) {
	path := writeTempMarkdown(t, "# Steps\n\n1. First\n   1. Nested first\n   2. Nested second\n2. Second\n")

	violations := runOrderedListRule(t, path)

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}

func TestOrderedListRule_InvalidNumbering_ReportsExpectedNumber(t *testing.T) {
	path := writeTempMarkdown(t, "# Steps\n\n1. First\n3. Third\n")

	violations := runOrderedListRule(t, path)

	requireSingleViolation(t, violations, interfaces.Violation{
		Path:    path,
		Line:    4,
		RuleID:  "markdown.ordered-list",
		Message: "ordered list item should be numbered 2, got 3",
	})
}

func TestOrderedListRule_NonOneStart_ReportsFirstExpectedNumber(t *testing.T) {
	path := writeTempMarkdown(t, "# Steps\n\n2. First\n3. Second\n")

	violations := runOrderedListRule(t, path)

	requireSingleViolation(t, violations, interfaces.Violation{
		Path:    path,
		Line:    3,
		RuleID:  "markdown.ordered-list",
		Message: "ordered list item should be numbered 1, got 2",
	})
}

func TestOrderedListRule_NestedInvalidNumbering_ReportsNestedLine(t *testing.T) {
	path := writeTempMarkdown(t, "# Steps\n\n1. First\n   1. Nested first\n   3. Nested third\n2. Second\n")

	violations := runOrderedListRule(t, path)

	requireSingleViolation(t, violations, interfaces.Violation{
		Path:    path,
		Line:    5,
		RuleID:  "markdown.ordered-list",
		Message: "ordered list item should be numbered 2, got 3",
	})
}

func runOrderedListRule(t *testing.T, path string) []interfaces.Violation {
	t.Helper()

	l := engine.New(engine.WithRules(rules.NewOrderedListRule()))
	violations, err := l.RunFile(context.Background(), path)
	if err != nil {
		t.Fatalf("RunFile returned error: %v", err)
	}

	return violations
}
