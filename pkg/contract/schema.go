package contract

import (
	"github.com/portpowered/markdown-linter/pkg/rulepack"
	"strings"
)

func closed(props map[string]any, required ...string) map[string]any {
	r := map[string]any{"type": "object", "additionalProperties": false, "properties": props}
	if len(required) > 0 {
		r["required"] = required
	}
	return r
}
func enum(values ...string) map[string]any { return map[string]any{"enum": values} }
func array(items any) map[string]any       { return map[string]any{"type": "array", "items": items} }
func textSchema() map[string]any           { return map[string]any{"type": "string", "minLength": 1} }
func optionSchemas(registry *rulepack.Registry) map[string]any {
	out := map[string]any{}
	for _, definition := range RuleDefinitions() {
		id := definition.Descriptor["id"].(string)
		if _, registered := registry.Describe(id); registered || has([]string{"core.limit", "core.match", "core.sequence", "markdown.table-schema", "mermaid.flowchart"}, id) {
			out[id] = definition.Descriptor["parametersSchema"]
		}
	}
	legacy := registry.ConfigurationSchema()
	props := legacy["properties"].(map[string]any)
	items := props["rules"].(map[string]any)["items"].(map[string]any)
	for _, branch := range items["oneOf"].([]any) {
		rule := branch.(map[string]any)
		ps := rule["properties"].(map[string]any)
		id := ps["check"].(map[string]any)["const"].(string)
		options := ps["options"].(map[string]any)
		if _, baseline := out[id]; baseline {
			continue
		}
		out[id] = options
	}
	return out
}

