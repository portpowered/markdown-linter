package rulepack

import (
	"strings"
	"testing"
)

func TestSTEGrammarStrictlyLessThanTwentyWords(t *testing.T) {
	for _, sample := range []struct{ words, findings int }{{19, 0}, {20, 1}} {
		findings := checkMarkdown(t, "text.ste100.grammar", strings.Repeat("word ", sample.words)+".\n", "max-sentence-words: 19")
		if len(findings) != sample.findings {
			t.Fatalf("%d words: %#v", sample.words, findings)
		}
	}
}
