package markdownlint_test

import (
	"bytes"
	"context"
	engine "github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/yuin/goldmark/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testHeadingRule struct{}

func (testHeadingRule) ID() string {
	return "test.heading"
}

type testFixRule struct{}

func (testFixRule) ID() string {
	return "test.fix"
}

const testTitleIndexKey interfaces.PassDataKey = "test.title-index"

type testTitleIndexAnalyzer struct{}

func (testTitleIndexAnalyzer) ID() string {
	return "test.title-index"
}

func (testTitleIndexAnalyzer) Analyze(_ context.Context, pass *interfaces.Pass) {
	titleIndex := map[string][]*interfaces.Document{}
	for _, doc := range pass.Documents {
		title := firstHeadingTitle(doc)
		if title == "" {
			continue
		}
		titleIndex[title] = append(titleIndex[title], doc)
	}
	pass.SetData(testTitleIndexKey, titleIndex)
}

type testDuplicateTitleAnalyzer struct{}

func (testDuplicateTitleAnalyzer) ID() string {
	return "test.duplicate-title"
}

func (a testDuplicateTitleAnalyzer) Analyze(_ context.Context, pass *interfaces.Pass) {
	value, ok := pass.Data(testTitleIndexKey)
	if !ok {
		return
	}

	titleIndex := value.(map[string][]*interfaces.Document)
	for title, documents := range titleIndex {
		if len(documents) < 2 {
			continue
		}

		duplicate := documents[1]
		pass.Report(interfaces.NewDiagnostic(
			duplicate.Path,
			1,
			0,
			0,
			a.ID(),
			"duplicate document title: "+title,
			interfaces.SeverityError,
		))
	}
}

func (r testFixRule) CheckDiagnostics(_ context.Context, doc *interfaces.Document) []interfaces.Diagnostic {
	startOffset := bytes.Index(doc.Source, []byte("bad"))
	if startOffset < 0 {
		return nil
	}

	endOffset := startOffset + len("bad")
	diagnostic := interfaces.NewDiagnostic(
		doc.Path,
		doc.LineForOffset(startOffset),
		startOffset,
		endOffset,
		r.ID(),
		"replace bad token",
		interfaces.SeverityError,
	)
	diagnostic.SuggestedFixes = []interfaces.SuggestedFix{
		{
			Title:      "Replace bad with good",
			Confidence: interfaces.FixConfidenceSafe,
			Edits: []interfaces.TextEdit{
				{
					StartOffset: startOffset,
					EndOffset:   endOffset,
					Replacement: "good",
				},
			},
		},
	}

	return []interfaces.Diagnostic{diagnostic}
}

func (r testHeadingRule) Check(_ context.Context, doc *interfaces.Document) []interfaces.Violation {
	var violations []interfaces.Violation

	err := doc.Walk(func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || node.Kind() != ast.KindHeading {
			return ast.WalkContinue, nil
		}

		violations = append(violations, interfaces.NewViolation(
			doc.Path,
			doc.NodeLine(node),
			r.ID(),
			"heading visited by custom rule",
		))

		return ast.WalkContinue, nil
	})
	if err != nil {
		violations = append(violations, interfaces.NewViolation(doc.Path, 0, r.ID(), err.Error()))
	}

	return violations
}

func TestLinter_RunFile_RegisteredCustomRuleEmitsViolation(t *testing.T) {
	path := writeTempMarkdown(t, "# Title\n\nBody\n")
	l := engine.New(engine.WithRules(testHeadingRule{}))

	violations, err := l.RunFile(context.Background(), path)
	if err != nil {
		t.Fatalf("RunFile returned error: %v", err)
	}

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %#v", len(violations), violations)
	}

	violation := violations[0]
	if violation.Path != path {
		t.Fatalf("expected violation path %q, got %q", path, violation.Path)
	}
	if violation.Line != 1 {
		t.Fatalf("expected violation line 1, got %d", violation.Line)
	}
	if violation.RuleID != "test.heading" {
		t.Fatalf("expected rule ID test.heading, got %q", violation.RuleID)
	}
	if violation.Message != "heading visited by custom rule" {
		t.Fatalf("unexpected message: %q", violation.Message)
	}
}

