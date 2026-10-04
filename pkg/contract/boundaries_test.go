package contract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
)

func TestReviewerRegressions(t *testing.T) {
	for _, source := range []string{"```go", "---\ninvalid: [\n---\n"} {
		if r := lint(t, compile(t, "version: 2\napply: []\n"), source); r.ExitCode != 2 || r.Complete {
			t.Fatal("syntax-only false clean")
		}
	}
	p := one(t, "core.match", `{mode: word, expect: absent, values: [utilize]}`, "document")
	for source, want := range map[string][2]int{"utilized utilize.": {9, 16}, "Safe &amp; utilize.": {11, 18}, "Safe util&#105;ze.": {5, 17}, "Safe **utilize**.": {7, 14}} {
		r := lint(t, p, source)
		loc := r.Diagnostics[0].Location
		if loc.StartOffset != want[0] || loc.EndOffset != want[1] {
			t.Errorf("%s: %+v", source, loc)
		}
	}
	r := lint(t, p, "# Utilize\n\n| Name |\n| --- |\n| utilize |\n")
	if r.ExitCode != 1 {
		t.Fatal("visible units missed")
	}
	for _, options := range []string{`{mode: regex, regex: utilize, case: fold}`, `{mode: equals, values: [utilize], view: prose}`} {
		_, err := Decode([]byte(fmt.Sprintf("version: 2\nrules: {a: {check: core.match, options: %s}}\nsets: {a: {rules: [{rule: a, on: document}]}}\napply: [{use: [a]}]", options)), "x", registry(t))
		if err == nil {
			t.Fatal("invalid matching contract")
		}
	}
	for _, source := range []string{"version: 2\nlanguage: null\napply: []", "version: 2\napply: []\nrules: {a: {check: mermaid.flowchart, description: 123}}", "version: 2\napply: []\nsets: null"} {
		if _, err := Decode([]byte(source), "x", registry(t)); err == nil {
			t.Fatal("raw type coercion")
		}
	}
	d, err := ParseMDX(context.Background(), "x.mdx", []byte("# Title\n\n## One\n\nFirst.\n\n## Two\n\nSecond.\n"), "stdin", nil)
	if err != nil {
		t.Fatal(err)
	}
	p = compile(t, `version: 2
templates:
  a: |-
    # Title
    ## One
    {{ paragraph[1] equals="First." }}
    ## Two
    {{ paragraph[1] equals="Second." }}
sets: {a: {templates: [a]}}
apply: [{use: [a]}]`)
	report := NewReport("test", "error")
	p.Run(context.Background(), t.TempDir(), []*Document{d}, report)
	if report.ExitCode != 0 {
		t.Fatal(report.Diagnostics)
	}
	d, err = ParseMDX(context.Background(), "x.mdx", []byte("| Name |\n| --- |\n| one | extra |\n"), "stdin", nil)
	if err != nil {
		t.Fatal(err)
	}
	p = one(t, "markdown.table-schema", `{header: [Name]}`, "each table")
	report = NewReport("test", "error")
	p.Run(context.Background(), t.TempDir(), []*Document{d}, report)
	if report.ExitCode != 1 {
		t.Fatal("MDX extra cell false clean")
	}
	d = Parse("x.md", []byte("| Name | Type |\n| --- | --- |\n| value | |\n"), "stdin")
	cell := d.selectTargets(d.Root, "table", "subtree")[0].Rows[0][1]
	if cell.Start == 0 || cell.End != cell.Start {
		t.Fatalf("empty cell location: %+v", cell)
	}
}
func descriptor() Descriptor {
	return Descriptor{ID: "test.score", Version: "1", Summary: "Test native score", Targets: []string{"document", "sentence"}, Formats: []string{"markdown"}, Languages: []string{"en"}, Capabilities: []string{"provider"}, ParametersSchema: closed(map[string]any{}), Execution: "local", Fixtures: []any{}, Unit: "fraction", ValueType: "number", Granularity: "target", Views: []string{"auto", "prose"}, Range: &[2]float64{0, 1}}
}
func TestInstalledMeasuresAndProviders(t *testing.T) {
	cases := []struct {
		name           string
		d              Descriptor
		samples        []Sample
		err            error
		unknown, error bool
	}{
		{"known", descriptor(), []Sample{{Status: "known", Value: .8, Inputs: map[string]any{}, Classification: "target"}}, nil, false, false},
		{"unknown", descriptor(), []Sample{{Status: "unknown"}}, nil, true, false},
		{"crash", descriptor(), nil, errors.New("provider failed"), false, true},
		{"empty", descriptor(), nil, nil, false, true},
		{"multi", descriptor(), []Sample{{Status: "known", Value: .1}, {Status: "known", Value: .1}}, nil, false, true},
		{"nonfinite", descriptor(), []Sample{{Status: "known", Value: math.Inf(1)}}, nil, false, true},
		{"invalid-status", descriptor(), []Sample{{Status: "invalid", Value: .1}}, nil, false, true},
		{"scale", descriptor(), []Sample{{Status: "known", Value: 2}}, nil, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			caps := NewCapabilities()
			if err := caps.RegisterMeasure(c.d, func(context.Context, MeasureInput) ([]Sample, error) { return c.samples, c.err }); err != nil {
				t.Fatal(err)
			}
			if err := caps.RegisterMeasure(c.d, func(context.Context, MeasureInput) ([]Sample, error) { return nil, nil }); err == nil {
				t.Fatal("duplicate measure")
			}
			source := []byte("version: 2\nrules: {a: {check: core.limit, options: {measure: test.score, max: 0.4}}}\nsets: {a: {rules: [{rule: a, on: document}]}}\napply: [{use: [a]}]")
			p, err := DecodeWithCapabilities(source, "policy.yaml", registry(t), caps)
			if err != nil {
				t.Fatal(err)
			}
			r := lint(t, p, "A target.")
			if (r.ExitCode == 2) != c.error || (r.Summary.UnknownTargets > 0) != c.unknown {
				t.Fatalf("%+v", r)
			}
		})
	}
	for _, granularity := range []string{"target", "window"} {
		for _, remote := range []bool{false, true} {
			for _, align := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%t/%t", granularity, remote, align), func(t *testing.T) {
					caps := NewCapabilities()
					d := descriptor()
					d.Granularity = granularity
					if remote {
						d.Execution = "remote"
					}
					if err := caps.RegisterProvider("test-provider", d, func(ctx context.Context, input MeasureInput) ([]Sample, error) {
						s := Sample{Status: "known", Value: .8, StartOffset: 0, EndOffset: 6, Text: "Target", Inputs: map[string]any{}, Classification: "native-window"}
						if !align {
							s.Text = "wrong"
						}
						return []Sample{s}, nil
					}); err != nil {
						t.Fatal(err)
					}
					aggregation := "target"
					if granularity == "window" {
						aggregation = "each-window"
					}
					source := []byte(fmt.Sprintf("version: 2\nrules: {a: {check: core.limit, options: {measure: provider.score, max: 0.4, parameters: {provider: test-provider, measure: test.score, aggregation: %s}}}}\nsets: {a: {rules: [{rule: a, on: document}]}}\napply: [{use: [a]}]", aggregation))
					p, err := DecodeWithCapabilities(source, "policy.yaml", registry(t), caps)
					if err != nil {
						t.Fatal(err)
					}
					r := lint(t, p, "Target text.")
					if remote && r.ExitCode != 2 {
						t.Fatal("network permission bypass")
					}
					p.Network = []string{"test-provider"}
					r = lint(t, p, "Target text.")
					if granularity == "window" && !align {
						if r.Complete || r.Summary.UnknownTargets != 1 {
							t.Fatal("unaligned score")
						}
					} else if r.ExitCode != 1 {
						t.Fatalf("%+v", r)
					}
					if err := caps.RegisterProvider("test-provider", d, func(context.Context, MeasureInput) ([]Sample, error) { return nil, nil }); err == nil {
						t.Fatal("duplicate provider")
					}
				})
			}
		}
	}
	for _, mutate := range []func(*Descriptor){func(d *Descriptor) { d.ID = "Bad" }, func(d *Descriptor) { d.Execution = "bad" }, func(d *Descriptor) { d.Range = &[2]float64{2, 1} }} {
		d := descriptor()
		mutate(&d)
		if err := NewCapabilities().RegisterMeasure(d, func(context.Context, MeasureInput) ([]Sample, error) { return nil, nil }); err == nil {
			t.Fatal("invalid descriptor")
		}
	}
	if err := NewCapabilities().RegisterProvider("Bad", descriptor(), nil); err == nil {
		t.Fatal("invalid provider ID")
	}
	d := descriptor()
	d.ID = "words"
	if err := NewCapabilities().RegisterMeasure(d, func(context.Context, MeasureInput) ([]Sample, error) { return nil, nil }); err == nil {
		t.Fatal("reserved measure")
	}
}
func TestAllCompatibilityBoundaries(t *testing.T) {
	cases := []struct {
		check       string
		options     map[string]any
		kind, scope string
	}{
		{"core.limit", map[string]any{"measure": "words"}, "table", ""}, {"core.limit", map[string]any{"measure": "lines", "view": "prose"}, "codeblock", ""}, {"core.limit", map[string]any{"measure": "graphemes"}, "codeblock", ""}, {"core.limit", map[string]any{"measure": "words", "view": "visible"}, "cell", ""}, {"core.limit", map[string]any{"measure": "bytes", "view": "prose"}, "document", ""}, {"core.limit", map[string]any{"measure": "words", "view": "source"}, "document", ""}, {"core.limit", map[string]any{"measure": "readability.flesch-kincaid-grade", "view": "visible"}, "document", ""}, {"core.limit", map[string]any{"measure": "rows", "view": "cell"}, "table", ""},
		{"core.match", map[string]any{"mode": "contains"}, "table", ""}, {"core.match", map[string]any{"mode": "contains", "view": "language"}, "paragraph", ""}, {"core.match", map[string]any{"mode": "contains"}, "codeblock", ""}, {"core.match", map[string]any{"mode": "contains", "view": "prose"}, "cell", ""}, {"core.match", map[string]any{"mode": "word", "view": "source"}, "document", ""}, {"core.match", map[string]any{"mode": "equals"}, "document", ""}, {"core.match", map[string]any{"mode": "regex", "anchor": "full"}, "document", ""},
		{"core.sequence", nil, "document", ""}, {"core.sequence", nil, "section", "subtree"}, {"markdown.table-schema", nil, "document", ""}, {"mermaid.flowchart", nil, "paragraph", ""}, {"markdown.final-newline", nil, "paragraph", ""},
	}
	for _, c := range cases {
		if compatible(Rule{Check: c.check, Options: c.options}, c.kind, c.scope) == nil {
			t.Errorf("accepted %+v", c)
		}
	}
	if compatible(Rule{Check: "core.limit", Options: map[string]any{"measure": "graphemes", "view": "cell"}}, "cell", "") != nil {
		t.Fatal("valid cell")
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("closed output") }

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("unreadable input") }
func TestCLIErrorReportsAndDiskFixes(t *testing.T) {
	root := t.TempDir()
	policy := filepath.Join(root, "policy.yaml")
	if err := os.WriteFile(policy, []byte("version: 2\napply: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(args []string, input string) (int, string) {
		var out, stderr bytes.Buffer
		code := Run(context.Background(), args, strings.NewReader(input), &out, &stderr, registry(t), "test")
		return code, out.String()
	}
	for _, args := range [][]string{
		{"config", "schema", "--schema", "invalid"}, {"config", "explain", "--config", policy}, {"config", "explain", "--config", policy, "--path", "../bad"}, {"config", "export", "--config", policy}, {"config", "export", "--config", policy, "--set", "missing"}, {"sets", "describe", "--config", policy}, {"sets", "describe", "--config", policy, "missing"}, {"rules", "describe"}, {"rules", "describe", "missing"}, {"--config", filepath.Join(root, "missing"), "--json", "-"}, {"--config", policy, "--root", filepath.Join(root, "missing"), "--json", "-"}, {"--config", policy, "--format", "invalid", "-"}, {"--config", policy, "--fail-on", "invalid", "-"}, {"config", "invalid", "--config", policy}, {"--config", policy, "--fix", "--fix-check", root}, {"--config", policy, "--stdin-filepath", "*.md", "-"}, {"--config", policy, root},
	} {
		code, _ := run(args, "")
		if code != 2 {
			t.Errorf("%v => %d", args, code)
		}
	}
	for _, kind := range []string{"report", "resolved", "check-descriptor", "measure-descriptor"} {
		code, out := run([]string{"config", "schema", "--schema", kind}, "")
		if code != 0 || len(out) < 10 {
			t.Fatal(out)
		}
	}
	var stderr bytes.Buffer
	if code := Run(context.Background(), []string{"--config", policy, "--json", "-"}, brokenReader{}, &bytes.Buffer{}, &stderr, registry(t), "test"); code != 2 {
		t.Fatal("read failure")
	}
	for _, args := range [][]string{{"--version"}, {"rules", "list"}, {"--config", policy, "--json", "-"}, {"--config", policy, "--format", "sarif", "-"}} {
		if code := Run(context.Background(), args, strings.NewReader("Text."), brokenWriter{}, &stderr, registry(t), "test"); code != 2 {
			t.Fatal("write failure")
		}
	}
	file := filepath.Join(root, "a.md")
	if err := os.WriteFile(file, []byte("# Title\n\nText \t\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policy, []byte("version: 2\napply: [{use: [markdown:recommended]}]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if code, _ := run([]string{"--config", policy, "--root", root, "--fix-check", file}, ""); code != 0 {
		t.Fatal("preview")
	}
	if code, out := run([]string{"--config", policy, "--root", root, "--fix", file}, ""); code != 0 {
		t.Fatal(out)
	}
	content, _ := os.ReadFile(file)
	if strings.Contains(string(content), "\t") {
		t.Fatal("safe fix not applied")
	}
	code, _ := run([]string{"--config", policy, "--root", root, "--stdin-filepath", "a.md", file, "-"}, "text")
	if code != 2 {
		t.Fatal("identity collision")
	}
	if err := os.WriteFile(filepath.Join(root, ".marklint.yaml"), []byte("version: 2\napply: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if code, out := run([]string{"--root", root, "--json", file}, ""); code != 0 {
		t.Fatal(out)
	}
	r := repeated{}
	_ = r.Set("provider")
	if r.String() != "provider" {
		t.Fatal(r)
	}
}
func TestDocumentCorpusAndUnknownExecution(t *testing.T) {
	p := compile(t, `version: 2
rules:
  title: {check: markdown.required-heading, options: {heading: Required, level: 2}}
templates:
  a: |-
    {{ document rule="title" }}
    # Title
sets: {a: {templates: [a]}}
apply: [{use: [a]}]`)
	if r := lint(t, p, "# Title\n"); r.ExitCode != 1 {
		t.Fatalf("template legacy check %+v", r)
	}
	p = Default(registry(t))
	r := NewReport("test", "error")
	d, err := ParseMDX(context.Background(), "x.mdx", []byte("# Title"), "stdin", nil)
	if err != nil {
		t.Fatal(err)
	}
	p.Run(context.Background(), t.TempDir(), []*Document{d}, r)
	if r.ExitCode != 2 {
		t.Fatal("unsupported adapter false clean")
	}
	p = one(t, "core.match", `{mode: contains, values: [witness]}`, "document")
	d, err = ParseMDX(context.Background(), "x.mdx", []byte("Static witness {unknown()}"), "stdin", nil)
	if err != nil {
		t.Fatal(err)
	}
	r = NewReport("test", "error")
	p.Run(context.Background(), t.TempDir(), []*Document{d}, r)
	if r.ExitCode != 0 {
		t.Fatal(r.Diagnostics)
	}
	r = NewReport("test", "error")
	for _, target := range d.Targets {
		if target.Kind == "paragraph" {
			target.Units = []Unit{{Text: "Static "}}
		}
	}
	p.Run(context.Background(), t.TempDir(), []*Document{d}, r)
}
func TestProjectionAndReportUtilityBoundaries(t *testing.T) {
	for _, source := range []string{"# Title\n\n> Quoted &amp; text.\n\n---\n\n<div>HTML</div>\n", "    indented code\n", "Paragraph with <b>HTML</b> and <https://site.test>.\n", ""} {
		d := Parse("x.md", []byte(source), "stdin")
		_ = d.measure(d.Root, "blocks", "auto", "subtree")
		_ = d.measure(d.Root, "graphemes", "auto", "subtree")
		_ = d.measure(d.Root, "sentences", "auto", "subtree")
	}
	n := ast.NewString([]byte("static"))
	_ = projection(n, []byte(""), false)
	_ = physicalLines("")
	_ = lineStart([]byte("x"), 10)
	_ = lineEnd([]byte("x"), 10)
	_ = lineEnd([]byte("x"), 0)
	_ = countTableCells(`| one\|two | three |`)
	_ = location("x", []byte("😀\nx"), -2, 999)
	_ = location("x", []byte("x"), 99, -1)
	p := Default(registry(t))
	_ = p.origin("/missing", 1)
	for _, value := range []any{int64(1), float64(1), "x"} {
		_, _ = number(value)
	}
	_ = str(map[string]any{"a": 1}, "a", "fallback")
	_ = list([]any{1, "x"})
	if err := stringsField(42, false); err == nil {
		t.Fatal("array type")
	}
	if err := stringField(map[string]any{"a": 1}, "a"); err == nil {
		t.Fatal("string type")
	}
	r := NewReport("test", "info")
	r.Diagnostics = []Diagnostic{{RuleID: "b", Code: "z", Severity: "warning", Result: "fail", Location: location("x", []byte("abc"), 2, 3)}, {RuleID: "a", Code: "a", Severity: "info", Result: "fail", Location: location("x", []byte("abc"), 0, 1)}}
	r.Finish()
	if r.ExitCode != 1 || r.Summary.Info != 1 {
		t.Fatal(r)
	}
	_ = sarif(r)
	r.Error("error", "problem", nil)
	r.Finish()
	_ = sarif(r)
	for _, raw := range []string{`"\uD83D\uDE00"`, `"\uDC00"`, `"\uD800\u1234"`, `"\uD800"`} {
		_ = validEscapes(raw)
	}
	p = one(t, "core.limit", `{measure: provider.score, max: 0.4, parameters: {provider: missing, measure: score}}`, "document")
	if r := lint(t, p, "Text."); r.ExitCode != 2 {
		t.Fatal("unavailable provider")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r = NewReport("test", "error")
	p.Run(ctx, t.TempDir(), []*Document{Parse("x.md", []byte("Text."), "stdin")}, r)
	if len(r.Diagnostics) == 0 {
		t.Fatal("cancellation")
	}
}
