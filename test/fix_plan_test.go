package markdownlint_test

import (
	"context"
	engine "github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"os"
	"path/filepath"
	"testing"
)

func TestPlanFixes_AcceptsNonOverlappingSafeEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	source := []byte("alpha beta gamma\n")
	diagnostics := []interfaces.Diagnostic{
		diagnosticWithFix(path, 0, 5, "test.first", "one"),
		diagnosticWithFix(path, 11, 16, "test.second", "three"),
	}

	plan := engine.PlanFixes(diagnostics, map[string][]byte{path: source})

	if len(plan.Rejected) != 0 {
		t.Fatalf("expected no rejected fixes, got %#v", plan.Rejected)
	}
	if len(plan.Accepted) != 2 {
		t.Fatalf("expected 2 accepted edits, got %#v", plan.Accepted)
	}
	if len(plan.Files) != 1 || plan.Files[0].Path != path || len(plan.Files[0].Edits) != 2 {
		t.Fatalf("unexpected file plan: %#v", plan.Files)
	}
	if !plan.HasAcceptedEdits() {
		t.Fatal("expected plan to report accepted edits")
	}
}

func TestPlanFixes_RejectsOverlappingEditsDeterministically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	source := []byte("alpha beta gamma\n")
	diagnostics := []interfaces.Diagnostic{
		diagnosticWithFix(path, 0, 10, "test.wide", "wide"),
		diagnosticWithFix(path, 6, 10, "test.narrow", "narrow"),
	}

	plan := engine.PlanFixes(diagnostics, map[string][]byte{path: source})

	if len(plan.Accepted) != 1 {
		t.Fatalf("expected 1 accepted edit, got %#v", plan.Accepted)
	}
	if plan.Accepted[0].RuleID != "test.wide" {
		t.Fatalf("expected first source-range fix to win, got %#v", plan.Accepted[0])
	}
	if len(plan.Rejected) != 1 {
		t.Fatalf("expected 1 rejected fix, got %#v", plan.Rejected)
	}
	if plan.Rejected[0].Reason != engine.FixRejectionOverlappingEdit {
		t.Fatalf("expected overlap rejection, got %#v", plan.Rejected[0])
	}
}

func TestPlanFixes_RejectsUnsafeFixesByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	diagnostic := interfaces.NewDiagnostic(path, 1, 0, 5, "test.unsafe", "unsafe fix", interfaces.SeverityError)
	diagnostic.SuggestedFixes = []interfaces.SuggestedFix{
		{
			Title:      "manual replacement",
			Confidence: interfaces.FixConfidenceUnsafe,
			Edits: []interfaces.TextEdit{
				{StartOffset: 0, EndOffset: 5, Replacement: "manual"},
			},
		},
	}

	plan := engine.PlanFixes([]interfaces.Diagnostic{diagnostic}, map[string][]byte{path: []byte("alpha\n")})

	if len(plan.Accepted) != 0 {
		t.Fatalf("expected no accepted edits, got %#v", plan.Accepted)
	}
	if len(plan.Rejected) != 1 {
		t.Fatalf("expected 1 rejected fix, got %#v", plan.Rejected)
	}
	if plan.Rejected[0].Reason != engine.FixRejectionNoSafeFix {
		t.Fatalf("expected no-safe-fix rejection, got %#v", plan.Rejected[0])
	}
}

func TestNewFixReviewReport_ClassifiesReviewOutcomes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	accepted := diagnosticWithFix(path, 0, 5, "test.accepted", "one")
	rejected := interfaces.NewDiagnostic(path, 1, 6, 10, "test.rejected", "unsafe fix", interfaces.SeverityError)
	rejected.SuggestedFixes = []interfaces.SuggestedFix{
		{
			Title:      "Unsafe replacement",
			Confidence: interfaces.FixConfidenceUnsafe,
			Edits: []interfaces.TextEdit{
				{StartOffset: 6, EndOffset: 10, Replacement: "manual"},
			},
		},
	}
	ambiguous := interfaces.NewDiagnostic(path, 1, 11, 16, "markdown.link-relocation", "local Markdown target is ambiguous: target.md", interfaces.SeverityError)
	ambiguous.Category = interfaces.DiagnosticCategoryRelocationAmbiguous
	unresolved := interfaces.NewDiagnostic(path, 1, 17, 22, "markdown.link-relocation", "local Markdown target could not be resolved uniquely", interfaces.SeverityError)
	unresolved.Category = interfaces.DiagnosticCategoryRelocationUnresolved

	diagnostics := []interfaces.Diagnostic{accepted, rejected, ambiguous, unresolved}
	plan := engine.PlanFixes(diagnostics, map[string][]byte{path: []byte("alpha beta gamma delta\n")})

	report := engine.NewFixReviewReport(plan, diagnostics)

	if len(report.Accepted) != 1 {
		t.Fatalf("expected 1 accepted edit, got %#v", report.Accepted)
	}
	if len(report.Rejected) != 1 {
		t.Fatalf("expected 1 rejected edit, got %#v", report.Rejected)
	}
	if len(report.Ambiguous) != 1 {
		t.Fatalf("expected 1 ambiguous diagnostic, got %#v", report.Ambiguous)
	}
	if len(report.OtherNonFixable) != 1 {
		t.Fatalf("expected 1 other non-fixable diagnostic, got %#v", report.OtherNonFixable)
	}
}

func TestDryRunFixes_DoesNotModifyFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	writeMarkdownFile(t, path, "alpha beta\n")
	diagnostics := []interfaces.Diagnostic{
		diagnosticWithFix(path, 0, 5, "test.first", "one"),
	}

	plan, err := engine.DryRunFixes(context.Background(), diagnostics)
	if err != nil {
		t.Fatalf("DryRunFixes returned error: %v", err)
	}

	if len(plan.Accepted) != 1 {
		t.Fatalf("expected 1 accepted edit, got %#v", plan.Accepted)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if string(content) != "alpha beta\n" {
		t.Fatalf("dry run changed file content: %q", content)
	}
}

func TestApplyFixes_WritesOnlyAcceptedSafeEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	writeMarkdownFile(t, path, "alpha beta gamma\n")
	diagnostics := []interfaces.Diagnostic{
		diagnosticWithFix(path, 0, 5, "test.first", "one"),
		diagnosticWithFix(path, 6, 10, "test.second", "two"),
		diagnosticWithFix(path, 6, 16, "test.overlap", "rejected"),
	}

	plan, err := engine.ApplyFixes(context.Background(), diagnostics)
	if err != nil {
		t.Fatalf("ApplyFixes returned error: %v", err)
	}

	if len(plan.Accepted) != 2 {
		t.Fatalf("expected 2 accepted edits, got %#v", plan.Accepted)
	}
	if len(plan.Rejected) != 1 {
		t.Fatalf("expected 1 rejected edit, got %#v", plan.Rejected)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if string(content) != "one two gamma\n" {
		t.Fatalf("unexpected applied content: %q", content)
	}
}

func diagnosticWithFix(path string, startOffset int, endOffset int, ruleID string, replacement string) interfaces.Diagnostic {
	diagnostic := interfaces.NewDiagnostic(path, 1, startOffset, endOffset, ruleID, "replace text", interfaces.SeverityError)
	diagnostic.SuggestedFixes = []interfaces.SuggestedFix{
		{
			Title:      "Replace text",
			Confidence: interfaces.FixConfidenceSafe,
			Edits: []interfaces.TextEdit{
				{
					StartOffset: startOffset,
					EndOffset:   endOffset,
					Replacement: replacement,
				},
			},
		},
	}
	return diagnostic
}
