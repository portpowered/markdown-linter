package rulepack

import (
	"context"
	"testing"

	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/yuin/goldmark/ast"
	"gopkg.in/yaml.v3"
)

func TestMarkdownPolicyContracts(t *testing.T) {
	cases := []struct {
		name, id, source, options string
		count                     int
	}{
		{"formatting combined", "markdown.formatting", "Text   \n\n```text\ncode\n", "", 2},
		{"hard breaks disabled", "markdown.trailing-whitespace", "Break  \n", "allow-hard-breaks: false", 1},
		{"code whitespace preserved", "markdown.trailing-whitespace", "    indented code   \n\n```text\nfenced code   \n```\n", "", 0},
		{"frontmatter whitespace preserved", "markdown.trailing-whitespace", "---\ntitle: Example   \n---\n\nText.\n", "", 0},
		{"tabs rejected", "markdown.trailing-whitespace", "Text\t\n", "", 1},
		{"empty final newline", "markdown.final-newline", "", "", 0},
		{"extra LF", "markdown.final-newline", "Text\n\n", "", 1},
		{"extra CRLF", "markdown.final-newline", "Text\r\n\r\n", "", 1},
		{"correct CRLF", "markdown.final-newline", "Text\r\n", "", 0},
		{"decorative image", "markdown.image-alt", "![](divider.svg)\n", "allow: [divider.svg]", 0},
		{"image text", "markdown.image-alt", "![Architecture](diagram.svg)\n", "", 0},
		{"descriptive link", "markdown.link-text", "[API guide](https://example.test)\n", "", 0},
		{"empty link", "markdown.link-text", "[](https://example.test)\n", "", 1},
		{"closed tilde fence", "markdown.fence-closed", "~~~text\ntext\n~~~~\n", "", 0},
		{"short fence fails", "markdown.fence-closed", "~~~~text\ntext\n~~~\n", "", 1},
		{"fence in indented code", "markdown.fence-closed", "    ```\n    text\n", "", 0},
		{"language declared", "markdown.fence-language", "```text\nexample\n```\n", "", 0},
		{"empty fence", "markdown.fence-language", "```\n```\n", "", 1},
		{"same parent duplicate", "markdown.heading-duplicates", "# Title\n\n## Section\n\n## Section\n", "", 1},
		{"no H1", "markdown.single-title", "## Section\n", "", 1},
		{"two H1", "markdown.single-title", "# First\n\n# Second\n", "", 1},
		{"frontmatter title", "markdown.single-title", "---\ntitle: Example\n---\n\n## Section\n", "frontmatter-title: true", 0},
		{"missing frontmatter title", "markdown.single-title", "---\nauthor: Example\n---\n\n## Section\n", "frontmatter-title: true", 1},
		{"unclosed frontmatter", "markdown.frontmatter-valid", "---\ntitle: Example\n", "", 1},
		{"valid CRLF frontmatter", "markdown.frontmatter-valid", "---\r\ntitle: Example\r\n---\r\n\r\n# Title\r\n", "", 0},
		{"ordered one style", "markdown.ordered-list", "1. First\n2. Second\n", "style: one", 1},
		{"ordered sequential", "markdown.ordered-list", "1. First\n2. Second\n", "style: ordered", 0},
		{"ordered hybrid", "markdown.ordered-list", "1. First\n1. Second\n3. Third\n", "", 1},
		{"unordered marker", "markdown.list-style", "* First\n* Second\n", "", 1},
		{"unordered custom marker", "markdown.list-style", "* First\n* Second\n", "allow: ['*']", 0},
		{"nested indent", "markdown.list-style", "- First\n    - Child\n", "", 1},
		{"nested configured indent", "markdown.list-style", "- First\n    - Child\n", "indent: 4", 0},
		{"nested expected indent", "markdown.list-style", "- First\n  - Child\n", "", 0},
		{"block HTML rejected", "markdown.html-policy", "<div>Content</div>\n", "", 2},
		{"block HTML allowed", "markdown.html-policy", "<div>Content</div>\n", "allow: [div]", 0},
		{"inline HTML allowed", "markdown.html-policy", "Text <kbd>x</kbd>\n", "allow: [kbd]", 0},
		{"blank lines missing", "markdown.blank-lines", "# Title\n## Section\n", "", 1},
		{"blank lines present", "markdown.blank-lines", "# Title\n\n## Section\n", "", 0},
		{"unicode character length", "markdown.line-length", "éééééé\n", "max: 5", 1},
		{"long URL accepted", "markdown.line-length", "https://example.test/long/path\n", "max: 5", 0},
		{"long table accepted", "markdown.line-length", "| column | column |\n| --- | --- |\n| value | value |\n", "max: 5", 0},
		{"long code accepted", "markdown.line-length", "```\nlong code line\n```\n", "max: 5", 0},
		{"normalized references", "markdown.reference-definitions", "[label][EXAMPLE]\n\n[example]: https://example.test\n", "", 0},
		{"collapsed missing reference", "markdown.reference-definitions", "[Example][]\n", "", 1},
		{"reference code excluded", "markdown.reference-definitions", "`[Example][missing]`\n\n```text\n[Example][missing]\n```\n", "", 0},
		{"prose brackets accepted", "markdown.reference-definitions", "[ordinary brackets]\n", "", 0},
		{"terminology allowed", "text.terminology", "Utilize the cache.\n", "terms: {utilize: use}\nallow: [utilize]", 0},
		{"terminology heading only", "text.terminology", "# Utilize\n\nUtilize the cache.\n", "terms: {utilize: use}\nscope: heading", 1},
		{"offline spelling", "text.spelling", "Known unknown word.\n", "language: en\ndictionary: [known, word]", 1},
		{"spelling allow domain word", "text.spelling", "Portos word.\n", "language: en\ndictionary: [word]\nallow: [portos]", 0},
		{"spelling excludes code URL", "text.spelling", "Known `identifier` <https://example.test/unknown>\n", "language: en\ndictionary: [known]", 0},
		{"repetition allow", "text.repeated-word", "Had had enough.\n", "allow: [had]", 0},
		{"repetition punctuation boundary", "text.repeated-word", "Word, word.\n", "", 0},
		{"repetition paragraph boundary", "text.repeated-word", "Word\n\nword.\n", "", 0},
		{"load bearing emphasis", "portos.banned-phrases", "Load *bearing* prose.\n", "", 1},
		{"load bearing unicode dash", "portos.banned-phrases", "Load—bearing prose.\n", "", 1},
		{"load bearing unrelated", "portos.banned-phrases", "Load records bearing identifiers.\n", "", 0},
		{"no dashes heading scope", "portos.prose-dashes", "# Well-known\n\nWell-known.\n", "scope: heading", 1},
		{"bare URL exclusion", "portos.prose-dashes", "https://example.test/a-b\n", "", 0},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			findings := checkMarkdown(t, tt.id, tt.source, tt.options)
			if len(findings) != tt.count {
				t.Fatalf("want %d got %#v", tt.count, findings)
			}
			for _, finding := range findings {
				if finding.StartOffset < 0 || finding.EndOffset < finding.StartOffset || finding.EndOffset > len(tt.source) {
					t.Fatalf("bad source range: %#v", finding)
				}
			}
		})
	}
}

