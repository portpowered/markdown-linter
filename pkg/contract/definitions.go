package contract

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"

	"gopkg.in/yaml.v3"
)

// ruleFiles is installed tool metadata, never a customer policy import.
//
//go:embed ruledefs/*.yaml
var ruleFiles embed.FS

type RuleDefinition struct {
	Descriptor          map[string]any          `yaml:"descriptor" json:"descriptor"`
	Defaults            map[string]any          `yaml:"defaults" json:"defaults"`
	RecommendedSeverity string                  `yaml:"recommendedSeverity" json:"recommendedSeverity"`
	Presets             []string                `yaml:"presets" json:"presets"`
	Documentation       map[string]any          `yaml:"documentation" json:"documentation"`
	BundledRules        []BundledRuleDefinition `yaml:"bundledRules,omitempty" json:"bundledRules,omitempty"`
}

// BundledRuleDefinition configures a named instance supplied by a public set.
type BundledRuleDefinition struct {
	ID       string         `yaml:"id" json:"id"`
	Check    string         `yaml:"check" json:"check"`
	Options  map[string]any `yaml:"options" json:"options"`
	Severity string         `yaml:"severity" json:"severity"`
	Presets  []string       `yaml:"presets" json:"presets"`
}

func readDefinitions(files fs.FS) ([]RuleDefinition, error) {
	names, err := fs.Glob(files, "ruledefs/*.yaml")
	if err != nil {
		return nil, err
	}
	result := []RuleDefinition{}
	for _, name := range names {
		source, err := fs.ReadFile(files, name)
		if err != nil {
			return nil, err
		}
		var tree yaml.Node
		if err := yaml.Unmarshal(source, &tree); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if err := strictTree(&tree); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		var definition RuleDefinition
		decoder := yaml.NewDecoder(bytes.NewReader(source))
		decoder.KnownFields(true)
		if err := decoder.Decode(&definition); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		id, _ := definition.Descriptor["id"].(string)
		if id+".yaml" != path.Base(name) {
			return nil, fmt.Errorf("%s: descriptor ID must match filename", name)
		}
		result = append(result, definition)
	}
	return result, nil
}

var installedDefinitions = func() []RuleDefinition {
	definitions, err := readDefinitions(ruleFiles)
	if err != nil {
		panic(err)
	}
	return definitions
}()

// RuleDefinitions returns independent copies of the canonical YAML baseline.
func RuleDefinitions() []RuleDefinition {
	data, _ := json.Marshal(installedDefinitions)
	var copies []RuleDefinition
	_ = json.Unmarshal(data, &copies)
	return copies
}

func RuleDefinitionSchema() map[string]any {
	text := textSchema()
	example := closed(map[string]any{"language": enum("markdown", "yaml"), "bad": text, "good": text, "explanation": text}, "language", "bad", "good", "explanation")
	docs := closed(map[string]any{"summary": text, "parameters": map[string]any{"type": "object", "additionalProperties": text}, "example-options": map[string]any{"type": "object"}, "example": example, "on": text, "notes": map[string]string{"type": "string"}}, "summary", "parameters", "example-options", "example", "on")
	instance := closed(map[string]any{"id": text, "check": text, "options": map[string]any{"type": "object"}, "severity": enum("error", "warning", "info"), "presets": array(text)}, "id", "check", "options", "severity", "presets")
	schema := closed(map[string]any{"descriptor": DescriptorSchema(false), "defaults": map[string]any{"type": "object"}, "recommendedSeverity": enum("error", "warning", "info"), "presets": array(text), "documentation": docs, "bundledRules": array(instance)}, "descriptor", "defaults", "recommendedSeverity", "presets", "documentation")
	schema["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	return schema
}
