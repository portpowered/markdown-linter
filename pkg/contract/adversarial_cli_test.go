package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCLIRejectsIgnoredArguments(t *testing.T) {
	for _, args := range [][]string{
		{"config", "schema", "unexpected"}, {"rules", "list", "unexpected"},
		{"config", "validate", "unexpected"}, {"config", "validate", "--fix"},
		{"sets", "list", "--stdin-filepath", "guide.md"}, {"config", "format", "--kind", "markdown"},
		{"--json", "--format", "text", "-"}, {"config", "format", "--json"},
		{"config", "export", "--json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, stderr bytes.Buffer
			if code := Run(context.Background(), args, strings.NewReader("text"), &out, &stderr, registry(t), "test"); code != 2 {
				t.Fatalf("ignored arguments: %d %s", code, out.String())
			}
		})
	}
}

func TestCLIManagementJSONAndHelp(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "policy.yaml")
	if err := os.WriteFile(config, []byte("version: 2\napply: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"config", "validate", "--config", config, "--json"}, strings.NewReader(""), &out, &stderr, registry(t), "test"); code != 0 {
		t.Fatal(code, stderr.String())
	}
	var report Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.SchemaVersion != 2 || !report.Complete {
		t.Fatal(out.String(), err)
	}
	for _, args := range [][]string{{"--help"}, {"config", "validate", "--help"}} {
		out.Reset()
		stderr.Reset()
		if code := Run(context.Background(), args, strings.NewReader(""), &out, &stderr, registry(t), "test"); code != 0 || !strings.Contains(out.String(), "Usage:") || stderr.Len() != 0 {
			t.Fatal(code, out.String(), stderr.String())
		}
	}
	for _, kind := range []string{"markdown", "mdx"} {
		out.Reset()
		stderr.Reset()
		if code := Run(context.Background(), []string{"rules", "list", "--kind", kind}, strings.NewReader(""), &out, &stderr, registry(t), "test"); code != 0 {
			t.Fatal(code, stderr.String())
		}
		var descriptors []map[string]any
		if err := json.Unmarshal(out.Bytes(), &descriptors); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, d := range descriptors {
			if d["id"] == "core.limit" {
				found = true
			}
		}
		if !found {
			t.Fatal("core checks missing from filtered catalog")
		}
	}
	if runtime.GOOS == "windows" {
		if err := os.WriteFile(filepath.Join(root, "simple.md"), []byte("text"), 0600); err != nil {
			t.Fatal(err)
		}
		out.Reset()
		stderr.Reset()
		args := []string{"--config", config, "--root", root, "--stdin-filepath", "SIMPLE.md", "--json", filepath.Join(root, "simple.md"), "-"}
		if code := Run(context.Background(), args, strings.NewReader("text"), &out, &stderr, registry(t), "test"); code != 2 {
			t.Fatal("Windows casing collision", code, out.String())
		}
	}
}

func TestCLIErrorReportsHonorSchemaAndParsedOutputMode(t *testing.T) {
	for _, args := range [][]string{{"--json", "--fail-on", "banana"}, {"--fail-on=", "--unknown", "--json"}} {
		var out, stderr bytes.Buffer
		if code := Run(context.Background(), args, strings.NewReader(""), &out, &stderr, registry(t), "test"); code != 2 {
			t.Fatal(code)
		}
		var report Report
		if err := json.Unmarshal(out.Bytes(), &report); err != nil {
			t.Fatal(err, out.String())
		}
		if err := validateJSON(ReportSchema(), report); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"config", "schema", "--format", "sarif"}, {"rules", "list", "--format", "sarif"}, {"config", "explain", "--format", "sarif", "--path", "simple.md"}} {
		var out, stderr bytes.Buffer
		if code := Run(context.Background(), args, strings.NewReader(""), &out, &stderr, registry(t), "test"); code != 2 {
			t.Fatal(code, out.String())
		}
		if !strings.Contains(out.String(), `"version":"2.1.0"`) {
			t.Fatal("SARIF error envelope", out.String())
		}
	}
	root := t.TempDir()
	config := filepath.Join(root, "policy.yaml")
	if err := os.WriteFile(config, []byte("version: 2\napply: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	args := []string{"--config", config, "--root", root, "--json", "--json=false", "-"}
	if code := Run(context.Background(), args, strings.NewReader("Text."), &out, &stderr, registry(t), "test"); code != 0 || out.Len() != 0 {
		t.Fatal("last boolean value ignored", code, out.String())
	}
	for _, args := range [][]string{{"--format=json", "--", "--format=sarif"}, {"--unknown", "--format=json", "--", "--format=sarif"}} {
		out.Reset()
		stderr.Reset()
		if code := Run(context.Background(), args, strings.NewReader(""), &out, &stderr, registry(t), "test"); code != 2 {
			t.Fatal(code)
		}
		var report Report
		if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.SchemaVersion != 2 {
			t.Fatal("operand changed output format", out.String(), err)
		}
	}
}

func TestAdversarialMatchingOperands(t *testing.T) {
	for _, value := range []string{"foo!", "foo bar", "..."} {
		source := "version: 2\nrules: {a: {check: core.match, options: {mode: word, values: [" + strconvQuote(value) + "]}}}\napply: []\n"
		if _, err := Decode([]byte(source), "policy.yaml", registry(t)); err == nil {
			t.Fatal("non-token operand accepted", value)
		}
	}
	p := one(t, "markdown.table-schema", `{header: ["A|B", C]}`, "each table")
	if report := lint(t, p, "| A | B\\|C |\n| --- | --- |\n| x | y |\n"); report.ExitCode != 1 {
		t.Fatal("ambiguous delimiter comparison", report.Diagnostics)
	}
}

func strconvQuote(s string) string { data, _ := json.Marshal(s); return string(data) }

func TestDiscoverCanonicalizesRootAncestors(t *testing.T) {
	parent := t.TempDir()
	real := filepath.Join(parent, "real")
	if err := os.MkdirAll(filepath.Join(real, "docs", "excluded"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"docs/guide.md", "docs/excluded/hidden.md"} {
		if err := os.WriteFile(filepath.Join(real, file), []byte("Text."), 0600); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Skip("directory symlinks unavailable", err)
	}
	root := filepath.Join(alias, "docs")
	for _, inputs := range [][]string{{filepath.Join(root, "guide.md")}, {root}} {
		files, err := Discover(root, inputs, []string{"excluded/**"})
		if err != nil || len(files) != 1 {
			t.Fatal(files, err)
		}
		resolved, err := filepath.EvalSymlinks(filepath.Join(real, "docs", "guide.md"))
		if err != nil || files[0] != resolved {
			t.Fatal(files, resolved, err)
		}
	}
	if _, err := Discover(filepath.Join(parent, "missing"), []string{root}, nil); err == nil {
		t.Fatal("missing root accepted")
	}
}
