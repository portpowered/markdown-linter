package interfaces

import "github.com/yuin/goldmark/ast"

// NodeText gathers visible inline text through Goldmark's supported node APIs.
func NodeText(n ast.Node, source []byte) []byte {
	switch v := n.(type) {
	case *ast.Text:
		return v.Value(source)
	case *ast.String:
		return v.Value
	case *ast.AutoLink:
		return v.Label(source)
	case *ast.RawHTML:
		return v.Segments.Value(source)
	case *ast.HTMLBlock:
		result := append([]byte(nil), v.Lines().Value(source)...)
		if v.HasClosure() {
			result = append(result, v.ClosureLine.Value(source)...)
		}
		return result
	}
	var result []byte
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		result = append(result, NodeText(c, source)...)
		if line, ok := c.(interface{ SoftLineBreak() bool }); ok && line.SoftLineBreak() {
			result = append(result, '\n')
		}
	}
	return result
}
