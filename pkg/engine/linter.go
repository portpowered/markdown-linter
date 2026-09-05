// Package engine parses Markdown and executes registered customer checks.
package engine

import (
	"context"
	"fmt"
	"os"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/text"
)

// Option configures a Linter.
type Option func(*Linter)

// Linter parses Markdown and runs registered rules.
type Linter struct {
	markdown        goldmark.Markdown
	rules           []Rule
	diagnosticRules []DiagnosticRule
	analyzers       []Analyzer
}

// New creates a Markdown linter with the supplied options.
func New(opts ...Option) *Linter {
	l := &Linter{
		markdown: goldmark.New(),
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// WithRules registers rules when constructing a linter.
func WithRules(rules ...Rule) Option {
	return func(l *Linter) {
		l.rules = append(l.rules, rules...)
	}
}

// WithDiagnosticRules registers fix-capable diagnostic rules when constructing a linter.
func WithDiagnosticRules(rules ...DiagnosticRule) Option {
	return func(l *Linter) {
		l.diagnosticRules = append(l.diagnosticRules, rules...)
	}
}

// WithAnalyzers registers repository-aware analyzers when constructing a linter.
func WithAnalyzers(analyzers ...Analyzer) Option {
	return func(l *Linter) {
		l.analyzers = append(l.analyzers, analyzers...)
	}
}

// RegisterRule adds a rule to an existing linter.
func (l *Linter) RegisterRule(rule Rule) {
	l.rules = append(l.rules, rule)
}

// RegisterDiagnosticRule adds a fix-capable diagnostic rule to an existing linter.
func (l *Linter) RegisterDiagnosticRule(rule DiagnosticRule) {
	l.diagnosticRules = append(l.diagnosticRules, rule)
}

// RegisterAnalyzer adds a repository-aware analyzer to an existing linter.
func (l *Linter) RegisterAnalyzer(analyzer Analyzer) {
	l.analyzers = append(l.analyzers, analyzer)
}

// ParseFile reads and parses a Markdown file.
func (l *Linter) ParseFile(path string) (*Document, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading markdown file %q: %w", path, err)
	}

	root := l.markdown.Parser().Parse(text.NewReader(source))
	return newDocument(path, source, root), nil
}

// RunFile parses a Markdown file and runs every registered rule.
func (l *Linter) RunFile(ctx context.Context, path string) ([]Violation, error) {
	doc, err := l.ParseFile(path)
	if err != nil {
		return nil, err
	}

	return l.Run(ctx, doc), nil
}

// Run executes every registered rule against a parsed Markdown document.
func (l *Linter) Run(ctx context.Context, doc *Document) []Violation {
	var violations []Violation
	for _, rule := range l.rules {
		violations = append(violations, rule.Check(ctx, doc)...)
	}
	return violations
}

// RunFileDiagnostics parses a Markdown file and returns structured diagnostics for every registered rule.
func (l *Linter) RunFileDiagnostics(ctx context.Context, path string) ([]Diagnostic, error) {
	doc, err := l.ParseFile(path)
	if err != nil {
		return nil, err
	}

	return l.RunDiagnostics(ctx, doc), nil
}

// RunFilesDiagnostics parses Markdown files and runs every diagnostic rule through one repository-level pass.
func (l *Linter) RunFilesDiagnostics(ctx context.Context, paths []string) ([]Diagnostic, error) {
	documents := make([]*Document, 0, len(paths))
	for _, path := range paths {
		doc, err := l.ParseFile(path)
		if err != nil {
			return nil, err
		}
		documents = append(documents, doc)
	}

	return l.RunDiagnosticsForDocuments(ctx, documents), nil
}

// RunDiagnostics executes diagnostics for one parsed Markdown document.
func (l *Linter) RunDiagnostics(ctx context.Context, doc *Document) []Diagnostic {
	return l.RunDiagnosticsForDocuments(ctx, []*Document{doc})
}

// RunDiagnosticsForDocuments executes fix-capable rules, violation-only rules, and analyzers in one pass.
func (l *Linter) RunDiagnosticsForDocuments(ctx context.Context, documents []*Document) []Diagnostic {
	pass := NewPass(documents)
	for _, rule := range l.diagnosticRules {
		diagnosticRuleAnalyzer{rule: rule}.Analyze(ctx, pass)
	}
	for _, rule := range l.rules {
		ruleAnalyzer{rule: rule}.Analyze(ctx, pass)
	}
	for _, analyzer := range l.analyzers {
		analyzer.Analyze(ctx, pass)
	}
	return pass.Diagnostics()
}
