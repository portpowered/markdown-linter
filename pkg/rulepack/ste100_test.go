package rulepack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

func stePass(root, source string) *interfaces.Pass {
	data := []byte(source)
	pass := interfaces.NewPass([]*interfaces.Document{interfaces.NewDocument(filepath.Join(root, "guide.md"), data, goldmark.New().Parser().Parse(text.NewReader(data)))})
	pass.Root = root
	return pass
}

func TestSTEDictionaryProse(t *testing.T) {
	for _, tc := range []struct {
		source, options string
		count           int
	}{
		{"Use the API.\n", "dictionary: [use, the, api]", 0},
		{"Use APIs and unknown.\n", "dictionary: [use, api, and]", 2},
		{"Use café and 東京.\n", "dictionary: [use, café, and]", 1},
		{"Use nextToken and next-token.\n", "dictionary: [use, nextToken, and]", 1},
		{"Use load bearing. Bearing load.\n", "dictionary: [use]\ntechnical-terms: [load bearing]", 2},
		{"Use load, bearing.\n", "dictionary: [use]\ntechnical-terms: [load bearing]", 2},
		{"Use *load* bearing.\n", "dictionary: [use]\ntechnical-terms: [load bearing]", 0},
		{"Use load `sample` bearing.\n", "dictionary: [use]\ntechnical-terms: [load bearing]", 2},
		{"Use load https://example.test bearing.\n", "dictionary: [use]\ntechnical-terms: [load bearing]", 2},
		{"Use load\n\nbearing.\n", "dictionary: [use]\ntechnical-terms: [load bearing]", 2},
		{"---\ntitle: unknown\n---\n\n# Use\n\nThe unknown.\n", "dictionary: [use]\nscope: heading", 0},
		{"---\ntitle: unknown\n", "dictionary: [use]", 0},
		{"Use `unknown`.\n\n```unknown\nunknown\n```\n\n    unknown\n\n[Use](https://unknown.test) ![unknown](unknown.png) <https://unknown.test> https://unknown.test\n\n<!-- unknown -->\n", "dictionary: [use]", 0},
		{"Use don't and don’t.\n", "dictionary: [use, and, don't]", 1},
		{"# Unknown\n\nUse unknown.\n", "dictionary: [use]", 2},
	} {
		if got := checkMarkdown(t, "text.ste100.dictionary", tc.source, tc.options); len(got) != tc.count {
			t.Fatalf("%q: want %d, got %#v", tc.source, tc.count, got)
		}
	}
	got := checkMarkdown(t, "text.ste100.dictionary", "Use café.\nUnknown.\n", "dictionary: [use]")
	if got[0].StartOffset != 4 || got[0].EndOffset != 9 || got[1].Line != 2 || len(got[0].SuggestedFixes) != 0 {
		t.Fatalf("ranges or unsafe fix: %#v", got)
	}
}

func TestSTEFormsAndAlternatives(t *testing.T) {
	root := t.TempDir()
	putPack(t, root, "words.yaml", `words: [to, will, the, use, do, a]
entries:
  - word: give
    part-of-speech: verb
    meaning: transfer to another person
    forms: [gives, gave, given]
  - word: check
    part-of-speech: noun
    forms: [checks]
technical-terms: [query shape]
alternatives:
  utilize: [use]
  inspect: [do a check]
  model: [query shape]
`)
	c, err := steFactory("text.ste100.dictionary")(coverageOptions(t, "dictionary-file: words.yaml\ndictionary: [and]"))
	if err != nil {
		t.Fatal(err)
	}
	pass := stePass(root, "To give. Gives gave given. Will give the query shape and checks. Giving utilizes utilize inspect model.\n")
	c.Analyze(context.Background(), pass)
	if pass.Err() != nil {
		t.Fatal(pass.Err())
	}
	got := pass.Diagnostics()
	if len(got) != 5 || !strings.Contains(got[2].Message, "consider: use") || !strings.Contains(got[3].Message, "do a check") || !strings.Contains(got[4].Message, "query shape") {
		t.Fatalf("forms/alternatives: %#v", got)
	}
	for _, d := range got {
		if len(d.SuggestedFixes) != 0 {
			t.Fatal("alternatives must remain editorial")
		}
	}
}

func TestSTEInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct{ id, options string }{
		{"dictionary", "{}"}, {"dictionary", "dictionary: [two words]"}, {"dictionary", "dictionary: ['']"},
		{"dictionary", "dictionary: [use]\ntechnical-terms: ['two  words']"},
		{"dictionary", "dictionary: [use]\nscope: code"}, {"dictionary", "dictionary: true"},
		{"dictionary", "dictionary-file: ../words.yaml"}, {"dictionary", "dictionary-file: '*.yaml'"},
		{"dictionary", "dictionary-file: 'words?.yaml'"}, {"dictionary", "dictionary-file: ''"},
		{"dictionary", "max-sentence-words: 3"}, {"grammar", "dictionary: [use]"},
		{"grammar", "max-sentence-words: 0"}, {"grammar", "forbidden-patterns: {bad: '['}"},
		{"grammar", "forbidden-patterns: {bad: 'a*'}"}, {"grammar", "forbidden-patterns: {'': abc}"},
	} {
		if _, err := steFactory("text.ste100." + tc.id)(coverageOptions(t, tc.options)); err == nil {
			t.Fatalf("accepted %s %s", tc.id, tc.options)
		}
	}
}

