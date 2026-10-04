package contract

import (
	"context"
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
)

func TestCodeViewsRetainSourceCoordinates(t *testing.T) {
	for _, format := range []string{"markdown", "mdx"} {
		for _, source := range []string{
			"# Title\r\n\r\n```go extra\r\né first\r\nsecond\r\n```\r\n",
			"# Title\n\n> ```go\n> é first\n> second\n> ```\n",
			"# Title\n\n- Item\n\n  ```go\n  é first\n  second\n  ```\n",
			"# Title\n\n    é first\n    second\n",
			"# Title\n\n  ```go\n\té first\n\tsecond\n  ```\n",
			"# Title\n\n```go\né first\nsecond",
		} {
			if format == "mdx" && strings.Contains(source, "    é") {
				continue // MDX treats indented code syntax as prose.
			}
			t.Run(format+"/"+source, func(t *testing.T) {
				d := Parse("guide.md", []byte(source), "stdin")
				if format == "mdx" {
					var err error
					d, err = ParseMDX(context.Background(), "guide.mdx", []byte(source), "stdin", nil)
					if err != nil {
						t.Fatal(err)
					}
				}
				code := d.selectTargets(d.Root, "codeblock", "subtree")
				if len(code) != 1 {
					t.Fatalf("codeblocks: %d", len(code))
				}
				if format == "markdown" {
					want := strings.ReplaceAll(string(code[0].Node.Lines().Value([]byte(source))), "\r\n", "\n")
					if code[0].Code != want {
						t.Fatalf("code projection changed parser payload: %q != %q", code[0].Code, want)
					}
				}
				for view, token := range map[string]string{"code": "second", "language": "go"} {
					if view == "language" && code[0].Language == "" {
						continue
					}
					u := d.units(code[0], view, "direct")[0]
					projected, starts, ends := comparisonUnit(u, false, false)
					index := strings.Index(projected, token)
					if index < 0 {
						t.Fatalf("%s missing %q in %q", view, token, projected)
					}
					if got := source[starts[index]:ends[index+len(token)-1]]; got != token {
						t.Fatalf("%s coordinates select %q instead of %q", view, got, token)
					}
				}
				unit := d.units(code[0], "code", "direct")[0]
				if strings.Contains(source, "\r\n") {
					index := strings.Index(unit.Text, "\n")
					if got := source[unit.Offsets[index]:unit.EndOffsets[index]]; got != "\r\n" {
						t.Fatalf("normalized newline lost CRLF range: %q", got)
					}
				}
			})
		}
	}
}

func TestCodeProjectionFallbackPreservesEvidence(t *testing.T) {
	u := mdxCodeUnit([]byte("opaque\n"), 0, 7, "decoded")
	_, starts, ends := comparisonUnit(u, false, false)
	for i := range starts {
		if starts[i] != 0 || ends[i] != 7 {
			t.Fatalf("fallback invented coordinate: %d:%d", starts[i], ends[i])
		}
	}
	if got := normalizeCodeNewlines(literalUnit("x\ry\n", 3)); got.Text != "x\ry\n" || got.Offsets[2] != 5 {
		t.Fatal(got)
	}
}

func TestEmptyMarkdownBlocksRetainStructuralAnchors(t *testing.T) {
	for _, marker := range []string{"-", "*", "+", "1."} {
		for _, newline := range []string{"\n", "\r\n"} {
			source := strings.ReplaceAll("# T\n## A\ntext\n\n"+marker+"\n", "\n", newline)
			d := Parse("guide.md", []byte(source), "stdin")
			lists := d.selectTargets(d.Root, "list", "subtree")
			if len(lists) != 1 || source[lists[0].Start:lists[0].End] != marker+newline {
				t.Fatalf("bare list %q lost its range: %+v", marker, lists)
			}
			p := one(t, "core.sequence", "{allow: [paragraph]}", "each section")
			if r := lint(t, p, source); r.ExitCode != 1 {
				t.Fatalf("bare list %q escaped sequence: %+v", marker, r.Diagnostics)
			}
		}
	}
	for kind, block := range map[string]string{
		"thematic-break": "***\n",
		"blockquote":     ">\n",
		"list":           "- \n",
		"codeblock":      "```\n```\n",
		"heading":        "###\n",
	} {
		for _, newline := range []string{"\n", "\r\n"} {
			source := strings.ReplaceAll("# T\n## A\ntext\n\n"+block, "\n", newline)
			d := Parse("guide.md", []byte(source), "stdin")
			var target *Target
			for _, candidate := range d.Targets {
				if candidate.Kind == kind && (kind != "heading" || candidate.Level == 3) {
					target = candidate
				}
			}
			if target == nil {
				t.Fatalf("missing %s", kind)
			}
			want := strings.ReplaceAll(block, "\n", newline)
			if got := source[target.Start:target.End]; got != want {
				t.Fatalf("%s %q range %d:%d selects %q", kind, newline, target.Start, target.End, got)
			}
			if kind == "heading" {
				sections := d.selectTargets(d.Root, "section", "subtree")
				if len(sections) != 2 || sections[0].Level != 2 || sections[1].Level != 3 || sections[1].Parent != sections[0] {
					t.Fatalf("empty heading changed outline: %+v", sections)
				}
				continue
			}
			p := one(t, "core.sequence", "{allow: [paragraph]}", "each section")
			r := lint(t, p, source)
			if r.ExitCode != 1 {
				t.Fatalf("empty %s escaped sequence: %+v", kind, r.Diagnostics)
			}
		}
	}
	for _, source := range []string{"# T\n\n> ***\n", "# T\r\n\r\n> ***\r\n", "# T\n\n- ```\n  ```\n"} {
		d := Parse("guide.md", []byte(source), "stdin")
		for _, target := range d.Targets {
			if target.Kind == "thematic-break" || target.Kind == "codeblock" {
				if target.Start == 0 || target.End <= target.Start || target.Parent == d.Root {
					t.Fatalf("nested syntax lost its coordinates/parent: %+v", target)
				}
			}
		}
	}
}

