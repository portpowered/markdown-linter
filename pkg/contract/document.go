package contract

import (
	"bytes"
	"github.com/rivo/uniseg"
	"sort"
	"strings"

	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	ea "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Unit keeps projected text separate from original UTF-8 source coordinates.
type Unit struct {
	Text       string
	Offsets    []int
	EndOffsets []int
	Start, End int
}
type Target struct {
	Heading           *Target
	CodeStart         int
	CodeUnit          Unit
	LanguageUnit      Unit
	Kind              string
	Start, End, Level int
	Units             []Unit
	Language, Code    string
	Node              ast.Node
	Parent            *Target
	Children          []*Target
	Header            []*Target
	Rows              [][]*Target
	ExtraCells        bool
	Unknown           bool
}
type Document struct {
	Path, SourceKind, Format string
	Source                   []byte
	Legacy                   *interfaces.Document
	Root                     *Target
	Targets                  []*Target
	Policy                   Policy
}

type blockSourceSpan struct{ start, end int }

// Goldmark intentionally leaves Lines empty for syntax-only nodes. Capture the
// parser's actual consumed lines so their source positions remain authoritative.
type sourceBlockParser struct {
	parser.BlockParser
	listSyntax bool
}

// Goldmark's list-marker indentation probe treats a bare marker followed by
// CRLF differently from LF. A same-width whitespace view corrects that probe
// while the reader's source, segments, and all consumed offsets stay original.
type listSyntaxReader struct{ text.Reader }

func (r listSyntaxReader) PeekLine() ([]byte, text.Segment) {
	line, segment := r.Reader.PeekLine()
	if len(line) >= 2 && line[len(line)-2] == '\r' && line[len(line)-1] == '\n' {
		line = append([]byte(nil), line...)
		line[len(line)-2] = ' '
	}
	return line, segment
}

func (p sourceBlockParser) syntaxReader(reader text.Reader) text.Reader {
	if p.listSyntax {
		return listSyntaxReader{reader}
	}
	return reader
}

func (p sourceBlockParser) Open(parent ast.Node, reader text.Reader, context parser.Context) (ast.Node, parser.State) {
	_, segment := reader.PeekLine()
	node, state := p.BlockParser.Open(parent, p.syntaxReader(reader), context)
	if node != nil {
		node.SetAttributeString("marklint-source-span", &blockSourceSpan{segment.Start, segment.Stop})
	}
	return node, state
}

func (p sourceBlockParser) Continue(node ast.Node, reader text.Reader, context parser.Context) parser.State {
	_, before := reader.PeekLine()
	state := p.BlockParser.Continue(node, p.syntaxReader(reader), context)
	_, after := reader.Position()
	if state&parser.Continue != 0 || after.Start > before.Start {
		if value, ok := node.AttributeString("marklint-source-span"); ok {
			value.(*blockSourceSpan).end = before.Stop
		}
	}
	return state
}

func sourceParser() parser.Parser {
	blocks := parser.DefaultBlockParsers()
	for i, block := range blocks {
		blockParser := block.Value.(parser.BlockParser)
		listSyntax := blockParser == parser.NewListParser() || blockParser == parser.NewListItemParser()
		blocks[i] = util.Prioritized(sourceBlockParser{BlockParser: blockParser, listSyntax: listSyntax}, block.Priority)
	}
	return parser.NewParser(parser.WithBlockParsers(blocks...), parser.WithInlineParsers(parser.DefaultInlineParsers()...), parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...))
}

func literalUnit(value string, start int) Unit {
	u := Unit{Text: value, Start: start, End: start + len(value)}
	for i := range []byte(value) {
		u.Offsets = append(u.Offsets, start+i)
		u.EndOffsets = append(u.EndOffsets, start+i+1)
	}
	return u
}

