package interfaces

import "context"

// Analyzer is the repository-aware extension point for diagnostic-producing rules.
type Analyzer interface {
	// ID returns the stable identifier used in diagnostic output.
	ID() string

	// Analyze inspects the pass documents and reports diagnostics through the pass.
	Analyze(ctx context.Context, pass *Pass)
}

// PassDataKey identifies shared computed data attached to an analyzer pass.
type PassDataKey string

// Pass contains the repository-level state available to analyzers.
type Pass struct {
	Documents []*Document

	// Root bounds filesystem access for checks run through a rule pack.
	// Empty preserves unrestricted behavior for direct library callers.
	Root string

	diagnostics []Diagnostic
	data        map[PassDataKey]any
}

func newPass(documents []*Document) *Pass {
	return &Pass{
		Documents: documents,
		data:      map[PassDataKey]any{},
	}
}

// Report records a diagnostic produced by an analyzer.
func (p *Pass) Report(diagnostic Diagnostic) {
	p.diagnostics = append(p.diagnostics, diagnostic)
}

// SetData attaches shared computed data to the pass for later analyzers.
func (p *Pass) SetData(key PassDataKey, value any) {
	p.data[key] = value
}

// Data returns shared computed data attached to the pass.
func (p *Pass) Data(key PassDataKey) (any, bool) {
	value, ok := p.data[key]
	return value, ok
}

// Diagnostics returns a copy of diagnostics reported during the pass.
func (p *Pass) Diagnostics() []Diagnostic {
	return append([]Diagnostic(nil), p.diagnostics...)
}

// NewPass constructs a pass with an initial document set and empty diagnostics.
func NewPass(documents []*Document) *Pass {
	return newPass(documents)
}
