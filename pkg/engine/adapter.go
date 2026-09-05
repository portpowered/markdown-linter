package engine

import "context"

type diagnosticRuleAnalyzer struct {
	rule DiagnosticRule
}

func (a diagnosticRuleAnalyzer) Analyze(ctx context.Context, pass *Pass) {
	for _, doc := range pass.Documents {
		for _, diagnostic := range a.rule.CheckDiagnostics(ctx, doc) {
			pass.Report(diagnostic)
		}
	}
}

type ruleAnalyzer struct {
	rule Rule
}

func (a ruleAnalyzer) Analyze(ctx context.Context, pass *Pass) {
	for _, doc := range pass.Documents {
		for _, violation := range a.rule.Check(ctx, doc) {
			pass.Report(DiagnosticFromViolation(violation))
		}
	}
}
