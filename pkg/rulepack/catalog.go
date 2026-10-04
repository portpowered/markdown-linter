package rulepack

import (
	"gopkg.in/yaml.v3"
	"reflect"
	"strings"
)

func registerAdditional(registry *Registry) error {
	if err := registry.Register("text.matcher", matcherFactory("text.matcher", matcherDefaults())); err != nil {
		return err
	}
	if err := registerSTE(registry); err != nil {
		return err
	}
	if err := registerStrunkWhite(registry); err != nil {
		return err
	}
	for _, id := range []string{"markdown.formatting",
		"markdown.ordered-list",
		"markdown.trailing-whitespace",
		"markdown.fence-closed",
		"markdown.image-alt",
		"markdown.link-text",
		"markdown.reference-definitions",
		"markdown.fence-language",
		"markdown.single-title",
		"markdown.heading-duplicates",
		"markdown.blank-lines",
		"markdown.final-newline",
		"markdown.list-style",
		"markdown.frontmatter-valid",
		"text.terminology",
		"text.repeated-word",
		"text.spelling",
		"markdown.line-length",
		"markdown.html-policy"} {
		if e := registry.Register(id, markdownFactory(id)); e != nil {
			return e
		}
	}
	for _, d := range registry.Catalog() {
		if !strings.Contains(d.ID, ".") {
			continue
		}
		switch strings.SplitN(d.ID, ".", 2)[0] {
		case "markdown", "text":
		default:
			continue
		}
		d.Kind = "markdown"
		d.Title = strings.ReplaceAll(strings.SplitN(d.ID, ".", 2)[1], "-", " ")
		d.Guidance = "See docs/rule-packs.md and docs/linter-roadmap.md for activation, scope, examples, and exceptions."
		d.Options = map[string]string{}
		if strings.HasPrefix(d.ID, "text.strunk-white.") {
			d.Options = map[string]string{"allow": "[]string", "scope": "string"}
		}
		defaults := defaultOptions()
		encoded, _ := yaml.Marshal(defaults)
		allDefaults := map[string]any{}
		_ = yaml.Unmarshal(encoded, &allDefaults)
		d.Defaults = map[string]any{}
		for _, key := range optionNames(d.ID) {
			d.Defaults[key] = allDefaults[key]
		}
		if strings.HasPrefix(d.ID, "text.strunk-white.") {
			d.Defaults = map[string]any{"allow": []string{}, "scope": "prose"}
			d.Title = "Strunk and White: " + strings.ReplaceAll(strings.TrimPrefix(d.ID, "text.strunk-white."), "-", " ")
			d.Guidance = "See docs/strunk-white.md for the finite patterns, examples, interactions, and editorial limitations."
		}
		if d.ID == "markdown.trailing-whitespace" || d.ID == "markdown.formatting" {
			d.Defaults["allow-hard-breaks"] = true
		}
		typ := reflect.TypeOf(defaults)
		types := map[string]string{}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			types[field.Tag.Get("yaml")] = field.Type.String()
		}

		for _, key := range optionNames(d.ID) {
			d.Options[key] = types[key]
			if d.Options[key] == "" {
				d.Options[key] = "See rule-pack reference"
			}
		}
		if strings.HasPrefix(d.ID, "text.ste100.") {
			d.Title = "STE100: " + strings.TrimPrefix(d.ID, "text.ste100.")
			d.Guidance = "See docs/ste100.md for supplied vocabulary, explicit forms, alternatives and grammar limitations."
			d.Options = map[string]string{"scope": "string"}
			d.Defaults = map[string]any{"scope": "prose"}
			if d.ID == "text.ste100.dictionary" {
				d.Options["dictionary"] = "[]string"
				d.Options["dictionary-file"] = "string"
				d.Options["technical-terms"] = "[]string"
			} else {
				d.Options["max-sentence-words"] = "int"
				d.Options["forbidden-patterns"] = "map[string]string"
				d.Defaults["max-sentence-words"] = 25
			}
		}
		if d.ID == "text.matcher" || strings.HasPrefix(d.ID, "text.strunk-white.") {
			defaults := matcherDefaults()
			for _, spec := range strunkSpecs {
				if d.ID == "text.strunk-white."+spec.name {
					defaults = strunkDefaults(spec)
				}
			}
			encoded, _ := yaml.Marshal(defaults)
			_ = yaml.Unmarshal(encoded, &d.Defaults)
			d.Options = map[string]string{}
			typ := reflect.TypeOf(defaults)
			for i := 0; i < typ.NumField(); i++ {
				field := typ.Field(i)
				d.Options[field.Tag.Get("yaml")] = field.Type.String()
			}
		}

		d.Presets = []string{}
		for _, name := range PresetNames() {
			p, _ := Preset(name)
			for _, rule := range p.Rules {
				if rule.Check == d.ID {
					d.Presets = append(d.Presets, name)
					break
				}
			}
		}
		d.Category = strings.SplitN(d.ID, ".", 2)[0]
		d.Severity = "warning"
		defaultPack, _ := Preset(DefaultPresetName())
		for _, rule := range defaultPack.Rules {
			if rule.Check == d.ID {
				d.Severity = string(rule.Severity)
			}
		}
		d.Fixable = d.ID == "markdown.final-newline" || d.ID == "markdown.trailing-whitespace" || d.ID == "markdown.link-relocation" || d.ID == "markdown.doc-id-unique"
		if strings.HasPrefix(d.ID, "text.ste100.") {
			d.Severity = "error"
		}

		if e := registry.SetDescriptor(d); e != nil {
			return e
		}
	}
	return nil
}
