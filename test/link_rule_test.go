package markdownlint_test

import (
	"context"
	engine "github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	rules "github.com/portpowered/markdown-linter/pkg/rules"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalLinkRule_RelativeLinks_ReportsMissingTargetFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.md")
	writeMarkdownFile(t, path, "# Source\n\nSee [missing](missing.md).\n")

	violations := runLocalLinkRule(t, path)

	requireSingleViolation(t, violations, interfaces.Violation{
		Path:    path,
		Line:    3,
		RuleID:  "markdown.link",
		Message: "local link target does not exist: missing.md",
	})
}

func TestLocalLinkRule_SameFileAnchor_ReportsMissingAnchor(t *testing.T) {
	path := writeTempMarkdown(t, "# Source\n\nSee [missing](#missing-heading).\n")

	violations := runLocalLinkRule(t, path)

	requireSingleViolation(t, violations, interfaces.Violation{
		Path:    path,
		Line:    3,
		RuleID:  "markdown.link",
		Message: "local link anchor does not exist: #missing-heading",
	})
}

func TestLocalLinkRule_CrossFileAnchor_ReportsMissingAnchor(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.md")
	targetPath := filepath.Join(dir, "target.md")
	writeMarkdownFile(t, sourcePath, "# Source\n\nSee [target](target.md#missing-heading).\n")
	writeMarkdownFile(t, targetPath, "# Target\n\n## Existing Heading\n")

	violations := runLocalLinkRule(t, sourcePath)

	requireSingleViolation(t, violations, interfaces.Violation{
		Path:    sourcePath,
		Line:    3,
		RuleID:  "markdown.link",
		Message: "local link anchor does not exist: target.md#missing-heading",
	})
}

func TestLocalLinkRule_LocalLinks_AllowsExistingFilesAndAnchors(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.md")
	targetPath := filepath.Join(dir, "target.md")
	writeMarkdownFile(t, sourcePath, "# Source\n\nSee [self](#source) and [target](target.md#existing-heading).\n")
	writeMarkdownFile(t, targetPath, "# Target\n\n## Existing Heading\n")

	violations := runLocalLinkRule(t, sourcePath)

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}

func TestLocalLinkRule_Images_ReportsMissingTargetFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.md")
	writeMarkdownFile(t, path, "# Source\n\n![missing](missing.png)\n")

	violations := runLocalLinkRule(t, path)

	requireSingleViolation(t, violations, interfaces.Violation{
		Path:    path,
		Line:    3,
		RuleID:  "markdown.link",
		Message: "local link target does not exist: missing.png",
	})
}

func TestLocalLinkRule_DirectoryTargets_CanBeAllowed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.md")
	if err := os.Mkdir(filepath.Join(dir, "target"), 0o700); err != nil {
		t.Fatalf("create target directory: %v", err)
	}
	writeMarkdownFile(t, path, "# Source\n\nSee [directory](target).\n")

	l := engine.New(engine.WithRules(
		rules.NewLocalLinkRule(rules.WithDirectoryLinkTargetsAllowed()),
	))
	violations, err := l.RunFile(context.Background(), path)
	if err != nil {
		t.Fatalf("RunFile returned error: %v", err)
	}

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}

func TestLocalLinkRule_ExternalLinks_IgnoresExternalURLs(t *testing.T) {
	path := writeTempMarkdown(t, "# Source\n\nSee [external](https://example.com/missing#anchor).\n")

	violations := runLocalLinkRule(t, path)

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}

func runLocalLinkRule(t *testing.T, path string) []interfaces.Violation {
	t.Helper()

	l := engine.New(engine.WithRules(rules.NewLocalLinkRule()))
	violations, err := l.RunFile(context.Background(), path)
	if err != nil {
		t.Fatalf("RunFile returned error: %v", err)
	}

	return violations
}

func requireSingleViolation(t *testing.T, violations []interfaces.Violation, want interfaces.Violation) {
	t.Helper()

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %#v", len(violations), violations)
	}

	got := violations[0]
	if got != want {
		t.Fatalf("unexpected violation:\nwant: %#v\n got: %#v", want, got)
	}
}

func writeMarkdownFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
