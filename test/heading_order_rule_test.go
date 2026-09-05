package markdownlint_test

import (
	"context"
	engine "github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	rules "github.com/portpowered/markdown-linter/pkg/rules"
	"testing"
)

func TestHeadingOrderRule_NormalHeadingDescent_AllowsValidHierarchy(t *testing.T) {
	path := writeTempMarkdown(t, "# Title\n\n## Section\n\n### Detail\n\n## Next section\n")

	violations := runHeadingOrderRule(t, path)

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}

func TestHeadingOrderRule_RepeatedHeadingLevels_AllowsPeers(t *testing.T) {
	path := writeTempMarkdown(t, "# Title\n\n## Section\n\n## Next section\n\n### Detail\n\n### Next detail\n")

	violations := runHeadingOrderRule(t, path)

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}

func TestHeadingOrderRule_SkippedHeadingLevel_ReportsOffendingLine(t *testing.T) {
	path := writeTempMarkdown(t, "# Title\n\n## Section\n\n#### Skipped detail\n")

	violations := runHeadingOrderRule(t, path)

	requireSingleViolation(t, violations, interfaces.Violation{
		Path:    path,
		Line:    5,
		RuleID:  "markdown.heading-order",
		Message: "heading level should not skip from 2 to 4",
	})
}

func TestHeadingOrderRule_FirstHeadingBelowH1_AllowsDocumentFragments(t *testing.T) {
	path := writeTempMarkdown(t, "### Fragment start\n\n#### Detail\n")

	violations := runHeadingOrderRule(t, path)

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}

func runHeadingOrderRule(t *testing.T, path string) []interfaces.Violation {
	t.Helper()

	l := engine.New(engine.WithRules(rules.NewHeadingOrderRule()))
	violations, err := l.RunFile(context.Background(), path)
	if err != nil {
		t.Fatalf("RunFile returned error: %v", err)
	}

	return violations
}
