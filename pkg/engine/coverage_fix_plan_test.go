package engine

import (
	"context"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixedDiagnostic(path, id, title string, start, end int, replacement string) Diagnostic {
	d := interfaces.NewDiagnostic(path, 1, start, end, id, "replace text", interfaces.SeverityError)
	d.SuggestedFixes = []SuggestedFix{{Title: title, Confidence: FixConfidenceSafe, Edits: []TextEdit{{StartOffset: start, EndOffset: end, Replacement: replacement}}}}
	return d
}

func TestFixPlannerRejectsInvalidAndConflictingEdits(t *testing.T) {
	cases := []struct {
		name   string
		edits  []TextEdit
		source bool
		reason FixRejectionReason
	}{
		{"missing source", []TextEdit{{StartOffset: 0, EndOffset: 1}}, false, FixRejectionInvalidRange},
		{"empty edits", nil, true, FixRejectionInvalidRange},
		{"negative", []TextEdit{{StartOffset: -1, EndOffset: 1}}, true, FixRejectionInvalidRange},
		{"reversed", []TextEdit{{StartOffset: 2, EndOffset: 1}}, true, FixRejectionInvalidRange},
		{"past end", []TextEdit{{StartOffset: 1, EndOffset: 10}}, true, FixRejectionInvalidRange},
		{"self overlap", []TextEdit{{StartOffset: 2, EndOffset: 4}, {StartOffset: 0, EndOffset: 3}}, true, FixRejectionOverlappingEdit},
		{"equal insertions", []TextEdit{{StartOffset: 1, EndOffset: 1}, {StartOffset: 1, EndOffset: 1}}, true, FixRejectionOverlappingEdit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := fixedDiagnostic("file.md", "test", "fix", 0, 1, "x")
			d.SuggestedFixes[0].Edits = tc.edits
			sources := map[string][]byte{}
			if tc.source {
				sources[d.Path] = []byte("hello")
			}
			plan := PlanFixes([]Diagnostic{d}, sources)
			if len(plan.Accepted) != 0 || len(plan.Rejected) != 1 || plan.Rejected[0].Reason != tc.reason {
				t.Fatalf("%+v", plan)
			}
		})
	}
	d := fixedDiagnostic("file.md", "test", "fix", 0, 1, "x")
	d.SuggestedFixes[0].Confidence = FixConfidence("review")
	if p := PlanFixes([]Diagnostic{d}, map[string][]byte{d.Path: []byte("hello")}); len(p.Rejected) != 1 || p.Rejected[0].Reason != FixRejectionNoSafeFix {
		t.Fatalf("%+v", p)
	}
	if p := PlanFixes([]Diagnostic{d}, map[string][]byte{d.Path: []byte("hello")}, WithFixConfidence(FixConfidence("review"))); len(p.Accepted) != 1 {
		t.Fatalf("%+v", p)
	}
}

func TestFixPlannerDeterministicOrderingAndMultipleEdits(t *testing.T) {
	first := fixedDiagnostic("a.md", "a", "a", 1, 2, "B")
	first.SuggestedFixes[0].Edits = append(first.SuggestedFixes[0].Edits, TextEdit{StartOffset: 4, EndOffset: 5, Replacement: "E"})
	second := fixedDiagnostic("b.md", "b", "b", 0, 1, "A")
	conflicting := fixedDiagnostic("a.md", "z", "z", 1, 5, "different")
	diagnostics := []Diagnostic{second, conflicting, first}
	plan := PlanFixes(diagnostics, map[string][]byte{"a.md": []byte("abcde"), "b.md": []byte("abcde")})
	if len(plan.Accepted) != 3 || len(plan.Rejected) != 1 || plan.Accepted[0].RuleID != "a" || plan.Files[0].Path != "a.md" {
		t.Fatalf("%+v", plan)
	}
	if got := string(applyPlannedEdits([]byte("abcde"), plan.Files[0].Edits)); got != "aBcdE" {
		t.Fatal(got)
	}
	empty := fixCandidate{diagnostic: Diagnostic{StartOffset: 7, EndOffset: 9}}
	if start, end := candidateRange(empty); start != 7 || end != 9 {
		t.Fatalf("%d %d", start, end)
	}
	reordered := fixCandidate{edits: []TextEdit{{StartOffset: 4, EndOffset: 5}, {StartOffset: 1, EndOffset: 2}}}
	if start, end := candidateRange(reordered); start != 1 || end != 5 {
		t.Fatalf("%d %d", start, end)
	}
}

func TestFixExecutionErrorsCancellationAndNoChange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "guide.md")
	if err := os.WriteFile(path, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	d := fixedDiagnostic(path, "fix", "same", 0, 1, "h")
	if p, err := ApplyFixes(context.Background(), []Diagnostic{d}); err != nil || !p.HasAcceptedEdits() {
		t.Fatalf("%+v %v", p, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, apply := range []bool{false, true} {
		var err error
		if apply {
			_, err = ApplyFixes(cancelled, []Diagnostic{d})
		} else {
			_, err = DryRunFixes(cancelled, []Diagnostic{d})
		}
		if err != context.Canceled {
			t.Fatalf("apply=%v err=%v", apply, err)
		}
	}
	missing := fixedDiagnostic(filepath.Join(root, "absent.md"), "fix", "replace", 0, 1, "x")
	if _, err := DryRunFixes(context.Background(), []Diagnostic{missing}); err == nil || !strings.Contains(err.Error(), "reading") {
		t.Fatalf("err=%v", err)
	}
	if _, err := fileMode(missing.Path); err == nil {
		t.Fatal("missing file mode accepted")
	}
	if _, err := New().RunFileDiagnostics(context.Background(), missing.Path); err == nil {
		t.Fatal("missing diagnostic input accepted")
	}
	if _, err := New().RunFilesDiagnostics(context.Background(), []string{path, missing.Path}); err == nil {
		t.Fatal("partly missing input accepted")
	}
}

func TestFixReviewSortsUnresolvedDiagnostics(t *testing.T) {
	diagnostics := []Diagnostic{{Path: "b.md", Line: 1, RuleID: "a", Message: "one"}, {Path: "a.md", Line: 2, RuleID: "a", Message: "one"}, {Path: "a.md", Line: 1, RuleID: "z", Message: "one"}, {Path: "a.md", Line: 1, RuleID: "a", Message: "z"}, {Path: "a.md", Line: 1, RuleID: "a", Message: "a"}}
	report := NewFixReviewReport(FixPlan{}, diagnostics)
	if len(report.OtherNonFixable) != 5 || report.OtherNonFixable[0].Message != "a" || report.OtherNonFixable[4].Path != "b.md" {
		t.Fatalf("%+v", report)
	}
	withFix := fixedDiagnostic("c.md", "c", "manual", 0, 1, "x")
	report = NewFixReviewReport(FixPlan{}, []Diagnostic{withFix})
	if len(report.OtherNonFixable) != 0 {
		t.Fatalf("unclassified fix: %+v", report)
	}
}
