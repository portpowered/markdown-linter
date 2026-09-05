package interfaces

import "github.com/yuin/goldmark/ast"

// Document wraps a parsed Markdown AST with source metadata used by rules.
type Document struct {
	Path   string
	Source []byte
	Root   ast.Node

	lineOffsets []int
}

// NewDocument constructs a parsed Markdown document with cached line offsets.
func NewDocument(path string, source []byte, root ast.Node) *Document {
	return &Document{
		Path:        path,
		Source:      source,
		Root:        root,
		lineOffsets: buildLineOffsets(source),
	}
}

// Walk traverses the parsed Markdown AST.
func (d *Document) Walk(fn func(ast.Node, bool) (ast.WalkStatus, error)) error {
	return ast.Walk(d.Root, fn)
}

// NodeLine returns the 1-based source line for a Markdown AST node, or 0 when unknown.
func (d *Document) NodeLine(node ast.Node) int {
	for current := node; current != nil; current = current.Parent() {
		if current.Type() == ast.TypeInline {
			continue
		}

		lines := current.Lines()
		if lines == nil || lines.Len() == 0 {
			continue
		}

		segment := lines.At(0)
		return d.LineForOffset(segment.Start)
	}

	return 0
}

// LineForOffset returns the 1-based source line for a byte offset.
func (d *Document) LineForOffset(offset int) int {
	if offset < 0 {
		return 0
	}

	line := 1
	for _, lineOffset := range d.lineOffsets {
		if lineOffset > offset {
			return line
		}
		line++
	}

	if offset <= len(d.Source) {
		return line
	}
	return 0
}

// SourceLine returns a 1-based source line without its trailing newline.
func (d *Document) SourceLine(line int) string {
	if line <= 0 {
		return ""
	}

	start := 0
	if line > 1 {
		if line-2 >= len(d.lineOffsets) {
			return ""
		}
		start = d.lineOffsets[line-2]
	}

	end := len(d.Source)
	if line-1 < len(d.lineOffsets) {
		end = d.lineOffsets[line-1]
	}
	for end > start && (d.Source[end-1] == '\n' || d.Source[end-1] == '\r') {
		end--
	}

	return string(d.Source[start:end])
}

func buildLineOffsets(source []byte) []int {
	offsets := []int{}
	for index, char := range source {
		if char == '\n' {
			offsets = append(offsets, index+1)
		}
	}

	return offsets
}
