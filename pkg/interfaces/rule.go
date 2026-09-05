package interfaces

import "context"

// Rule is the public extension point for Markdown lint checks.
type Rule interface {
	// ID returns the stable identifier used in violation output.
	ID() string

	// Check inspects a parsed Markdown document and returns all violations found by the rule.
	Check(ctx context.Context, doc *Document) []Violation
}

// DiagnosticRule is the public extension point for checks that can report suggested fixes.
type DiagnosticRule interface {
	// ID returns the stable identifier used in diagnostic output.
	ID() string

	// CheckDiagnostics inspects a parsed Markdown document and returns structured diagnostics.
	CheckDiagnostics(ctx context.Context, doc *Document) []Diagnostic
}
