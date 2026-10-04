package contract

import (
	"bytes"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}
type Location struct {
	Path         string   `json:"path"`
	StartOffset  int      `json:"startOffset"`
	EndOffset    int      `json:"endOffset"`
	Start        Position `json:"start"`
	End          Position `json:"end"`
	Pointer      string   `json:"pointer,omitempty"`
	TemplateLine int      `json:"templateLine,omitempty"`
}
type Related struct {
	Role     string    `json:"role"`
	Location *Location `json:"location"`
	Message  string    `json:"message"`
}
type Measurement struct {
	Name   string         `json:"name"`
	Value  float64        `json:"value"`
	Unit   string         `json:"unit"`
	Min    any            `json:"min,omitempty"`
	Max    any            `json:"max,omitempty"`
	Inputs map[string]any `json:"inputs"`
	Sample string         `json:"sample"`
}
type Diagnostic struct {
	Code         string         `json:"code"`
	RuleID       string         `json:"ruleId"`
	CheckID      string         `json:"checkId"`
	Severity     string         `json:"severity"`
	Result       string         `json:"result"`
	Message      string         `json:"message"`
	Target       string         `json:"target"`
	Attribution  string         `json:"attribution"`
	Location     *Location      `json:"location"`
	Related      []Related      `json:"related"`
	Expected     any            `json:"expected"`
	Actual       any            `json:"actual"`
	Measurements []Measurement  `json:"measurements"`
	Provenance   map[string]any `json:"provenance"`
	Remediation  string         `json:"remediation"`
}
type Input struct {
	Path       string   `json:"path"`
	Source     string   `json:"source"`
	Format     string   `json:"format"`
	Language   string   `json:"language"`
	Profile    string   `json:"profile"`
	Sets       []string `json:"sets"`
	Templates  []string `json:"templates"`
	SyntaxOnly bool     `json:"syntaxOnly"`
}
type Summary struct {
	Files                 int `json:"files"`
	EvaluatedTargets      int `json:"evaluatedTargets"`
	FailedTargets         int `json:"failedTargets"`
	UnknownTargets        int `json:"unknownTargets"`
	ZeroSelectionBindings int `json:"zeroSelectionBindings"`
	Suppressed            int `json:"suppressed"`
	Errors                int `json:"errors"`
	Warnings              int `json:"warnings"`
	Info                  int `json:"info"`
}
type Report struct {
	SchemaVersion int               `json:"schemaVersion"`
	Tool          map[string]string `json:"tool"`
	Status        string            `json:"status"`
	FailOn        string            `json:"failOn"`
	Complete      bool              `json:"complete"`
	ExitCode      int               `json:"exitCode"`
	Inputs        []Input           `json:"inputs"`
	Summary       Summary           `json:"summary"`
	Diagnostics   []Diagnostic      `json:"diagnostics"`
}

func NewReport(version, failOn string) *Report {
	return &Report{SchemaVersion: 2, Tool: map[string]string{"name": "marklint", "version": version}, Status: "pass", FailOn: failOn, Complete: true, Inputs: []Input{}, Diagnostics: []Diagnostic{}}
}
func location(path string, source []byte, a, b int) *Location {
	if a < 0 {
		a = 0
	}
	if a > len(source) {
		a = len(source)
	}
	if b < a {
		b = a
	}
	if b > len(source) {
		b = len(source)
	}
	pos := func(off int) Position {
		line := bytes.Count(source[:off], []byte("\n")) + 1
		start := bytes.LastIndexByte(source[:off], '\n') + 1
		return Position{Line: line, Column: utf8.RuneCount(source[start:off]) + 1}
	}
	return &Location{Path: path, StartOffset: a, EndOffset: b, Start: pos(a), End: pos(b)}
}
func (p *Program) origin(pointer string, templateLine int) *Location {
	node := &p.tree
	if node.Kind != 0 && len(node.Content) > 0 {
		node = node.Content[0]
	}
	for _, part := range splitPointer(pointer) {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		found := false
		if node.Kind == 2 {
			if i, err := strconv.Atoi(part); err == nil && i >= 0 && i < len(node.Content) {
				node = node.Content[i]
				found = true
			}
		}
		if node.Kind == 4 {
			for i := 0; i < len(node.Content); i += 2 {
				if node.Content[i].Value == part {
					node = node.Content[i+1]
					found = true
					break
				}
			}
		}
		if !found {
			break
		}
	}
	line, col := node.Line, node.Column
	if line < 1 {
		line = 1
	}
	if col < 1 {
		col = 1
	}
	if templateLine > 0 && node.Style&8 != 0 {
		line += templateLine
		col = 1
	}
	off := 0
	for i := 1; i < line && off < len(p.Source); i++ {
		end := bytes.IndexByte(p.Source[off:], '\n')
		if end < 0 {
			off = len(p.Source)
			break
		}
		off += end + 1
	}
	if templateLine > 0 && node.Style&8 != 0 {
		for off < len(p.Source) && (p.Source[off] == ' ' || p.Source[off] == '\t') {
			off++
		}
	}
	off += col - 1
	if off > len(p.Source) {
		off = len(p.Source)
	}
	loc := location(p.Path, p.Source, off, off)
	loc.Pointer = pointer
	loc.TemplateLine = templateLine
	return loc
}
func splitPointer(s string) []string {
	out := []string{}
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '/' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	return out
}
func (r *Report) Error(code, message string, loc *Location) {
	r.Diagnostics = append(r.Diagnostics, Diagnostic{Code: code, RuleID: code, CheckID: code, Severity: "error", Result: "error", Message: message, Target: "document", Attribution: "region", Location: loc, Related: []Related{}, Measurements: []Measurement{}, Provenance: map[string]any{}, Remediation: "Resolve the reported error and rerun lint."})
	r.Complete = false
}
func (r *Report) Finish() {
	sort.SliceStable(r.Diagnostics, func(i, j int) bool {
		a, b := r.Diagnostics[i], r.Diagnostics[j]
		pa, pb := "", ""
		oa, ob := -1, -1
		if a.Location != nil {
			pa = a.Location.Path
			oa = a.Location.StartOffset
		}
		if b.Location != nil {
			pb = b.Location.Path
			ob = b.Location.StartOffset
		}
		if pa != pb {
			return pa < pb
		}
		if oa != ob {
			return oa < ob
		}
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		return a.Code < b.Code
	})
	r.Summary.Files = len(r.Inputs)
	r.Summary.Errors, r.Summary.Warnings, r.Summary.Info = 0, 0, 0
	r.ExitCode = 0
	r.Status = "pass"
	for _, d := range r.Diagnostics {
		switch d.Severity {
		case "error":
			r.Summary.Errors++
		case "warning":
			r.Summary.Warnings++
		case "info":
			r.Summary.Info++
		}
		if d.Result == "error" {
			r.ExitCode = 2
			r.Status = "error"
			r.Complete = false
		} else if r.ExitCode != 2 && (d.Severity == "error" || r.FailOn == "warning" && d.Severity == "warning" || r.FailOn == "info") {
			r.ExitCode = 1
			r.Status = "fail"
		}
	}
}
