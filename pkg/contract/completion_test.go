package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/markdown-linter/pkg/rulepack"
)

func TestFinalReviewRegressions(t *testing.T) {
	p := one(t, "core.match", `{mode: pattern, pattern: '{X}, BUT {Y}.', case: fold}`, "each sentence")
	if r := lint(t, p, "It works, but it fails."); r.ExitCode != 0 {
		t.Fatal(r.Diagnostics)
	}
	p = compile(t, "version: 2\ntemplates:\n  a: |-\n    # **Title** &amp; [Label](https://example.test)\n    ````text\n    ```\n    {{ invalid }}\n    ````\nsets: {a: {templates: [a]}}\napply: [{use: [a]}]\n")
	if r := lint(t, p, "# **Title** &amp; [Label](https://example.test)\n"); r.ExitCode != 0 {
		t.Fatal(r.Diagnostics)
	}
	if _, err := Decode([]byte("version: 2\napply: []\nsuppressions: [{rule: template.typo.anything, path: '**', reason: exception}]"), "x", registry(t)); err == nil {
		t.Fatal("unknown generated rule accepted")
	}
	p = one(t, "core.match", `{mode: word, expect: absent, values: [utilize]}`, "each paragraph")
	for _, sample := range []struct {
		source string
		code   int
	}{
		{"<!-- marklint-disable-next-line test reason: reviewed exception -->\nutilize.\n", 0},
		{"<!-- marklint-disable-next-line test reason: -->\nutilize.\n", 1},
		{"<!-- marklint-disable-next-line other reason: exception -->\nutilize.\n", 1},
		{"```html\n<!-- marklint-disable-next-line test reason: exception -->\n```\nutilize.\n", 1},
	} {
		if r := lint(t, p, sample.source); r.ExitCode != sample.code {
			t.Fatal(r.Diagnostics)
		}
	}
	root := t.TempDir()
	for _, args := range [][]string{{"--unknown", "--format", "json", "-"}, {"--unknown", "--format=json", "-"}} {
		var out, stderr bytes.Buffer
		if code := Run(context.Background(), args, strings.NewReader("x"), &out, &stderr, registry(t), "test"); code != 2 {
			t.Fatal(code)
		}
		var r Report
		if err := json.Unmarshal(out.Bytes(), &r); err != nil || r.ExitCode != 2 {
			t.Fatal(out.String(), err)
		}
	}
	if _, err := Discover(root, []string{t.TempDir()}, nil); err == nil {
		t.Fatal("outside empty directory accepted")
	}
}

