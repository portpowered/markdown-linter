package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/portpowered/markdown-linter/pkg/rulepack"
)

type unavailableWriter struct{}

func (unavailableWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestCommandUsageErrors(t *testing.T) {
	for _, args := range [][]string{{}, {"--invalid"}, {"--fail-on", "info"}, {"--format", "yaml"}, {"--fix", "--fix-check"}, {"--fix", "--baseline", "debt"}, {"--fix-check", "--format", "sarif"}, {"--baseline", "a", "--baseline-write", "b"}, {"baseline"}} {
		var out, stderr bytes.Buffer
		if code := Run(context.Background(), args, &out, &stderr); code != ExitOperational || stderr.Len() == 0 {
			t.Fatalf("%v: code=%d stderr=%s", args, code, stderr.String())
		}
	}
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"--version"}, &out, &stderr); code != 0 || !strings.Contains(out.String(), "marklint") {
		t.Fatalf("version: %d %s", code, out.String())
	}
}

func TestFixCommandAppliesOnlySelectedChecks(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "guide.md")
	config := filepath.Join(root, "rules.yaml")
	if err := os.WriteFile(path, []byte("# Guide\n\nText \t\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("version: 1\nrules:\n  - id: whitespace\n    check: markdown.trailing-whitespace\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"--fix-check", "--fix"} {
		var out, stderr bytes.Buffer
		code := Run(context.Background(), []string{"--root", root, "--rules", config, mode, "--fix-report-verbose", path}, &out, &stderr)
		if mode == "--fix-check" && code != 1 {
			t.Fatalf("preview: %d %s", code, stderr.String())
		}
		if mode == "--fix" && code != 0 {
			t.Fatalf("apply: %d %s", code, stderr.String())
		}
		if !strings.Contains(out.String(), "Accepted edits: 1") {
			t.Fatal(out.String())
		}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Guide\n\nText\n" {
		t.Fatalf("content=%q", content)
	}
	var stderr bytes.Buffer
	if code := Run(context.Background(), []string{"--root", root, "--rules", config, "--format", "json", path}, unavailableWriter{}, &stderr); code != 2 {
		t.Fatalf("output error=%d", code)
	}
}

func TestMoveMappingFormatsAndSelection(t *testing.T) {
	for _, input := range []string{"", `{" old\\file.md ":"new/file.md","":"ignored"}`, "# comment\nold/file.md=new/file.md\n\n"} {
		mappings, err := ParseMoveMappingFile([]byte(input))
		if err != nil {
			t.Fatal(err)
		}
		if input != "" && mappings["old/file.md"] != "new/file.md" {
			t.Fatalf("%q: %#v", input, mappings)
		}
	}
	for _, input := range []string{"{bad}", "missing equal", "old="} {
		if _, err := ParseMoveMappingFile([]byte(input)); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
	var flags moveMappingFlags
	if flags.String() != "" {
		t.Fatal(flags.String())
	}
	if err := flags.Set("bad"); err == nil {
		t.Fatal("accepted invalid flag")
	}
	if err := flags.Set("z.md=n.md"); err != nil {
		t.Fatal(err)
	}
	if err := flags.Set("a.md=b.md"); err != nil {
		t.Fatal(err)
	}
	if flags.String() != "a.md=b.md,z.md=n.md" {
		t.Fatal(flags.String())
	}
	root := t.TempDir()
	mappingPath := filepath.Join(root, "moves.json")
	if err := os.WriteFile(mappingPath, []byte(`{"a.md":"override.md"}`), 0600); err != nil {
		t.Fatal(err)
	}
	mapped, err := loadMoveMappings(flags, mappingPath)
	if err != nil || mapped["a.md"] != "override.md" {
		t.Fatalf("%#v %v", mapped, err)
	}
	if _, err := loadMoveMappings(flags, filepath.Join(root, "absent")); err == nil {
		t.Fatal("missing map accepted")
	}
	if _, err := loadPack(commandOptions{root: root, moves: flags}); err == nil {
		t.Fatal("moves accepted without relocation")
	}
	pack := rulepack.Pack{Version: 1, Rules: []rulepack.Rule{{ID: "a", Check: "markdown.heading-order"}, {ID: "b", Check: "markdown.heading-order"}}, Suppressions: []rulepack.Suppression{{Rule: "a"}, {Rule: "b"}}}
	selected, err := selectRules(pack, " a ")
	if err != nil || len(selected.Rules) != 1 || len(selected.Suppressions) != 1 {
		t.Fatalf("%+v %v", selected, err)
	}
	if _, err := selectRules(pack, "missing"); err == nil {
		t.Fatal("unknown selected rule accepted")
	}
}

func TestFixReportIncludesManualReview(t *testing.T) {
	d := interfaces.Diagnostic{Path: "guide.md", Line: 3, RuleID: "links", Message: "review target"}
	report := engine.FixReviewReport{Accepted: []engine.PlannedEdit{{Path: d.Path, RuleID: d.RuleID, Diagnostic: d, TextEdit: engine.TextEdit{Replacement: "new.md"}}}, Rejected: []engine.RejectedFix{{Path: d.Path, RuleID: d.RuleID, Diagnostic: d, Reason: engine.FixRejectionOverlappingEdit, Message: "overlaps"}}, Ambiguous: []interfaces.Diagnostic{d}, OtherNonFixable: []interfaces.Diagnostic{d}}
	for _, apply := range []bool{false, true} {
		for _, verbose := range []bool{false, true} {
			var out bytes.Buffer
			RenderFixReport(&out, report, apply, verbose)
			for _, want := range []string{"Accepted edits: 1", "Rejected edits: 1", "Ambiguous cases: 1", "Other non-fixable diagnostics: 1"} {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("missing %s: %s", want, out.String())
				}
			}
		}
	}
	var out bytes.Buffer
	RenderFixReport(&out, engine.FixReviewReport{}, false, false)
	if !strings.Contains(out.String(), "Accepted edits: none") {
		t.Fatal(out.String())
	}
}

func TestDiscoveryAndInputFailures(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "GUIDE.MD")
	if err := os.WriteFile(path, []byte("# Guide\n"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := DiscoverMarkdownFiles([]string{root, path})
	if err != nil || len(files) != 1 {
		t.Fatalf("%v %v", files, err)
	}
	if _, err := DiscoverMarkdownFiles([]string{filepath.Join(root, "absent")}); err == nil {
		t.Fatal("missing input accepted")
	}
	empty := t.TempDir()
	if _, err := parseInputs(commandOptions{root: empty, paths: []string{empty}}); err == nil {
		t.Fatal("empty input accepted")
	}
	if _, err := parseInputs(commandOptions{root: filepath.Join(root, "absent"), paths: []string{path}}); err == nil {
		t.Fatal("missing root accepted")
	}
}
