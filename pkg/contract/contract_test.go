package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/markdown-linter/pkg/rulepack"
)

func registry(t *testing.T) *rulepack.Registry {
	t.Helper()
	r := rulepack.NewRegistry()
	if err := rulepack.RegisterStock(r); err != nil {
		t.Fatal(err)
	}
	return r
}
func compile(t *testing.T, s string) *Program {
	t.Helper()
	p, err := Decode([]byte(s), "policy.yaml", registry(t))
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func lint(t *testing.T, p *Program, s string) *Report {
	t.Helper()
	r := NewReport("test", "error")
	p.Run(context.Background(), t.TempDir(), []*Document{Parse("guide.md", []byte(s), "stdin")}, r)
	return r
}
func one(t *testing.T, check, options, on string) *Program {
	t.Helper()
	return compile(t, fmt.Sprintf("version: 2\nrules:\n  test:\n    check: %s\n    options: %s\nsets:\n  test:\n    rules: [{rule: test, on: %s}]\napply: [{use: [test]}]\n", check, options, on))
}
func TestCompleteExample(t *testing.T) {
	source, err := os.ReadFile("../../examples/v2/.marklint.yaml")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Decode(source, "example.yaml", registry(t))
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile("../../examples/v2/docs/guide.md")
	if err != nil {
		t.Fatal(err)
	}
	r := NewReport("test", "error")
	p.Run(context.Background(), t.TempDir(), []*Document{Parse("docs/guide.md", content, "stdin")}, r)
	if r.ExitCode != 0 || !r.Complete {
		t.Fatalf("example: %+v", r.Diagnostics)
	}
	r = NewReport("test", "error")
	bad := strings.Replace(string(content), "boolean", "float", 1)
	bad = strings.Replace(bad, "but entries", "and entries", 1)
	p.Run(context.Background(), t.TempDir(), []*Document{Parse("docs/guide.md", []byte(bad), "stdin")}, r)
	if r.ExitCode != 1 || len(r.Diagnostics) < 2 {
		t.Fatalf("invalid guide: %+v", r)
	}
	if p.Rules["word-budget"].Options["max"] != 25 || p.Rules["file-budget"].Options["max"] != 1500 {
		t.Fatal("inheritance mutated parent")
	}
}
func TestProjectionCountsAndScope(t *testing.T) {
	source := "---\ntitle: Protected\n---\n\n# Title\n\n## Parent\n\nA **clear** [label](https://host/a-b) `excluded code` ![ignored alt](image.png) café.\n\n- Nested text.\n\n| Name | Type |\n| --- | --- |\n| `one` | string |\n\n```go\ncode\n```\n\n### Child\n\nChild text.\n\n## Next\n\nNext text.\n"
	d := Parse("guide.md", []byte(source), "stdin")
	secs := d.selectTargets(d.Root, "section", "subtree")
	if len(secs) != 3 {
		t.Fatalf("sections %d", len(secs))
	}
	parent := secs[0]
	if got := d.measure(parent, "words", "auto", "direct"); got != 4 {
		t.Fatalf("direct words %g", got)
	}
	if got := d.measure(parent, "words", "auto", "subtree"); got != 8 {
		t.Fatalf("subtree words %g", got)
	}
	counts := map[string]float64{"sections": 3, "headings": 4, "paragraphs": 4, "tables": 1, "codeblocks": 1, "lists": 1, "max-heading-level": 3, "blockquotes": 0, "bytes": float64(len(source)), "source-lines": float64(physicalLines(source))}
	for m, want := range counts {
		if got := d.measure(d.Root, m, "auto", "subtree"); got != want {
			t.Errorf("%s: %g != %g", m, got, want)
		}
	}
	if d.measure(parent, "subsections", "auto", "direct") != 1 {
		t.Fatal("subsection count")
	}
	tables := d.selectTargets(d.Root, "table", "subtree")
	if d.measure(tables[0], "rows", "auto", "") != 1 || d.measure(tables[0], "columns", "auto", "") != 2 {
		t.Fatal("table dimensions")
	}
	code := d.selectTargets(d.Root, "codeblock", "subtree")[0]
	if d.measure(code, "lines", "auto", "") != 1 || d.measure(code, "graphemes", "language", "") != 2 {
		t.Fatal("code counts")
	}
	if d.measure(d.Root, "words", "visible", "subtree") <= d.measure(d.Root, "words", "auto", "subtree") {
		t.Fatal("visible view excludes headings/cells")
	}
	if d.measure(parent, "blocks", "auto", "direct") != 4 {
		t.Fatal("direct blocks")
	}
}
func TestMatcherModesAndBoundaries(t *testing.T) {
	cases := []struct {
		options, on, source string
		fail                bool
	}{
		{`{mode: word, expect: absent, case: fold, values: [utilize]}`, "document", "Utilize the cache.", true},
		{`{mode: word, expect: absent, values: [utilize]}`, "document", "Utilization and `utilize` are protected.", false},
		{`{mode: contains, values: ["alpha beta"]}`, "document", "alpha\n\nbeta", true},
		{`{mode: contains, expect: absent, view: source, values: [utilize]}`, "document", "`utilize`", true},
		{`{mode: equals, values: ["café"]}`, "each paragraph", "cafe\u0301", false},
		{`{mode: regex, regex: 'cache', anchor: search}`, "document", "A cache works.", false},
		{`{mode: regex, regex: 'cache', anchor: full}`, "each sentence", "A cache works.", true},
		{`{mode: equals, view: source, values: ["cache\n"]}`, "document", "cache\n", false},
		{`{mode: pattern, pattern: '{X}, but {Y}.'}`, "each sentence", "The cache is fast, but entries expire.", false},
		{`{mode: equals, view: language, values: [go]}`, "each codeblock", "```go\nx\n```", false},
		{`{mode: contains, view: code, values: [x]}`, "each codeblock", "```go\nx\n```", false},
	}
	for i, c := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			p := one(t, "core.match", c.options, c.on)
			r := lint(t, p, c.source)
			if (r.ExitCode == 1) != c.fail {
				t.Fatalf("%+v", r)
			}
		})
	}
	p := one(t, "core.match", `{mode: word, expect: absent, values: [utilize]}`, "document")
	r := lint(t, p, "😀 We utilize it.")
	loc := r.Diagnostics[0].Location
	if loc.StartOffset != 8 || loc.Start.Column != 6 || loc.EndOffset != 15 {
		t.Fatalf("UTF-8/scalar coordinates: %+v", loc)
	}
}
func TestLimitsAndSequences(t *testing.T) {
	for _, options := range []string{`{measure: words, min: 3}`, `{measure: words, max: 1}`, `{measure: words, min: 3, max: 4}`} {
		r := lint(t, one(t, "core.limit", options, "each paragraph"), "Two words.")
		if r.ExitCode != 1 || len(r.Diagnostics[0].Measurements) != 1 {
			t.Fatalf("%+v", r)
		}
	}
	r := lint(t, one(t, "core.limit", `{measure: graphemes, max: 1}`, "each paragraph"), "👩‍👩‍👧‍👦")
	if r.ExitCode != 0 {
		t.Fatal("extended grapheme")
	}
	source := "# Title\n\n## Data\n\nText.\n\n| A |\n| --- |\n| x |\n\n```go\nx\n```\n"
	for _, options := range []string{`{sequence: [paragraph, table, codeblock]}`, `{allow: [paragraph, table, codeblock]}`} {
		if r := lint(t, one(t, "core.sequence", options, "each section"), source); r.ExitCode != 0 {
			t.Fatalf("%+v", r.Diagnostics)
		}
	}
	if r := lint(t, one(t, "core.sequence", `{sequence: [table, paragraph]}`, "each section"), source); r.ExitCode != 1 {
		t.Fatal("order accepted")
	}
}
func TestStrictConfiguration(t *testing.T) {
	cases := []string{
		"version: 1\napply: []", "version: 2", "version: 2\napply: []\nunknown: yes", "version: 2\nversion: 2\napply: []", "version: 2\napply: &anchor []", "version: 2\napply: []\nrules: {test: {check: core.limit, options: {measure: words, max: .inf}}}", "version: 2\napply: []\n---\nversion: 2", "version: 2\napply: []\ncomponents: {Thing: dynamic}", "version: 2\napply: []\nlanguage: ja", "version: 2\napply: []\nprofile: missing",
		"version: 2\napply: []\nrules: {test: {check: core.limit, extends: parent}}", "version: 2\napply: []\nrules: {test: {extends: missing}}", "version: 2\napply: []\nrules: {test: {extends: other}, other: {extends: test}}", "version: 2\napply: []\nrules: {Bad: {check: core.limit}}", "version: 2\napply: []\nrules: {test: {check: missing}}", "version: 2\napply: []\nrules: {test: {check: markdown.final-newline, options: {bad: yes}}}", "version: 2\napply: []\nrules: {test: {check: text.ste100.dictionary, options: {dictionary-file: words.yaml}}}",
		"version: 2\napply: [{use: [missing]}]", "version: 2\napply: [{}]", "version: 2\napply: [{files: [], use: []}]", "version: 2\napply: [{files: [../docs], use: []}]", "version: 2\napply: []\nsets: {a: {use: [b]}, b: {use: [a]}}", "version: 2\napply: []\nsets: {portos: {}}", "version: 2\napply: [{use: [portos-defaults]}]", "version: 2\napply: []\nsets: {a: {templates: [missing]}}", "version: 2\napply: []\nsuppressions: [{rule: missing, path: a.md, reason: yes}]",
	}
	for i, s := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if _, err := Decode([]byte(s), "bad.yaml", registry(t)); err == nil {
				t.Fatalf("accepted:\n%s", s)
			}
		})
	}
	invalid := map[string][]string{
		"core.limit":            {`{measure: words}`, `{measure: bad, max: 3}`, `{measure: words, max: -1}`, `{measure: words, min: 3, max: 2}`, `{measure: words, max: 1.5}`, `{measure: words, max: null}`, `{measure: words, max: 2, extra: yes}`, `{measure: words, max: 2, parameters: {bad: 1}}`, `{measure: words, max: 2, view: bad}`},
		"core.match":            {`{mode: bad}`, `{mode: word, values: ["two words"]}`, `{mode: contains, values: []}`, `{mode: contains, values: [one, one]}`, `{mode: contains, values: [one], regex: one}`, `{mode: regex, regex: '('}`, `{mode: regex, regex: '.*'}`, `{mode: regex, regex: x, anchor: bad}`, `{mode: contains, values: [one], expect: bad}`, `{mode: contains, values: [one], case: bad}`, `{mode: contains, values: [one], anchor: full}`},
		"core.sequence":         {`{}`, `{sequence: [paragraph], allow: [paragraph]}`, `{allow: [paragraph, paragraph]}`, `{sequence: [bad]}`},
		"markdown.table-schema": {`{header: []}`, `{header: [Name, Name]}`, `{header: [Name], columns: {Other: {}}}`, `{header: [Name], columns: {Name: {unique: yes}}}`, `{header: [Name], columns: {Name: {rules: [missing]}}}`},
		"mermaid.flowchart":     {`{direction: bad}`, `{accessible-title: bad}`, `{source-indent: 9}`, `{source-indent: 1.5}`},
	}
	for check, options := range invalid {
		for i, o := range options {
			t.Run(check+fmt.Sprint(i), func(t *testing.T) {
				s := fmt.Sprintf("version: 2\napply: []\nrules: {test: {check: %s, options: %s}}", check, o)
				if _, err := Decode([]byte(s), "bad.yaml", registry(t)); err == nil {
					t.Fatalf("accepted %s %s", check, o)
				}
			})
		}
	}
}
func TestPatterns(t *testing.T) {
	valid := []string{"{X}, but {Y}.", "literal.*", `\{literal\}`, "# Comment\n{X}。\n{X}, but {Y}.", `a\#b\\c`}
	for _, s := range valid {
		if _, err := compilePattern(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	invalid := []string{"", "# comment", "{X}", "{X}{Y}", "{X} {Y}", "{X}, {X}", "{x} done", `a\q`, "{X done", "a}", strings.Repeat("x", 65537), strings.Repeat("a\n", 65), strings.Repeat("😀", 4097), "{A},{B},{C},{D},{E},{F},{G},{H},{I}"}
	for _, s := range invalid {
		if _, err := compilePattern(s); err == nil {
			t.Errorf("accepted %q", s[:min(len(s), 40)])
		}
	}
}
func TestTemplateGrammar(t *testing.T) {
	valid := `version: 2
templates:
  outline: |-
    {{ document sections=1 headings=2 }}
    # {{ heading words = 1..3 }}
    ## Data
    {{ section paragraphs>=1 paragraphs<=2 scope="subtree" }}
    {{ paragraph[1].sentence[1] contains="}}" }}
sets:
  guides: {templates: [outline]}
apply: [{use: [guides]}]
`
	p := compile(t, valid)
	if r := lint(t, p, "# Title\n\n## Data\n\nContains }} here.\n"); r.ExitCode != 0 {
		t.Fatalf("%+v", r.Diagnostics)
	}
	for _, s := range []string{"# Title\n\n## Data\n", "# Title\n\n## Extra\n\n## Data\n\nText.", "# Title\n"} {
		if r := lint(t, p, s); r.ExitCode != 1 {
			t.Fatalf("outline accepted %s", s)
		}
	}
	invalid := []string{`{{ paragraph[0] words=1 }}`, `{{ section scope="direct" }}`, `{{ document words=1 }}`, `{{ heading scope="subtree" words=1 }}`, `{{ paragraph[1].heading[1] words=1 }}`, `{{ paragraph[1] words=1 words=1 }}`, `{{ paragraph[1] words=3..1 }}`, `{{ paragraph[1] words>=3 words<=1 }}`, `{{ paragraph[1] matches="file" }}`, `{{ paragraph[1] contains="x" contains="y" }}`, `{{ paragraph[1] contains="\uD800" }}`, `{{ paragraph[1] contains="x" trailing }}`, `{{ paragraph[1] words<2 }}`, `{{ paragraph[1] words=2.5 }}`, `{{ paragraph[1] rule="missing" }}`, `{{ each table words=1 }}`, `{{ codeblock[1] graphemes=1 }}`, `{{ paragraph[1] contains>=1 }}`, `{{ paragraph[1] contains="unterminated }}`, `{{ paragraph[1] words=1}} junk`, "prose {{ paragraph[1] words=1 }}", `{{ }}`}
	for i, s := range invalid {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			_, err := parseTemplate("# Title\n"+s, Default(registry(t)))
			if err == nil {
				t.Fatalf("accepted %s", s)
			}
		})
	}
	for _, s := range []string{"## Missing title", "# First\n# Second", "# Title\n### Skipped", "ordinary text", "# {{ paragraph[1] words=1 }}"} {
		if _, err := parseTemplate(s, Default(registry(t))); err == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	if _, err := parseTemplate("# Title\n\n```text\n{{ invalid }}\n```\n\n<!-- {{ invalid }} -->\n\n`{{ invalid }}`\n", Default(registry(t))); err != nil {
		t.Fatal(err)
	}
}
func TestRoutingAndComposition(t *testing.T) {
	p := compile(t, `version: 2
rules:
  base: {check: core.limit, options: {measure: words, max: 10}}
  child: {extends: base, options: {max: 2}, severity: warning}
sets:
  a: {rules: [{rule: child, on: each paragraph}]}
  b: {use: [a]}
apply:
  - {use: [a, b]}
  - {files: [docs/**], use: [a], language: en-US}
`)
	q, err := p.Policy("docs/x.md")
	if err != nil || len(q.Bindings) != 1 || q.Language != "en-US" || len(q.Origins["child|each paragraph|"]) != 2 {
		t.Fatalf("%+v %v", q, err)
	}
	if r := lint(t, p, "Three word sentence."); r.ExitCode != 0 || len(r.Diagnostics) != 1 {
		t.Fatalf("severity: %+v", r)
	}
	q, err = p.Policy("x.md")
	if err != nil || q.Language != "en" {
		t.Fatal(err)
	}
	for _, tail := range []string{`apply: [{files: [docs/**], use: []}]`, `apply: [{use: [], language: en}, {use: [], language: fr}]`, `templates: {a: "# A", b: "# B"}
sets: {a: {templates: [a]}, b: {templates: [b]}}
apply: [{use: [a,b]}]`} {
		p := compile(t, "version: 2\n"+tail)
		if _, err := p.Policy("x.md"); err == nil {
			t.Fatal("routing error accepted")
		}
	}
	p = compile(t, `version: 2
rules:
  min: {check: core.limit, options: {measure: words, min: 4}}
  max: {check: core.limit, options: {measure: words, max: 2}}
sets:
  a: {rules: [{rule: min, on: document}, {rule: max, on: document}]}
apply: [{use: [a]}]`)
	if _, err := p.Policy("x.md"); err == nil {
		t.Fatal("contradictions accepted")
	}
	for _, pat := range []string{"/absolute", "../escape", "a\\b", "a/**/b", "a/../b", "", "a/{b}"} {
		if Pattern(pat) == nil {
			t.Fatal(pat)
		}
	}
	if !Match("docs/**", "docs/a/b.md") || !Match("*.md", "a.md") || Match("*.md", "docs/a.md") {
		t.Fatal("path matching")
	}
}
func TestTablesAndMermaid(t *testing.T) {
	p := compile(t, `version: 2
rules:
  cell: {check: core.limit, options: {measure: graphemes, view: cell, min: 1}}
  table: {check: markdown.table-schema, options: {header: [Name, Description], columns: {Name: {unique: true, rules: [cell]}, Description: {rules: [cell]}}}}
sets: {a: {rules: [{rule: table, on: each table}]}}
apply: [{use: [a]}]`)
	for _, s := range []string{"| Name | Description |\n| --- | --- |\n| a | |\n| a | good |\n", "| Wrong | Description |\n| --- | --- |\n| a | good |\n", "| Name | Description |\n| --- | --- |\n| a | good | extra |\n"} {
		if r := lint(t, p, s); r.ExitCode != 1 {
			t.Fatalf("bad table %q %+v", s, r)
		}
	}
	for _, text := range []string{"graph LR\n A --> B", "flowchart TB\n  accTitle: \n  accTitle: Again\n  accDescr {\n", "flowchart LR\n\tA --> B", ""} {
		issues := flowchart(&Target{Language: "text", Code: text}, map[string]any{"direction": "LR", "accessible-title": "required", "accessible-description": "forbidden", "source-indent": 2})
		if len(issues) == 0 {
			t.Fatal("bad Mermaid accepted")
		}
	}
	if err := validateMermaid(context.Background(), "flowchart LR\n A --> B"); err != nil {
		t.Fatal(err)
	}
	if err := validateMermaid(context.Background(), "flowchart LR\n A -->"); err == nil {
		t.Fatal("bad Mermaid syntax accepted")
	}
}
func TestReadabilityUnknownAndSuppression(t *testing.T) {
	p := one(t, "core.limit", `{measure: readability.flesch-kincaid-grade, max: 2}`, "document")
	r := lint(t, p, "Short sample.")
	if r.Complete || r.Summary.UnknownTargets != 1 || r.Diagnostics[0].Result != "unknown" {
		t.Fatalf("%+v", r)
	}
	p.Rules["test"] = Rule{Check: "core.limit", Severity: "error", Unknown: "report", Options: map[string]any{"measure": "readability.flesch-kincaid-grade", "max": 2}}
	r = lint(t, p, "Short sample.")
	if r.ExitCode != 0 || r.Complete {
		t.Fatal("report unknown")
	}
	p = one(t, "core.limit", `{measure: readability.flesch-reading-ease, max: 0, parameters: {min-words: 1, min-sentences: 1}}`, "each paragraph")
	if r := lint(t, p, "The cache is fast."); r.ExitCode != 1 {
		t.Fatal("readability bounds")
	}
	p = one(t, "core.limit", `{measure: words, max: 1}`, "each paragraph")
	p.Config.Suppressions = []rulepack.Suppression{{Rule: "test", Path: "guide.md", Reason: "test exception"}}
	if r := lint(t, p, "Two words."); r.ExitCode != 0 || r.Summary.Suppressed != 1 {
		t.Fatalf("%+v", r)
	}
	p = one(t, "core.match", `{mode: word, values: [cache]}`, "each sentence")
	if r := lint(t, p, "# Title\n"); r.Summary.ZeroSelectionBindings != 1 {
		t.Fatal("zero selection missing")
	}
}
func TestMDXStaticAndOpaque(t *testing.T) {
	source := []byte("import X from 'never-executed'\n\n# Title\n\nHello **world** {danger()}!\n\n<Note>\n\nStatic text.\n\n</Note>\n")
	d, err := ParseMDX(context.Background(), "guide.mdx", source, "stdin", map[string]string{"Note": "block"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Root.Unknown || d.Legacy != nil {
		t.Fatal("MDX opaque or neutral model")
	}
	p := one(t, "core.limit", `{measure: words, max: 10}`, "document")
	r := NewReport("test", "error")
	p.Run(context.Background(), t.TempDir(), []*Document{d}, r)
	if r.Complete || r.Summary.UnknownTargets != 1 {
		t.Fatalf("%+v", r)
	}
	if _, err := ParseMDX(context.Background(), "x.mdx", []byte("<Bad>"), "stdin", nil); err == nil {
		t.Fatal("invalid MDX accepted")
	}
	d, err = ParseMDX(context.Background(), "x.mdx", []byte("# Title\n\nStatic text.\n"), "stdin", nil)
	if err != nil || d.Root.Unknown {
		t.Fatal(err)
	}
	r = NewReport("test", "error")
	p.Run(context.Background(), t.TempDir(), []*Document{d}, r)
	if r.ExitCode != 0 {
		t.Fatalf("%+v", r)
	}
}
func TestCLIAndManagement(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "policy.yaml")
	if err := os.WriteFile(config, []byte("version: 2\napply: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(args []string, input string) (int, string) {
		var out, errOut bytes.Buffer
		code := Run(context.Background(), args, strings.NewReader(input), &out, &errOut, registry(t), "test")
		return code, out.String()
	}
	for _, command := range []string{"validate", "explain", "format", "schema", "export"} {
		args := []string{"config", command}
		if command != "schema" {
			args = append(args, "--config", config)
		}
		if command == "explain" {
			args = append(args, "--path", "guide.md")
		}
		if command == "export" {
			args = append(args, "--set", "markdown:recommended")
		}
		code, out := run(args, "")
		if code != 0 {
			t.Fatalf("%s: %d %s", command, code, out)
		}
	}
	for _, args := range [][]string{{"sets", "list", "--config", config}, {"sets", "describe", "--config", config, "markdown:recommended"}, {"rules", "list"}, {"rules", "describe", "core.limit"}, {"--version"}} {
		code, out := run(args, "")
		if code != 0 {
			t.Fatalf("%v: %d %s", args, code, out)
		}
	}
	code, out := run([]string{"--config", config, "--root", root, "--json", "-"}, "Text.\n")
	if code != 0 {
		t.Fatal(out)
	}
	var r Report
	if err := json.Unmarshal([]byte(out), &r); err != nil || r.SchemaVersion != 2 || r.Inputs[0].Source != "stdin" || !r.Inputs[0].SyntaxOnly {
		t.Fatal(out)
	}
	for _, args := range [][]string{{"--config", config, "--root", root, "--json", "-", "-"}, {"--config", config, "--json", "--format", "sarif", "-"}, {"--config", "-", "--json", "-"}, {"--config", config, "--json", "--stdin-filepath", "../x.md", "-"}, {"--config", config, "--json", "--fix", "-"}, {"--config", config, "--json", "--stdin-filepath", "x.md"}, {"--config", config, "--json", "--bogus"}, {"--config", config, "--json", "--stdin-filepath", "x.txt", "-"}} {
		code, out := run(args, "text")
		if code != 2 || json.Unmarshal([]byte(out), &r) != nil || r.ExitCode != 2 || r.Complete {
			t.Fatalf("%v: %d %s", args, code, out)
		}
	}
	code, out = run([]string{"--config", config, "--root", root, "--format", "sarif", "-"}, "Text.")
	if code != 0 || !strings.Contains(out, `"version":"2.1.0"`) {
		t.Fatal(out)
	}
	if err := os.WriteFile(filepath.Join(root, "a.MARKDOWN"), []byte("Text."), 0600); err != nil {
		t.Fatal(err)
	}
	if files, err := Discover(root, []string{root}, nil); err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	if _, err := Discover(root, []string{config}, nil); err == nil {
		t.Fatal("unsupported input")
	}
}
func TestFormatExportAndReportSchema(t *testing.T) {
	p := compile(t, "# Policy comment\napply: []\nversion: 2\ntemplates:\n  example: |-\n    # Title\n    {{ section paragraphs=1 }}\nsets:\n  a: {templates: [example]}\n")
	formatted, err := p.Format()
	if err != nil || !bytes.Contains(formatted, []byte("# Policy comment")) {
		t.Fatal(err, string(formatted))
	}
	if _, err := Decode(formatted, "formatted.yaml", registry(t)); err != nil {
		t.Fatal(err)
	}
	export, err := p.Export("a", "test")
	if err != nil {
		t.Fatal(err)
	}
	q, err := Decode(export, "export.yaml", registry(t))
	if err != nil {
		t.Fatal(err, string(export))
	}
	if _, ok := q.Config.Templates["example"]; !ok {
		t.Fatal("closure missing")
	}
	if _, err := json.Marshal(ConfigSchema(registry(t))); err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(ReportSchema()); err != nil {
		t.Fatal(err)
	}
}
