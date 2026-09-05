package engine

import (
	"bytes"
	"context"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// SourceRange identifies a byte range in a Markdown document. Offsets are -1 when unavailable.
type SourceRange struct {
	StartOffset int
	EndOffset   int
	Line        int
}

// VisitorContext contains shared state for Markdown visitor callbacks.
type VisitorContext struct {
	Pass     *Pass
	Document *Document
	Node     ast.Node
	Range    SourceRange
}

// LinkVisit describes a Markdown link callback.
type LinkVisit struct {
	VisitorContext
	Link             *ast.Link
	Destination      string
	DestinationRange SourceRange
}

// ImageVisit describes a Markdown image callback.
type ImageVisit struct {
	VisitorContext
	Image            *ast.Image
	Destination      string
	DestinationRange SourceRange
}

// HeadingVisit describes a Markdown heading callback.
type HeadingVisit struct {
	VisitorContext
	Heading *ast.Heading
}

// ListVisit describes a Markdown list callback.
type ListVisit struct {
	VisitorContext
	List *ast.List
}

// TextVisit describes a Markdown text callback.
type TextVisit struct {
	VisitorContext
	Text  *ast.Text
	Value string
}

// VisitorCallbacks groups callbacks for common Markdown node types.
type VisitorCallbacks struct {
	Link    func(context.Context, LinkVisit)
	Image   func(context.Context, ImageVisit)
	Heading func(context.Context, HeadingVisit)
	List    func(context.Context, ListVisit)
	Text    func(context.Context, TextVisit)
}

// VisitorRule adapts common Markdown node visitors into a diagnostic rule and analyzer.
type VisitorRule struct {
	id        string
	callbacks VisitorCallbacks
}

var _ Analyzer = (*VisitorRule)(nil)
var _ DiagnosticRule = (*VisitorRule)(nil)

// NewVisitorRule creates a rule that visits common Markdown AST nodes.
func NewVisitorRule(id string, callbacks VisitorCallbacks) *VisitorRule {
	return &VisitorRule{
		id:        id,
		callbacks: callbacks,
	}
}

// ID returns the stable visitor rule identifier.
func (r *VisitorRule) ID() string {
	return r.id
}

// CheckDiagnostics runs visitor callbacks against one document.
func (r *VisitorRule) CheckDiagnostics(ctx context.Context, doc *Document) []Diagnostic {
	pass := NewPass([]*Document{doc})
	r.Analyze(ctx, pass)
	return pass.Diagnostics()
}

// Analyze runs visitor callbacks against every document in the pass.
func (r *VisitorRule) Analyze(ctx context.Context, pass *Pass) {
	for _, doc := range pass.Documents {
		WalkVisitorCallbacks(ctx, pass, doc, r.callbacks)
	}
}

func WalkVisitorCallbacks(ctx context.Context, pass *Pass, doc *Document, callbacks VisitorCallbacks) {
	links := newDestinationRanges(doc)
	images := newDestinationRanges(doc)

	_ = doc.Walk(func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if err := ctx.Err(); err != nil {
			return ast.WalkStop, err
		}

		base := VisitorContext{
			Pass:     pass,
			Document: doc,
			Node:     node,
			Range:    nodeSourceRange(doc, node),
		}

		switch typed := node.(type) {
		case *ast.Link:
			if callbacks.Link == nil {
				return ast.WalkContinue, nil
			}
			destination := string(typed.Destination)
			match := links.next(false, base.Range.Line, destination)
			base.Range = match.sourceRange
			callbacks.Link(ctx, LinkVisit{
				VisitorContext:   base,
				Link:             typed,
				Destination:      destination,
				DestinationRange: match.destinationRange,
			})
		case *ast.Image:
			if callbacks.Image == nil {
				return ast.WalkContinue, nil
			}
			destination := string(typed.Destination)
			match := images.next(true, base.Range.Line, destination)
			base.Range = match.sourceRange
			callbacks.Image(ctx, ImageVisit{
				VisitorContext:   base,
				Image:            typed,
				Destination:      destination,
				DestinationRange: match.destinationRange,
			})
		case *ast.Heading:
			if callbacks.Heading != nil {
				callbacks.Heading(ctx, HeadingVisit{VisitorContext: base, Heading: typed})
			}
		case *ast.List:
			if callbacks.List != nil {
				callbacks.List(ctx, ListVisit{VisitorContext: base, List: typed})
			}
		case *ast.Text:
			if callbacks.Text != nil {
				callbacks.Text(ctx, TextVisit{
					VisitorContext: base,
					Text:           typed,
					Value:          string(typed.Value(doc.Source)),
				})
			}
		}

		return ast.WalkContinue, nil
	})
}