func TestMarkdownFactoryOptionContracts(t *testing.T) {
	cases := []struct{ id, options string }{
		{"markdown.list-style", "indent: 0"},
		{"markdown.list-style", "indent: 9"},
		{"markdown.line-length", "max: 0"},
		{"markdown.ordered-list", "style: random"},
		{"text.matcher", "scope: code"},
		{"text.terminology", "{}"},
		{"text.spelling", "language: en"},
		{"text.spelling", "dictionary: [known]"},
		{"text.matcher", "scope: true"},
		{"text.repeated-word", "allow: 1"},
		{"text.matcher", "unknown: true"},
		{"text.matcher", "[heading]"},
	}
	for _, tt := range cases {
		t.Run(tt.id+tt.options, func(t *testing.T) {
			var options yaml.Node
			if err := yaml.Unmarshal([]byte(tt.options), &options); err != nil {
				t.Fatal(err)
			}
			factory := markdownFactory(tt.id)
			if tt.id == "text.matcher" {
				factory = matcherFactory(tt.id, matcherDefaults())
			}
			if _, err := factory(*options.Content[0]); err == nil {
				t.Fatalf("invalid options accepted: %s", tt.options)
			}
		})
	}
	check, err := matcherFactory("text.matcher", matcherOptions{Scope: "prose", Message: "dashes", Patterns: []string{`[-\p{Pd}]`}})(yaml.Node{})
	if err != nil || check.ID() != "text.matcher" {
		t.Fatalf("wrong analyzer ID: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pass := interfaces.NewPass([]*interfaces.Document{{}})
	check.Analyze(ctx, pass)
	if len(pass.Diagnostics()) != 0 {
		t.Fatal("canceled analysis emitted findings")
	}
}

func TestMarkdownSafeEditContracts(t *testing.T) {
	for _, tc := range []struct{ source, replacement string }{{"Text", "\n"}, {"Text\r\n\r\n", "\r\n"}} {
		findings := checkMarkdown(t, "markdown.final-newline", tc.source, "")
		if len(findings) != 1 || len(findings[0].SuggestedFixes) != 1 {
			t.Fatalf("missing safe fix: %#v", findings)
		}
		fix := findings[0].SuggestedFixes[0]
		if fix.Confidence != interfaces.FixConfidenceSafe || fix.Edits[0].Replacement != tc.replacement {
			t.Fatalf("newline convention changed: %#v", fix)
		}
		edit := fix.Edits[0]
		result := tc.source[:edit.StartOffset] + edit.Replacement + tc.source[edit.EndOffset:]
		if result != "Text"+tc.replacement {
			t.Fatalf("bad applied result: %q", result)
		}
	}
	findings := checkMarkdown(t, "markdown.trailing-whitespace", "Text   \n", "")
	if len(findings) != 1 || findings[0].SuggestedFixes[0].Edits[0].Replacement != "" {
		t.Fatalf("missing whitespace deletion: %#v", findings)
	}
	if normLabel(" EXAMPLE   Label ") != "example label" {
		t.Fatal("reference normalization failed")
	}
	if offsetForNode(nil) != 0 || offsetForNode(ast.NewDocument()) != 0 || allowed([]string{"Portos"}, "different") {
		t.Fatal("empty node or allow boundary violated")
	}
}

func TestEmptyRegistryEditorSchemaHasNoExecutableRules(t *testing.T) {
	schema := NewRegistry().ConfigurationSchema()
	properties := schema["properties"].(map[string]any)
	rules := properties["rules"].(map[string]any)
	if rules["items"] != false {
		t.Fatal("an empty registry must prohibit rule items while permitting an empty array")
	}
}
