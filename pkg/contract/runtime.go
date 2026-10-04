package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type runtimeNode struct {
	Type       string        `json:"type"`
	Value      string        `json:"value"`
	Name       string        `json:"name"`
	Lang       string        `json:"lang"`
	Depth      int           `json:"depth"`
	Attributes int           `json:"attributes"`
	Start      int           `json:"start"`
	End        int           `json:"end"`
	Children   []runtimeNode `json:"children"`
}
type parserResult struct {
	OK     bool        `json:"ok"`
	Error  string      `json:"error"`
	Line   int         `json:"line"`
	Column int         `json:"column"`
	Tree   runtimeNode `json:"tree"`
}

func runtimePath() (string, error) {
	if directory := os.Getenv("MARKLINT_RUNTIME"); directory != "" {
		return filepath.Join(directory, "analyze.mjs"), nil
	}
	exe, err := os.Executable()
	if err == nil {
		p := filepath.Join(filepath.Dir(exe), "runtime", "analyze.mjs")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	} // Source checkout is an explicit development installation.
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		p := filepath.Join(dir, "runtime", "analyze.mjs")
		if _, err := os.Stat(p); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return p, nil
			}
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return "", fmt.Errorf("analysis runtime unavailable; install runtime dependencies or set MARKLINT_RUNTIME")
}
func parseRuntime(ctx context.Context, mode, source string) (parserResult, error) {
	p, err := runtimePath()
	if err != nil {
		return parserResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	payload, err := json.Marshal(map[string]string{"mode": mode, "source": source})
	if err != nil {
		return parserResult{}, err
	}
	cmd := exec.CommandContext(ctx, "node", p)
	cmd.Stdin = bytes.NewReader(payload)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return parserResult{}, fmt.Errorf("analysis runtime: %w: %s", err, stderr.String())
	}
	var result parserResult
	if err := json.Unmarshal(out, &result); err != nil {
		return result, fmt.Errorf("invalid parser result: %w", err)
	}
	return result, nil
}
func validateMermaid(ctx context.Context, source string) error {
	result, err := parseRuntime(ctx, "mermaid", source)
	if err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("mermaid syntax at payload line %d: %s", result.Line, result.Error)
	}
	return nil
}

