package markdownlint_test

import (
	"context"
	engine "github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	rules "github.com/portpowered/markdown-linter/pkg/rules"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDocIDUniquenessAnalyzer_ReportsDuplicateDocIDWithAllFiles(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "docs", "standards", "first.md")
	secondPath := filepath.Join(dir, "docs", "standards", "nested", "second.md")
	writeMarkdownFileWithDirs(t, firstPath, "---\ndoc-id: STD-011\n---\n# First\n")
	writeMarkdownFileWithDirs(t, secondPath, "---\ndoc-id: STD-011\n---\n# Second\n")

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		rules.NewDocIDUniquenessAnalyzer(),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{firstPath, secondPath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("expected 1 duplicate doc-id diagnostic, got %d: %#v", len(diagnostics), diagnostics)
	}
	diagnostic := diagnostics[0]
	if diagnostic.RuleID != "markdown.doc-id-unique" {
		t.Fatalf("expected doc-id rule, got %q", diagnostic.RuleID)
	}
	if diagnostic.Line != 2 {
		t.Fatalf("expected diagnostic on doc-id frontmatter line 2, got %d", diagnostic.Line)
	}
	message := normalizeDocIDTestPath(diagnostic.Message)
	if !strings.Contains(message, "duplicate doc-id STD-011") {
		t.Fatalf("diagnostic did not include duplicated ID: %q", diagnostic.Message)
	}
	if !strings.Contains(message, normalizeDocIDTestPath(firstPath)) || !strings.Contains(message, normalizeDocIDTestPath(secondPath)) {
		t.Fatalf("diagnostic did not include both files: %q", diagnostic.Message)
	}
}

func TestDocIDUniquenessAnalyzer_AllowsUniqueDocIDsAndMissingDocIDs(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "docs", "standards", "first.md")
	secondPath := filepath.Join(dir, "docs", "standards", "second.md")
	withoutDocIDPath := filepath.Join(dir, "docs", "guide.md")
	writeMarkdownFileWithDirs(t, firstPath, "---\ndoc-id: STD-011\n---\n# First\n")
	writeMarkdownFileWithDirs(t, secondPath, "---\ndoc-id: STD-012\n---\n# Second\n")
	writeMarkdownFileWithDirs(t, withoutDocIDPath, "# Guide\n")

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		rules.NewDocIDUniquenessAnalyzer(),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{firstPath, secondPath, withoutDocIDPath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 0 {
		t.Fatalf("expected no duplicate doc-id diagnostics, got %#v", diagnostics)
	}
}

func TestDocumentIdentifierAnalyzer_ValidatesConfiguredFormatsAndProcessFilename(t *testing.T) {
	dir := t.TempDir()
	processPath := filepath.Join(dir, "docs", "processes", "example-process.md")
	standardPath := filepath.Join(dir, "docs", "standards", "example-standard.md")
	intentPath := filepath.Join(dir, "docs", "intents", "example-intent.md")
	architecturePath := filepath.Join(dir, "docs", "architecture", "example-system.md")
	writeMarkdownFileWithDirs(t, processPath, "---\ndoc-id: STD-100\nprocess-id: wrong-process\n---\n# Process\n")
	writeMarkdownFileWithDirs(t, standardPath, "---\ndoc-id: PROC-100\n---\n# Standard\n")
	writeMarkdownFileWithDirs(t, intentPath, "---\ndoc-id: STD-101\n---\n# Intent\n")
	writeMarkdownFileWithDirs(t, architecturePath, "---\ndoc-id: ARCH-1\n---\n# Architecture\n")

	diagnostics := runDocsCheckIdentifierDiagnostics(t, []string{processPath, standardPath, intentPath, architecturePath})
	messages := documentIdentifierMessages(diagnostics)

	for _, expected := range []string{
		"docs/processes/example-process.md has invalid process doc-id STD-100.",
		"docs/processes/example-process.md process-id must match filename (example-process).",
		"docs/standards/example-standard.md has invalid standard doc-id PROC-100.",
		"docs/intents/example-intent.md has invalid intent doc-id STD-101.",
		"docs/architecture/example-system.md has invalid architecture doc-id ARCH-1.",
	} {
		if !strings.Contains(messages, expected) {
			t.Fatalf("expected diagnostic %q in:\n%s", expected, messages)
		}
	}
}

