package rulepack

import (
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/yuin/goldmark/ast"
	"regexp"
	"strings"
)

var disableNext = regexp.MustCompile(`^\s*<!--\s*marklint-disable-next-line\s+(\S+)\s+reason:\s*(.+?)\s*-->\s*$`)

func inlineSuppressed(doc *interfaces.Document, d interfaces.Diagnostic) bool {
	if doc == nil || d.Line < 2 {
		return false
	}
	directiveLine := d.Line - 1
	match := disableNext.FindStringSubmatch(doc.SourceLine(directiveLine))
	if match == nil || match[1] != d.RuleID || strings.TrimSpace(match[2]) == "" {
		return false
	}
	code := false
	ast.Walk(doc.Root, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		if !enter {
			return ast.WalkContinue, nil
		}
		switch n.(type) {
		case *ast.CodeBlock, *ast.FencedCodeBlock:
			for i := 0; i < n.Lines().Len(); i++ {
				segment := n.Lines().At(i)
				first, last := doc.LineForOffset(segment.Start), doc.LineForOffset(segment.Stop)
				if directiveLine >= first && directiveLine <= last {
					code = true
				}
			}
		}
		return ast.WalkContinue, nil
	})
	return !code
}