// A normalized newline retains the complete original CRLF as its source range.
func normalizeCodeNewlines(u Unit) Unit {
	out := Unit{Start: u.Start, End: u.End}
	var text strings.Builder
	for i := 0; i < len(u.Text); i++ {
		end := i
		if u.Text[i] == '\r' && i+1 < len(u.Text) && u.Text[i+1] == '\n' {
			end++
		}
		text.WriteString(u.Text[end : end+1])
		out.Offsets = append(out.Offsets, u.Offsets[i])
		out.EndOffsets = append(out.EndOffsets, u.EndOffsets[end])
		i = end
	}
	out.Text = text.String()
	return out
}

// mdast removes container prefixes and code indentation. Align each decoded
// payload line with its original suffix, retaining offsets through those gaps.
func mdxCodeUnit(source []byte, start, end int, code string) Unit {
	u := Unit{Text: code, Start: start, End: end}
	position := start
	parts := strings.Split(code, "\n")
	for index, part := range parts {
		stop := min(lineEnd(source, position+1), end)
		raw := strings.TrimRight(string(source[position:stop]), "\r\n")
		a, z := position, stop
		matched := strings.HasSuffix(raw, part)
		content := strings.TrimLeft(part, " \t")
		indent := len(part) - len(content)
		expandedIndent := !matched && content != "" && strings.HasSuffix(raw, content)
		contentStart := position + len(raw) - len(content)
		if matched {
			a = position + len(raw) - len(part)
			z = a + len(part)
		}
		for i := range []byte(part) {
			if matched {
				u.Offsets = append(u.Offsets, a+i)
				u.EndOffsets = append(u.EndOffsets, a+i+1)
			} else if expandedIndent && i >= indent {
				u.Offsets = append(u.Offsets, contentStart+i-indent)
				u.EndOffsets = append(u.EndOffsets, contentStart+i-indent+1)
			} else if expandedIndent {
				prefix := raw[:len(raw)-len(content)]
				whitespaceStart := position + len(strings.TrimRight(prefix, " \t"))
				u.Offsets = append(u.Offsets, whitespaceStart)
				u.EndOffsets = append(u.EndOffsets, contentStart)
			} else {
				u.Offsets = append(u.Offsets, a)
				u.EndOffsets = append(u.EndOffsets, z)
			}
		}
		if index < len(parts)-1 {
			u.Offsets = append(u.Offsets, position+len(raw))
			u.EndOffsets = append(u.EndOffsets, stop)
		}
		position = stop
	}
	return u
}

