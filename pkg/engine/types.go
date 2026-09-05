package engine

import "github.com/portpowered/markdown-linter/pkg/interfaces"

type Rule = interfaces.Rule
type DiagnosticRule = interfaces.DiagnosticRule
type Analyzer = interfaces.Analyzer
type Pass = interfaces.Pass
type PassDataKey = interfaces.PassDataKey
type Document = interfaces.Document
type Violation = interfaces.Violation
type Diagnostic = interfaces.Diagnostic
type Severity = interfaces.Severity
type FixConfidence = interfaces.FixConfidence
type TextEdit = interfaces.TextEdit
type SuggestedFix = interfaces.SuggestedFix
type DiagnosticCategory = interfaces.DiagnosticCategory

const (
	FixConfidenceSafe                               = interfaces.FixConfidenceSafe
	FixConfidenceUnsafe                             = interfaces.FixConfidenceUnsafe
	DiagnosticCategoryGeneral                       = interfaces.DiagnosticCategoryGeneral
	DiagnosticCategoryRelocationAmbiguous           = interfaces.DiagnosticCategoryRelocationAmbiguous
	DiagnosticCategoryRelocationUnresolved          = interfaces.DiagnosticCategoryRelocationUnresolved
	DiagnosticCategoryRelocationInvalidAnchor       = interfaces.DiagnosticCategoryRelocationInvalidAnchor
	DiagnosticCategoryRelocationMissingMappedTarget = interfaces.DiagnosticCategoryRelocationMissingMappedTarget
	DiagnosticCategoryRelocationUnavailableRange    = interfaces.DiagnosticCategoryRelocationUnavailableRange
	DiagnosticCategoryRelocationOperational         = interfaces.DiagnosticCategoryRelocationOperational
)

func NewPass(documents []*Document) *Pass {
	return interfaces.NewPass(documents)
}

func DiagnosticFromViolation(violation Violation) Diagnostic {
	return interfaces.DiagnosticFromViolation(violation)
}