func TestSTEInvalidDictionaryFiles(t *testing.T) {
	root := t.TempDir()
	for _, source := range []string{
		"words: []", "words: [two words]", "unknown: true", "words: true", "words: [use]\n---\nwords: [the]",
		"words: [use]\n---\n[", "entries: [{word: ''}]", "entries: [{word: give, forms: ['giving!']}]",
		"entries: [{word: give, part-of-speech: nonsense}]", "technical-terms: ['']",
		"words: [use]\nalternatives: {use: [use]}", "words: [use]\nalternatives: {utilize: []}",
		"words: [use]\ntechnical-terms: [utilize]\nalternatives: {utilize: [use]}",
		"words: [use]\nalternatives: {utilize: [unknown]}", "words: [use]\nalternatives: {utilize: ['use!']}",
		"words: [use]\nalternatives: {'two words': [use]}", "words: [use]\nalternatives: {Utilize: [use], utilize: [use]}",
	} {
		putPack(t, root, "words.yaml", source)
		c, err := steFactory("text.ste100.dictionary")(coverageOptions(t, "dictionary-file: words.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		pass := stePass(root, "Use.\n")
		c.Analyze(context.Background(), pass)
		if pass.Err() == nil || len(pass.Diagnostics()) != 0 {
			t.Fatalf("accepted broken file %q", source)
		}
	}
	missing, _ := steFactory("text.ste100.dictionary")(coverageOptions(t, "dictionary-file: missing.yaml"))
	pass := stePass(root, "Use.\n")
	missing.Analyze(context.Background(), pass)
	if pass.Err() == nil {
		t.Fatal("missing dictionary accepted")
	}
	outside := putPack(t, t.TempDir(), "words.yaml", "words: [use]")
	if err := os.Symlink(outside, filepath.Join(root, "escape.yaml")); err == nil {
		c, _ := steFactory("text.ste100.dictionary")(coverageOptions(t, "dictionary-file: escape.yaml"))
		pass := stePass(root, "Use.\n")
		c.Analyze(context.Background(), pass)
		if pass.Err() == nil {
			t.Fatal("symlink escape accepted")
		}
	} else {
		t.Logf("symlink unavailable: %v", err)
	}
}

func TestSTEGrammarAndComposition(t *testing.T) {
	for _, tc := range []struct {
		source, options string
		count           int
	}{
		{"Use the API. Send the request.\n", "max-sentence-words: 3", 0},
		{"Use the API and send the request.\n", "max-sentence-words: 3", 1},
		{"Use the\nAPI.\n", "max-sentence-words: 2", 1},
		{"Use the\n\nAPI.\n", "max-sentence-words: 2", 0},
		{"# Use the API\n\nUse the API and send.\n", "max-sentence-words: 3\nscope: heading", 0},
		{"Use `some long unknown code` and https://unknown.test.\n", "max-sentence-words: 2", 0},
		{"Do not utilize.\n", "forbidden-patterns: {word-choice: '(?i)\\butilize\\b'}", 1},
	} {
		if got := checkMarkdown(t, "text.ste100.grammar", tc.source, tc.options); len(got) != tc.count {
			t.Fatalf("%q: %#v", tc.source, got)
		}
	}
	root := t.TempDir()
	putPack(t, root, "ste100-dictionary.yaml", "words: [use, the, api, do, not]\n")
	p, ok := Preset("text:ste100")
	if !ok {
		t.Fatal("preset missing")
	}
	r := NewRegistry()
	if err := RegisterStock(r); err != nil {
		t.Fatal(err)
	}
	program, err := r.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	pass := stePass(root, "Use the API.\n")
	if got, err := program.Run(context.Background(), root, pass.Documents); err != nil || len(got) != 0 {
		t.Fatalf("valid: %#v %v", got, err)
	}
	pass = stePass(root, "Don't utilize the API.\n")
	got, err := program.Run(context.Background(), root, pass.Documents)
	if err != nil || len(got) != 3 {
		t.Fatalf("preset: %#v %v", got, err)
	}
	putPack(t, root, "ste100-dictionary.yaml", "words: []")
	p.Suppressions = []Suppression{{Rule: "text.ste100.dictionary", Path: "guide.md", Reason: "fixture"}}
	program, err = r.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := program.Run(context.Background(), root, pass.Documents); err == nil {
		t.Fatal("operational error suppressed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, err := steFactory("text.ste100.dictionary")(coverageOptions(t, "dictionary-file: missing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	c.Analyze(ctx, pass)
	empty := interfaces.NewPass(nil)
	c.Analyze(context.Background(), empty)
	if empty.Err() != nil {
		t.Fatal("empty pass attempted file load")
	}
	duplicate := NewRegistry()
	if err := registerSTE(duplicate); err != nil {
		t.Fatal(err)
	}
	if err := registerSTE(duplicate); err == nil {
		t.Fatal("duplicate STE registration")
	}
	if c.ID() != "text.ste100.dictionary" {
		t.Fatal(c.ID())
	}
	if _, err := steFactory("text.ste100.grammar")(yaml.Node{Kind: yaml.SequenceNode}); err == nil {
		t.Fatal("sequence options accepted")
	}
}
