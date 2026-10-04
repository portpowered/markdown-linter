package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplatePredicatesAndInvalidValues(t *testing.T) {
	p := compile(t, `version: 2
templates:
  a: |-
    <!--
    {{ inert }}
    -->
    # Title
    {{ paragraph[1] not-contains="missing" has-word="cache" no-word="hype" }}
sets: {a: {templates: [a]}}
apply: [{use: [a]}]`)
	if r := lint(t, p, "# Title\n\nA cache works.\n"); r.ExitCode != 0 {
		t.Fatal(r.Diagnostics)
	}
	for _, directive := range []string{`heading[1].sentence[1] words=1`, `paragraph[1] words=`, `paragraph[1] contains<="x"`, `paragraph[1] contains="unclosed`, `paragraph[1] contains="\q"`, `paragraph[1] words=huge`, `paragraph[1] words>=1..2`, `paragraph[1] words=99999999999999999999`, `paragraph[1] words=1..99999999999999999999`, `paragraph[1] has-word="two words"`} {
		if _, err := parseDirective(directive, p, -1, 1); err == nil {
			t.Fatal("invalid directive", directive)
		}
	}
	if _, err := parseTemplate("# <!--empty-->", p); err == nil {
		t.Fatal("empty visible heading")
	}
	d, err := ParseMDX(context.Background(), "x.mdx", []byte("# Title\n\n{dynamic()}\n"), "stdin", nil)
	if err != nil {
		t.Fatal(err)
	}
	r := NewReport("test", "error")
	p.Run(context.Background(), t.TempDir(), []*Document{d}, r)
	if r.Complete || r.Summary.UnknownTargets < 2 {
		t.Fatal("opaque indexed target", r)
	}
}

