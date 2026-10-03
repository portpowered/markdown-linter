package engine

import (
	"context"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"testing"
)

type registrationRule struct{}

func (registrationRule) ID() string { return "legacy" }
func (registrationRule) Check(_ context.Context, d *Document) []Violation {
	return []Violation{interfaces.NewViolation(d.Path, 1, "legacy", "legacy finding")}
}
func TestRegistrationAndVisitorCancellation(t *testing.T) {
	source := []byte("# Title\n\nText\n")
	doc := interfaces.NewDocument("guide.md", source, goldmark.New().Parser().Parse(text.NewReader(source)))
	callback := NewVisitorRule("visitor", VisitorCallbacks{Heading: func(_ context.Context, v HeadingVisit) {
		v.Pass.Report(interfaces.NewDiagnostic(v.Document.Path, v.Range.Line, v.Range.StartOffset, v.Range.EndOffset, "visitor", "heading", interfaces.SeverityWarning))
	}})
	if callback.ID() != "visitor" {
		t.Fatal(callback.ID())
	}
	l := New()
	l.RegisterRule(registrationRule{})
	l.RegisterDiagnosticRule(callback)
	l.RegisterAnalyzer(callback)
	if got := l.RunDiagnostics(context.Background(), doc); len(got) != 3 {
		t.Fatalf("%+v", got)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	pass := NewPass([]*Document{doc})
	WalkVisitorCallbacks(cancelled, pass, doc, callback.callbacks)
	if len(pass.Diagnostics()) != 0 {
		t.Fatal(pass.Diagnostics())
	}
	empty := interfaces.NewDocument("empty.md", nil, ast.NewDocument())
	WalkVisitorCallbacks(context.Background(), NewPass(nil), empty, VisitorCallbacks{})
}
func TestFixOrderingTieBreaks(t *testing.T) {
	// A report must remain deterministic when multiple rejected suggestions share locations.
	diagnostics := []Diagnostic{fixedDiagnostic("b.md", "z", "z", 0, 9, "x"), fixedDiagnostic("a.md", "z", "z", 1, 9, "x"), fixedDiagnostic("a.md", "a", "z", 1, 9, "x"), fixedDiagnostic("a.md", "a", "a", 1, 9, "x"), fixedDiagnostic("a.md", "a", "a", 0, 9, "x")}
	p := PlanFixes(diagnostics, map[string][]byte{"a.md": []byte("a"), "b.md": []byte("b")})
	if len(p.Rejected) != 5 || p.Rejected[0].Path != "a.md" || p.Rejected[0].Diagnostic.StartOffset != 0 || p.Rejected[1].FixTitle != "a" || p.Rejected[4].Path != "b.md" {
		t.Fatalf("%+v", p.Rejected)
	}
	// Equal byte locations use check identity, then fix title for stable sorting.
	first := PlannedEdit{Path: "a", RuleID: "a", FixTitle: "a", TextEdit: TextEdit{StartOffset: 0, EndOffset: 1}}
	second := first
	second.RuleID = "b"
	if !comparePlannedEdits(first, second) {
		t.Fatal("rule tie-break")
	}
	second = first
	second.FixTitle = "b"
	if !comparePlannedEdits(first, second) {
		t.Fatal("title tie-break")
	}
	second = first
	second.TextEdit.EndOffset = 2
	if !comparePlannedEdits(first, second) {
		t.Fatal("end tie-break")
	}
}
