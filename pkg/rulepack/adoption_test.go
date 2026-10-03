package rulepack

import (
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaselineCountsNewDebtAndSurvivesMovedLines(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "input.md")
	path := filepath.Join(root, "baseline.json")
	if e := os.WriteFile(file, []byte("Bad evidence\n"), 0600); e != nil {
		t.Fatal(e)
	}
	finding := interfaces.Diagnostic{Path: file, Line: 1, RuleID: "customer.rule", CheckID: "customer.check", Message: "Fix evidence", Severity: interfaces.SeverityError}
	if e := WriteBaseline(path, root, []interfaces.Diagnostic{finding}, false); e != nil {
		t.Fatal(e)
	}
	if e := WriteBaseline(path, root, nil, false); e == nil {
		t.Fatal("overwrote baseline without explicit permission")
	}
	remaining, known, e := ApplyBaseline(path, root, []interfaces.Diagnostic{finding, finding})
	if e != nil || known != 1 || len(remaining) != 1 {
		t.Fatalf("%#v %d %v", remaining, known, e)
	}
	os.WriteFile(file, []byte("New preface\nBad evidence\n"), 0600)
	finding.Line = 2
	remaining, known, e = ApplyBaseline(path, root, []interfaces.Diagnostic{finding})
	if e != nil || known != 1 || len(remaining) != 0 {
		t.Fatalf("moved finding became new debt: %#v %d %v", remaining, known, e)
	}
	os.WriteFile(file, []byte("New preface\nDifferent evidence\n"), 0600)
	remaining, known, e = ApplyBaseline(path, root, []interfaces.Diagnostic{finding})
	if e != nil || known != 0 || len(remaining) != 1 {
		t.Fatalf("changed evidence was hidden: %#v %d %v", remaining, known, e)
	}
}
func FuzzDecodePack(f *testing.F) {
	for _, seed := range []string{"version: 1\nrules: []", "version: 1\noverrides: [{id: base, options: {one: true}}]", "---\n---"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) { _, _ = Decode(strings.NewReader(source)) })
}
