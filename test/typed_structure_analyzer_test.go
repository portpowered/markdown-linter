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

func TestTypedDocumentStructureAnalyzer_AllowsRepresentativeTypedDocs(t *testing.T) {
	dir := t.TempDir()
	paths := []string{
		filepath.Join(dir, "docs", "processes", "example-process.md"),
		filepath.Join(dir, "docs", "standards", "example-standard.md"),
		filepath.Join(dir, "docs", "intents", "example-intent.md"),
		filepath.Join(dir, "docs", "architecture", "example-system.md"),
	}
	writeMarkdownFileWithDirs(t, paths[0], validProcessDoc("PROC-100", "example-process"))
	writeMarkdownFileWithDirs(t, paths[1], validStandardDoc("STD-100"))
	writeMarkdownFileWithDirs(t, paths[2], validIntentDoc("INTENT-100"))
	writeMarkdownFileWithDirs(t, paths[3], validArchitectureDoc("ARCH-100"))

	diagnostics := runTypedStructureDiagnostics(t, paths)

	if len(diagnostics) != 0 {
		t.Fatalf("expected representative typed docs to pass, got %#v", diagnostics)
	}
}

func TestTypedDocumentStructureAnalyzer_ReportsTypedDocStructureFailures(t *testing.T) {
	dir := t.TempDir()
	processPath := filepath.Join(dir, "docs", "processes", "example-process.md")
	standardPath := filepath.Join(dir, "docs", "standards", "example-standard.md")
	intentPath := filepath.Join(dir, "docs", "intents", "example-intent.md")
	architecturePath := filepath.Join(dir, "docs", "architecture", "example-system.md")
	writeMarkdownFileWithDirs(t, processPath, strings.Join([]string{
		"---",
		"author: docs",
		"last modified: 2026, april, 12",
		"doc-id: STD-100",
		"process-id: wrong-process",
		"---",
		"# Example Process",
		"",
		"## Purpose",
		"## Scope",
		"## Prerequisites",
		"## Procedure",
		"",
	}, "\n"))
	writeMarkdownFileWithDirs(t, standardPath, strings.Join([]string{
		"---",
		"author: docs",
		"last modified: 2026, april, 12",
		"doc-id: PROC-100",
		"---",
		"# Example Standard",
		"",
		"## Usage",
		"## References",
		"## Changelog",
		"",
	}, "\n"))
	writeMarkdownFileWithDirs(t, intentPath, strings.Join([]string{
		"---",
		"author: docs",
		"last modified: 2026, april, 12",
		"doc-id: STD-101",
		"---",
		"# Example Intent",
		"",
		"## Purpose & Vision",
		"## Intended Users",
		"## Core Capabilities",
		"## Intended Workflows",
		"## Boundaries",
		"## Success Indicators",
		"## Related Intents",
		"",
	}, "\n"))
	writeMarkdownFileWithDirs(t, architecturePath, strings.Join([]string{
		"---",
		"last modified: 2026, april, 12",
		"doc-id: ARCH-1",
		"---",
		"# Example System",
		"",
		"## Overview",
		"## Design Decisions",
		"",
	}, "\n"))

	diagnostics := runTypedStructureDiagnostics(t, []string{processPath, standardPath, intentPath, architecturePath})
	messages := typedStructureMessages(diagnostics)

	for _, expected := range []string{
		"docs/processes/example-process.md must include a Verification or Checklist section.",
		"docs/standards/example-standard.md must include a Standard Summary or Quick Rules section.",
		"docs/intents/example-intent.md is missing frontmatter field component.",
		"docs/architecture/example-system.md is missing frontmatter field author.",
		"docs/architecture/example-system.md is missing required heading \"System Context\".",
	} {
		if !strings.Contains(messages, expected) {
			t.Fatalf("expected diagnostic %q in:\n%s", expected, messages)
		}
	}
}

