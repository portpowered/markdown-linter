package rules

import (
	"context"
	"fmt"
	"github.com/portpowered/markdown-linter/pkg/interfaces"

	"github.com/yuin/goldmark/ast"
)

const headingOrderRuleID = "markdown.heading-order"

// HeadingOrderRule validates Markdown heading hierarchy.
type HeadingOrderRule struct{}

var _ interfaces.Rule = (*HeadingOrderRule)(nil)

// NewHeadingOrderRule creates a rule for heading hierarchy.
func NewHeadingOrderRule() *HeadingOrderRule {
	return &HeadingOrderRule{}
}

// ID returns the stable heading-order rule identifier.
func (r *HeadingOrderRule) ID() string {
	return headingOrderRuleID
}

// Check validates that headings do not skip levels when descending.
func (r *HeadingOrderRule) Check(ctx context.Context, doc *interfaces.Document) []interfaces.Violation {
	var violations []interfaces.Violation
	previousLevel := 0

	err := doc.Walk(func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if err := ctx.Err(); err != nil {
			return ast.WalkStop, err
		}

		heading, ok := node.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		currentLevel := heading.Level
		if previousLevel > 0 && currentLevel > previousLevel+1 {
			violations = append(violations, interfaces.NewViolation(
				doc.Path,
				doc.NodeLine(heading),
				r.ID(),
				fmt.Sprintf("heading level should not skip from %d to %d", previousLevel, currentLevel),
			))
		}
		previousLevel = currentLevel

		return ast.WalkContinue, nil
	})
	if err != nil {
		violations = append(violations, interfaces.NewViolation(doc.Path, 0, r.ID(), err.Error()))
	}

	return violations
}
