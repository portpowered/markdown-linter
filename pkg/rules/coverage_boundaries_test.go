package rules

import (
	"context"
	"github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/text"
	"os"
	"path/filepath"
	"testing"
)

func coverageDocument(path, source string) *interfaces.Document {
	raw := []byte(source)
	return interfaces.NewDocument(path, raw, goldmark.New().Parser().Parse(text.NewReader(raw)))
}
func TestIndexResolutionStrategiesAndAmbiguity(t *testing.T) {
	root := t.TempDir()
	doc := coverageDocument(filepath.Join(root, "guide.md"), "---\ndoc-id: DOC-1\nprocess-id: PROC-1\ntitle: Named guide\n---\n# Visible title\n")
	index := NewDocumentationIndex([]*interfaces.Document{doc}, WithoutAssetDiscovery(), WithMoveMappings(map[string]string{"old.md": doc.Path}))
	for _, tc := range []struct{ query, strategy string }{{"old.md", "move-mapping"}, {"DOC-1", "doc-id"}, {"PROC-1", "process-id"}, {doc.Path, "path"}, {"guide.md", "basename"}, {"guide", "filename"}, {"Visible title", "title"}} {
		result := index.ResolveDocument(tc.query)
		if result.Target == nil || result.Target.Path != normalizeIndexPath(doc.Path) || result.Strategy != tc.strategy {
			t.Fatalf("%+v: %+v", tc, result)
		}
	}
	if result := index.ResolveDocument("absent"); result.Target != nil || result.Ambiguous {
		t.Fatalf("%+v", result)
	}
	duplicate := coverageDocument(filepath.Join(root, "other", "guide.md"), "---\nprocess-id: PROC-1\n---\n# Other\n")
	index = NewDocumentationIndex([]*interfaces.Document{doc, duplicate}, WithoutAssetDiscovery())
	for _, query := range []string{"guide.md", "PROC-1"} {
		if result := index.ResolveDocument(query); !result.Ambiguous || len(result.Candidates) != 2 {
			t.Fatalf("%s: %+v", query, result)
		}
	}
	if got := index.resolveAssetCandidates("test", []string{"a.png", "a.png", "", "b.png"}); !got.Ambiguous || len(got.Candidates) != 2 {
		t.Fatalf("%+v", got)
	}
	if got := NewDocumentationIndexAnalyzer().ID(); got == "" {
		t.Fatal("empty index ID")
	}
	if got := NewTypedDocumentStructureAnalyzer().ID(); got == "" {
		t.Fatal("empty structure ID")
	}
}
func TestRelocationOperationalAndMissingTargets(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.md")
	outside := filepath.Join(t.TempDir(), "outside.md")
	source := coverageDocument(path, "# Source\n")
	base := IndexedLink{SourceFile: path, Line: 2, RawTarget: "old.md", NormalizedFile: normalizeIndexPath(filepath.Join(root, "old.md")), Kind: IndexedLinkKindMarkdown, DestinationRange: engine.SourceRange{StartOffset: 1, EndOffset: 7}}
	cases := []struct {
		name     string
		link     IndexedLink
		moves    map[string]string
		cancel   bool
		category interfaces.DiagnosticCategory
	}{
		{"unavailable", func() IndexedLink { x := base; x.DestinationRange.StartOffset = -1; return x }(), nil, false, interfaces.DiagnosticCategoryRelocationUnavailableRange},
		{"outside", func() IndexedLink { x := base; x.NormalizedFile = outside; return x }(), nil, false, interfaces.DiagnosticCategoryRelocationOperational},
		{"mapped outside", base, map[string]string{base.NormalizedFile: outside}, false, interfaces.DiagnosticCategoryRelocationMissingMappedTarget},
		{"mapped missing", base, map[string]string{base.NormalizedFile: filepath.Join(root, "new.md")}, false, interfaces.DiagnosticCategoryRelocationMissingMappedTarget},
		{"cancel", base, nil, true, interfaces.DiagnosticCategoryRelocationOperational},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pass := interfaces.NewPass([]*interfaces.Document{source})
			pass.Root = root
			index := NewDocumentationIndex(pass.Documents, WithoutAssetDiscovery(), WithMoveMappings(tc.moves))
			index.Links = []IndexedLink{tc.link}
			pass.SetData(DocumentationIndexPassDataKey, index)
			ctx := context.Background()
			if tc.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			NewLinkRelocationAnalyzer().Analyze(ctx, pass)
			diagnostics := pass.Diagnostics()
			if len(diagnostics) != 1 || diagnostics[0].Category != tc.category {
				t.Fatalf("%+v", diagnostics)
			}
		})
	}
	pass := interfaces.NewPass(nil)
	NewLinkRelocationAnalyzer().Analyze(context.Background(), pass)
	pass.SetData(DocumentationIndexPassDataKey, "wrong type")
	NewLinkRelocationAnalyzer().Analyze(context.Background(), pass)
	pass.SetData(DocumentationIndexPassDataKey, (*DocumentationIndex)(nil))
	NewLinkRelocationAnalyzer().Analyze(context.Background(), pass)
	if len(pass.Diagnostics()) != 0 {
		t.Fatal(pass.Diagnostics())
	}
	image := base
	image.Kind = IndexedLinkKindImage
	image.NormalizedFile = normalizeIndexPath(filepath.Join(root, "old.png"))
	image.RawTarget = "old.png"
	pass = interfaces.NewPass([]*interfaces.Document{source})
	index := NewDocumentationIndex(pass.Documents, WithoutAssetDiscovery(), WithMoveMappings(map[string]string{image.NormalizedFile: filepath.Join(root, "new.png")}))
	index.Links = []IndexedLink{image}
	pass.SetData(DocumentationIndexPassDataKey, index)
	NewLinkRelocationAnalyzer().Analyze(context.Background(), pass)
	if len(pass.Diagnostics()) != 1 || pass.Diagnostics()[0].Category != interfaces.DiagnosticCategoryRelocationMissingMappedTarget {
		t.Fatal(pass.Diagnostics())
	}
}
func TestLocalLinkDestinationParsing(t *testing.T) {
	for _, raw := range []string{"", "%zz", "https://example.com", "//host/path", "/absolute/path"} {
		if _, ok := parseLocalLinkTarget(raw); ok {
			t.Fatalf("accepted %q", raw)
		}
	}
	got, ok := parseLocalLinkTarget("guide%20name.md#Section%20One")
	if !ok || got.path != "guide name.md" || got.anchor != "section-one" {
		t.Fatalf("%+v %v", got, ok)
	}
	root := t.TempDir()
	target := filepath.Join(root, "directory")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	doc := coverageDocument(filepath.Join(root, "guide.md"), "# Guide\n\n[target](directory)\n")
	if got := NewLocalLinkRule(WithDirectoryLinkTargetsAllowed()).Check(context.Background(), doc); len(got) != 0 {
		t.Fatal(got)
	}
}