func nodeSourceRange(doc *Document, node ast.Node) SourceRange {
	if textNode, ok := node.(*ast.Text); ok {
		return SourceRange{
			StartOffset: textNode.Segment.Start,
			EndOffset:   textNode.Segment.Stop,
			Line:        doc.LineForOffset(textNode.Segment.Start),
		}
	}

	line := doc.NodeLine(node)
	for current := node; current != nil; current = current.Parent() {
		if current.Type() == ast.TypeInline {
			continue
		}

		lines := current.Lines()
		if lines == nil || lines.Len() == 0 {
			continue
		}

		first := lines.At(0)
		last := lines.At(lines.Len() - 1)
		return SourceRange{
			StartOffset: first.Start,
			EndOffset:   last.Stop,
			Line:        line,
		}
	}

	if child := node.FirstChild(); child != nil {
		return nodeSourceRange(doc, child)
	}

	return SourceRange{
		StartOffset: -1,
		EndOffset:   -1,
		Line:        line,
	}
}

type inlineDestinationRange struct {
	image            bool
	line             int
	destination      string
	sourceRange      SourceRange
	destinationRange SourceRange
	used             bool
}

type inlineDestinationRanges struct {
	ranges []inlineDestinationRange
}

type inlineDestinationMatch struct {
	sourceRange      SourceRange
	destinationRange SourceRange
}

func newDestinationRanges(doc *Document) *inlineDestinationRanges {
	var ranges []inlineDestinationRange
	lineStart := 0
	line := 1
	source := doc.Source
	references := scanReferenceDefinitionRanges(source)
	inFencedCode := false

	for index, char := range source {
		if char == '\n' {
			lineSource := source[lineStart:index]
			if isFenceLine(lineSource) {
				inFencedCode = !inFencedCode
			}
			if !inFencedCode {
				ranges = append(ranges, scanInlineDestinationRanges(lineSource, lineStart, line)...)
				ranges = append(ranges, scanReferenceUsageDestinationRanges(lineSource, lineStart, line, references)...)
			}
			lineStart = index + 1
			line++
		}
	}
	if lineStart <= len(source) {
		lineSource := source[lineStart:]
		if isFenceLine(lineSource) {
			inFencedCode = !inFencedCode
		}
		if !inFencedCode {
			ranges = append(ranges, scanInlineDestinationRanges(lineSource, lineStart, line)...)
			ranges = append(ranges, scanReferenceUsageDestinationRanges(lineSource, lineStart, line, references)...)
		}
	}

	return &inlineDestinationRanges{ranges: ranges}
}

func (r *inlineDestinationRanges) next(image bool, line int, destination string) inlineDestinationMatch {
	for index := range r.ranges {
		candidate := &r.ranges[index]
		if candidate.used || candidate.image != image || candidate.line != line || candidate.destination != destination {
			continue
		}

		candidate.used = true
		return inlineDestinationMatch{
			sourceRange:      candidate.sourceRange,
			destinationRange: candidate.destinationRange,
		}
	}

	unknown := SourceRange{
		StartOffset: -1,
		EndOffset:   -1,
		Line:        line,
	}
	return inlineDestinationMatch{
		sourceRange:      unknown,
		destinationRange: unknown,
	}
}

func scanInlineDestinationRanges(line []byte, lineStart int, lineNumber int) []inlineDestinationRange {
	var ranges []inlineDestinationRange
	for index := 0; index < len(line); index++ {
		image := false
		linkStart := index
		if line[index] == '!' && index+1 < len(line) && line[index+1] == '[' {
			image = true
			index++
		}
		if line[index] != '[' || isEscaped(line, index) {
			continue
		}

		closeLabel := bytesIndexByteUnescaped(line[index+1:], ']')
		if closeLabel < 0 {
			continue
		}
		closeLabel += index + 1
		if closeLabel+1 >= len(line) || line[closeLabel+1] != '(' {
			continue
		}

		destinationStart, destinationEnd, ok := inlineDestinationBounds(line, closeLabel+2)
		if !ok {
			continue
		}
		closeDestination := bytesIndexByteUnescaped(line[destinationEnd:], ')')
		if closeDestination < 0 {
			continue
		}
		closeDestination += destinationEnd

		ranges = append(ranges, inlineDestinationRange{
			image:       image,
			line:        lineNumber,
			destination: string(line[destinationStart:destinationEnd]),
			sourceRange: SourceRange{
				StartOffset: lineStart + linkStart,
				EndOffset:   lineStart + closeDestination + 1,
				Line:        lineNumber,
			},
			destinationRange: SourceRange{
				StartOffset: lineStart + destinationStart,
				EndOffset:   lineStart + destinationEnd,
				Line:        lineNumber,
			},
		})

		index = destinationEnd
	}

	return ranges
}

type referenceDestinationRange struct {
	destination      string
	destinationRange SourceRange
}

