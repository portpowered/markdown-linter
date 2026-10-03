package rules

import (
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"strings"
	"testing"
)

func TestFrontmatterSourceRangeAndFallbackTitle(t *testing.T) {
	for _, source := range []string{"---\n: ignored\nline without delimiter\n title : 'Quoted'\nshort: x\n", "# Title\n\n---\ntitle: \"Quoted\"\n---\n"} {
		fields := parseFrontmatterFields([]byte(source))
		f, ok := fields["title"]
		if !ok || f.Value != "Quoted" || source[f.ValueStartOffset:f.ValueEndOffset] != "Quoted" {
			t.Fatalf("source=%q fields=%+v", source, fields)
		}
	}
	if _, _, ok := frontmatterBlockStart(nil); ok {
		t.Fatal("empty frontmatter")
	}
	doc := newIndexedDocument(coverageDocument("guide.md", "---\ntitle: Fallback\nText\n"))
	if doc.Title != "Fallback" {
		t.Fatal(doc.Title)
	}
	doc = newIndexedDocument(coverageDocument("guide.md", "## Fallback\n"))
	if doc.Title != "Fallback" {
		t.Fatal(doc.Title)
	}
	if n, err := typedDocIDNumber("untyped"); err == nil || n != 0 {
		t.Fatalf("%d %v", n, err)
	}
	index := NewDocumentationIndex([]*interfaces.Document{coverageDocument("one.md", "---\ndoc-id: DOC-001\n---\n# One\n"), coverageDocument("other.md", "---\ndoc-id: OTHER-200\n---\n# Other\n")}, WithoutAssetDiscovery())
	if got, ok := nextUnusedTypedDocID(index, "DOC-001", map[string]struct{}{"DOC-004": {}, "OTHER-999": {}, "invalid": {}, "DOC-" + strings.Repeat("9", 100): {}}); !ok || got != "DOC-005" {
		t.Fatalf("%s %v", got, ok)
	}
	if _, ok := nextUnusedTypedDocID(index, "untyped", nil); ok {
		t.Fatal("untyped replacement")
	}
	if _, ok := safeIdentifierReassignmentFix(index, DocumentIdentifierConfig{Field: "doc-id"}, []*IndexedDocument{{DocID: "untyped"}, {}}, nil); ok {
		t.Fatal("unsafe missing field replacement")
	}
	if _, ok := safeIdentifierReassignmentFix(index, DocumentIdentifierConfig{Field: "doc-id"}, []*IndexedDocument{{DocID: "untyped"}, {FrontmatterFields: map[string]IndexedFrontmatterField{"doc-id": {ValueStartOffset: 0, ValueEndOffset: 1}}}}, nil); ok {
		t.Fatal("unsafe untyped replacement")
	}
}
