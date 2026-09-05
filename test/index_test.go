package markdownlint_test

import (
	"context"
	engine "github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	rules "github.com/portpowered/markdown-linter/pkg/rules"
	"path/filepath"
	"testing"
)

func TestDocumentationIndex_RecordsDocumentsIdentifiersAnchorsAndLocalLinks(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "docs", "source.md")
	targetPath := filepath.Join(dir, "docs", "processes", "target.md")
	imagePath := filepath.Join(dir, "docs", "assets", "diagram.png")
	writeMarkdownFileWithDirs(t, sourcePath, "---\ndoc-id: DOC-001\nprocess-id: source-process\n---\n# Source Title\n\nSee [target](processes/target.md#deep-heading).\n\n![diagram](assets/diagram.png)\n")
	writeMarkdownFileWithDirs(t, targetPath, "---\ndoc-id: DOC-002\nprocess-id: target-process\n---\n# Target Title\n\n## Deep Heading\n")
	writeBinaryFileWithDirs(t, imagePath, []byte("png"))

	docs := parseMarkdownFiles(t, sourcePath, targetPath)
	index := rules.NewDocumentationIndex(docs)

	if len(index.Documents) != 2 {
		t.Fatalf("expected 2 indexed documents, got %d", len(index.Documents))
	}

	source, ok := index.Document(sourcePath)
	if !ok {
		t.Fatalf("expected source document to be indexed")
	}
	if source.Path != filepath.ToSlash(filepath.Clean(sourcePath)) {
		t.Fatalf("expected normalized path, got %q", source.Path)
	}
	if source.Title != "Source Title" {
		t.Fatalf("expected title from heading, got %q", source.Title)
	}
	if source.DocID != "DOC-001" || source.ProcessID != "source-process" {
		t.Fatalf("expected frontmatter identifiers, got doc-id=%q process-id=%q", source.DocID, source.ProcessID)
	}
	if _, ok := source.Anchors["source-title"]; !ok {
		t.Fatalf("expected generated source-title anchor, got %#v", source.Anchors)
	}

	if len(index.Links) != 2 {
		t.Fatalf("expected 2 local links, got %d: %#v", len(index.Links), index.Links)
	}

	markdownLink := findIndexedLink(t, index.Links, rules.IndexedLinkKindMarkdown)
	if markdownLink.RawTarget != "processes/target.md#deep-heading" {
		t.Fatalf("expected raw markdown target, got %q", markdownLink.RawTarget)
	}
	if markdownLink.Anchor != "deep-heading" {
		t.Fatalf("expected anchor deep-heading, got %q", markdownLink.Anchor)
	}
	if markdownLink.NormalizedFile != filepath.ToSlash(filepath.Clean(targetPath)) {
		t.Fatalf("expected normalized markdown target %q, got %q", filepath.ToSlash(filepath.Clean(targetPath)), markdownLink.NormalizedFile)
	}
	if markdownLink.DestinationRange.StartOffset < 0 || markdownLink.DestinationRange.EndOffset <= markdownLink.DestinationRange.StartOffset {
		t.Fatalf("expected markdown destination source range, got %#v", markdownLink.DestinationRange)
	}

	imageLink := findIndexedLink(t, index.Links, rules.IndexedLinkKindImage)
	if imageLink.RawTarget != "assets/diagram.png" {
		t.Fatalf("expected raw image target, got %q", imageLink.RawTarget)
	}
	if imageLink.NormalizedFile != filepath.ToSlash(filepath.Clean(imagePath)) {
		t.Fatalf("expected normalized image target %q, got %q", filepath.ToSlash(filepath.Clean(imagePath)), imageLink.NormalizedFile)
	}
	if len(index.AssetPaths) != 1 || index.AssetPaths[0] != filepath.ToSlash(filepath.Clean(imagePath)) {
		t.Fatalf("expected image asset path to be indexed, got %#v", index.AssetPaths)
	}
}

func TestDocumentationIndex_RecordsFrontmatterAfterTitle(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "docs", "standards", "standard.md")
	writeMarkdownFileWithDirs(t, sourcePath, "# Standard\n\n---\ndoc-id: STD-011\n---\n\n## Usage\n")

	index := rules.NewDocumentationIndex(parseMarkdownFiles(t, sourcePath))
	source, ok := index.Document(sourcePath)
	if !ok {
		t.Fatalf("expected indexed source document")
	}

	if source.DocID != "STD-011" {
		t.Fatalf("expected doc-id from title-prefixed frontmatter, got %q", source.DocID)
	}
	field := source.FrontmatterFields["doc-id"]
	if field.Line != 4 {
		t.Fatalf("expected doc-id on line 4, got %d", field.Line)
	}
}

