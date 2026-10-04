package rulepack

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/text"
)

func TestDocumentationBootstrapPolicyRejectsDefects(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	pack, err := Load(filepath.Join(root, "docs", "lint.yaml"), root)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := RegisterStock(registry); err != nil {
		t.Fatal(err)
	}
	program, err := registry.Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	good := "---\ntitle: Guide\ncheck: text.matcher\n---\n\n# Guide\n\n## Configuration\n\nUse the API.\n\n## Violation example\n\nUse the API.\n\n## Parameters\n\nUse the API.\n\n## Rule packs\n\nUse the API.\n"
	for _, sample := range []struct{ name, source, rule string }{
		{"accepted", good, ""},
		{"unknown word", good + "\nUnapprovedbootstrapword.\n", "text.ste100.dictionary"},
		{"contraction", good + "\nYou don't use the API.\n", "text.ste100.grammar"},
		{"typographic contraction", good + "\nYou don’t use the API.\n", "text.ste100.grammar"},
		{"long sentence", good + "\n" + strings.Repeat("use ", 20) + ".\n", "text.ste100.grammar"},
		{"missing section", strings.Replace(good, "## Parameters\n\nUse the API.\n\n", "", 1), "docs.page-shape"},
		{"protected examples", good + "\n```text\nUnapprovedbootstrapword.\n```\n", ""},
	} {
		t.Run(sample.name, func(t *testing.T) {
			source := []byte(sample.source)
			doc := interfaces.NewDocument(filepath.Join(root, ".site-docs", "rules", "fixture.md"), source, goldmark.New().Parser().Parse(text.NewReader(source)))
			findings, err := program.Run(context.Background(), root, []*interfaces.Document{doc})
			if err != nil {
				t.Fatal(err)
			}
			if sample.rule == "" {
				if len(findings) != 0 {
					t.Fatalf("accepted documentation failed: %#v", findings)
				}
				return
			}
			for _, finding := range findings {
				if finding.RuleID == sample.rule {
					return
				}
			}
			t.Fatalf("defect escaped %s: %#v", sample.rule, findings)
		})
	}
}

func TestSTEGrammarStrictlyLessThanTwentyWords(t *testing.T) {
	for _, sample := range []struct{ words, findings int }{{19, 0}, {20, 1}} {
		findings := checkMarkdown(t, "text.ste100.grammar", strings.Repeat("word ", sample.words)+".\n", "max-sentence-words: 19")
		if len(findings) != sample.findings {
			t.Fatalf("%d words: %#v", sample.words, findings)
		}
	}
}