func TestDocumentIdentifierAnalyzer_ReportsDuplicateConfiguredDocIDAcrossDocTypes(t *testing.T) {
	dir := t.TempDir()
	processPath := filepath.Join(dir, "docs", "processes", "example-process.md")
	architecturePath := filepath.Join(dir, "docs", "architecture", "example-system.md")
	writeMarkdownFileWithDirs(t, processPath, "---\ndoc-id: DOC-100\n---\n# Process\n")
	writeMarkdownFileWithDirs(t, architecturePath, "---\ndoc-id: DOC-100\n---\n# Architecture\n")

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		rules.NewDocumentIdentifierAnalyzer(rules.DocumentIdentifierConfig{
			Name:         "shared doc-id",
			Field:        "doc-id",
			PathPrefixes: []string{"docs/processes", "docs/architecture"},
			Required:     true,
			Format:       regexp.MustCompile(`^DOC-\d+$`),
			FormatLabel:  "shared",
			Unique:       true,
		}),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{processPath, architecturePath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("expected 1 duplicate identifier diagnostic, got %d: %#v", len(diagnostics), diagnostics)
	}
	message := normalizeDocIDTestPath(diagnostics[0].Message)
	if !strings.Contains(message, "duplicate doc-id DOC-100 declared in:") {
		t.Fatalf("diagnostic did not include duplicated ID: %q", diagnostics[0].Message)
	}
	if !strings.Contains(message, "docs/processes/example-process.md") || !strings.Contains(message, "docs/architecture/example-system.md") {
		t.Fatalf("diagnostic did not include both files: %q", diagnostics[0].Message)
	}
}

func TestDocumentIdentifierAnalyzer_ReportsDuplicateProcessIDs(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "docs", "processes", "first.md")
	secondPath := filepath.Join(dir, "docs", "processes", "second.md")
	writeMarkdownFileWithDirs(t, firstPath, "---\ndoc-id: PROC-101\nprocess-id: duplicate-process\n---\n# First\n")
	writeMarkdownFileWithDirs(t, secondPath, "---\ndoc-id: PROC-102\nprocess-id: duplicate-process\n---\n# Second\n")

	diagnostics := runDocsCheckIdentifierDiagnostics(t, []string{firstPath, secondPath})
	messages := documentIdentifierMessages(diagnostics)

	if !strings.Contains(messages, "duplicate process-id DUPLICATE-PROCESS declared in:") {
		t.Fatalf("expected duplicate process-id diagnostic in:\n%s", messages)
	}
	if !strings.Contains(messages, "docs/processes/first.md") || !strings.Contains(messages, "docs/processes/second.md") {
		t.Fatalf("expected duplicate process-id message to include both files:\n%s", messages)
	}
}

