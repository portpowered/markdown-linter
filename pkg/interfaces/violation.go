package interfaces

import "fmt"

// Violation is a single actionable Markdown lint finding.
type Violation struct {
	Path    string
	Line    int
	RuleID  string
	Message string
}

// NewViolation creates a structured lint violation.
func NewViolation(path string, line int, ruleID string, message string) Violation {
	return Violation{
		Path:    path,
		Line:    line,
		RuleID:  ruleID,
		Message: message,
	}
}

// String renders the violation in a stable, human-readable form.
func (v Violation) String() string {
	location := v.Path
	if v.Line > 0 {
		location = fmt.Sprintf("%s:%d", location, v.Line)
	}
	return fmt.Sprintf("%s: %s: %s", location, v.RuleID, v.Message)
}
