package markdownlint_test

import (
	"context"
	engine "github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	rules "github.com/portpowered/markdown-linter/pkg/rules"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinkRelocationAnalyzer_RewritesMovedMarkdownLinkWithValidAnchor(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "docs", "guides", "source.md")
	oldTargetPath := filepath.Join(dir, "docs", "processes", "old.md")
	newTargetPath := filepath.Join(dir, "docs", "processes", "backend", "old.md")
	writeMarkdownFileWithDirs(t, sourcePath, "# Source\n\nSee [target](../processes/old.md#deep-heading).\n")
	writeMarkdownFileWithDirs(t, newTargetPath, "# Target\n\n## Deep Heading\n")

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(rules.WithMoveMappings(map[string]string{
			oldTargetPath: newTargetPath,
		})),
		rules.NewLinkRelocationAnalyzer(),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{sourcePath, newTargetPath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %#v", len(diagnostics), diagnostics)
	}
	diagnostic := diagnostics[0]
	if diagnostic.RuleID != "markdown.link-relocation" {
		t.Fatalf("expected relocation rule, got %q", diagnostic.RuleID)
	}
	if len(diagnostic.SuggestedFixes) != 1 {
		t.Fatalf("expected one safe fix, got %#v", diagnostic.SuggestedFixes)
	}
	edit := diagnostic.SuggestedFixes[0].Edits[0]
	if edit.Replacement != "../processes/backend/old.md#deep-heading" {
		t.Fatalf("unexpected replacement: %q", edit.Replacement)
	}

	plan := engine.PlanFixes(diagnostics, map[string][]byte{
		filepath.ToSlash(filepath.Clean(sourcePath)): []byte("# Source\n\nSee [target](../processes/old.md#deep-heading).\n"),
	})
	if len(plan.Accepted) != 1 || len(plan.Rejected) != 0 {
		t.Fatalf("expected one accepted fix and no rejections, got %#v", plan)
	}
}

func TestLinkRelocationAnalyzer_RewritesMovedImageLinkFromExplicitMapping(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "docs", "guides", "source.md")
	oldImagePath := filepath.Join(dir, "docs", "assets", "diagram.png")
	newImagePath := filepath.Join(dir, "docs", "images", "diagram.png")
	writeMarkdownFileWithDirs(t, sourcePath, "# Source\n\n![diagram](../assets/diagram.png)\n")
	writeBinaryFileWithDirs(t, newImagePath, []byte("png"))

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(rules.WithMoveMappings(map[string]string{
			oldImagePath: newImagePath,
		})),
		rules.NewLinkRelocationAnalyzer(),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{sourcePath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %#v", len(diagnostics), diagnostics)
	}
	if len(diagnostics[0].SuggestedFixes) != 1 {
		t.Fatalf("expected one safe image fix, got %#v", diagnostics[0].SuggestedFixes)
	}
	edit := diagnostics[0].SuggestedFixes[0].Edits[0]
	if edit.Replacement != "../images/diagram.png" {
		t.Fatalf("unexpected image replacement: %q", edit.Replacement)
	}
}

func TestLinkRelocationAnalyzer_UsesUniqueExactImageFilenameMatchWithoutMapping(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "docs", "guides", "source.md")
	newImagePath := filepath.Join(dir, "docs", "images", "diagram.png")
	writeMarkdownFileWithDirs(t, sourcePath, "# Source\n\n![diagram](../assets/diagram.png)\n")
	writeBinaryFileWithDirs(t, newImagePath, []byte("png"))

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		rules.NewLinkRelocationAnalyzer(),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{sourcePath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %#v", len(diagnostics), diagnostics)
	}
	if len(diagnostics[0].SuggestedFixes) != 1 {
		t.Fatalf("expected one safe image fix, got %#v", diagnostics[0].SuggestedFixes)
	}
	edit := diagnostics[0].SuggestedFixes[0].Edits[0]
	if edit.Replacement != "../images/diagram.png" {
		t.Fatalf("unexpected best-effort image replacement: %q", edit.Replacement)
	}
}