func TestDocIDUniquenessAnalyzer_SuggestsNextTypedIDForSingleEligibleDuplicate(t *testing.T) {
	dir := t.TempDir()
	canonicalPath := filepath.Join(dir, "docs", "standards", "alpha.md")
	duplicatePath := filepath.Join(dir, "docs", "standards", "zeta.md")
	otherPath := filepath.Join(dir, "docs", "standards", "other.md")
	writeMarkdownFileWithDirs(t, canonicalPath, "---\ndoc-id: STD-011\n---\n# Alpha\n")
	writeMarkdownFileWithDirs(t, duplicatePath, "---\ndoc-id: STD-011\n---\n# Zeta\n")
	writeMarkdownFileWithDirs(t, otherPath, "---\ndoc-id: STD-012\n---\n# Other\n")

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		rules.NewDocIDUniquenessAnalyzer(),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{canonicalPath, duplicatePath, otherPath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("expected 1 duplicate doc-id diagnostic, got %d: %#v", len(diagnostics), diagnostics)
	}
	diagnostic := diagnostics[0]
	if diagnostic.Path != normalizeDocIDTestPath(duplicatePath) {
		t.Fatalf("expected diagnostic on eligible duplicate path %q, got %q", duplicatePath, diagnostic.Path)
	}
	if len(diagnostic.SuggestedFixes) != 1 {
		t.Fatalf("expected one safe suggested fix, got %#v", diagnostic.SuggestedFixes)
	}
	fix := diagnostic.SuggestedFixes[0]
	if fix.Confidence != interfaces.FixConfidenceSafe {
		t.Fatalf("expected safe confidence, got %q", fix.Confidence)
	}
	if len(fix.Edits) != 1 {
		t.Fatalf("expected one exact frontmatter edit, got %#v", fix.Edits)
	}
	if fix.Edits[0].Replacement != "STD-013" {
		t.Fatalf("expected next unused replacement STD-013, got %q", fix.Edits[0].Replacement)
	}
	plan := engine.PlanFixes(diagnostics, map[string][]byte{
		normalizeDocIDTestPath(duplicatePath): []byte("---\ndoc-id: STD-011\n---\n# Zeta\n"),
	})
	if len(plan.Accepted) != 1 || len(plan.Rejected) != 0 {
		t.Fatalf("expected one accepted safe fix and no rejections, got %#v", plan)
	}
}

func TestDocIDUniquenessAnalyzer_ReportsAmbiguousDuplicatesWithoutSafeFix(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "docs", "standards", "alpha.md")
	secondPath := filepath.Join(dir, "docs", "standards", "beta.md")
	thirdPath := filepath.Join(dir, "docs", "standards", "gamma.md")
	writeMarkdownFileWithDirs(t, firstPath, "---\ndoc-id: STD-011\n---\n# Alpha\n")
	writeMarkdownFileWithDirs(t, secondPath, "---\ndoc-id: STD-011\n---\n# Beta\n")
	writeMarkdownFileWithDirs(t, thirdPath, "---\ndoc-id: STD-011\n---\n# Gamma\n")

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		rules.NewDocIDUniquenessAnalyzer(),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{firstPath, secondPath, thirdPath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 1 {
		t.Fatalf("expected 1 duplicate doc-id diagnostic, got %d: %#v", len(diagnostics), diagnostics)
	}
	if len(diagnostics[0].SuggestedFixes) != 0 {
		t.Fatalf("expected ambiguous duplicate group without safe fix, got %#v", diagnostics[0].SuggestedFixes)
	}
}

func TestDocIDUniquenessAnalyzer_ReservesReplacementsAcrossDuplicateGroups(t *testing.T) {
	dir := t.TempDir()
	alphaPath := filepath.Join(dir, "docs", "standards", "alpha.md")
	zetaPath := filepath.Join(dir, "docs", "standards", "zeta.md")
	betaPath := filepath.Join(dir, "docs", "standards", "beta.md")
	yankeePath := filepath.Join(dir, "docs", "standards", "yankee.md")
	existingPath := filepath.Join(dir, "docs", "standards", "existing.md")
	writeMarkdownFileWithDirs(t, alphaPath, "---\ndoc-id: STD-011\n---\n# Alpha\n")
	writeMarkdownFileWithDirs(t, zetaPath, "---\ndoc-id: STD-011\n---\n# Zeta\n")
	writeMarkdownFileWithDirs(t, betaPath, "---\ndoc-id: STD-012\n---\n# Beta\n")
	writeMarkdownFileWithDirs(t, yankeePath, "---\ndoc-id: STD-012\n---\n# Yankee\n")
	writeMarkdownFileWithDirs(t, existingPath, "---\ndoc-id: STD-013\n---\n# Existing\n")

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		rules.NewDocIDUniquenessAnalyzer(),
	))

	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{
		alphaPath,
		zetaPath,
		betaPath,
		yankeePath,
		existingPath,
	})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if len(diagnostics) != 2 {
		t.Fatalf("expected 2 duplicate doc-id diagnostics, got %d: %#v", len(diagnostics), diagnostics)
	}
	replacements := map[string]bool{}
	for _, diagnostic := range diagnostics {
		if len(diagnostic.SuggestedFixes) != 1 {
			t.Fatalf("expected diagnostic to have one safe fix, got %#v", diagnostic.SuggestedFixes)
		}
		if len(diagnostic.SuggestedFixes[0].Edits) != 1 {
			t.Fatalf("expected diagnostic to have one edit, got %#v", diagnostic.SuggestedFixes[0].Edits)
		}
		replacements[diagnostic.SuggestedFixes[0].Edits[0].Replacement] = true
	}
	if !replacements["STD-014"] || !replacements["STD-015"] || len(replacements) != 2 {
		t.Fatalf("expected reserved unique replacements STD-014 and STD-015, got %#v", replacements)
	}
}

