package rulepack

import (
	"context"
	"github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
)

func checkMarkdown(t *testing.T, id, source, options string) []interfaces.Diagnostic {
	t.Helper()
	root := t.TempDir()
	file := filepath.Join(root, "doc.md")
	if e := os.WriteFile(file, []byte(source), 0600); e != nil {
		t.Fatal(e)
	}
	doc, e := engine.New().ParseFile(file)
	if e != nil {
		t.Fatal(e)
	}
	r := NewRegistry()
	if e = RegisterStock(r); e != nil {
		t.Fatal(e)
	}
	var opts yaml.Node
	if options != "" {
		if e = yaml.Unmarshal([]byte(options), &opts); e != nil {
			t.Fatal(e)
		}
		opts = *opts.Content[0]
	}
	p, e := r.Compile(Pack{Version: 1, Rules: []Rule{{ID: "customer.check", Check: id, Options: opts}}})
	if e != nil {
		t.Fatal(e)
	}
	findings, e := p.Run(context.Background(), root, []*interfaces.Document{doc})
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range findings {
		if f.RuleID != "customer.check" || f.Line < 1 {
			t.Fatalf("bad diagnostic %#v", f)
		}
	}
	return findings
}
func TestMarkdownAndProseCases(t *testing.T) {
	cases := []struct {
		id, source, options string
		count               int
	}{
		{"text.no-dashes", "# Well-written\n\nText—here. [link](https://site.test/a-b) `a-b`\n\n```text\na-b\n```\n", "", 2},
		{"text.no-load-bearing", "Load bearing prose.\n\n`load bearing` [ok](https://site.test/load-bearing)\n", "", 1},
		{"text.terminology", "Utilize the CLI.\n\n`utilize`\n", "terms: {utilize: use}", 1},
		{"text.repeated-word", "The the text.\n\nThe next paragraph.\n", "", 1},
		{"markdown.trailing-whitespace", "Hard break  \nTrailing   \n\n```text\nCode   \n```\n", "", 1},
		{"markdown.fence-closed", "```text\ntext\n```\n", "", 0},
		{"markdown.fence-closed", "```text\ntext\n", "", 1},
		{"markdown.fence-language", "```\ntext\n```\n", "", 1},
		{"markdown.image-alt", "![](image.png)\n", "", 1},
		{"markdown.link-text", "[click here](https://site.test)\n", "", 1},
		{"markdown.reference-definitions", "[thing][missing]\n\n[dup]: one.md\n[dup]: two.md\n", "", 2},
		{"markdown.single-title", "# Title\n\n## Section\n", "", 0},
		{"markdown.heading-duplicates", "# Title\n\n## First\n\n### Details\n\n## Second\n\n### Details\n", "", 0},
		{"markdown.ordered-list", "1. One\n1. Two\n", "", 0},
		{"markdown.ordered-list", "1. One\n3. Two\n", "style: ordered", 1},
		{"markdown.frontmatter-valid", "---\ntitle: One\ntitle: Two\n---\n\n# Title\n", "", 1},
		{"markdown.final-newline", "Text", "", 1},
		{"markdown.html-policy", "Text <script>bad</script>\n", "", 2},
	}
	for _, tt := range cases {
		t.Run(tt.id, func(t *testing.T) {
			got := checkMarkdown(t, tt.id, tt.source, tt.options)
			if len(got) != tt.count {
				t.Fatalf("want %d got %#v", tt.count, got)
			}
		})
	}
}
func TestAllEmbeddedPresetsCompile(t *testing.T) {
	r := NewRegistry()
	if e := RegisterStock(r); e != nil {
		t.Fatal(e)
	}
	for _, name := range PresetNames() {
		p, _ := Preset(name)
		if _, e := r.Compile(p); e != nil {
			t.Fatalf("%s: %v", name, e)
		}
	}
}

func FuzzProseSourceRanges(f *testing.F) {
	for _, seed := range []string{"Text - here", "`code-with-dashes`", "[a-b](https://site.test/a-b)", "load *bearing*", "---\ntitle: example\n---\n# Title"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		for _, id := range []string{"text.no-dashes", "text.no-load-bearing", "markdown.trailing-whitespace"} {
			for _, d := range checkMarkdown(t, id, source, "") {
				if d.StartOffset < 0 || d.EndOffset < d.StartOffset || d.EndOffset > len(source) {
					t.Fatalf("invalid range %#v for %q", d, source)
				}
			}
		}
	})
}

func TestInlineSuppression(t *testing.T) {
	cases := []struct {
		source string
		count  int
	}{
		{"<!-- marklint-disable-next-line customer.check reason: quoted product name -->\nWell-known.\n", 0},
		{"<!-- marklint-disable-next-line text.no-dashes reason: wrong instance -->\nWell-known.\n", 1},
		{"<!-- marklint-disable-next-line customer.check reason: -->\nWell-known.\n", 1},
		{"```html\n<!-- marklint-disable-next-line customer.check reason: example -->\n```\nWell-known.\n", 1},
	}
	for _, c := range cases {
		if got := len(checkMarkdown(t, "text.no-dashes", c.source, "")); got != c.count {
			t.Fatalf("source %q: got %d want %d", c.source, got, c.count)
		}
	}
	findings := checkMarkdown(t, "markdown.trailing-whitespace", "<!-- marklint-disable-next-line customer.check reason: fixture -->\nTrailing   \n", "")
	if len(findings) != 0 {
		t.Fatalf("suppressed fix remained: %#v", findings)
	}
}