func TestLinkRelocationAnalyzer_ReportsAmbiguousImageFilenameWithoutFix(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "docs", "source.md")
	firstImagePath := filepath.Join(dir, "docs", "backend", "diagram.png")
	secondImagePath := filepath.Join(dir, "docs", "website", "diagram.png")
	writeMarkdownFileWithDirs(t, sourcePath, "# Source\n\n![diagram](missing/diagram.png)\n")
	writeBinaryFileWithDirs(t, firstImagePath, []byte("backend"))
	writeBinaryFileWithDirs(t, secondImagePath, []byte("website"))

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		rules.NewLinkRelocationAnalyzer(),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{sourcePath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %#v", len(diagnostics), diagnostics)
	}
	if len(diagnostics[0].SuggestedFixes) != 0 {
		t.Fatalf("expected ambiguous image target without safe fix, got %#v", diagnostics[0].SuggestedFixes)
	}
}

func TestLinkRelocationAnalyzer_UsesUniqueExactFilenameMatchWithoutMapping(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "docs", "guides", "source.md")
	newTargetPath := filepath.Join(dir, "docs", "archive", "target.md")
	writeMarkdownFileWithDirs(t, sourcePath, "# Source\n\nSee [target](../processes/target.md).\n")
	writeMarkdownFileWithDirs(t, newTargetPath, "# Target\n")

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		rules.NewLinkRelocationAnalyzer(),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{sourcePath, newTargetPath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %#v", len(diagnostics), diagnostics)
	}
	edit := diagnostics[0].SuggestedFixes[0].Edits[0]
	if edit.Replacement != "../archive/target.md" {
		t.Fatalf("unexpected best-effort replacement: %q", edit.Replacement)
	}
}

func TestLinkRelocationAnalyzer_RewritesDenseInlineLinkDestination(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "docs", "guides", "source.md")
	newTargetPath := filepath.Join(dir, "docs", "archive", "target.md")
	writeMarkdownFileWithDirs(t, sourcePath, "# Source\n\nSee [first](../processes/target.md)[second](../processes/target.md).\n")
	writeMarkdownFileWithDirs(t, newTargetPath, "# Target\n")

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		rules.NewLinkRelocationAnalyzer(),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{sourcePath, newTargetPath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d: %#v", len(diagnostics), diagnostics)
	}
	plan := engine.PlanFixes(diagnostics, map[string][]byte{
		filepath.ToSlash(filepath.Clean(sourcePath)): []byte("# Source\n\nSee [first](../processes/target.md)[second](../processes/target.md).\n"),
	})
	if len(plan.Accepted) != 2 || len(plan.Rejected) != 0 {
		t.Fatalf("expected two accepted dense inline edits and no rejections, got %#v", plan)
	}
	for _, accepted := range plan.Accepted {
		if accepted.TextEdit.Replacement != "../archive/target.md" {
			t.Fatalf("unexpected dense inline replacement: %q", accepted.TextEdit.Replacement)
		}
	}
}

func TestLinkRelocationAnalyzer_RewritesReferenceStyleDefinitionDestination(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "docs", "guides", "source.md")
	newTargetPath := filepath.Join(dir, "docs", "archive", "target.md")
	writeMarkdownFileWithDirs(t, sourcePath, "# Source\n\nSee [target][target-ref].\n\n[target-ref]: ../processes/target.md \"Target\"\n")
	writeMarkdownFileWithDirs(t, newTargetPath, "# Target\n")

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		rules.NewLinkRelocationAnalyzer(),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{sourcePath, newTargetPath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %#v", len(diagnostics), diagnostics)
	}
	if diagnostics[0].Category == interfaces.DiagnosticCategoryRelocationUnavailableRange {
		t.Fatalf("expected supported reference-style link to have a destination range, got %#v", diagnostics[0])
	}
	edit := diagnostics[0].SuggestedFixes[0].Edits[0]
	if edit.Replacement != "../archive/target.md" {
		t.Fatalf("unexpected reference-style replacement: %q", edit.Replacement)
	}

	plan := engine.PlanFixes(diagnostics, map[string][]byte{
		filepath.ToSlash(filepath.Clean(sourcePath)): []byte("# Source\n\nSee [target][target-ref].\n\n[target-ref]: ../processes/target.md \"Target\"\n"),
	})
	if len(plan.Accepted) != 1 || len(plan.Rejected) != 0 {
		t.Fatalf("expected one accepted reference-style edit and no rejections, got %#v", plan)
	}
}

