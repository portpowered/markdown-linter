package cli

import (
	"bytes"
	"context"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/portpowered/markdown-linter/pkg/rulepack"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type shortOutput struct{}

func (shortOutput) Write(p []byte) (int, error) { return len(p) / 2, nil }
func TestReportWriterPreservesFirstFailure(t *testing.T) {
	w := reportWriter{Writer: shortOutput{}}
	if _, err := w.Write([]byte("output")); err != io.ErrShortWrite {
		t.Fatal(err)
	}
	if n, err := w.Write([]byte("again")); n != 0 || err != io.ErrShortWrite {
		t.Fatalf("%d %v", n, err)
	}
}
func TestFixOutputAndBoundaryFailures(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "guide.md")
	if err := os.WriteFile(path, []byte("text"), 0600); err != nil {
		t.Fatal(err)
	}
	d := interfaces.NewDiagnostic(path, 1, 0, 4, "fix", "replace", interfaces.SeverityError)
	d.SuggestedFixes = []interfaces.SuggestedFix{{Confidence: interfaces.FixConfidenceSafe, Edits: []interfaces.TextEdit{{StartOffset: 0, EndOffset: 4, Replacement: "new"}}}}
	for _, format := range []string{"text", "json"} {
		var stderr bytes.Buffer
		if code := renderFixes(context.Background(), commandOptions{root: root, preview: true, format: format}, []interfaces.Diagnostic{d}, unavailableWriter{}, &stderr); code != 2 {
			t.Fatalf("%s code=%d", format, code)
		}
	}
	d.Path = filepath.Join(t.TempDir(), "outside.md")
	var out, stderr bytes.Buffer
	if code := renderFixes(context.Background(), commandOptions{root: root, preview: true}, []interfaces.Diagnostic{d}, &out, &stderr); code != 2 {
		t.Fatalf("outside code=%d", code)
	}
	d.Path = filepath.Join(root, "absent.md")
	if code := renderFixes(context.Background(), commandOptions{root: root, preview: true}, []interfaces.Diagnostic{d}, &out, &stderr); code != 2 {
		t.Fatalf("missing code=%d", code)
	}
	d.Path = path
	d.SuggestedFixes = nil
	d.Severity = interfaces.SeverityWarning
	if code := renderFixes(context.Background(), commandOptions{root: root, preview: true, failOn: "warning"}, []interfaces.Diagnostic{d}, &out, &stderr); code != 1 {
		t.Fatalf("warning code=%d", code)
	}
	if code := renderFixes(context.Background(), commandOptions{root: root, preview: true}, nil, &out, &stderr); code != 0 {
		t.Fatalf("no findings code=%d", code)
	}
}
func TestMoveMappingsLoadedIntoSelectedPack(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "rules.yaml")
	source := "version: 1\nrules:\n  - id: moves\n    check: markdown.link-relocation\n    options:\n      moves: {old.md: prior.md}\n"
	if err := os.WriteFile(config, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	pack, err := loadPack(commandOptions{root: root, pack: config, moves: moveMappingFlags{"old.md": "new.md"}})
	if err != nil {
		t.Fatal(err)
	}
	var opts struct {
		Moves map[string]string `yaml:"moves"`
	}
	if err := rulepack.DecodeOptions(pack.Rules[0].Options, &opts); err != nil {
		t.Fatal(err)
	}
	if opts.Moves["old.md"] != "new.md" {
		t.Fatal(opts.Moves)
	}
	if err := os.WriteFile(config, []byte("version: 1\nrules:\n  - id: moves\n    check: markdown.link-relocation\n    options: {unknown: true}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPack(commandOptions{root: root, pack: config, moves: moveMappingFlags{"a": "b"}}); err == nil {
		t.Fatal("invalid options accepted")
	}
	mapping := filepath.Join(root, "bad.map")
	if err := os.WriteFile(mapping, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadMoveMappings(nil, mapping); err == nil {
		t.Fatal("invalid file accepted")
	}
}

type outsideAnalyzer struct{ path string }

func (outsideAnalyzer) ID() string { return "custom" }
func (a outsideAnalyzer) Analyze(_ context.Context, p *interfaces.Pass) {
	p.Report(interfaces.NewDiagnostic(a.path, 1, 0, 0, "custom", "outside", interfaces.SeverityError))
}
func TestCustomCheckEscapingScopeFailsCommand(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "guide.md")
	config := filepath.Join(root, "rules.yaml")
	if err := os.WriteFile(path, []byte("# Guide\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("version: 1\nrules: [{id: custom, check: custom}]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r := rulepack.NewRegistry()
	if err := r.Register("custom", func(yaml.Node) (interfaces.Analyzer, error) {
		return outsideAnalyzer{path: filepath.Join(t.TempDir(), "outside.md")}, nil
	}); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := RunWithRegistry(context.Background(), []string{"--root", root, "--rules", config, path}, &out, &stderr, r); code != 2 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	// Config and baseline operational failures are never converted to findings.
	for _, args := range [][]string{{"--root", root, "--rules", filepath.Join(root, "missing.yaml"), path}, {"--root", root, "--only", "missing", path}, {"--root", root, "--baseline", filepath.Join(root, "missing.json"), path}, {"--root", root, "--baseline-write", root, path}} {
		if code := Run(context.Background(), args, &out, &stderr); code != 2 {
			t.Fatalf("%v: %d", args, code)
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if code := Run(cancelled, []string{"--root", root, path}, &out, &stderr); code != 2 {
		t.Fatalf("cancelled code=%d", code)
	}
}
func TestSuccessfulCommandsReportWriterFailures(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "guide.md")
	if err := os.WriteFile(path, []byte("# Guide\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--version"}, {"--root", root, "--baseline-write", filepath.Join(root, "baseline.json"), path}, {"--root", root, "--format", "json", path}} {
		var stderr bytes.Buffer
		if code := Run(context.Background(), args, unavailableWriter{}, &stderr); code != 2 {
			t.Fatalf("%v code=%d stderr=%s", args, code, stderr.String())
		}
	}
}
