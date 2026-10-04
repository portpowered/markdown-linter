package rulepack

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"gopkg.in/yaml.v3"
)

func TestSiteRuleConfigurations(t *testing.T) {
	data, err := os.ReadFile("../../docs/rule-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var references map[string]struct {
		Options json.RawMessage `json:"example-options"`
	}
	if err := json.Unmarshal(data, &references); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := RegisterStock(registry); err != nil {
		t.Fatal(err)
	}
	if len(references) != len(registry.Catalog()) {
		t.Fatal("rule reference must cover the entire registry")
	}
	for _, descriptor := range registry.Catalog() {
		t.Run(descriptor.ID, func(t *testing.T) {
			var options yaml.Node
			if err := yaml.Unmarshal(references[descriptor.ID].Options, &options); err != nil {
				t.Fatal(err)
			}
			if _, err := registry.Compile(Pack{Version: 1, Rules: []Rule{{ID: descriptor.ID, Check: descriptor.ID, Options: *options.Content[0]}}}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLibraryGuideIntegration(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "guide.md")
	if err := os.WriteFile(filename, []byte("# Guide\n\n### Skipped level\n"), 0600); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := RegisterStock(registry); err != nil {
		t.Fatal(err)
	}
	pack, err := Load("markdown:recommended", root)
	if err != nil {
		t.Fatal(err)
	}
	program, err := registry.Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	if err := interfaces.CheckPathRoot(root, filename); err != nil {
		t.Fatal(err)
	}
	doc, err := engine.New().ParseFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	findings, err := program.Run(context.Background(), root, []*interfaces.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range findings {
		if finding.CheckID == "markdown.heading-order" && finding.Line == 3 {
			return
		}
	}
	t.Fatalf("public integration missed heading defect: %#v", findings)
}