func TestListLineEndingViewPreservesInlineBreaks(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n"} {
		for _, spaces := range []string{" ", "  "} {
			source := "# T" + newline + newline + "- First" + spaces + newline + "  next." + newline
			d := Parse("guide.md", []byte(source), "stdin")
			if len(d.selectTargets(d.Root, "list", "subtree")) != 1 {
				t.Fatal("list syntax changed")
			}
			found := false
			_ = ast.Walk(d.Legacy.Root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
				if text, ok := node.(*ast.Text); entering && ok && (text.SoftLineBreak() || text.HardLineBreak()) {
					found = true
					if text.HardLineBreak() != (len(spaces) == 2) {
						t.Fatalf("%q with %d spaces changed inline break", newline, len(spaces))
					}
				}
				return ast.WalkContinue, nil
			})
			if !found {
				t.Fatal("missing inline line break")
			}
		}
	}
}

func TestSectionSourceScopeExcludesDescendants(t *testing.T) {
	source := "# Title\n\n## Parent\n\nOwn text.\n\n### Child\n\nMarker only here.\n"
	for _, format := range []string{"markdown", "mdx"} {
		d := Parse("guide.md", []byte(source), "stdin")
		if format == "mdx" {
			var err error
			d, err = ParseMDX(context.Background(), "guide.mdx", []byte(source), "stdin", nil)
			if err != nil {
				t.Fatal(err)
			}
		}
		parent := d.selectTargets(d.Root, "section", "subtree")[0]
		direct := d.units(parent, "source", "direct")[0]
		if direct.Text != "\nOwn text.\n\n" {
			t.Fatalf("%s direct source: %q", format, direct.Text)
		}
		if got := d.measure(parent, "bytes", "source", "direct"); got != float64(len(direct.Text)) {
			t.Fatalf("%s direct bytes: %v", format, got)
		}
		if got := d.measure(parent, "source-lines", "source", "direct"); got != 3 {
			t.Fatalf("%s direct source-lines: %v", format, got)
		}
		subtree := d.units(parent, "source", "subtree")[0]
		if !strings.Contains(subtree.Text, "### Child") || !strings.Contains(subtree.Text, "Marker") {
			t.Fatalf("%s subtree source: %q", format, subtree.Text)
		}
		if d.measure(parent, "bytes", "source", "subtree") <= float64(len(direct.Text)) || d.measure(parent, "source-lines", "source", "subtree") <= 3 {
			t.Fatal("subtree counts excluded child")
		}
		for _, scope := range []string{"direct", "subtree"} {
			p := compile(t, strings.ReplaceAll(`version: 2
rules:
  marker: {check: core.match, options: {mode: regex, view: source, regex: Marker}}
sets:
  a: {rules: [{rule: marker, on: each section, scope: SCOPE}]}
apply: [{use: [a]}]
`, "SCOPE", scope))
			r := NewReport("test", "error")
			p.Run(context.Background(), t.TempDir(), []*Document{d}, r)
			if scope == "direct" && r.Summary.FailedTargets != 1 || scope == "subtree" && r.Summary.FailedTargets != 0 {
				t.Fatalf("%s %s matched descendant marker: %+v", format, scope, r.Diagnostics)
			}
		}
	}
}