func nodeRange(n ast.Node, source []byte) (int, int) {
	a, b := len(source), 0
	fallbackStart, fallbackEnd := len(source), 0
	_ = ast.Walk(n, func(c ast.Node, enter bool) (ast.WalkStatus, error) {
		if !enter {
			return ast.WalkContinue, nil
		}
		if value, ok := c.AttributeString("marklint-source-span"); ok {
			span := value.(*blockSourceSpan)
			fallbackStart = min(fallbackStart, span.start)
			fallbackEnd = max(fallbackEnd, span.end)
		}
		if t, ok := c.(*ast.Text); ok {
			if t.Segment.Start < a {
				a = t.Segment.Start
			}
			if t.Segment.Stop > b {
				b = t.Segment.Stop
			}
		}
		if c.Type() == ast.TypeBlock && c.Lines() != nil {
			for i := 0; i < c.Lines().Len(); i++ {
				l := c.Lines().At(i)
				if l.Start < a {
					a = l.Start
				}
				if l.Stop > b {
					b = l.Stop
				}
			}
		}
		return ast.WalkContinue, nil
	})
	if a > b {
		if fallbackStart <= fallbackEnd {
			return fallbackStart, fallbackEnd
		}
		return 0, 0
	}
	return a, b
}
func projection(n ast.Node, source []byte, cell bool) Unit {
	u := Unit{}
	var b strings.Builder
	appendText := func(s string, start int) {
		b.WriteString(s)
		for i := 0; i < len(s); i++ {
			u.Offsets = append(u.Offsets, start+i)
			u.EndOffsets = append(u.EndOffsets, start+i+1)
		}
	}
	var walk func(ast.Node)
	walk = func(c ast.Node) {
		switch t := c.(type) {
		case *ast.CodeSpan:
			if !cell {
				a, _ := nodeRange(c, source)
				appendText(" ", a)
				return
			}
		case *ast.Image, *ast.RawHTML, *ast.AutoLink:
			a, _ := nodeRange(c, source)
			appendText(" ", a)
			return
		case *ast.Text:
			s := string(t.Value(source))
			projected := projectText(s, t.Segment.Start)
			b.WriteString(projected.Text)
			u.Offsets = append(u.Offsets, projected.Offsets...)
			u.EndOffsets = append(u.EndOffsets, projected.EndOffsets...)
			if t.SoftLineBreak() || t.HardLineBreak() {
				appendText("\n", t.Segment.Stop)
			}
			return
		case *ast.String:
			appendText(string(t.Value), 0)
			return
		}
		for ch := c.FirstChild(); ch != nil; ch = ch.NextSibling() {
			walk(ch)
		}
	}
	walk(n)
	u.Text = b.String()
	u.Start, u.End = nodeRange(n, source)
	return u
}
func Parse(path string, source []byte, sourceKind string) *Document {
	md := goldmark.New(goldmark.WithParser(sourceParser()), goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.TaskList))
	parsed := md.Parser().Parse(text.NewReader(source))
	d := &Document{Path: path, Source: source, SourceKind: sourceKind, Format: "markdown", Legacy: interfaces.NewDocument(path, source, parsed)}
	root := &Target{Kind: "document", End: len(source)}
	d.Root = root
	d.Targets = []*Target{root}
	frontEnd := 0
	if bytes.HasPrefix(source, []byte("---\n")) || bytes.HasPrefix(source, []byte("---\r\n")) {
		lines := bytes.SplitAfter(source, []byte("\n"))
		pos := len(lines[0])
		for _, l := range lines[1:] {
			pos += len(l)
			if strings.TrimSpace(string(l)) == "---" || strings.TrimSpace(string(l)) == "..." {
				frontEnd = pos
				break
			}
		}
	}
	var collect func(ast.Node, *Target)
	collect = func(n ast.Node, parent *Target) {
		if n.Type() == ast.TypeInline {
			return
		}
		a, b := nodeRange(n, source)
		if b <= frontEnd && frontEnd > 0 {
			return
		}
		kind := ""
		level := 0
		switch v := n.(type) {
		case *ast.Heading:
			kind = "heading"
			level = v.Level
		case *ast.Paragraph, *ast.TextBlock:
			kind = "paragraph"
		case *ast.FencedCodeBlock:
			kind = "codeblock"
		case *ast.CodeBlock:
			kind = "codeblock"
		case *ast.List:
			kind = "list"
		case *ast.Blockquote:
			kind = "blockquote"
		case *ast.ThematicBreak:
			kind = "thematic-break"
		case *ast.HTMLBlock:
			kind = "html"
		case *ea.Table:
			kind = "table"
		}
		if kind == "" {
			for c := n.FirstChild(); c != nil; c = c.NextSibling() {
				collect(c, parent)
			}
			return
		}
		t := &Target{Kind: kind, Start: a, End: b, Level: level, Node: n, Parent: parent}
		parent.Children = append(parent.Children, t)
		d.Targets = append(d.Targets, t)
		if kind == "heading" || kind == "paragraph" {
			u := projection(n, source, false)
			t.Units = []Unit{u}
			if kind == "paragraph" {
				for _, span := range sentenceSpans(u.Text) {
					su := Unit{Text: u.Text[span[0]:span[1]], Offsets: u.Offsets[span[0]:span[1]]}
					if len(u.EndOffsets) == len(u.Text) {
						su.EndOffsets = u.EndOffsets[span[0]:span[1]]
					}
					s := &Target{Kind: "sentence", Parent: t, Units: []Unit{su}}
					if len(s.Units[0].Offsets) > 0 {
						s.Start = s.Units[0].Offsets[0]
						s.End = s.Units[0].Offsets[len(s.Units[0].Offsets)-1] + 1
					}
					t.Children = append(t.Children, s)
					d.Targets = append(d.Targets, s)
				}
				lineUnits := map[int]*Unit{}
				for i := 0; i < len(u.Text); i++ {
					if i >= len(u.Offsets) {
						break
					}
					line := d.Legacy.LineForOffset(u.Offsets[i])
					lu := lineUnits[line]
					if lu == nil {
						lu = &Unit{Start: u.Offsets[i]}
						lineUnits[line] = lu
					}
					lu.Text += u.Text[i : i+1]
					lu.Offsets = append(lu.Offsets, u.Offsets[i])
					lu.End = u.Offsets[i] + 1
				}
				for _, line := range sortedIntKeys(lineUnits) {
					lu := lineUnits[line]
					sl := &Target{Kind: "source-line", Start: lu.Start, End: lu.End, Parent: t, Units: []Unit{*lu}}
					t.Children = append(t.Children, sl)
					d.Targets = append(d.Targets, sl)
				}
			}
			return
		}
		if kind == "codeblock" {
			t.CodeStart = t.Start
			var payload strings.Builder
			for i := 0; i < n.Lines().Len(); i++ {
				segment := n.Lines().At(i)
				part := literalUnit(string(source[segment.Start:segment.Stop]), segment.Start)
				if segment.Padding > 0 {
					padding := Unit{Text: strings.Repeat(" ", segment.Padding)}
					for range segment.Padding {
						padding.Offsets = append(padding.Offsets, max(0, segment.Start-1))
						padding.EndOffsets = append(padding.EndOffsets, segment.Start)
					}
					part.Text = padding.Text + part.Text
					part.Offsets = append(padding.Offsets, part.Offsets...)
					part.EndOffsets = append(padding.EndOffsets, part.EndOffsets...)
				}
				if segment.ForceNewline && part.Text != "" && !strings.HasSuffix(part.Text, "\n") {
					part.Text += "\n"
					part.Offsets = append(part.Offsets, segment.Stop)
					part.EndOffsets = append(part.EndOffsets, segment.Stop)
				}
				part = normalizeCodeNewlines(part)
				payload.WriteString(part.Text)
				t.CodeUnit.Offsets = append(t.CodeUnit.Offsets, part.Offsets...)
				t.CodeUnit.EndOffsets = append(t.CodeUnit.EndOffsets, part.EndOffsets...)
			}
			t.CodeUnit.Text = payload.String()
			t.CodeUnit.Start, t.CodeUnit.End = a, b
			t.Code = t.CodeUnit.Text
			if c, ok := n.(*ast.FencedCodeBlock); ok {
				t.Language = string(c.Language(source))
				if value, ok := c.AttributeString("marklint-source-span"); ok {
					span := value.(*blockSourceSpan)
					t.Start, t.End = span.start, span.end
					t.CodeStart = lineEnd(source, span.start+1)
				}
				t.LanguageUnit = Unit{Start: t.Start, End: t.Start}
				if c.Info != nil {
					infoStart := c.Info.Segment.Start + strings.Index(string(c.Info.Value(source)), t.Language)
					t.LanguageUnit = literalUnit(t.Language, infoStart)
				}
			}
			return
		}
		if kind == "table" {
			rowNumber := 0
			tableStart := lineStart(source, t.Start)
			for row := n.FirstChild(); row != nil; row = row.NextSibling() {
				rowStart := tableStart
				lineNumber := rowNumber
				if rowNumber > 0 {
					lineNumber++
				}
				for i := 0; i < lineNumber; i++ {
					rowStart = lineEnd(source, rowStart+1)
				}
				rowEnd := lineEnd(source, rowStart+1)
				ranges := tableCellRanges(string(source[rowStart:rowEnd]), rowStart)
				cells := []*Target{}
				for c := row.FirstChild(); c != nil; c = c.NextSibling() {
					ca, cb := rowEnd, rowEnd
					if len(cells) < len(ranges) {
						ca, cb = ranges[len(cells)][0], ranges[len(cells)][1]
					}
					ct := &Target{Kind: "cell", Start: ca, End: cb, Parent: t, Node: c, Units: []Unit{projection(c, source, true)}}
					cells = append(cells, ct)
				}
				if _, ok := row.(*ea.TableHeader); ok {
					t.Header = cells
				} else {
					t.Rows = append(t.Rows, cells)
				}
				if len(ranges) > len(t.Header) && len(t.Header) > 0 {
					t.ExtraCells = true
				}
				rowNumber++
			}
			return
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			collect(c, t)
		}
	}
	for n := parsed.FirstChild(); n != nil; n = n.NextSibling() {
		collect(n, root)
	}
	// Sections form an independent outline view over the block tree.
	headings := []*Target{}
	for _, t := range d.Targets {
		if t.Kind == "heading" && t.Parent == root {
			headings = append(headings, t)
		}
	}
	stack := []*Target{}
	for _, h := range headings {
		for len(stack) > 0 && stack[len(stack)-1].Level >= h.Level {
			stack[len(stack)-1].End = lineStart(source, h.Start)
			stack = stack[:len(stack)-1]
		}
		s := &Target{Kind: "section", Start: lineEnd(source, h.End), End: len(source), Level: h.Level, Node: h.Node, Heading: h, Parent: root}
		if len(stack) > 0 {
			s.Parent = stack[len(stack)-1]
		}
		stack = append(stack, s)
		d.Targets = append(d.Targets, s)
	}
	return d
}
func sortedIntKeys[V any](m map[int]V) []int {
	ks := []int{}
	for k := range m {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	return ks
}
func lineStart(s []byte, offset int) int {
	if offset > len(s) {
		offset = len(s)
	}
	i := bytes.LastIndexByte(s[:offset], '\n')
	return i + 1
}
func lineEnd(s []byte, offset int) int {
	if offset > len(s) {
		return len(s)
	}
	if offset > 0 && s[offset-1] == '\n' {
		return offset
	}
	if i := bytes.IndexByte(s[offset:], '\n'); i >= 0 {
		return offset + i + 1
	}
	return len(s)
}
func countTableCells(line string) int {
	return len(tableCellRanges(line, 0))
}
func tableCellRanges(line string, base int) [][2]int {
	left := len(line) - len(strings.TrimLeft(line, " \t>"))
	right := len(strings.TrimRight(line, " \t\r\n"))
	if left < right && line[left] == '|' {
		left++
	}
	if right > left && line[right-1] == '|' {
		slashes := 0
		for i := right - 2; i >= left && line[i] == '\\'; i-- {
			slashes++
		}
		if slashes%2 == 0 {
			right--
		}
	}
	out := [][2]int{}
	cell := left
	escape := false
	emit := func(end int) {
		a, b := cell, end
		for a < b && (line[a] == ' ' || line[a] == '\t') {
			a++
		}
		for b > a && (line[b-1] == ' ' || line[b-1] == '\t') {
			b--
		}
		out = append(out, [2]int{base + a, base + b})
	}
	for i := left; i < right; i++ {
		if escape {
			escape = false
			continue
		}
		if line[i] == '\\' {
			escape = true
			continue
		}
		if line[i] == '|' {
			emit(i)
			cell = i + 1
		}
	}
	emit(right)
	return out
}
func (d *Document) selectTargets(context *Target, kind, scope string) []*Target {
	out := []*Target{}
	for _, t := range d.Targets {
		if t.Kind != kind {
			continue
		}
		if kind == "section" && context.Kind == "document" && t.Level == 1 {
			continue
		}
		if t == context {
			continue
		}
		if context.Kind == "paragraph" {
			if t.Parent != context {
				continue
			}
		} else if context.Kind == "section" {
			if t.Start < context.Start || t.Start >= context.End {
				continue
			}
			if scope != "subtree" {
				if t.Kind == "section" {
					if t.Parent != context {
						continue
					}
				} else {
					child := false
					for _, s := range d.Targets {
						if s.Kind == "section" && s != context && s.Start >= context.Start && s.Start < context.End && t.Start >= s.Start && t.Start < s.End {
							child = true
							break
						}
					}
					if child {
						continue
					}
					if t.Kind == "paragraph" && t.Parent != d.Root {
						continue
					}
					if (t.Kind == "table" || t.Kind == "codeblock") && t.Parent != d.Root {
						continue
					}
				}
			}
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}
func (d *Document) units(t *Target, view, scope string) []Unit {
	if view == "source" {
		end := t.End
		if t.Kind == "section" && scope != "subtree" {
			for _, child := range d.Targets {
				if child.Kind == "section" && child.Parent == t && child.Heading != nil {
					end = min(end, lineStart(d.Source, child.Heading.Start))
				}
			}
		}
		return []Unit{{Text: string(d.Source[t.Start:end]), Start: t.Start, End: end}}
	}
	if view == "code" {
		return []Unit{t.CodeUnit}
	}
	if view == "language" {
		return []Unit{t.LanguageUnit}
	}
	if t.Kind != "document" && t.Kind != "section" {
		return t.Units
	}
	out := []Unit{}
	for _, p := range d.selectTargets(t, "paragraph", scope) {
		out = append(out, p.Units...)
	}
	if view == "visible" {
		for _, h := range d.selectTargets(t, "heading", scope) {
			out = append(out, h.Units...)
		}
		for _, table := range d.selectTargets(t, "table", scope) {
			for _, c := range table.Header {
				out = append(out, c.Units...)
			}
			for _, row := range table.Rows {
				for _, c := range row {
					out = append(out, c.Units...)
				}
			}
		}
	}
	return out
}
func physicalLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}
func (d *Document) measure(t *Target, name, view, scope string) float64 {
	switch name {
	case "bytes":
		return float64(len(d.units(t, "source", scope)[0].Text))
	case "source-lines":
		return float64(physicalLines(d.units(t, "source", scope)[0].Text))
	case "lines":
		return float64(physicalLines(t.Code))
	case "level":
		return float64(t.Level)
	case "rows":
		return float64(len(t.Rows))
	case "columns":
		return float64(len(t.Header))
	case "max-heading-level":
		max := 0
		for _, h := range d.selectTargets(t, "heading", "subtree") {
			if h.Level > max {
				max = h.Level
			}
		}
		return float64(max)
	case "subsections":
		return float64(len(d.selectTargets(t, "section", "direct")))
	case "blocks":
		n := 0
		for _, b := range d.Targets {
			if b.Kind == "heading" || b.Kind == "section" || b.Kind == "sentence" || b.Kind == "source-line" || b == t {
				continue
			}
			if b.Start < t.Start || b.Start >= t.End {
				continue
			}
			if scope != "subtree" {
				if b.Parent != d.Root {
					continue
				}
				inside := false
				for _, s := range d.Targets {
					if s.Kind == "section" && s != t && s.Start >= t.Start && s.Start < t.End && b.Start >= s.Start && b.Start < s.End {
						inside = true
						break
					}
				}
				if inside && t.Kind == "section" {
					continue
				}
			}
			n++
		}
		return float64(n)
	}
	kind := map[string]string{"paragraphs": "paragraph", "tables": "table", "codeblocks": "codeblock", "lists": "list", "blockquotes": "blockquote", "headings": "heading", "sections": "section"}[name]
	if kind != "" {
		return float64(len(d.selectTargets(t, kind, scope)))
	}
	n := 0
	for _, u := range d.units(t, view, scope) {
		s := u.Text
		if view != "code" && view != "source" {
			s = normalized(s)
		}
		switch name {
		case "words":
			n += len(words(s))
		case "graphemes":
			n += uniseg.GraphemeClusterCount(s)
		case "sentences":
			n += len(sentenceSpans(s))
		}
	}
	return float64(n)
}