func scanReferenceDefinitionRanges(source []byte) map[string]referenceDestinationRange {
	references := map[string]referenceDestinationRange{}
	lineStart := 0
	lineNumber := 1
	inFencedCode := false
	for index, char := range source {
		if char == '\n' {
			line := source[lineStart:index]
			if isFenceLine(line) {
				inFencedCode = !inFencedCode
			}
			if !inFencedCode {
				scanReferenceDefinitionRange(line, lineStart, lineNumber, references)
			}
			lineStart = index + 1
			lineNumber++
		}
	}
	if lineStart <= len(source) {
		line := source[lineStart:]
		if isFenceLine(line) {
			inFencedCode = !inFencedCode
		}
		if !inFencedCode {
			scanReferenceDefinitionRange(line, lineStart, lineNumber, references)
		}
	}
	return references
}

func scanReferenceDefinitionRange(line []byte, lineStart int, lineNumber int, references map[string]referenceDestinationRange) {
	index := 0
	for index < len(line) && index < 3 && (line[index] == ' ' || line[index] == '\t') {
		index++
	}
	if index >= len(line) || line[index] != '[' {
		return
	}

	closeLabel := bytesIndexByteUnescaped(line[index+1:], ']')
	if closeLabel < 0 {
		return
	}
	closeLabel += index + 1
	if closeLabel+1 >= len(line) || line[closeLabel+1] != ':' {
		return
	}

	label := normalizeReferenceLabel(string(line[index+1 : closeLabel]))
	if label == "" {
		return
	}
	destinationStart, destinationEnd, ok := inlineDestinationBounds(line, closeLabel+2)
	if !ok {
		return
	}
	if _, exists := references[label]; exists {
		return
	}

	references[label] = referenceDestinationRange{
		destination: string(line[destinationStart:destinationEnd]),
		destinationRange: SourceRange{
			StartOffset: lineStart + destinationStart,
			EndOffset:   lineStart + destinationEnd,
			Line:        lineNumber,
		},
	}
}

func scanReferenceUsageDestinationRanges(line []byte, lineStart int, lineNumber int, references map[string]referenceDestinationRange) []inlineDestinationRange {
	if len(references) == 0 {
		return nil
	}

	var ranges []inlineDestinationRange
	for index := 0; index < len(line); index++ {
		image := false
		linkStart := index
		if line[index] == '!' && index+1 < len(line) && line[index+1] == '[' {
			image = true
			index++
		}
		if line[index] != '[' || isEscaped(line, index) {
			continue
		}

		closeLabel := bytesIndexByteUnescaped(line[index+1:], ']')
		if closeLabel < 0 {
			continue
		}
		closeLabel += index + 1
		if closeLabel+1 >= len(line) || line[closeLabel+1] != '[' {
			continue
		}

		closeReference := bytesIndexByteUnescaped(line[closeLabel+2:], ']')
		if closeReference < 0 {
			continue
		}
		closeReference += closeLabel + 2

		label := string(line[closeLabel+2 : closeReference])
		if label == "" {
			label = string(line[index+1 : closeLabel])
		}
		reference, ok := references[normalizeReferenceLabel(label)]
		if !ok {
			continue
		}

		ranges = append(ranges, inlineDestinationRange{
			image:       image,
			line:        lineNumber,
			destination: reference.destination,
			sourceRange: SourceRange{
				StartOffset: lineStart + linkStart,
				EndOffset:   lineStart + closeReference + 1,
				Line:        lineNumber,
			},
			destinationRange: reference.destinationRange,
		})

		index = closeReference
	}
	return ranges
}

func normalizeReferenceLabel(label string) string {
	return strings.ToLower(strings.Join(strings.Fields(label), " "))
}

func isFenceLine(line []byte) bool {
	index := 0
	for index < len(line) && index < 3 && (line[index] == ' ' || line[index] == '\t') {
		index++
	}
	if index+2 >= len(line) {
		return false
	}
	if line[index] != '`' && line[index] != '~' {
		return false
	}
	return line[index+1] == line[index] && line[index+2] == line[index]
}

func inlineDestinationBounds(line []byte, start int) (int, int, bool) {
	for start < len(line) && (line[start] == ' ' || line[start] == '\t') {
		start++
	}
	if start >= len(line) || line[start] == ')' {
		return 0, 0, false
	}

	if line[start] == '<' {
		end := bytesIndexByteUnescaped(line[start+1:], '>')
		if end < 0 {
			return 0, 0, false
		}
		return start + 1, start + 1 + end, true
	}

	end := start
	for end < len(line) {
		if line[end] == ')' || line[end] == ' ' || line[end] == '\t' || line[end] == '\n' || line[end] == '\r' {
			break
		}
		end++
	}
	return start, end, end > start
}

func bytesIndexByteUnescaped(value []byte, target byte) int {
	offset := 0
	for {
		index := bytes.IndexByte(value[offset:], target)
		if index < 0 {
			return -1
		}
		index += offset
		if !isEscaped(value, index) {
			return index
		}
		offset = index + 1
	}
}

func isEscaped(value []byte, index int) bool {
	slashes := 0
	for current := index - 1; current >= 0 && value[current] == '\\'; current-- {
		slashes++
	}
	return slashes%2 == 1
}
