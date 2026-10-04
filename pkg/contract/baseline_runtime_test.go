package contract

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/portpowered/markdown-linter/pkg/rulepack"
	"gopkg.in/yaml.v3"
)

func TestCanonicalBaselineControlsBuiltinsAndSets(t *testing.T) {
	r := registry(t)
	// Registry documentation is deliberately inconsistent. Canonical YAML
	// supplies v2 metadata rather than the legacy descriptor's defaults.
	descriptor, _ := r.Describe("markdown.line-length")
	descriptor.Defaults = map[string]any{"max": 1}
	descriptor.Presets = []string{"legacy:unexpected"}
	if err := r.SetDescriptor(descriptor); err != nil {
		t.Fatal(err)
	}
	p, err := Decode([]byte("version: 2\napply: []\n"), "policy.yaml", r)
	if err != nil {
		t.Fatal(err)
	}
	expectedSets := map[string][]string{}
	for _, definition := range RuleDefinitions() {
		id := definition.Descriptor["id"].(string)
		if _, installed := r.Describe(id); !installed {
			if _, exists := p.builtin[id]; exists {
				t.Errorf("uninstalled adapter %s was added to stock rules", id)
			}
			continue
		}
		stock := p.builtin[id]
		var options map[string]any
		if err := stock.Options.Decode(&options); err != nil {
			t.Fatal(err)
		}
		if stock.Check != id || string(stock.Severity) != definition.RecommendedSeverity {
			t.Errorf("%s metadata diverges from canonical definition", id)
		}
		if err := validateJSON(definition.Descriptor["parametersSchema"].(map[string]any), options); err != nil && len(definition.Presets) > 0 {
			t.Errorf("%s active builtin defaults: %v", id, err)
		}
		for key, expected := range definition.Defaults {
			// yaml.Node preserves numeric YAML types, while RuleDefinitions
			// returns a JSON-safe clone. Compare their serialized values.
			if fmt.Sprint(options[key]) != fmt.Sprint(expected) {
				t.Errorf("%s option %s: got %v, want %v", id, key, options[key], expected)
			}
		}
		for _, preset := range definition.Presets {
			expectedSets[preset] = append(expectedSets[preset], id)
		}
		for _, instance := range definition.BundledRules {
			stock := p.builtin[instance.ID]
			if stock.Check != instance.Check || string(stock.Severity) != instance.Severity {
				t.Errorf("%s bundled instance differs from its canonical metadata", instance.ID)
			}
			if err := stock.Options.Decode(&options); err != nil {
				t.Fatal(err)
			}
			for key, expected := range instance.Options {
				if !reflect.DeepEqual(options[key], expected) && fmt.Sprint(options[key]) != fmt.Sprint(expected) {
					t.Errorf("%s option %s: got %v, want %v", instance.ID, key, options[key], expected)
				}
			}
			for _, preset := range instance.Presets {
				expectedSets[preset] = append(expectedSets[preset], instance.ID)
			}
		}
	}
	if !reflect.DeepEqual(sortedKeys(expectedSets), sortedKeys(p.bundledSets)) {
		t.Fatalf("public sets differ: got %v, want %v", sortedKeys(p.bundledSets), sortedKeys(expectedSets))
	}
	for name, expected := range expectedSets {
		_, bindings, _, err := p.expand([]string{name})
		if err != nil {
			t.Fatal(err)
		}
		actual := []string{}
		for _, binding := range bindings {
			if binding.On != "document" || binding.Scope != "" {
				t.Errorf("unexpected stock binding: %+v", binding)
			}
			actual = append(actual, binding.Rule)
		}
		sort.Strings(actual)
		sort.Strings(expected)
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s: got %v, want %v", name, actual, expected)
		}
	}
}

func TestCanonicalDefaultsAndExplicitPatchesExecute(t *testing.T) {
	p := one(t, "markdown.line-length", "{}", "document")
	if got := fmt.Sprint(p.Rules["test"].Options["max"]); got != "120" {
		t.Fatalf("canonical default max: %s", got)
	}
	if result := lint(t, p, strings.Repeat("A", 121)+"\n"); result.Summary.Warnings != 0 || result.Summary.Errors != 1 {
		t.Fatalf("default limit did not execute: %+v", result.Summary)
	}
	p = one(t, "markdown.line-length", "{max: 130}", "document")
	if result := lint(t, p, strings.Repeat("A", 121)+"\n"); result.ExitCode != 0 {
		t.Fatalf("explicit options did not override defaults: %+v", result.Diagnostics)
	}
	portos := compile(t, "version: 2\napply: [{use: [portos]}]\n")
	for _, text := range []string{"This is load bearing.\n", "A prose-dash occurs here.\n"} {
		result := lint(t, portos, text)
		if result.Summary.Errors == 0 {
			t.Fatalf("Portos named instance no longer reports %q", text)
		}
	}
	if result := lint(t, portos, "# Guide\n\nA short sentence.\n"); result.ExitCode != 0 {
		t.Fatalf("Portos accepted example: %+v", result.Diagnostics)
	}
	ste := compile(t, "version: 2\napply: [{use: [\"text:ste100\"]}]\n")
	if result := lint(t, ste, "Use the API.\n"); result.ExitCode != 0 {
		t.Fatalf("grammar preset requires no external dictionary: %+v", result.Diagnostics)
	}
	if result := lint(t, ste, "Don't use the API.\n"); result.Summary.Errors != 1 {
		t.Fatalf("canonical contraction default did not execute: %+v", result.Diagnostics)
	}
}

func TestCanonicalBaselineLeavesCustomRegistryIndependent(t *testing.T) {
	r := rulepack.NewRegistry()
	if err := r.Register("customer.check", func(_ yaml.Node) (interfaces.Analyzer, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	p, err := Decode([]byte("version: 2\napply: []\n"), "policy.yaml", r)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.builtin) != 0 || len(p.bundledSets) != 0 {
		t.Fatalf("uninstalled stock rules leaked into custom registry: %v", p.builtin)
	}
}

func TestEveryCanonicalPublicSetRunsWithoutPolicyImports(t *testing.T) {
	baseline := compile(t, "version: 2\napply: []\n")
	for _, set := range sortedKeys(baseline.bundledSets) {
		t.Run(set, func(t *testing.T) {
			p := compile(t, fmt.Sprintf("version: 2\napply: [{use: [%q]}]\n", set))
			result := lint(t, p, "# Guide\n\nUse the API.\n")
			if !result.Complete {
				t.Fatalf("standalone set requires unavailable policy: %+v", result.Diagnostics)
			}
			for _, diagnostic := range result.Diagnostics {
				if strings.HasPrefix(diagnostic.Code, "execution.") {
					t.Errorf("standalone set execution failed: %+v", diagnostic)
				}
			}
		})
	}
}
