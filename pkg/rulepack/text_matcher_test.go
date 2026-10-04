package rulepack

import (
	"gopkg.in/yaml.v3"
	"strings"
	"testing"
)

func TestGenericMatcherPolicies(t *testing.T) {
	for _, tc := range []struct {
		source, options string
		want            int
	}{
		{"Utilize the tool.\n", "banned-words: [utilize]\nmessage: Use familiar words", 1},
		{"Utilization is a technical term.\n", "banned-words: [utilize]", 0},
		{"Run in *order* to test.\n", "banned-words: [in order to]", 1},
		{"Run in\n\norder to test.\n", "banned-words: [in order to]", 0},
		{"In `order` to test.\n", "banned-words: [in order to]", 0},
		{"[Utilize](https://example.test/utilize) `utilize`\n", "banned-words: [utilize]", 1},
		{"# Utilize\n\nUtilize.\n", "banned-words: [utilize]\nscope: heading", 1},
		{"Utilize utilize.\n", "banned-words: [utilize]\nignore-case: false", 1},
		{"Utilize the tool.\n", "banned-words: [utilize]\nallow: [utilize]", 0},
		{"Ready! Really?\n", "banned-characters: '!?'", 2},
		{"a-b[c]\\d^e\n", "banned-characters: '-[]\\^'", 5},
		{"Text—here and x-y.\n", "patterns: ['[-\\p{Pd}]']", 2},
		{"Done. There is a result.\n", "patterns: ['(?:^|[.!?]\\s+)(there\\s+is)']\ncapture-group: 1", 1},
		{"Use red blue.\n", "patterns: ['(optional)?red']\ncapture-group: 1", 0},
		{"Red.\n", "patterns: ['()red']\ncapture-group: 1\npaired-conjunctions: {both: and}", 0},
		{"Both red and blue or green.\n", "patterns: ['\\bboth\\b[^.!?]*?\\bor\\b']\npaired-conjunctions: {both: and}", 0},
	} {
		got := checkMarkdown(t, "text.matcher", tc.source, tc.options)
		if len(got) != tc.want {
			t.Fatalf("%s with %s: %#v", tc.source, tc.options, got)
		}
		for _, finding := range got {
			matched := tc.source[finding.StartOffset:finding.EndOffset]
			if matched == "" || !strings.Contains(finding.Message, "matched") {
				t.Fatalf("finding must explain matched text: %#v", finding)
			}
		}
	}
}
func TestGenericMatcherRejectsInvalidPolicies(t *testing.T) {
	for _, source := range []string{"{}", "[]", "unknown: true", "scope: code", "message: ''", "banned-words: ['']", "patterns: ['[']", "patterns: ['x*']", "patterns: [x]\ncapture-group: -1", "patterns: [x]\ncapture-group: 1", "patterns: [x]\npaired-conjunctions: {'two words': and}", "patterns: [x]\npaired-conjunctions: {both: ''}"} {
		var node yaml.Node
		if err := yaml.Unmarshal([]byte(source), &node); err != nil {
			t.Fatal(err)
		}
		if _, err := matcherFactory("text.matcher", matcherDefaults())(*node.Content[0]); err == nil {
			t.Fatalf("accepted invalid policy %s", source)
		}
	}
}
func TestStrunkWhiteUsesGenericMatcher(t *testing.T) {
	check, err := strunkFactory("text.strunk-white.fancy-words", strunkSpecs[1])(yaml.Node{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := check.(matcherCheck); !ok {
		t.Fatal("Strunk checks must use the generic matcher")
	}
	got := checkMarkdown(t, "text.strunk-white.fancy-words", "Utilize the tool. Fancy prose.\n", "patterns: ['\\bfancy\\b']\nmessage: Prefer plain words")
	if len(got) != 1 || !strings.Contains(got[0].Message, "Fancy") {
		t.Fatalf("customization did not replace defaults: %#v", got)
	}
	got = checkMarkdown(t, "text.strunk-white.correlative-pairs", "Both red and blue or green.\n", "paired-conjunctions: {}")
	if len(got) != 1 {
		t.Fatalf("empty pairing options must replace defaults: %#v", got)
	}
}
