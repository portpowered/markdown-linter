package rulepack

import (
	"context"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
	"path/filepath"
	"testing"
)

func TestComposedRelocationUsesSharedIndex(t *testing.T) {
	root := t.TempDir()
	source := []byte("# Source\n\n[Guide](old.md)\n")
	doc := interfaces.NewDocument(filepath.Join(root, "source.md"), source, goldmark.New().Parser().Parse(text.NewReader(source)))
	targetRaw := []byte("# Guide\n")
	target := interfaces.NewDocument(filepath.Join(root, "new.md"), targetRaw, goldmark.New().Parser().Parse(text.NewReader(targetRaw)))
	n := coverageOptions(t, "moves: {'"+filepath.ToSlash(filepath.Join(root, "old.md"))+"': '"+filepath.ToSlash(target.Path)+"'}")
	analyzer, err := newRelocation(n)
	if err != nil {
		t.Fatal(err)
	}
	pass := interfaces.NewPass([]*interfaces.Document{doc, target})
	analyzer.Analyze(context.Background(), pass)
	findings := pass.Diagnostics()
	if len(findings) != 1 || len(findings[0].SuggestedFixes) != 1 || findings[0].SuggestedFixes[0].Edits[0].Replacement != "new.md" {
		t.Fatalf("%+v", findings)
	}
	allow := localLinks{allowDirectories: true}
	allow.Analyze(context.Background(), interfaces.NewPass([]*interfaces.Document{target}))
}
func TestInlineSuppressionRequiresMatchingReasonOutsideCode(t *testing.T) {
	for _, tc := range []struct {
		source string
		line   int
		want   bool
	}{
		{"<!-- marklint-disable-next-line rule reason: intent -->\ntext\n", 2, true},
		{"<!-- marklint-disable-next-line other reason: intent -->\ntext\n", 2, false},
		{"<!-- marklint-disable-next-line rule reason:  -->\ntext\n", 2, false},
		{"<!-- marklint-disable-next-line rule -->\ntext\n", 2, false},
		{"```text\n<!-- marklint-disable-next-line rule reason: sample -->\ntext\n```\n", 3, false},
		{"    <!-- marklint-disable-next-line rule reason: sample -->\n    text\n", 2, false},
	} {
		raw := []byte(tc.source)
		doc := interfaces.NewDocument("guide.md", raw, goldmark.New().Parser().Parse(text.NewReader(raw)))
		if got := inlineSuppressed(doc, interfaces.Diagnostic{Line: tc.line, RuleID: "rule"}); got != tc.want {
			t.Fatalf("%q got=%v", tc.source, got)
		}
	}
	if inlineSuppressed(nil, interfaces.Diagnostic{Line: 2}) {
		t.Fatal("nil doc suppressed")
	}
}
func TestCloneOptionsIsIndependent(t *testing.T) {
	original := coverageOptions(t, "nested: {list: [one, two]}")
	copy := cloneNode(original)
	copy.Content[1].Content[1].Content[0].Value = "changed"
	var out map[string]any
	if err := original.Decode(&out); err != nil {
		t.Fatal(err)
	}
	nested := out["nested"].(map[string]any)
	if nested["list"].([]any)[0] != "one" {
		t.Fatal(out)
	}
	if copy.Kind != yaml.MappingNode {
		t.Fatal(copy.Kind)
	}
}
func TestDuplicateStockRegistrationIsAlwaysRejected(t *testing.T) {
	registered := NewRegistry()
	if err := RegisterStock(registered); err != nil {
		t.Fatal(err)
	}
	for _, d := range registered.Catalog() {
		r := NewRegistry()
		if err := r.Register(d.ID, registered.factories[d.ID]); err != nil {
			t.Fatal(err)
		}
		if err := RegisterStock(r); err == nil {
			t.Fatalf("duplicate %s accepted", d.ID)
		}
	}
}