func TestProviderSentenceContextAndSharedSamples(t *testing.T) {
	caps := NewCapabilities()
	d := descriptor()
	d.Targets = []string{"sentence"}
	d.SentenceAttribution = true
	calls := 0
	if err := caps.RegisterProvider("native", d, func(_ context.Context, in MeasureInput) ([]Sample, error) {
		calls++
		if in.Text != "First sentence. Second sentence." || in.TargetText == in.Text {
			t.Fatalf("context %+v", in)
		}
		return []Sample{{Status: "known", Value: .8, StartOffset: in.TargetStartOffset, EndOffset: in.TargetEndOffset, Text: in.TargetText, Inputs: map[string]any{}, Classification: "native"}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	source := `version: 2
rules:
  score: {check: core.limit, options: {measure: provider.score, max: 0.4, parameters: {provider: native, measure: test.score, context: paragraph}}}
  another: {extends: score, options: {max: 0.5}}
sets: {a: {rules: [{rule: score, on: each sentence}, {rule: another, on: each sentence}]}}
apply: [{use: [a]}]`
	p, err := DecodeWithCapabilities([]byte(source), "policy.yaml", registry(t), caps)
	if err != nil {
		t.Fatal(err)
	}
	p.Config.Suppressions = []rulepack.Suppression{{Rule: "another", Path: "guide.md", Reason: "Reviewed"}}
	r := lint(t, p, "First sentence. Second sentence.")
	if calls != 2 || len(r.Diagnostics) != 2 || r.Summary.Suppressed != 2 {
		t.Fatalf("calls=%d report=%+v", calls, r)
	}
	for _, diag := range r.Diagnostics {
		if diag.Provenance["measure"] != "test.score" {
			t.Fatal(diag)
		}
	}
	// A context adapter must return attribution for the requested sentence.
	caps.Providers["native"][d.ID] = InstalledMeasure{Descriptor: d, Evaluate: func(context.Context, MeasureInput) ([]Sample, error) {
		return []Sample{{Status: "known", Value: .8, Text: "paragraph"}}, nil
	}}
	if r := lint(t, p, "First sentence. Second sentence."); r.Complete || r.Summary.UnknownTargets != 4 {
		t.Fatal(r)
	}
	if _, err := DecodeWithCapabilities([]byte(strings.ReplaceAll(source, "each sentence", "document")), "x", registry(t), caps); err == nil {
		t.Fatal("paragraph context on document")
	}
}

func TestSharedActivationOriginsAndExportClosure(t *testing.T) {
	p := compile(t, `version: 2
rules:
  parent: {check: core.limit, options: {measure: graphemes, min: 1, view: cell}}
  cell: {extends: parent}
  table: {check: markdown.table-schema, options: {header: [Name], columns: {Name: {rules: [cell]}}}}
  wording: {check: core.match, options: {mode: word, expect: absent, values: [utilize]}}
templates:
  a: |-
    # Title
    {{ each table rule="table" }}
    {{ each paragraph rule="wording" }}
sets:
  a: {templates: [a], rules: [{rule: wording, on: each paragraph}]}
apply: [{use: [a]}]
suppressions:
  - {rule: cell, path: guide.md, reason: reviewed}
  - {rule: template.a.heading.1, path: guide.md, reason: reviewed}
`)
	r := lint(t, p, "# Title\n\nutilize.\n")
	if len(r.Diagnostics) != 1 || len(r.Diagnostics[0].Related) != 3 {
		t.Fatalf("origins %+v", r.Diagnostics)
	}
	origin := p.origin("/apply/0/use/0", 0)
	if origin.Start.Line != 14 {
		t.Fatalf("sequence origin %+v", origin)
	}
	out, err := p.Export("a", "test")
	if err != nil {
		t.Fatal(err)
	}
	exported := compile(t, string(out))
	for _, id := range []string{"parent", "cell", "table", "wording"} {
		if _, ok := exported.Rules[id]; !ok {
			t.Fatal("closure missing", id)
		}
	}
	if len(exported.Config.Suppressions) != 2 {
		t.Fatal("lost exceptions")
	}
	if _, err := p.Export("missing", "test"); err == nil {
		t.Fatal("unknown set")
	}
}

func TestResolvedOptionValidation(t *testing.T) {
	p := Default(registry(t))
	cases := map[string][]map[string]any{
		"core.limit": {
			{"measure": "unknown", "max": 1}, {"measure": "words"}, {"measure": "words", "max": -1}, {"measure": "words", "max": 1, "view": "invalid"}, {"measure": "words", "max": 1, "parameters": map[string]any{"unused": 1}}, {"measure": "provider.score", "max": 1},
			{"measure": "words", "max": 1, "extra": true}, {"measure": "words", "max": nil}, {"measure": 3, "max": 1},
			{"measure": "words", "max": "1"}, {"measure": "words", "max": 1, "parameters": 3},
			{"measure": "readability.flesch-reading-ease", "max": 1, "parameters": map[string]any{"extra": 1}},
			{"measure": "readability.flesch-reading-ease", "max": 1, "parameters": map[string]any{"min-words": 0}},
			{"measure": "provider.score", "max": 1, "parameters": map[string]any{"provider": "a", "measure": "b", "extra": true}},
			{"measure": "provider.score", "max": 1, "parameters": map[string]any{"provider": "a", "measure": "b", "aggregation": "average"}},
		},
		"core.match": {
			{"mode": "invalid"}, {"mode": "equals", "values": []any{"x"}, "expect": "invalid"}, {"mode": "regex", "regex": "x", "anchor": "invalid"},
			{"mode": "word", "values": []any{"x"}, "extra": true}, {"mode": 1}, {"mode": "equals", "values": []any{1}},
			{"mode": "equals", "values": []any{}}, {"mode": "equals", "values": []any{"x"}, "pattern": "x"},
			{"mode": "equals", "values": []any{"x"}, "view": "invalid"}, {"mode": "regex", "regex": 3},
			{"mode": "pattern", "pattern": "{X}"}, {"mode": "regex", "regex": strings.Repeat("x", 4097)},
		},
		"core.sequence":          {{"allow": []any{"paragraph"}, "extra": true}, {"sequence": 3}, {"sequence": []any{}}, {"allow": []any{"paragraph", "paragraph"}}},
		"markdown.table-schema":  {{"header": []any{"Name"}, "extra": true}, {"header": []any{}}, {"header": []any{"Name", "Name"}}, {"header": []any{"Name"}, "columns": 3}, {"header": []any{"Name"}, "columns": map[string]any{"Name": 3}}, {"header": []any{"Name"}, "columns": map[string]any{"Name": map[string]any{"unknown": true}}}, {"header": []any{"Name"}, "columns": map[string]any{"Name": map[string]any{"rules": 3}}}},
		"mermaid.flowchart":      {{"extra": true}, {"direction": "bad"}, {"accessible-title": "bad"}, {"source-indent": 9}, {"source-indent": "2"}},
		"missing.check":          {{}},
		"text.ste100.dictionary": {{"dictionary-file": "dictionary.yaml"}},
	}
	for check, options := range cases {
		if check == "core.sequence" {
			options = append(options, map[string]any{}, map[string]any{"sequence": []any{"paragraph"}, "allow": []any{"table"}})
		}
		if check == "markdown.table-schema" {
			options = append(options, map[string]any{"header": []any{"Name"}, "columns": map[string]any{" Name ": map[string]any{}}}, map[string]any{"header": []any{"Name"}, "columns": map[string]any{"Name": map[string]any{"unique": "yes"}}})
		}
		for i, opts := range options {
			t.Run(fmt.Sprintf("%s/%d", check, i), func(t *testing.T) {
				if err := p.validateRule(Rule{Check: check, Options: opts}); err == nil {
					t.Fatal("invalid resolved options accepted", opts)
				}
			})
		}
	}
	if err := stringsField([]string{"valid"}, true); err != nil {
		t.Fatal(err)
	}
	if err := stringsField([]string{" "}, true); err == nil {
		t.Fatal("blank term")
	}
}

func TestRepositoryDocumentationPolicy(t *testing.T) {
	source, err := os.ReadFile("../../docs/lint.yaml")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Decode(source, "docs/lint.yaml", registry(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct{ text, rule string }{
		{"# Title\n\nZzunknownword.\n", "docs.approved-vocabulary"},
		{"# Title\n\nThis isn't allowed.\n", "docs.simple-grammar"},
		{"# Title\n\n" + strings.Repeat("word ", 20) + ".\n", "docs.simple-grammar"},
	} {
		r := lint(t, p, sample.text)
		found := false
		for _, d := range r.Diagnostics {
			found = found || d.RuleID == sample.rule
		}
		if !found {
			t.Fatal("policy failed to reject", sample.rule, r.Diagnostics)
		}
	}
	root := t.TempDir()
	r := NewReport("test", "error")
	p.Run(context.Background(), root, []*Document{Parse(".site-docs/rules/check.md", []byte("# Title\n\nText.\n"), "stdin")}, r)
	found := false
	for _, d := range r.Diagnostics {
		found = found || d.RuleID == "docs.page-shape"
	}
	if !found {
		t.Fatal("missing reference shape")
	}
	// Preserve exact cell spans when inline code uses more than one backtick.
	d, err := ParseMDX(context.Background(), "x.mdx", []byte("| Type |\n| --- |\n| ``string`` |\n"), "stdin", nil)
	if err != nil {
		t.Fatal(err)
	}
	cell := d.selectTargets(d.Root, "table", "subtree")[0].Rows[0][0]
	u := cell.Units[0]
	if string(d.Source[u.Offsets[0]:u.EndOffsets[len(u.EndOffsets)-1]]) != "string" {
		t.Fatal("code span mapping", u)
	}
}