func TestMDXNestedViewsAndRuntimeFailures(t *testing.T) {
	source := []byte("# Title\n\n> Quoted text.\n\n- Tight item.\n\nInline `code` and <Note>static</Note>, <Unknown>dynamic</Unknown>.\n\n| Type |\n| --- |\n| `` string `` |\n\n```go\nfmt.Print()\n```\n\n---\n")
	d, err := ParseMDX(context.Background(), "x.mdx", source, "stdin", map[string]string{"Note": "inline"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Root.Unknown || len(d.selectTargets(d.Root, "codeblock", "subtree")) != 1 {
		t.Fatal("MDX structure")
	}
	if code := d.selectTargets(d.Root, "codeblock", "subtree")[0]; code.Code != "fmt.Print()" || code.Language != "go" {
		t.Fatal(code)
	}
	runtime := t.TempDir()
	t.Setenv("MARKLINT_RUNTIME", runtime)
	if _, err := ParseMDX(context.Background(), "x", []byte("Text."), "stdin", nil); err == nil {
		t.Fatal("missing runtime")
	}
	if err := validateMermaid(context.Background(), "flowchart LR"); err == nil {
		t.Fatal("missing runtime")
	}
	p := one(t, "mermaid.flowchart", `{direction: LR}`, "each codeblock")
	if r := lint(t, p, "```mermaid\nflowchart LR\n  A --> B\n```\n"); r.ExitCode != 2 {
		t.Fatal(r)
	}
	if err := os.WriteFile(filepath.Join(runtime, "analyze.mjs"), []byte("process.stdout.write('not JSON')"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := parseRuntime(context.Background(), "mdx", "Text"); err == nil {
		t.Fatal("corrupt parser output")
	}
}

func TestAdditionalCLIAndRoutingBoundaries(t *testing.T) {
	root := t.TempDir()
	policy := filepath.Join(root, "policy.yaml")
	p := compile(t, "version: 2\napply: []\n")
	canonical, _ := p.Format()
	if err := os.WriteFile(policy, canonical, 0600); err != nil {
		t.Fatal(err)
	}
	run := func(args []string, input string) (int, string) {
		var out, stderr bytes.Buffer
		code := Run(context.Background(), args, strings.NewReader(input), &out, &stderr, registry(t), "test")
		return code, out.String()
	}
	if code, _ := run([]string{"config", "format", "--config", policy, "--check"}, ""); code != 0 {
		t.Fatal("canonical config")
	}
	if err := os.WriteFile(policy, []byte("apply: []\nversion: 2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if code, _ := run([]string{"config", "format", "--config", policy, "--check"}, ""); code != 1 {
		t.Fatal("noncanonical config")
	}
	for _, command := range []string{"validate", "format", "export"} {
		args := []string{"config", command, "--config", policy}
		if command == "export" {
			args = append(args, "--set", "portos")
		}
		if code := Run(context.Background(), args, strings.NewReader(""), brokenWriter{}, &bytes.Buffer{}, registry(t), "test"); code != 2 {
			t.Fatal("closed output", command)
		}
	}
	if code, _ := run([]string{"rules", "list", "--kind", "markdown"}, ""); code != 0 {
		t.Fatal("adapter catalog")
	}
	if code, _ := run([]string{"rules", "list", "--kind", "unknown"}, ""); code != 2 {
		t.Fatal("unsupported catalog")
	}
	if err := os.WriteFile(policy, []byte("version: 2\napply: []\nrules:\n  a: {check: core.limit, options: {measure: words, max: nope}}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out := run([]string{"--config", policy, "--json", "-"}, "Text")
	var report Report
	if err := json.Unmarshal([]byte(out), &report); err != nil || code != 2 || report.Diagnostics[0].Location.Start.Line != 4 {
		t.Fatal("configuration origin", code, out)
	}
	if err := os.WriteFile(policy, []byte("version: 2\napply: [{files: [docs/**], use: []}]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if code, _ := run([]string{"config", "explain", "--config", policy, "--path", "README.md"}, ""); code != 2 {
		t.Fatal("unmatched routing")
	}
	p = compile(t, "version: 2\napply: [{use: [], profile: unicode-v1}]\n")
	if _, err := p.Policy("x.md"); err != nil {
		t.Fatal(err)
	}
	p.Config.Apply[0].Use = []string{"missing"}
	if _, err := p.Policy("x.md"); err == nil {
		t.Fatal("invalid graph")
	}
	for _, dir := range []string{".git", "node_modules", "excluded"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, dir, "x.md"), []byte("Text"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if files, err := Discover(root, []string{root}, []string{"excluded/**"}); err != nil || len(files) != 0 {
		t.Fatal(files, err)
	}
	outside := filepath.Join(t.TempDir(), "x.md")
	if err := os.WriteFile(outside, []byte("Text"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(root, []string{outside}, nil); err == nil {
		t.Fatal("outside file")
	}
}

func TestScoreSequenceAndLanguageBoundaries(t *testing.T) {
	p := one(t, "core.limit", `{measure: readability.flesch-kincaid-grade, max: -10, parameters: {min-words: 1, min-sentences: 1}}`, "each paragraph")
	if r := lint(t, p, "The cache is fast."); r.ExitCode != 1 || r.Diagnostics[0].Measurements[0].Sample != "short" {
		t.Fatal(r)
	}
	if r := lint(t, p, "Unresolved café."); r.Complete || r.Summary.UnknownTargets != 1 {
		t.Fatal(r)
	}
	p.Config.Language = "fr"
	if r := lint(t, p, "Le cache est rapide."); r.ExitCode != 2 {
		t.Fatal("unsupported readability", r)
	}
	p = one(t, "core.sequence", `{allow: [paragraph]}`, "each section")
	if r := lint(t, p, "# Title\n\n## Data\n\nText.\n\n```go\nx\n```\n\n### Child\n\nText.\n"); r.ExitCode != 1 {
		t.Fatal("sequence allow", r)
	}
	p = compile(t, "version: 2\napply: [{files: [docs/**], use: []}]\n")
	if r := lint(t, p, "Text"); r.ExitCode != 2 {
		t.Fatal("unmatched input", r)
	}
	p = compile(t, "version: 2\napply: []\n")
	if r := lint(t, p, "# Title\n\n```go\n"); r.ExitCode != 2 || r.Diagnostics[0].Location.Start.Line != 3 {
		t.Fatal("fence source", r)
	}
	p = one(t, "core.match", `{mode: contains, values: [CACHE], case: fold}`, "each paragraph")
	if r := lint(t, p, "The cache works."); r.ExitCode != 0 {
		t.Fatal("folded literal", r)
	}
	root := t.TempDir()
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"--root", root, "--stdin-filepath", "x.mdx", "--json", "-"}, strings.NewReader("<Invalid>"), &out, &stderr, registry(t), "test"); code != 2 {
		t.Fatal("MDX parse failure", out.String())
	}
	out.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"--root", root, "--json", "-"}, strings.NewReader(string([]byte{0xff})), &out, &stderr, registry(t), "test"); code != 2 {
		t.Fatal("UTF8 error", out.String())
	}
	policy := filepath.Join(root, "policy.yaml")
	if err := os.WriteFile(policy, []byte("version: 2\napply: []\ntemplates:\n  a: |-\n    # Title\n    {{ paragraph[1] unknown=1 }}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"--config", policy, "--json", "-"}, strings.NewReader("Text"), &out, &stderr, registry(t), "test"); code != 2 {
		t.Fatal("template error")
	}
	var r Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || r.Diagnostics[0].Location.TemplateLine != 2 || r.Diagnostics[0].Location.Start.Line != 6 {
		t.Fatal(out.String(), err)
	}
}

func TestInstalledProviderCommand(t *testing.T) {
	caps := NewCapabilities()
	d := descriptor()
	d.Execution = "remote"
	if err := caps.RegisterProvider("native", d, func(context.Context, MeasureInput) ([]Sample, error) {
		return []Sample{{Status: "known", Value: .8, Inputs: map[string]any{}, Classification: "native"}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	policy := filepath.Join(root, "policy.yaml")
	if err := os.WriteFile(policy, []byte("version: 2\nrules: {score: {check: core.limit, options: {measure: provider.score, max: 0.4, parameters: {provider: native, measure: test.score}}}}\nsets: {a: {rules: [{rule: score, on: document}]}}\napply: [{use: [a]}]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, permit := range []bool{false, true} {
		args := []string{"--config", policy, "--root", root, "--json"}
		if permit {
			args = append(args, "--allow-provider-network", "native")
		}
		args = append(args, "-")
		var out, stderr bytes.Buffer
		code := RunWithCapabilities(context.Background(), args, strings.NewReader("A target."), &out, &stderr, registry(t), caps, "test")
		want := 2
		if permit {
			want = 1
		}
		if code != want {
			t.Fatal(code, out.String())
		}
	}
}
