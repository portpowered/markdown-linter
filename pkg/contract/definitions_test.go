package contract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"testing/fstest"

	"gopkg.in/yaml.v3"
)

func TestCanonicalRuleDefinitions(t *testing.T) {
	r := registry(t)
	definitions := RuleDefinitions()
	descriptors := CheckDescriptors(r)
	if len(definitions) != len(descriptors) || len(definitions) != len(r.Catalog())+5 {
		t.Fatal("canonical definitions must cover every installed check")
	}
	seen := map[string]bool{}
	for _, definition := range definitions {
		id := definition.Descriptor["id"].(string)
		t.Run(id, func(t *testing.T) {
			if seen[id] {
				t.Fatal("duplicate check")
			}
			seen[id] = true
			if err := validateJSON(RuleDefinitionSchema(), definition); err != nil {
				t.Fatal(err)
			}
			schema := definition.Descriptor["parametersSchema"].(map[string]any)
			params := definition.Documentation["parameters"].(map[string]any)
			props := schema["properties"].(map[string]any)
			if !reflect.DeepEqual(sortedKeys(params), sortedKeys(props)) {
				t.Fatal("parameter documentation must cover exactly the option schema")
			}
			options := definition.Documentation["example-options"].(map[string]any)
			if err := validateJSON(schema, options); err != nil {
				t.Fatal("documented options", err)
			}
			config := map[string]any{"version": 2, "rules": map[string]any{"example.rule": map[string]any{"check": id, "options": options}}, "sets": map[string]any{"example": map[string]any{"rules": []any{map[string]any{"rule": "example.rule", "on": definition.Documentation["on"]}}}}, "apply": []any{map[string]any{"use": []string{"example"}}}}
			source, err := yaml.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			p, err := Decode(source, "example.yaml", r)
			if err != nil {
				t.Fatal("documented v2 configuration", err)
			}
			if id == "core.limit" || id == "core.match" || id == "core.sequence" || id == "markdown.table-schema" || id == "mermaid.flowchart" {
				examples := definition.Documentation["example"].(map[string]any)
				for _, sample := range []struct {
					key  string
					exit int
				}{{"bad", 1}, {"good", 0}} {
					if report := lint(t, p, examples[sample.key].(string)); report.ExitCode != sample.exit {
						t.Fatalf("%s example: %+v", sample.key, report.Diagnostics)
					}
				}
			}
		})
	}
	definitions[0].Descriptor["id"] = "mutated"
	if RuleDefinitions()[0].Descriptor["id"] == "mutated" {
		t.Fatal("caller changed embedded baseline")
	}
	data, err := json.Marshal(RuleDefinitionSchema())
	if err != nil || len(data) == 0 {
		t.Fatal(err)
	}
}

func TestReadDefinitionsRejectsMalformedBaseline(t *testing.T) {
	for name, source := range map[string]string{
		"yaml":      "descriptor: [",
		"duplicate": "descriptor: {id: a, id: a}",
		"unknown":   "descriptor: {id: a}\nunknown: true",
		"identity":  "descriptor: {id: b}",
		"type":      "descriptor: {id: a}\ndefaults: []",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readDefinitions(fstest.MapFS{"ruledefs/a.yaml": &fstest.MapFile{Data: []byte(source)}}); err == nil {
				t.Fatal("malformed baseline accepted")
			}
		})
	}
	if definitions, err := readDefinitions(fstest.MapFS{}); err != nil || len(definitions) != 0 {
		t.Fatal(fmt.Sprint(definitions, err))
	}
}
