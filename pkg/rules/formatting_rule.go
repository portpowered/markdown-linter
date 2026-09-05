package rules

import (
	"context"
	"fmt"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"strings"
)

const docsCheckFormattingRuleID = "markdown.docs-check-formatting"

// DocsCheckFormattingRule validates formatting failures owned by docs-check.
type DocsCheckFormattingRule struct{}

var _ interfaces.Rule = (*DocsCheckFormattingRule)(nil)

// NewDocsCheckFormattingRule creates the docs-check formatting rule.
func NewDocsCheckFormattingRule() *DocsCheckFormattingRule {
	return &DocsCheckFormattingRule{}
}

// ID returns the stable docs-check formatting rule identifier.
func (r *DocsCheckFormattingRule) ID() string {
	return docsCheckFormattingRuleID
}

// Check reports trailing whitespace and unclosed fenced code blocks.
func (r *DocsCheckFormattingRule) Check(_ context.Context, doc *interfaces.Document) []interfaces.Violation {
	var violations []interfaces.Violation
	var openFence string

	lines := strings.Split(string(doc.Source), "\n")
	for index, rawLine := range lines {
		lineNumber := index + 1
		line := strings.TrimSuffix(rawLine, "\r")
		if line != "" && hasTrailingWhitespace(line) {
			violations = append(violations, interfaces.NewViolation(
				doc.Path,
				lineNumber,
				r.ID(),
				fmt.Sprintf("%s:%d has trailing whitespace.", doc.Path, lineNumber),
			))
		}

		if fence := openingFence(line); fence != "" {
			if openFence == "" {
				openFence = fence
				continue
			}
			if closesFence(openFence, fence) {
				openFence = ""
			}
		}
	}

	if openFence != "" {
		violations = append(violations, interfaces.NewViolation(
			doc.Path,
			0,
			r.ID(),
			fmt.Sprintf("%s has an unclosed fenced code block.", doc.Path),
		))
	}

	return violations
}

func hasTrailingWhitespace(line string) bool {
	return strings.HasSuffix(line, " ") || strings.HasSuffix(line, "\t")
}

func openingFence(line string) string {
	if strings.HasPrefix(line, "```") {
		return repeatedPrefix(line, '`')
	}
	if strings.HasPrefix(line, "~~~") {
		return repeatedPrefix(line, '~')
	}
	return ""
}

func repeatedPrefix(line string, marker rune) string {
	count := 0
	for _, char := range line {
		if char != marker {
			break
		}
		count++
	}
	if count < 3 {
		return ""
	}
	return strings.Repeat(string(marker), count)
}

func closesFence(openFence string, candidateFence string) bool {
	return candidateFence[0] == openFence[0] && len(candidateFence) >= len(openFence)
}
