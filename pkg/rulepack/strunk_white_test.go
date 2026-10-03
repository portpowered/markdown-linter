package rulepack

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"gopkg.in/yaml.v3"
)

func TestStrunkWhiteEachRule(t *testing.T) {
	cases := []struct {
		name, bad, good, innocent string
	}{
		{"needless-words", "Run in order to verify.", "Run to verify.", "Put the records in order."},
		{"fancy-words", "Utilize the cache.", "Use the cache.", "The utilization_metric is `utilization`."},
		{"qualifiers", "A very large result.", "A result of 20 MB.", "Every result is cached."},
		{"negative-phrases", "This is not uncommon.", "This is common.", "Do not retry failed writes."},
		{"passive-voice", "The record was created by the worker.", "The worker created the record.", "The worker was ready by noon."},
		{"usage", "It should of worked.", "It should have worked.", "A person of interest arrived."},
		{"exclamations", "Done!", "Done.", "![diagram](image.png)"},
		{"existential-openings", "There are three records.", "Three records exist.", "Move there is not a command."},
		{"redundant-pairs", "Review the final outcome.", "Review the outcome.", "This is the final record."},
		{"correlative-pairs", "Use both red or blue.", "Use both red and blue.", "Use both red and blue or choose green."},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			id := "text.strunk-white." + tt.name
			bad := checkMarkdown(t, id, tt.bad+"\n", "")
			if len(bad) != 1 || bad[0].CheckID != id || len(bad[0].SuggestedFixes) != 0 {
				t.Fatalf("expected one review warning without a rewrite: %#v", bad)
			}
			if got := checkMarkdown(t, id, tt.good+"\n", ""); len(got) != 0 {
				t.Fatalf("valid expression flagged: %#v", got)
			}
			if got := checkMarkdown(t, id, tt.innocent+"\n", ""); len(got) != 0 {
				t.Fatalf("false positive: %#v", got)
			}
			protected := "`" + tt.bad + "`\n\n```text\n" + tt.bad + "\n```\n\n[Link](https://example.test/" + strings.ReplaceAll(tt.bad, " ", "-") + ")\n"
			if got := checkMarkdown(t, id, protected, ""); len(got) != 0 {
				t.Fatalf("code or URL flagged: %#v", got)
			}
		})
	}
}

func TestStrunkWhiteScopesExceptionsAndPairs(t *testing.T) {
	cases := []struct {
		name, source, options string
		count                 int
	}{
		{"needless-words", "In *order* to verify.\n", "", 1},
		{"needless-words", "In\norder to verify.\n", "", 1},
		{"needless-words", "In\n\norder to verify.\n", "", 0},
		{"needless-words", "In `sample` order to verify.\n", "", 0},
		{"needless-words", "In ![sample](diagram.png) order to verify.\n", "", 0},
		{"qualifiers", "---\ntitle: very large\n---\n\n# Really large\n\nQuite useful.\n", "scope: heading", 1},
		{"qualifiers", "Very large and really useful.\n", "allow: [very]", 1},
		{"negative-phrases", "Not uncommon.\n", "allow: [not uncommon]", 0},
		{"passive-voice", "The record is automatically returned by the worker.\n", "", 1},
		{"passive-voice", "The record is stored.\n", "", 0},
		{"usage", "Irregardless, it could of worked.\n", "", 2},
		{"existential-openings", "Done. There is a result.\n", "", 1},
		{"existential-openings", "There is a result.\n", "allow: [there is]", 0},
		{"existential-openings", "Done. There is a result.\n", "allow: [there is]", 0},
		{"correlative-pairs", "Either red and blue. Neither red or blue.\n", "", 2},
		{"correlative-pairs", "Either red or blue and green. Neither red nor blue or green.\n", "", 0},
		{"correlative-pairs", "Both\nred and blue or green.\n", "", 0},
		{"correlative-pairs", "Both red\n\nor blue.\n", "", 0},
		{"qualifiers", "https://example.test/very `really`\n", "", 0},
		{"qualifiers", "---\ntitle: very large\n", "", 0},
	}
	for _, tt := range cases {
		t.Run(tt.name+tt.source, func(t *testing.T) {
			if got := checkMarkdown(t, "text.strunk-white."+tt.name, tt.source, tt.options); len(got) != tt.count {
				t.Fatalf("want %d got %#v", tt.count, got)
			}
		})
	}
}

func TestStrunkWhiteOptionsValidation(t *testing.T) {
	for _, options := range []string{"scope: code", "max: 5", "allow: true", "scope: ''"} {
		var n yaml.Node
		if err := yaml.Unmarshal([]byte(options), &n); err != nil {
			t.Fatal(err)
		}
		if _, err := strunkFactory("text.strunk-white.qualifiers", strunkSpecs[2])(*n.Content[0]); err == nil {
			t.Fatalf("invalid options accepted: %s", options)
		}
	}
}

func TestStrunkWhiteRegistrationAndIdentity(t *testing.T) {
	r := NewRegistry()
	if err := registerStrunkWhite(r); err != nil {
		t.Fatal(err)
	}
	if err := registerStrunkWhite(r); err == nil {
		t.Fatal("duplicate registration accepted")
	}
	check, err := strunkFactory("text.strunk-white.qualifiers", strunkSpecs[2])(yaml.Node{})
	if err != nil || check.ID() != "text.strunk-white.qualifiers" {
		t.Fatalf("incorrect analyzer identity: %v", err)
	}
}

func TestStrunkWhitePackAndComposition(t *testing.T) {
	r := NewRegistry()
	if err := RegisterStock(r); err != nil {
		t.Fatal(err)
	}
	preset, ok := Preset("text:strunk-white")
	if !ok || len(preset.Rules) != 10 {
		t.Fatalf("missing ten-rule preset: %#v", preset)
	}
	root := t.TempDir()
	config := filepath.Join(root, "rules.yaml")
	if err := os.WriteFile(config, []byte("version: 1\nextends: [markdown:recommended, text:strunk-white]\noverrides:\n  - id: text.strunk-white.qualifiers\n    enabled: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	pack, err := Load(config, root)
	if err != nil {
		t.Fatal(err)
	}
	program, err := r.Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "doc.md")
	if err = os.WriteFile(file, []byte("# Example\n\nUtilize the really useful cache in order to read the final outcome.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	doc, err := engine.New().ParseFile(file)
	if err != nil {
		t.Fatal(err)
	}
	findings, err := program.Run(context.Background(), root, []*interfaces.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 3 {
		t.Fatalf("expected three enabled Strunk checks: %#v", findings)
	}
	for _, finding := range findings {
		if finding.Severity != interfaces.SeverityWarning || !strings.HasPrefix(finding.CheckID, "text.strunk-white.") || finding.Origin != "preset:text:strunk-white" {
			t.Fatalf("missing warning attribution: %#v", finding)
		}
	}
	pass := interfaces.NewPass([]*interfaces.Document{doc})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	check := strunkCheck{spec: strunkSpecs[0], pattern: regexp.MustCompile(strunkSpecs[0].pattern)}
	check.Analyze(ctx, pass)
	if len(pass.Diagnostics()) != 0 {
		t.Fatal("canceled check emitted findings")
	}
}
