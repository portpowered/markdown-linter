package rulepack

import (
	"context"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"gopkg.in/yaml.v3"
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