// ParseMDX consumes a neutral mdast projection, without fabricating a Goldmark tree.
func ParseMDX(ctx context.Context, path string, source []byte, sourceKind string, components map[string]string) (*Document, error) {
	result, err := parseRuntime(ctx, "mdx", string(source))
	if err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("MDX parse at %d:%d: %s", result.Line, result.Column, result.Error)
	}
	d := &Document{Path: path, Source: source, SourceKind: sourceKind, Format: "mdx", Root: &Target{Kind: "document", End: len(source)}}
	d.Targets = []*Target{d.Root}
	var inline func(runtimeNode, bool) Unit
	inline = func(n runtimeNode, cell bool) Unit {
		u := Unit{Start: n.Start, End: n.End}
		switch n.Type {
		case "text":
			u = projectText(string(source[n.Start:n.End]), n.Start)
			if u.Text != n.Value {
				u.Text = n.Value
				u.Offsets = nil
				u.EndOffsets = nil
				for range []byte(n.Value) {
					u.Offsets = append(u.Offsets, n.Start)
					u.EndOffsets = append(u.EndOffsets, n.End)
				}
			}
			return u
		case "inlineCode":
			if cell {
				u.Text = n.Value
				raw := string(source[n.Start:n.End])
				delimiter := len(raw) - len(strings.TrimLeft(raw, "`"))
				start := n.Start + delimiter
				payload := strings.ReplaceAll(raw[delimiter:len(raw)-delimiter], "\n", " ")
				if strings.HasPrefix(payload, " ") && strings.HasSuffix(payload, " ") && strings.TrimSpace(payload) != "" {
					payload = payload[1 : len(payload)-1]
					start++
				}
				for i := range []byte(n.Value) {
					a, b := start+i, start+i+1
					if payload != n.Value {
						a, b = n.Start, n.End
					}
					u.Offsets = append(u.Offsets, a)
					u.EndOffsets = append(u.EndOffsets, b)
				}
			} else {
				u.Text = " "
				u.Offsets = []int{n.Start}
				u.EndOffsets = []int{n.End}
			}
			return u
		case "image", "mdxTextExpression":
			u.Text = " "
			u.Offsets = []int{n.Start}
			u.EndOffsets = []int{n.End}
			return u
		case "mdxJsxTextElement":
			if components[n.Name] != "inline" {
				u.Text = " "
				u.Offsets = []int{n.Start}
				u.EndOffsets = []int{n.End}
				return u
			}
		}
		for _, c := range n.Children {
			part := inline(c, cell)
			u.Text += part.Text
			u.Offsets = append(u.Offsets, part.Offsets...)
			u.EndOffsets = append(u.EndOffsets, part.EndOffsets...)
		}
		return u
	}
	var opaque func(runtimeNode) bool
	opaque = func(n runtimeNode) bool {
		if strings.Contains(n.Type, "Expression") {
			return true
		}
		if n.Type == "mdxJsxTextElement" && components[n.Name] != "inline" || n.Type == "mdxJsxFlowElement" && components[n.Name] != "block" {
			return true
		}
		for _, c := range n.Children {
			if opaque(c) {
				return true
			}
		}
		return false
	}
	var visit func(runtimeNode, *Target)
	visit = func(n runtimeNode, parent *Target) {
		if n.Type == "mdxjsEsm" {
			return
		}
		kind := map[string]string{"heading": "heading", "paragraph": "paragraph", "code": "codeblock", "table": "table", "list": "list", "blockquote": "blockquote", "thematicBreak": "thematic-break", "html": "html", "mdxFlowExpression": "opaque", "mdxJsxFlowElement": "component"}[n.Type]
		if kind == "" {
			for _, c := range n.Children {
				visit(c, parent)
			}
			return
		}
		t := &Target{Kind: kind, Start: n.Start, End: n.End, Level: n.Depth, Parent: parent, Unknown: opaque(n)}
		parent.Children = append(parent.Children, t)
		d.Targets = append(d.Targets, t)
		if t.Unknown {
			d.Root.Unknown = true
		}
		switch kind {
		case "heading", "paragraph":
			t.Units = []Unit{inline(n, false)}
			if kind == "paragraph" {
				u := t.Units[0]
				for _, span := range sentenceSpans(u.Text) {
					s := &Target{Kind: "sentence", Parent: t, Unknown: t.Unknown, Start: t.Start, End: t.End, Units: []Unit{{Text: u.Text[span[0]:span[1]], Offsets: u.Offsets[span[0]:span[1]], EndOffsets: u.EndOffsets[span[0]:span[1]]}}}
					if len(s.Units[0].Offsets) > 0 {
						s.Start = s.Units[0].Offsets[0]
						s.End = s.Units[0].EndOffsets[len(s.Units[0].EndOffsets)-1]
					}
					t.Children = append(t.Children, s)
					d.Targets = append(d.Targets, s)
				}
				lineUnits := map[int]*Unit{}
				for i, offset := range u.Offsets {
					line := bytes.Count(source[:min(offset, len(source))], []byte("\n")) + 1
					lu := lineUnits[line]
					if lu == nil {
						lu = &Unit{Start: offset}
						lineUnits[line] = lu
					}
					lu.Text += u.Text[i : i+1]
					lu.Offsets = append(lu.Offsets, offset)
					lu.EndOffsets = append(lu.EndOffsets, u.EndOffsets[i])
					lu.End = u.EndOffsets[i]
				}
				for _, line := range sortedIntKeys(lineUnits) {
					u := *lineUnits[line]
					sl := &Target{Kind: "source-line", Parent: t, Start: u.Start, End: u.End, Unknown: t.Unknown, Units: []Unit{u}}
					t.Children = append(t.Children, sl)
					d.Targets = append(d.Targets, sl)
				}
			}
		case "codeblock":
			t.Code = strings.ReplaceAll(n.Value, "\r\n", "\n")
			t.Language = n.Lang
			opening := string(source[n.Start:lineEnd(source, n.Start+1)])
			t.CodeStart = n.Start
			if strings.HasPrefix(opening, "```") || strings.HasPrefix(opening, "~~~") {
				t.CodeStart = lineEnd(source, n.Start+1)
			}
			t.CodeUnit = mdxCodeUnit(source, t.CodeStart, n.End, t.Code)
			if n.Lang != "" {
				languageStart := n.Start + strings.Index(opening, n.Lang)
				t.LanguageUnit = literalUnit(n.Lang, languageStart)
			}
		case "table":
			for i, row := range n.Children {
				if i > 0 && (len(row.Children) > len(t.Header) || countTableCells(string(source[lineStart(source, row.Start):min(lineEnd(source, row.End), len(source))])) > len(t.Header)) {
					t.ExtraCells = true
				}
				cells := []*Target{}
				for _, cell := range row.Children {
					cells = append(cells, &Target{Kind: "cell", Parent: t, Start: cell.Start, End: cell.End, Unknown: opaque(cell), Units: []Unit{inline(cell, true)}})
				}
				if i == 0 {
					t.Header = cells
				} else {
					t.Rows = append(t.Rows, cells)
				}
			}
		case "component":
			if components[n.Name] == "block" {
				for _, c := range n.Children {
					visit(c, t)
				}
			}
		default:
			for _, c := range n.Children {
				visit(c, t)
			}
		}
	}
	for _, n := range result.Tree.Children {
		visit(n, d.Root)
	}
	buildMDXSections(d)
	return d, nil
}
func buildMDXSections(d *Document) {
	hs := d.selectTargets(d.Root, "heading", "subtree")
	stack := []*Target{}
	for _, h := range hs {
		for len(stack) > 0 && stack[len(stack)-1].Level >= h.Level {
			stack[len(stack)-1].End = h.Start
			stack = stack[:len(stack)-1]
		}
		s := &Target{Kind: "section", Start: lineEnd(d.Source, h.End), End: len(d.Source), Level: h.Level, Heading: h, Parent: d.Root}
		if len(stack) > 0 {
			s.Parent = stack[len(stack)-1]
		}
		stack = append(stack, s)
		d.Targets = append(d.Targets, s)
	}
	for _, s := range d.Targets {
		if s.Kind != "section" {
			continue
		}
		for _, t := range d.Targets {
			if t != s && t.Unknown && t.Start >= s.Start && t.Start < s.End {
				s.Unknown = true
				break
			}
		}
	}
}
