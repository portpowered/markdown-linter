package rulepack

import (
	"context"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/portpowered/markdown-linter/pkg/rules"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
	"path/filepath"
	"testing"
)

func coverageOptions(t *testing.T, source string) yaml.Node {
	t.Helper()
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(source), &n); err != nil {
		t.Fatal(err)
	}
	return *n.Content[0]
}
func TestStockFactoryValidation(t *testing.T) {
	cases := []struct{ check, options string }{
		{"markdown.required-heading", "heading: Purpose\nlevel: 7"}, {"markdown.required-heading", "unknown: true"},
		{"markdown.document-identifier", "identifiers: []"}, {"markdown.document-identifier", "identifiers: [{field: '', pattern: x}]"}, {"markdown.document-identifier", "identifiers: [{field: id, pattern: '['}]"}, {"markdown.document-identifier", "unknown: true"},
		{"markdown.document-structure", "types: []"}, {"markdown.document-structure", "unknown: true"}, {"markdown.link-relocation", "unknown: true"}, {"markdown.local-links", "unknown: true"}, {"markdown.heading-order", "unknown: true"}, {"markdown.doc-id-unique", "unknown: true"},
		{"markdown.list-style", "indent: 0"}, {"markdown.line-length", "max: 0"}, {"markdown.ordered-list", "style: random"}, {"text.repeated-word", "scope: sentence"}, {"text.terminology", "terms: {}"}, {"text.spelling", "dictionary: [hello]"},
	}
	r := NewRegistry()
	if err := RegisterStock(r); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.check+tc.options, func(t *testing.T) {
			if _, err := r.Compile(Pack{Version: 1, Rules: []Rule{{ID: "test", Check: tc.check, Options: coverageOptions(t, tc.options)}}}); err == nil {
				t.Fatal("invalid options accepted")
			}
		})
	}
	if err := RegisterStock(r); err == nil {
		t.Fatal("duplicate registration accepted")
	}
}
func TestCustomerIdentifierAndStructureFactories(t *testing.T) {
	root := t.TempDir()
	source := []byte("---\nid: INVALID\n---\n# Guide\n\n## Purpose\n")
	doc := interfaces.NewDocument(filepath.Join(root, "guide.md"), source, goldmark.New().Parser().Parse(text.NewReader(source)))
	r := NewRegistry()
	if err := RegisterStock(r); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		check, opts string
		want        int
	}{
		{"markdown.document-identifier", "identifiers:\n  - field: id\n    pattern: '^DOC-[0-9]+$'\n    required: true\n    unique: true\n    match-filename: true", 2},
		{"markdown.document-structure", "types:\n  - paths: ['" + filepath.ToSlash(root) + "']\n    required-fields: [title]\n    required-headings: [Missing]", 2},
		{"markdown.required-heading", "heading: Purpose\nlevel: 2", 0},
	} {
		analyzer, err := r.factories[tc.check](coverageOptions(t, tc.opts))
		if err != nil {
			t.Fatal(err)
		}
		if analyzer.ID() == "" {
			t.Fatal("missing ID")
		}
		pass := interfaces.NewPass([]*interfaces.Document{doc})
		analyzer.Analyze(context.Background(), pass)
		if len(pass.Diagnostics()) != tc.want {
			t.Fatalf("%s diagnostics=%+v", tc.check, pass.Diagnostics())
		}
	}
	group, err := newRelocation(yaml.Node{})
	if err != nil {
		t.Fatal(err)
	}
	if group.ID() != "markdown.link-relocation" {
		t.Fatal(group.ID())
	}
	adapter := AdaptRule(rules.NewHeadingOrderRule())
	if adapter.ID() != "markdown.heading-order" {
		t.Fatal(adapter.ID())
	}
	if (localLinks{}).ID() != "markdown.local-links" || (markdownCheck{id: "custom"}).ID() != "custom" {
		t.Fatal("unexpected analyzer ID")
	}
}
func TestRegistryAndPackValidation(t *testing.T) {
	r := NewRegistry()
	if err := r.Register("", nil); err == nil {
		t.Fatal("empty factory accepted")
	}
	if err := r.Register("nil", func(yaml.Node) (interfaces.Analyzer, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.SetDescriptor(Descriptor{ID: "missing"}); err == nil {
		t.Fatal("unknown descriptor accepted")
	}
	if _, ok := r.Describe("missing"); ok {
		t.Fatal("unknown descriptor found")
	}
	if d, ok := r.Describe("nil"); !ok || d.Kind != "custom" {
		t.Fatalf("%+v %v", d, ok)
	}
	if err := r.SetDescriptor(Descriptor{ID: "nil"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []Pack{{Version: 2}, {Version: 1, Extends: []string{"parent"}}, {Version: 1, Rules: []Rule{{ID: "", Check: "nil"}}}, {Version: 1, Rules: []Rule{{ID: "x", Check: "nil", Severity: "invalid"}}}, {Version: 1, Rules: []Rule{{ID: "x", Check: "nil", Include: []string{"../escape"}}}}, {Version: 1, Rules: []Rule{{ID: "x", Check: "nil"}}}} {
		if _, err := r.Compile(p); err == nil {
			t.Fatalf("accepted %+v", p)
		}
	}
}
