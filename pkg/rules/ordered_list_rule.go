package rules

import (
	"context"
	"fmt"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"regexp"
	"strconv"

	"github.com/yuin/goldmark/ast"
)

const orderedListRuleID = "markdown.ordered-list"

var orderedListMarkerPattern = regexp.MustCompile(`^\s*(\d+)[.)]\s+`)

// OrderedListRule validates explicit ordered-list numbering.
type OrderedListRule struct{}

var _ interfaces.Rule = (*OrderedListRule)(nil)

// NewOrderedListRule creates a rule for ordered-list numbering.
func NewOrderedListRule() *OrderedListRule {
	return &OrderedListRule{}
}

// ID returns the stable ordered-list rule identifier.
func (r *OrderedListRule) ID() string {
	return orderedListRuleID
}

// Check validates that each ordered list starts at 1 and increments by 1.
func (r *OrderedListRule) Check(ctx context.Context, doc *interfaces.Document) []interfaces.Violation {
	var violations []interfaces.Violation

	err := doc.Walk(func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if err := ctx.Err(); err != nil {
			return ast.WalkStop, err
		}

		list, ok := node.(*ast.List)
		if !ok || !list.IsOrdered() {
			return ast.WalkContinue, nil
		}

		violations = append(violations, r.checkList(doc, list)...)
		return ast.WalkContinue, nil
	})
	if err != nil {
		violations = append(violations, interfaces.NewViolation(doc.Path, 0, r.ID(), err.Error()))
	}

	return violations
}

func (r *OrderedListRule) checkList(doc *interfaces.Document, list *ast.List) []interfaces.Violation {
	var violations []interfaces.Violation

	expected := 1
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		if item.Kind() != ast.KindListItem {
			continue
		}

		line := listItemLine(doc, item)
		actual, ok := orderedListMarker(doc.SourceLine(line))
		if !ok {
			expected++
			continue
		}

		if actual != expected {
			violations = append(violations, interfaces.NewViolation(
				doc.Path,
				line,
				r.ID(),
				fmt.Sprintf("ordered list item should be numbered %d, got %d", expected, actual),
			))
			expected = actual
		}

		expected++
	}

	return violations
}

func listItemLine(doc *interfaces.Document, item ast.Node) int {
	if line := doc.NodeLine(item); line > 0 {
		return line
	}
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		if line := doc.NodeLine(child); line > 0 {
			return line
		}
	}
	return 0
}

func orderedListMarker(line string) (int, bool) {
	matches := orderedListMarkerPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return 0, false
	}

	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, false
	}

	return value, true
}
