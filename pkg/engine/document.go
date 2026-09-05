package engine

import (
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/yuin/goldmark/ast"
)

func newDocument(path string, source []byte, root ast.Node) *Document {
	return interfaces.NewDocument(path, source, root)
}