func TestDocumentationIndex_ResolveDocument_PrefersMoveMappingAndStableIdentifiers(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "docs", "processes", "old-name.md")
	newPath := filepath.Join(dir, "docs", "processes", "backend", "new-name.md")
	writeMarkdownFileWithDirs(t, newPath, "---\nprocess-id: backend-feature\n---\n# Backend Feature\n")

	docs := parseMarkdownFiles(t, newPath)
	index := rules.NewDocumentationIndex(docs, rules.WithMoveMappings(map[string]string{
		oldPath: newPath,
	}))

	moved := index.ResolveDocument(oldPath)
	if moved.Target == nil {
		t.Fatalf("expected move mapping to resolve a target, got %#v", moved)
	}
	if moved.Strategy != "move-mapping" {
		t.Fatalf("expected move-mapping strategy, got %q", moved.Strategy)
	}
	if moved.Target.Path != filepath.ToSlash(filepath.Clean(newPath)) {
		t.Fatalf("expected moved target %q, got %q", filepath.ToSlash(filepath.Clean(newPath)), moved.Target.Path)
	}

	process := index.ResolveDocument("backend-feature")
	if process.Target == nil {
		t.Fatalf("expected process-id to resolve a target, got %#v", process)
	}
	if process.Strategy != "process-id" {
		t.Fatalf("expected process-id strategy, got %q", process.Strategy)
	}
}

func TestDocumentationIndex_ResolveDocument_AmbiguousBasenameDoesNotReturnSafeTarget(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "docs", "backend", "common.md")
	secondPath := filepath.Join(dir, "docs", "website", "common.md")
	writeMarkdownFileWithDirs(t, firstPath, "# Backend Common\n")
	writeMarkdownFileWithDirs(t, secondPath, "# Website Common\n")

	docs := parseMarkdownFiles(t, firstPath, secondPath)
	index := rules.NewDocumentationIndex(docs)

	resolution := index.ResolveDocument("common.md")
	if !resolution.Ambiguous {
		t.Fatalf("expected ambiguous basename resolution, got %#v", resolution)
	}
	if resolution.Target != nil {
		t.Fatalf("expected no safe target for ambiguous resolution, got %#v", resolution.Target)
	}
	if len(resolution.Candidates) != 2 {
		t.Fatalf("expected 2 ambiguous candidates, got %d", len(resolution.Candidates))
	}
}

func TestDocumentationIndex_ResolveAssetExactFilename(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "docs", "guide", "source.md")
	assetPath := filepath.Join(dir, "docs", "assets", "diagram.png")
	writeMarkdownFileWithDirs(t, sourcePath, "# Source\n\n![diagram](../old-assets/diagram.png)\n")
	writeBinaryFileWithDirs(t, assetPath, []byte("png"))

	index := rules.NewDocumentationIndex(parseMarkdownFiles(t, sourcePath))

	resolution := index.ResolveAssetExactFilename(filepath.Join(dir, "docs", "old-assets", "diagram.png"))
	if resolution.Target != filepath.ToSlash(filepath.Clean(assetPath)) {
		t.Fatalf("expected unique asset target %q, got %#v", filepath.ToSlash(filepath.Clean(assetPath)), resolution)
	}
}

func TestDocumentationIndexAnalyzer_SharesIndexThroughPassData(t *testing.T) {
	path := writeTempMarkdown(t, "# Shared Index\n")
	capture := &captureDocumentationIndexAnalyzer{}
	linter := engine.New(engine.WithAnalyzers(
		rules.NewDocumentationIndexAnalyzer(),
		capture,
	))

	if _, err := linter.RunFilesDiagnostics(context.Background(), []string{path}); err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	if capture.index == nil {
		t.Fatal("expected documentation index pass data")
	}
	if _, ok := capture.index.Document(path); !ok {
		t.Fatalf("expected document %q in pass index", path)
	}
}

type captureDocumentationIndexAnalyzer struct {
	index *rules.DocumentationIndex
}

func (a *captureDocumentationIndexAnalyzer) ID() string {
	return "test.capture-documentation-index"
}

func (a *captureDocumentationIndexAnalyzer) Analyze(_ context.Context, pass *interfaces.Pass) {
	value, ok := pass.Data(rules.DocumentationIndexPassDataKey)
	if !ok {
		return
	}
	a.index, _ = value.(*rules.DocumentationIndex)
}

func parseMarkdownFiles(t *testing.T, paths ...string) []*interfaces.Document {
	t.Helper()

	linter := engine.New()
	docs := make([]*interfaces.Document, 0, len(paths))
	for _, path := range paths {
		doc, err := linter.ParseFile(path)
		if err != nil {
			t.Fatalf("ParseFile(%q): %v", path, err)
		}
		docs = append(docs, doc)
	}
	return docs
}

func findIndexedLink(t *testing.T, links []rules.IndexedLink, kind rules.IndexedLinkKind) rules.IndexedLink {
	t.Helper()

	for _, link := range links {
		if link.Kind == kind {
			return link
		}
	}
	t.Fatalf("expected indexed link kind %q in %#v", kind, links)
	return rules.IndexedLink{}
}

func writeBinaryFileWithDirs(t *testing.T, path string, content []byte) {
	t.Helper()

	writeMarkdownFileWithDirs(t, path, string(content))
}
