package interfaces

// Severity classifies the diagnostic impact for callers that need to filter or render findings.
type Severity string

const (
	// SeverityError indicates a finding that should fail lint verification.
	SeverityError Severity = "error"
	// SeverityWarning indicates a finding that should be reviewed but may not fail verification.
	SeverityWarning Severity = "warning"
	// SeverityInfo indicates an informational finding.
	SeverityInfo Severity = "info"
)

// FixConfidence describes how safe a suggested fix is to apply automatically.
type FixConfidence string

const (
	// FixConfidenceSafe indicates a fix that is safe for automated application.
	FixConfidenceSafe FixConfidence = "safe"
	// FixConfidenceUnsafe indicates a fix that needs manual review before application.
	FixConfidenceUnsafe FixConfidence = "unsafe"
)

// DiagnosticCategory describes the machine-reviewable reason for a diagnostic.
type DiagnosticCategory string

const (
	// DiagnosticCategoryGeneral is used when a diagnostic has no narrower category.
	DiagnosticCategoryGeneral DiagnosticCategory = "general"
	// DiagnosticCategoryRelocationAmbiguous means multiple plausible relocation targets exist.
	DiagnosticCategoryRelocationAmbiguous DiagnosticCategory = "relocation-ambiguous"
	// DiagnosticCategoryRelocationUnresolved means no unique relocation target could be found.
	DiagnosticCategoryRelocationUnresolved DiagnosticCategory = "relocation-unresolved"
	// DiagnosticCategoryRelocationInvalidAnchor means the relocated Markdown target lacks the requested anchor.
	DiagnosticCategoryRelocationInvalidAnchor DiagnosticCategory = "relocation-invalid-anchor"
	// DiagnosticCategoryRelocationMissingMappedTarget means an explicit move mapping points outside the lint surface.
	DiagnosticCategoryRelocationMissingMappedTarget DiagnosticCategory = "relocation-missing-mapped-target"
	// DiagnosticCategoryRelocationUnavailableRange means the scanner could not locate the destination byte range.
	DiagnosticCategoryRelocationUnavailableRange DiagnosticCategory = "relocation-unavailable-range"
	// DiagnosticCategoryRelocationOperational means relocation analysis stopped because of an operational condition.
	DiagnosticCategoryRelocationOperational DiagnosticCategory = "relocation-operational"
)

// TextEdit replaces the byte range [StartOffset, EndOffset) in a single Markdown file.
type TextEdit struct {
	StartOffset int
	EndOffset   int
	Replacement string
}

// SuggestedFix describes one candidate repair for a diagnostic.
type SuggestedFix struct {
	Title      string
	Confidence FixConfidence
	Edits      []TextEdit
}

// Diagnostic is a structured lint finding with optional machine-applicable fixes.
type Diagnostic struct {
	Path           string
	Line           int
	StartOffset    int
	EndOffset      int
	RuleID         string
	Message        string
	Severity       Severity
	Category       DiagnosticCategory
	SuggestedFixes []SuggestedFix
}

// NewDiagnostic creates a structured diagnostic without suggested fixes.
func NewDiagnostic(path string, line int, startOffset int, endOffset int, ruleID string, message string, severity Severity) Diagnostic {
	return Diagnostic{
		Path:        path,
		Line:        line,
		StartOffset: startOffset,
		EndOffset:   endOffset,
		RuleID:      ruleID,
		Message:     message,
		Severity:    severity,
		Category:    DiagnosticCategoryGeneral,
	}
}

func diagnosticFromViolation(violation Violation) Diagnostic {
	return NewDiagnostic(
		violation.Path,
		violation.Line,
		-1,
		-1,
		violation.RuleID,
		violation.Message,
		SeverityError,
	)
}

// DiagnosticFromViolation converts a violation to a diagnostic.
func DiagnosticFromViolation(violation Violation) Diagnostic {
	return diagnosticFromViolation(violation)
}