func TestLinter_RunFile_ReturnsStructuredViolationString(t *testing.T) {
	path := writeTempMarkdown(t, "# Title\n")
	l := engine.New(engine.WithRules(testHeadingRule{}))

	violations, err := l.RunFile(context.Background(), path)
	if err != nil {
		t.Fatalf("RunFile returned error: %v", err)
	}

	got := violations[0].String()
	want := path + ":1: test.heading: heading visited by custom rule"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestLinter_RunFileDiagnostics_ReportsSuggestedTextEdit(t *testing.T) {
	path := writeTempMarkdown(t, "# Title\n\nThis is bad.\n")
	l := engine.New(engine.WithDiagnosticRules(testFixRule{}))

	diagnostics, err := l.RunFileDiagnostics(context.Background(), path)
	if err != nil {
		t.Fatalf("RunFileDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %#v", len(diagnostics), diagnostics)
	}

	diagnostic := diagnostics[0]
	if diagnostic.Path != path {
		t.Fatalf("expected diagnostic path %q, got %q", path, diagnostic.Path)
	}
	if diagnostic.Line != 3 {
		t.Fatalf("expected diagnostic line 3, got %d", diagnostic.Line)
	}
	if diagnostic.RuleID != "test.fix" {
		t.Fatalf("expected rule ID test.fix, got %q", diagnostic.RuleID)
	}
	if diagnostic.Severity != interfaces.SeverityError {
		t.Fatalf("expected severity %q, got %q", interfaces.SeverityError, diagnostic.Severity)
	}
	if len(diagnostic.SuggestedFixes) != 1 {
		t.Fatalf("expected 1 suggested fix, got %#v", diagnostic.SuggestedFixes)
	}

	edit := diagnostic.SuggestedFixes[0].Edits[0]
	if edit.StartOffset != diagnostic.StartOffset || edit.EndOffset != diagnostic.EndOffset {
		t.Fatalf("edit range = [%d,%d), want diagnostic range [%d,%d)", edit.StartOffset, edit.EndOffset, diagnostic.StartOffset, diagnostic.EndOffset)
	}
	if edit.Replacement != "good" {
		t.Fatalf("expected replacement %q, got %q", "good", edit.Replacement)
	}
}

func TestLinter_RunFileDiagnostics_AdaptsViolationRulesWithoutFixes(t *testing.T) {
	path := writeTempMarkdown(t, "# Title\n")
	l := engine.New(engine.WithRules(testHeadingRule{}))

	diagnostics, err := l.RunFileDiagnostics(context.Background(), path)
	if err != nil {
		t.Fatalf("RunFileDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %#v", len(diagnostics), diagnostics)
	}

	diagnostic := diagnostics[0]
	if diagnostic.RuleID != "test.heading" {
		t.Fatalf("expected adapted rule ID test.heading, got %q", diagnostic.RuleID)
	}
	if diagnostic.Severity != interfaces.SeverityError {
		t.Fatalf("expected severity %q, got %q", interfaces.SeverityError, diagnostic.Severity)
	}
	if len(diagnostic.SuggestedFixes) != 0 {
		t.Fatalf("expected no suggested fixes, got %#v", diagnostic.SuggestedFixes)
	}
}

func TestLinter_RunFilesDiagnostics_AnalyzerInspectsMultipleDocuments(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.md")
	secondPath := filepath.Join(dir, "nested", "second.md")
	writeMarkdownFile(t, firstPath, "# Shared Title\n\nFirst body.\n")
	writeMarkdownFileWithDirs(t, secondPath, "# Shared Title\n\nSecond body.\n")

	l := engine.New(engine.WithAnalyzers(
		testTitleIndexAnalyzer{},
		testDuplicateTitleAnalyzer{},
	))

	diagnostics, err := l.RunFilesDiagnostics(context.Background(), []string{firstPath, secondPath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %#v", len(diagnostics), diagnostics)
	}

	diagnostic := diagnostics[0]
	if diagnostic.Path != secondPath {
		t.Fatalf("expected duplicate diagnostic path %q, got %q", secondPath, diagnostic.Path)
	}
	if diagnostic.RuleID != "test.duplicate-title" {
		t.Fatalf("expected analyzer rule ID test.duplicate-title, got %q", diagnostic.RuleID)
	}
	if diagnostic.Message != "duplicate document title: Shared Title" {
		t.Fatalf("unexpected diagnostic message: %q", diagnostic.Message)
	}
}

func TestLinter_RunFile_ReturnsErrorForMissingFile(t *testing.T) {
	l := engine.New(engine.WithRules(testHeadingRule{}))

	_, err := l.RunFile(context.Background(), "missing.md")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func writeTempMarkdown(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fixture.md")
	writeMarkdownFile(t, path, content)
	return path
}

func writeMarkdownFileWithDirs(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	writeMarkdownFile(t, path, content)
}

func firstHeadingTitle(doc *interfaces.Document) string {
	for _, line := range strings.Split(string(doc.Source), "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}
