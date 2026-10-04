package rulepack

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"gopkg.in/yaml.v3"
)

func TestSiteRuleConfigurations(t *testing.T) {
	names, err := filepath.Glob("../contract/ruledefs/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	type reference struct {
		Options yaml.Node `yaml:"example-options"`
		Example struct {
			Bad  string `yaml:"bad"`
			Good string `yaml:"good"`
		} `yaml:"example"`
	}
	references := map[string]reference{}
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var definition struct {
			Descriptor struct {
				ID string `yaml:"id"`
			} `yaml:"descriptor"`
			Documentation reference `yaml:"documentation"`
		}
		if err := yaml.Unmarshal(data, &definition); err != nil {
			t.Fatal(err)
		}
		references[definition.Descriptor.ID] = definition.Documentation
	}
	registry := NewRegistry()
	if err := RegisterStock(registry); err != nil {
		t.Fatal(err)
	}
	for _, descriptor := range registry.Catalog() {
		t.Run(descriptor.ID, func(t *testing.T) {
			if _, exists := references[descriptor.ID]; !exists {
				t.Fatal("canonical rule definition missing")
			}
			program, err := registry.Compile(Pack{Version: 1, Rules: []Rule{{ID: descriptor.ID, Check: descriptor.ID, Options: references[descriptor.ID].Options}}})
			if err != nil {
				t.Fatal(err)
			}
			for _, sample := range []struct {
				source string
				fails  bool
			}{{references[descriptor.ID].Example.Bad, true}, {references[descriptor.ID].Example.Good, false}} {
				root := t.TempDir()
				if err := os.Mkdir(filepath.Join(root, "docs"), 0700); err != nil {
					t.Fatal(err)
				}
				file := filepath.Join(root, "docs", "guide.md")
				if err := os.WriteFile(file, []byte(sample.source), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "docs", "diagram.png"), []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "docs", "new.md"), []byte("# Destination\n"), 0600); err != nil {
					t.Fatal(err)
				}
				doc, err := engine.New().ParseFile(file)
				if err != nil {
					t.Fatal(err)
				}
				documents := []*interfaces.Document{doc}
				if descriptor.ID == "markdown.link-relocation" {
					extra, err := engine.New().ParseFile(filepath.Join(root, "docs", "new.md"))
					if err != nil {
						t.Fatal(err)
					}
					documents = append(documents, extra)
				}
				if descriptor.ID == "markdown.doc-id-unique" {
					other := filepath.Join(root, "docs", "other.md")
					if err := os.WriteFile(other, []byte("---\ndoc-id: DOC-1\n---\n# Other\n"), 0600); err != nil {
						t.Fatal(err)
					}
					extra, err := engine.New().ParseFile(other)
					if err != nil {
						t.Fatal(err)
					}
					documents = append(documents, extra)
				}
				findings, err := program.Run(context.Background(), root, documents)
				if err != nil {
					t.Fatal(err)
				}
				if (len(findings) > 0) != sample.fails {
					t.Fatalf("documented example expected fails=%t, got %#v", sample.fails, findings)
				}
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