// ConfigSchema is generated from the installed check registry, never fetched.
func ConfigSchema(registry *rulepack.Registry) map[string]any {
	return ConfigSchemaWithCapabilities(registry, NewCapabilities())
}
func ConfigSchemaWithCapabilities(registry *rulepack.Registry, capabilities *Capabilities) map[string]any {
	text := textSchema()
	texts := array(text)
	name := map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9.-]*$"}
	branches := []any{}
	options := optionSchemas(registry)
	limit := options["core.limit"].(map[string]any)
	names := append(sortedKeys(measures), sortedKeys(capabilities.Measures)...)
	limit["properties"].(map[string]any)["measure"] = enum(names...)
	for _, id := range sortedKeys(options) {
		rule := closed(map[string]any{"check": map[string]any{"const": id}, "options": options[id], "severity": enum("error", "warning", "info"), "on-unknown": enum("fail", "report"), "description": map[string]string{"type": "string"}}, "check")
		if has([]string{"core.limit", "core.match", "core.sequence", "markdown.table-schema"}, id) {
			rule["required"] = []string{"check", "options"}
		}
		branches = append(branches, rule)
	}
	branches = append(branches, closed(map[string]any{"extends": name, "options": map[string]string{"type": "object"}, "severity": enum("error", "warning", "info"), "on-unknown": enum("fail", "report"), "description": map[string]string{"type": "string"}}, "extends"))
	binding := closed(map[string]any{"rule": name, "on": map[string]any{"type": "string", "pattern": "^(document|each (heading|section|paragraph|sentence|table|codeblock|source-line))$"}, "scope": enum("direct", "subtree")}, "rule", "on")
	sets := closed(map[string]any{"description": map[string]string{"type": "string"}, "use": texts, "rules": array(binding), "templates": array(name)})
	apply := closed(map[string]any{"files": map[string]any{"type": "array", "items": text, "minItems": 1}, "exclude": texts, "use": texts, "language": text, "profile": text}, "use")
	suppression := closed(map[string]any{"rule": text, "path": text, "reason": map[string]any{"type": "string", "pattern": `\S`}, "line": map[string]any{"type": "integer", "minimum": 0}}, "rule", "path", "reason")
	root := closed(map[string]any{"version": map[string]any{"const": 2}, "language": text, "profile": text, "components": map[string]any{"type": "object", "additionalProperties": enum("inline", "block", "opaque")}, "rules": map[string]any{"type": "object", "propertyNames": name, "additionalProperties": map[string]any{"oneOf": branches}}, "templates": map[string]any{"type": "object", "propertyNames": name, "additionalProperties": text}, "sets": map[string]any{"type": "object", "propertyNames": name, "additionalProperties": sets}, "apply": array(apply), "exclude": texts, "suppressions": array(suppression)}, "version", "apply")
	root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	root["title"] = "Marklint single-file configuration v2"
	return root
}
func CheckDescriptors(registry *rulepack.Registry) []map[string]any {
	out := []map[string]any{}
	options := optionSchemas(registry)
	for _, definition := range RuleDefinitions() {
		id := definition.Descriptor["id"].(string)
		if _, installed := options[id]; installed {
			out = append(out, definition.Descriptor)
			delete(options, id)
		}
	}
	for _, id := range sortedKeys(options) {
		out = append(out, map[string]any{"id": id, "version": "2", "summary": id, "targets": []string{"document"}, "formats": []string{"markdown"}, "languages": []string{"*"}, "capabilities": []string{"structure"}, "parametersSchema": options[id], "execution": "local", "fixtures": []any{}, "resultKind": "assertion", "fix": "none"})
	}
	return out
}
func ResolvedRuleSchema(registry *rulepack.Registry) map[string]any {
	branches := []any{}
	options := optionSchemas(registry)
	for _, id := range sortedKeys(options) {
		branches = append(branches, closed(map[string]any{"check": map[string]any{"const": id}, "options": options[id], "severity": enum("error", "warning", "info"), "on-unknown": enum("fail", "report")}, "check", "options"))
	}
	return map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "oneOf": branches}
}
func DescriptorSchema(measure bool) map[string]any {
	stringsSchema := array(textSchema())
	props := map[string]any{"id": textSchema(), "version": textSchema(), "summary": textSchema(), "targets": stringsSchema, "formats": array(enum("markdown", "mdx")), "languages": stringsSchema, "capabilities": array(enum("structure", "text", "words", "sentences", "graphemes", "syllables", "morphology", "syntax", "provider")), "parametersSchema": map[string]string{"type": "object"}, "execution": enum("local", "remote"), "fixtures": array(map[string]string{"type": "object"})}
	fixture := closed(map[string]any{"name": textSchema(), "input": map[string]string{"type": "string"}, "parameters": map[string]string{"type": "object"}, "expected": enum("pass", "fail", "unknown", "invalid")}, "name", "input", "parameters", "expected")
	if measure {
		fixture["properties"].(map[string]any)["expected"] = map[string]any{"oneOf": []any{
			closed(map[string]any{"status": map[string]any{"const": "known"}, "values": array(map[string]string{"type": "number"})}, "status", "values"),
			closed(map[string]any{"status": enum("unknown", "invalid")}, "status"),
		}}
	}
	props["fixtures"] = array(fixture)
	required := sortedKeys(props)
	if measure {
		props["sentenceAttribution"] = map[string]string{"type": "boolean"}
		props["unit"] = textSchema()
		props["valueType"] = enum("integer", "number")
		props["granularity"] = enum("target", "window")
		props["views"] = array(enum("auto", "prose", "visible", "source", "code", "language", "cell"))
		props["range"] = map[string]any{"type": "array", "items": map[string]string{"type": "number"}, "minItems": 2, "maxItems": 2}
		required = append(required, "unit", "valueType", "granularity", "views")
	} else {
		props["resultKind"] = enum("assertion", "metric", "review")
		props["fix"] = enum("none", "safe-edits")
		required = append(required, "resultKind", "fix")
	}
	out := closed(props, required...)
	out["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	return out
}
func ReportSchema() map[string]any {
	integer := map[string]any{"type": "integer", "minimum": 0}
	position := closed(map[string]any{"line": map[string]any{"type": "integer", "minimum": 1}, "column": map[string]any{"type": "integer", "minimum": 1}}, "line", "column")
	location := closed(map[string]any{"path": map[string]string{"type": "string"}, "startOffset": integer, "endOffset": integer, "start": position, "end": position, "pointer": map[string]string{"type": "string"}, "templateLine": integer}, "path", "startOffset", "endOffset", "start", "end")
	summary := map[string]any{}
	for _, k := range strings.Fields("files evaluatedTargets failedTargets unknownTargets zeroSelectionBindings suppressed errors warnings info") {
		summary[k] = integer
	}
	diag := closed(map[string]any{"code": textSchema(), "ruleId": textSchema(), "checkId": textSchema(), "severity": enum("error", "warning", "info"), "result": enum("fail", "unknown", "error"), "message": textSchema(), "target": textSchema(), "attribution": enum("exact", "region", "insertion"), "location": map[string]any{"anyOf": []any{location, map[string]string{"type": "null"}}}, "related": array(closed(map[string]any{"role": textSchema(), "location": location, "message": map[string]string{"type": "string"}}, "role", "location", "message")), "expected": map[string]any{}, "actual": map[string]any{}, "measurements": array(closed(map[string]any{"name": textSchema(), "value": map[string]string{"type": "number"}, "unit": textSchema(), "min": map[string]any{}, "max": map[string]any{}, "inputs": map[string]string{"type": "object"}, "sample": textSchema()}, "name", "value", "unit", "inputs", "sample")), "provenance": map[string]string{"type": "object"}, "remediation": textSchema()}, strings.Fields("code ruleId checkId severity result message target attribution location related expected actual measurements provenance remediation")...)
	root := closed(map[string]any{"schemaVersion": map[string]any{"const": 2}, "tool": closed(map[string]any{"name": map[string]any{"const": "marklint"}, "version": textSchema()}, "name", "version"), "status": enum("pass", "fail", "error"), "failOn": enum("error", "warning", "info"), "complete": map[string]string{"type": "boolean"}, "exitCode": map[string]any{"enum": []int{0, 1, 2}}, "inputs": array(closed(map[string]any{"path": textSchema(), "source": enum("file", "stdin"), "format": enum("markdown", "mdx"), "language": textSchema(), "profile": textSchema(), "sets": array(textSchema()), "templates": array(textSchema()), "syntaxOnly": map[string]string{"type": "boolean"}}, strings.Fields("path source format language profile sets templates syntaxOnly")...)), "summary": closed(summary, sortedKeys(summary)...), "diagnostics": array(diag)}, strings.Fields("schemaVersion tool status failOn complete exitCode inputs summary diagnostics")...)
	root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	return root
}