func TestDocIDUniquenessAnalyzer_DryRunAndApplyRewriteOnlyEligibleFrontmatterValue(t *testing.T) {
	dir := t.TempDir()
	canonicalPath := filepath.Join(dir, "docs", "standards", "alpha.md")
	duplicatePath := filepath.Join(dir, "docs", "standards", "zeta.md")
	writeMarkdownFileWithDirs(t, canonicalPath, "---\ndoc-id: STD-011\n---\n# Alpha\n")
	writeMarkdownFileWithDirs(t, duplicatePath, "---\nauthor: docs\ndoc-id: STD-011\n---\n# Zeta\n\nSTD-011 should remain in prose.\n")

	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		rules.NewDocIDUniquenessAnalyzer(),
	))
	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), []string{canonicalPath, duplicatePath})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	dryRunPlan, err := engine.DryRunFixes(context.Background(), diagnostics)
	if err != nil {
		t.Fatalf("DryRunFixes returned error: %v", err)
	}
	if len(dryRunPlan.Accepted) != 1 {
		t.Fatalf("expected dry run to propose one accepted edit, got %#v", dryRunPlan)
	}
	if dryRunPlan.Accepted[0].TextEdit.Replacement != "STD-012" {
		t.Fatalf("expected dry-run replacement STD-012, got %q", dryRunPlan.Accepted[0].TextEdit.Replacement)
	}
	beforeApply, err := os.ReadFile(duplicatePath)
	if err != nil {
		t.Fatalf("read duplicate before apply: %v", err)
	}
	if !strings.Contains(string(beforeApply), "doc-id: STD-011") {
		t.Fatalf("dry run changed duplicate file: %q", beforeApply)
	}

	applyPlan, err := engine.ApplyFixes(context.Background(), diagnostics)
	if err != nil {
		t.Fatalf("ApplyFixes returned error: %v", err)
	}
	if len(applyPlan.Accepted) != 1 {
		t.Fatalf("expected apply to accept one edit, got %#v", applyPlan)
	}
	updated, err := os.ReadFile(duplicatePath)
	if err != nil {
		t.Fatalf("read duplicate after apply: %v", err)
	}
	want := "---\nauthor: docs\ndoc-id: STD-012\n---\n# Zeta\n\nSTD-011 should remain in prose.\n"
	if string(updated) != want {
		t.Fatalf("unexpected applied content: %q", updated)
	}
}

func normalizeDocIDTestPath(value string) string {
	return filepath.ToSlash(filepath.Clean(value))
}

func runDocsCheckIdentifierDiagnostics(t *testing.T, paths []string) []interfaces.Diagnostic {
	t.Helper()
	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		rules.NewDocumentIdentifierAnalyzer(customerIdentifierConfigs()...),
	))
	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), paths)
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}
	return diagnostics
}

func documentIdentifierMessages(diagnostics []interfaces.Diagnostic) string {
	messages := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		messages = append(messages, normalizeDocIDTestPath(diagnostic.Message))
	}
	return strings.Join(messages, "\n")
}
