package rulepack_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/portpowered/markdown-linter/pkg/rulepack"
	"gopkg.in/yaml.v3"
)

type customerCheck struct {
	Message string `yaml:"message"`
}

func (c customerCheck) ID() string { return "implementation-id" }
func (c customerCheck) Analyze(_ context.Context, pass *interfaces.Pass) {
	for _, doc := range pass.Documents {
		pass.Report(interfaces.NewDiagnostic(doc.Path, 1, -1, -1, c.ID(), c.Message, interfaces.SeverityError))
	}
}

func TestCustomerAndStockChecksShareConfigurationAndDiagnostics(t *testing.T) {
	registry := rulepack.NewRegistry()
	if err := rulepack.RegisterStock(registry); err != nil {
		t.Fatal(err)
	}
	err := registry.Register("customer.check", func(node yaml.Node) (interfaces.Analyzer, error) {
		var check customerCheck
		if err := rulepack.DecodeOptions(node, &check); err != nil {
			return nil, err
		}
		return check, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	pack, err := rulepack.Decode(strings.NewReader(`version: 1
rules:
  - id: customer.warning
    check: customer.check
    severity: warning
    options: {message: hello}
  - id: required-purpose
    check: markdown.required-heading
    options: {heading: Purpose, level: 2}
suppressions:
  - rule: customer.warning
    path: ignored.md
    reason: fixture exception
`))
	if err != nil {
		t.Fatal(err)
	}
	program, err := registry.Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	var docs []*interfaces.Document
	for _, name := range []string{"included.md", "ignored.md"} {
		file := filepath.Join(root, name)
		if err := os.WriteFile(file, []byte("# Title\n"), 0600); err != nil {
			t.Fatal(err)
		}
		doc, err := engine.New().ParseFile(file)
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, doc)
	}
	diagnostics, err := program.Run(context.Background(), root, docs)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 3 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	warnings := 0
	for _, d := range diagnostics {
		if d.RuleID == "customer.warning" {
			warnings++
			if d.Severity != interfaces.SeverityWarning || d.Message != "hello" {
				t.Fatalf("custom result: %#v", d)
			}
		}
	}
	if warnings != 1 {
		t.Fatalf("warnings = %d", warnings)
	}
}

func TestInvalidConfigurationFailsBeforeExecution(t *testing.T) {
	registry := rulepack.NewRegistry()
	if err := rulepack.RegisterStock(registry); err != nil {
		t.Fatal(err)
	}
	cases := []string{
		"version: 2\nrules: []",
		"version: 1\nunknown: true",
		"version: 1\n---\nversion: 1",
		"version: 1\nrules: [{id: x, check: missing}]",
		"version: 1\nrules: [{id: x, check: markdown.formatting, options: {typo: true}}]",
		"version: 1\nrules: [{id: x, check: markdown.formatting, severity: loud}]",
		"version: 1\nrules: [{id: x, check: markdown.formatting}, {id: x, check: markdown.formatting}]",
		"version: 1\nrules: [{id: x, check: markdown.required-heading, options: {heading: Purpose, level: 9}}]",
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			pack, err := rulepack.Decode(strings.NewReader(input))
			if err == nil {
				_, err = registry.Compile(pack)
			}
			if err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}
