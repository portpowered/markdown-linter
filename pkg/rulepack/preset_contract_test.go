package rulepack

import (
	"gopkg.in/yaml.v3"
	"testing"
)

func TestYAMLOptionCopyPreservesAliasIndependence(t *testing.T) {
	var source yaml.Node
	if err := yaml.Unmarshal([]byte("base: &base {name: Original}\nuse: *base\n"), &source); err != nil {
		t.Fatal(err)
	}
	copied := cloneNode(source)
	originalMapping := source.Content[0]
	copiedMapping := copied.Content[0]
	originalValue := originalMapping.Content[1]
	copiedValue := copiedMapping.Content[1]
	copiedAlias := copiedMapping.Content[3]
	if copiedAlias.Alias != copiedValue || copiedAlias.Alias == originalValue {
		t.Fatal("alias targets must point inside independent copy")
	}
	copiedValue.Content[1].Value = "Changed"
	var original, changed map[string]map[string]string
	if err := source.Decode(&original); err != nil {
		t.Fatal(err)
	}
	if err := copied.Decode(&changed); err != nil {
		t.Fatal(err)
	}
	if original["base"]["name"] != "Original" || original["use"]["name"] != "Original" || changed["base"]["name"] != "Changed" || changed["use"]["name"] != "Changed" {
		t.Fatalf("alias copy changed source or lost shared meaning: %#v %#v", original, changed)
	}
}

func TestYAMLOptionCopyHandlesRecursiveAliasGraph(t *testing.T) {
	var source yaml.Node
	if err := yaml.Unmarshal([]byte("&recursive [*recursive]"), &source); err != nil {
		t.Fatal(err)
	}
	copied := cloneNode(source)
	sequence := copied.Content[0]
	alias := sequence.Content[0]
	if alias.Alias != sequence || alias.Alias == source.Content[0] {
		t.Fatal("recursive aliases must retain their cycle within copied graph")
	}
	sequence.Style = yaml.FlowStyle
	if source.Content[0].Style == 0 {
		t.Fatal("parser lost flow-style source fixture")
	}
	sequence.Anchor = "changed"
	if source.Content[0].Anchor != "recursive" {
		t.Fatal("copied graph mutated source")
	}
}

func TestPresetResultsAreIndependent(t *testing.T) {
	first, ok := Preset(DefaultPresetName())
	if !ok || len(first.Rules) == 0 || len(first.Imports) == 0 {
		t.Fatal("default preset missing rules or imports")
	}
	originalID := first.Rules[0].ID
	originalHash := first.Imports[0].Hash
	first.Rules[0].ID = "changed"
	first.Imports[0].Hash = "changed"
	first.Rules[0].Include = []string{"changed/**"}
	second, ok := Preset(DefaultPresetName())
	if !ok || second.Rules[0].ID != originalID || second.Imports[0].Hash != originalHash || len(second.Rules[0].Include) != 0 {
		t.Fatal("mutating a preset result changed later callers")
	}
	if _, ok := Preset("missing:preset"); ok {
		t.Fatal("unknown preset returned a pack")
	}
	if _, ok := rawPreset("missing:preset"); ok {
		t.Fatal("unknown raw preset returned a pack")
	}
}
