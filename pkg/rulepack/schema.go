package rulepack

import "strings"

// ConfigurationSchema derives completion/validation branches from the registry.
// Runtime compilation additionally validates references, options and kinds.
func (r *Registry) ConfigurationSchema() map[string]any {
	text := map[string]any{"type": "string", "minLength": 1}
	texts := map[string]any{"type": "array", "items": text}
	rules := []any{}
	for _, d := range r.Catalog() {
		props := map[string]any{}
		for key, kind := range d.Options {
			value := map[string]any{}
			switch {
			case strings.Contains(kind, "map[string]string"):
				value["type"] = "object"
				value["additionalProperties"] = map[string]string{"type": "string"}
			case strings.HasPrefix(kind, "[]"):
				value["type"] = "array"
				value["items"] = map[string]string{"type": "string"}
			case strings.Contains(kind, "bool"):
				value["type"] = "boolean"
			case kind == "int":
				value["type"] = "integer"
			default:
				if kind == "string" {
					value["type"] = "string"
				}
			}
			props[key] = value
		}
		requiredOptions := []string{}
		switch d.ID {
		case "text.terminology":
			requiredOptions = []string{"terms"}
		case "text.spelling":
			requiredOptions = []string{"language", "dictionary"}
		case "markdown.required-heading":
			requiredOptions = []string{"heading", "level"}
		}
		options := map[string]any{"type": "object", "properties": props, "additionalProperties": d.Kind == "custom"}
		if len(requiredOptions) > 0 {
			options["required"] = requiredOptions
		}
		if d.ID == "text.ste100.dictionary" {
			options["anyOf"] = []any{
				map[string]any{"required": []string{"dictionary"}, "properties": map[string]any{"dictionary": map[string]any{"minItems": 1}}},
				map[string]any{"required": []string{"dictionary-file"}, "properties": map[string]any{"dictionary-file": map[string]any{"minLength": 1}}},
			}
		}
		if d.ID == "text.matcher" {
			options["anyOf"] = []any{
				map[string]any{"required": []string{"patterns"}, "properties": map[string]any{"patterns": map[string]any{"minItems": 1}}},
				map[string]any{"required": []string{"banned-words"}, "properties": map[string]any{"banned-words": map[string]any{"minItems": 1}}},
				map[string]any{"required": []string{"banned-characters"}, "properties": map[string]any{"banned-characters": map[string]any{"minLength": 1}}},
			}
		}
		rule := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"id", "check"}, "properties": map[string]any{"id": text, "check": map[string]any{"const": d.ID}, "description": map[string]string{"type": "string"}, "enabled": map[string]string{"type": "boolean"}, "severity": map[string]any{"enum": []string{"error", "warning", "info"}}, "include": texts, "exclude": texts, "options": options}}
		if len(requiredOptions) > 0 || d.ID == "text.ste100.dictionary" || d.ID == "text.matcher" {
			rule["required"] = []string{"id", "check", "options"}
		}
		rules = append(rules, rule)
	}
	var ruleItems any = false
	if len(rules) > 0 {
		ruleItems = map[string]any{"oneOf": rules}
	}
	overrides := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"id"}, "properties": map[string]any{"id": text, "enabled": map[string]string{"type": "boolean"}, "severity": map[string]any{"enum": []string{"error", "warning", "info"}}, "include": texts, "exclude": texts, "options": map[string]string{"type": "object"}}}
	suppressions := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"rule", "path", "reason"}, "properties": map[string]any{"rule": text, "path": text, "reason": map[string]any{"type": "string", "pattern": `\S`}, "line": map[string]any{"type": "integer", "minimum": 0}}}
	return map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "title": CommandName() + " configuration", "type": "object", "additionalProperties": false, "required": []string{"version"}, "properties": map[string]any{"version": map[string]any{"const": 1}, "extends": texts, "rules": map[string]any{"type": "array", "items": ruleItems}, "overrides": map[string]any{"type": "array", "items": overrides}, "suppressions": map[string]any{"type": "array", "items": suppressions}}}
}
