package interfaces

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"path/filepath"
	"testing"
)

func TestNodeTextRetainsVisibleMarkdownAndHTML(t *testing.T) {
	for _, tc := range []struct{ source, want string }{{"a *bold*\nline\n", "a bold\nline"}, {"<https://example.com>\n", "https://example.com"}, {"a <span>x</span>\n", "a <span>x</span>"}, {"<script>\nvalue\n</script>\n", "<script>\nvalue\n</script>"}} {
		source := []byte(tc.source)
		root := goldmark.New().Parser().Parse(text.NewReader(source))
		got := string(NodeText(root, source))
		if got != tc.want && got != tc.want+"\n" {
			t.Fatalf("source=%q got=%q want=%q", tc.source, got, tc.want)
		}
	}
	if got := string(NodeText(ast.NewString([]byte("literal")), nil)); got != "literal" {
		t.Fatal(got)
	}
}

func TestDocumentOffsetBoundaries(t *testing.T) {
	d := NewDocument("guide.md", []byte("first\r\nsecond\n"), ast.NewDocument())
	for _, tc := range []struct{ offset, want int }{{-1, 0}, {0, 1}, {7, 2}, {14, 3}, {15, 0}} {
		if got := d.LineForOffset(tc.offset); got != tc.want {
			t.Fatalf("offset=%d got=%d want=%d", tc.offset, got, tc.want)
		}
	}
	for _, tc := range []struct {
		line int
		want string
	}{{-1, ""}, {0, ""}, {1, "first"}, {2, "second"}, {3, ""}, {4, ""}} {
		if got := d.SourceLine(tc.line); got != tc.want {
			t.Fatalf("line=%d got=%q", tc.line, got)
		}
	}
	if d.NodeLine(nil) != 0 || d.NodeLine(ast.NewDocument()) != 0 {
		t.Fatal("unexpected source line")
	}
}

func TestPathRootChecksNearestExistingAncestor(t *testing.T) {
	root := t.TempDir()
	if err := CheckPathRoot(root, filepath.Join(root, "absent", "nested", "file.md")); err != nil {
		t.Fatal(err)
	}
	if err := CheckPathRoot("", filepath.Join(t.TempDir(), "anything")); err != nil {
		t.Fatal(err)
	}
	if err := CheckPathRoot(filepath.Join(root, "absent"), root); err == nil {
		t.Fatal("missing root accepted")
	}
	if err := CheckPathRoot(root, filepath.Join(t.TempDir(), "absent", "file.md")); err == nil {
		t.Fatal("outside ancestor accepted")
	}
}
