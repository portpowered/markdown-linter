package rulepack

import (
	"context"
	"encoding/json"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
)

type contractAnalyzer struct {
	diagnostics []interfaces.Diagnostic
	cancel      context.CancelFunc
}

func (contractAnalyzer) ID() string { return "contract" }
func (a contractAnalyzer) Analyze(_ context.Context, p *interfaces.Pass) {
	for _, d := range a.diagnostics {
		p.Report(d)
	}
	if a.cancel != nil {
		a.cancel()
	}
}
func TestProgramSortsFindingsAndRejectsInvalidSurface(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.md")
	b := filepath.Join(root, "b.md")
	docs := []*interfaces.Document{{Path: a}, {Path: b}}
	findings := []interfaces.Diagnostic{{Path: b, Line: 1, Message: "a"}, {Path: a, Line: 2, Message: "a"}, {Path: a, Line: 1, Message: "z"}, {Path: a, Line: 1, Message: "a"}}
	r := NewRegistry()
	if err := r.Register("contract", func(yaml.Node) (interfaces.Analyzer, error) { return contractAnalyzer{diagnostics: findings}, nil }); err != nil {
		t.Fatal(err)
	}
	p, err := r.Compile(Pack{Version: 1, Rules: []Rule{{ID: "z", Check: "contract"}, {ID: "a", Check: "contract"}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run(context.Background(), root, docs)
	if err != nil || len(got) != 8 || got[0].Path != a || got[0].RuleID != "a" || got[0].Message != "a" || got[7].Path != b {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := p.Run(context.Background(), root, []*interfaces.Document{nil}); err == nil {
		t.Fatal("nil doc accepted")
	}
	if _, err := p.Run(context.Background(), root, []*interfaces.Document{{Path: filepath.Join(t.TempDir(), "outside.md")}}); err == nil {
		t.Fatal("outside doc accepted")
	}
	p.rules[0].spec.Include = []string{"b.md"}
	if _, err := p.Run(context.Background(), root, docs); err == nil {
		t.Fatal("out-of-scope finding accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	p = &Program{rules: []compiledRule{{spec: Rule{ID: "cancel"}, analyzer: contractAnalyzer{cancel: cancel}}}}
	if _, err := p.Run(ctx, root, docs); err != context.Canceled {
		t.Fatalf("cancelled=%v", err)
	}
}
func TestBaselineMissingSourcesAndInfoSARIF(t *testing.T) {
	root := t.TempDir()
	missing := interfaces.Diagnostic{Path: filepath.Join(root, "absent.md"), RuleID: "rule", Severity: interfaces.SeverityInfo}
	if err := WriteBaseline(filepath.Join(root, "baseline.json"), root, []interfaces.Diagnostic{missing}, false); err == nil {
		t.Fatal("missing evidence accepted")
	}
	baseline := filepath.Join(root, "empty.json")
	if err := os.WriteFile(baseline, []byte(`{"version":1,"findings":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ApplyBaseline(baseline, root, []interfaces.Diagnostic{missing}); err == nil {
		t.Fatal("missing evidence applied")
	}
	outside := missing
	outside.Path = filepath.Join(t.TempDir(), "outside.md")
	if _, err := fingerprint(root, outside); err == nil {
		t.Fatal("outside evidence accepted")
	}
	sarif, err := json.Marshal(SARIF([]interfaces.Diagnostic{missing}))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(sarif, &parsed); err != nil {
		t.Fatal(err)
	}
	results := parsed["runs"].([]any)[0].(map[string]any)["results"].([]any)
	if results[0].(map[string]any)["level"] != "note" {
		t.Fatalf("%s", sarif)
	}
}