func TestTypedDocumentStructureAnalyzer_ReportsMissingYAMLFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docs", "architecture", "example-system.md")
	writeMarkdownFileWithDirs(t, path, "# Example System\n\n## Overview\n\n## System Context\n\n## Design Decisions\n")

	diagnostics := runTypedStructureDiagnostics(t, []string{path})

	if len(diagnostics) != 1 {
		t.Fatalf("expected one missing frontmatter diagnostic, got %#v", diagnostics)
	}
	if !strings.Contains(normalizeDocIDTestPath(diagnostics[0].Message), "docs/architecture/example-system.md is missing YAML frontmatter.") {
		t.Fatalf("unexpected missing frontmatter diagnostic: %#v", diagnostics[0])
	}
}

func TestTypedDocumentStructureAnalyzer_SkipsDocsHubAndSelectedPackageReadme(t *testing.T) {
	dir := t.TempDir()
	docsHubPath := filepath.Join(dir, "docs", "README.md")
	packageReadmePath := filepath.Join(dir, "backend", "pkg", "api", "plugins", "README.md")
	writeMarkdownFileWithDirs(t, docsHubPath, "# Documentation Hub\n")
	writeMarkdownFileWithDirs(t, packageReadmePath, "# Plugin API\n\nContextual package documentation.\n")

	diagnostics := runTypedStructureDiagnostics(t, []string{docsHubPath, packageReadmePath})

	if len(diagnostics) != 0 {
		t.Fatalf("expected docs hub and selected package README to skip typed structure, got %#v", diagnostics)
	}
}

func TestTypedDocumentStructureAnalyzer_AcceptsTitlePrefixedFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docs", "standards", "title-prefixed.md")
	writeMarkdownFileWithDirs(t, path, "# Title Prefixed\n\n---\nauthor: docs\nlast modified: 2026, april, 12\ndoc-id: STD-101\n---\n\n## Usage\n\n## Standard Summary\n\n## References\n\n## Changelog\n")

	diagnostics := runTypedStructureDiagnostics(t, []string{path})

	if len(diagnostics) != 0 {
		t.Fatalf("expected title-prefixed frontmatter to pass, got %#v", diagnostics)
	}
}

func runTypedStructureDiagnostics(t *testing.T, paths []string) []interfaces.Diagnostic {
	t.Helper()
	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		rules.NewTypedDocumentStructureAnalyzer(customerStructureConfigs()...),
	))
	diagnostics, err := linter.RunFilesDiagnostics(context.Background(), paths)
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}
	return diagnostics
}

func typedStructureMessages(diagnostics []interfaces.Diagnostic) string {
	messages := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		messages = append(messages, normalizeDocIDTestPath(diagnostic.Message))
	}
	return strings.Join(messages, "\n")
}

func validProcessDoc(docID string, processID string) string {
	return strings.Join([]string{
		"---",
		"author: docs",
		"last modified: 2026, april, 12",
		"doc-id: " + docID,
		"process-id: " + processID,
		"---",
		"# Example Process",
		"",
		"## Purpose",
		"## Scope",
		"## Prerequisites",
		"## Procedure",
		"## Verification",
		"",
	}, "\n")
}

func validStandardDoc(docID string) string {
	return strings.Join([]string{
		"---",
		"author: docs",
		"last modified: 2026, april, 12",
		"doc-id: " + docID,
		"---",
		"# Example Standard",
		"",
		"## Usage",
		"## Standard Summary",
		"## References",
		"## Changelog",
		"",
	}, "\n")
}

func validIntentDoc(docID string) string {
	return strings.Join([]string{
		"---",
		"author: docs",
		"last modified: 2026, april, 12",
		"component: example",
		"doc-id: " + docID,
		"---",
		"# Example Intent",
		"",
		"## Purpose & Vision",
		"## Intended Users",
		"## Core Capabilities",
		"## Intended Workflows",
		"## Boundaries",
		"## Success Indicators",
		"## Related Intents",
		"",
	}, "\n")
}

func validArchitectureDoc(docID string) string {
	return strings.Join([]string{
		"---",
		"author: docs",
		"last modified: 2026, april, 12",
		"doc-id: " + docID,
		"---",
		"# Example System",
		"",
		"## Overview",
		"## System Context",
		"## Design Decisions",
		"",
	}, "\n")
}