func TestLinkRelocationAnalyzer_IgnoresReferenceDefinitionsInFencedCode(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "docs", "guides", "source.md")
	newTargetPath := filepath.Join(dir, "docs", "archive", "target.md")
	writeMarkdownFileWithDirs(t, sourcePath, strings.Join([]string{
		"# Source",
		"",
		"```md",
		"[target-ref]: code/target.md",
		"```",
		"",
		"See [target][target-ref].",
		"",
		"[target-ref]: ../processes/target.md",
		"",
	}, "\n"))
	writeMarkdownFileWithDirs(t, newTargetPath, "# Target\n")

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		rules.NewLinkRelocationAnalyzer(),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{sourcePath, newTargetPath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %#v", len(diagnostics), diagnostics)
	}
	plan, err := engine.ApplyFixes(context.Background(), diagnostics)
	if err != nil {
		t.Fatalf("ApplyFixes returned error: %v", err)
	}
	if len(plan.Accepted) != 1 || len(plan.Rejected) != 0 {
		t.Fatalf("expected one accepted reference-style edit and no rejections, got %#v", plan)
	}

	content, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	expected := strings.Join([]string{
		"# Source",
		"",
		"```md",
		"[target-ref]: code/target.md",
		"```",
		"",
		"See [target][target-ref].",
		"",
		"[target-ref]: ../archive/target.md",
		"",
	}, "\n")
	if string(content) != expected {
		t.Fatalf("unexpected applied content:\n%s", content)
	}
}

func TestLinkRelocationAnalyzer_ReportsAmbiguousFilenameWithoutFix(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "docs", "source.md")
	firstTargetPath := filepath.Join(dir, "docs", "backend", "target.md")
	secondTargetPath := filepath.Join(dir, "docs", "website", "target.md")
	writeMarkdownFileWithDirs(t, sourcePath, "# Source\n\nSee [target](missing/target.md).\n")
	writeMarkdownFileWithDirs(t, firstTargetPath, "# Backend Target\n")
	writeMarkdownFileWithDirs(t, secondTargetPath, "# Website Target\n")

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		rules.NewLinkRelocationAnalyzer(),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{sourcePath, firstTargetPath, secondTargetPath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %#v", len(diagnostics), diagnostics)
	}
	if len(diagnostics[0].SuggestedFixes) != 0 {
		t.Fatalf("expected ambiguous target without safe fix, got %#v", diagnostics[0].SuggestedFixes)
	}
}

func TestLinkRelocationAnalyzer_ReportsInvalidMovedAnchorWithoutFix(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "docs", "source.md")
	oldTargetPath := filepath.Join(dir, "docs", "old.md")
	newTargetPath := filepath.Join(dir, "docs", "new", "old.md")
	writeMarkdownFileWithDirs(t, sourcePath, "# Source\n\nSee [target](old.md#missing-heading).\n")
	writeMarkdownFileWithDirs(t, newTargetPath, "# Target\n\n## Present Heading\n")

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(rules.WithMoveMappings(map[string]string{
			oldTargetPath: newTargetPath,
		})),
		rules.NewLinkRelocationAnalyzer(),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{sourcePath, newTargetPath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %#v", len(diagnostics), diagnostics)
	}
	if len(diagnostics[0].SuggestedFixes) != 0 {
		t.Fatalf("expected invalid anchor without safe fix, got %#v", diagnostics[0].SuggestedFixes)
	}
}

func TestLinkRelocationAnalyzer_ApplyFixesUpdatesOnlyMovedDestination(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "docs", "source.md")
	oldTargetPath := filepath.Join(dir, "docs", "old.md")
	newTargetPath := filepath.Join(dir, "docs", "nested", "old.md")
	writeMarkdownFileWithDirs(t, sourcePath, "# Source\n\nBefore [target](old.md) after.\n")
	writeMarkdownFileWithDirs(t, newTargetPath, "# Target\n")

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(rules.WithMoveMappings(map[string]string{
			oldTargetPath: newTargetPath,
		})),
		rules.NewLinkRelocationAnalyzer(),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{sourcePath, newTargetPath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}
	plan, err := engine.ApplyFixes(context.Background(), diagnostics)
	if err != nil {
		t.Fatalf("ApplyFixes returned error: %v", err)
	}
	if len(plan.Accepted) != 1 {
		t.Fatalf("expected one accepted edit, got %#v", plan)
	}

	content, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if string(content) != "# Source\n\nBefore [target](nested/old.md) after.\n" {
		t.Fatalf("unexpected applied content: %q", content)
	}
}
