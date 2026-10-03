package rulepack

import (
	"embed"
	"gopkg.in/yaml.v3"
	"sort"
	"strings"
	"sync"
)

//go:embed packs/*
var embeddedPacks embed.FS
var packFiles = map[string]string{"markdown:core": "markdown-core.yaml", "markdown:documentation": "markdown-documentation.yaml", "markdown:maintenance": "markdown-maintenance.yaml", "markdown:recommended": "markdown-recommended.yaml", "markdown:style": "markdown-style.yaml", "portos-defaults": "portos-defaults.yaml", "portos:internal": "portos-internal.yaml", "text:prose": "text-prose.yaml"}

func DefaultKind() string       { return "markdown" }
func DefaultPresetName() string { return "markdown:recommended" }
func CommandName() string       { return "marklint" }
func PresetNames() []string {
	out := []string{}
	for name := range packFiles {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
func rawPreset(name string) (Pack, bool) {
	filename, ok := packFiles[name]
	if !ok {
		return Pack{}, false
	}
	data, e := embeddedPacks.ReadFile("packs/" + filename)
	if e != nil {
		return Pack{}, false
	}
	p, e := Decode(strings.NewReader(string(data)))
	return p, e == nil
}

var presetOnce sync.Once
var resolvedPresets map[string]Pack

func Preset(name string) (Pack, bool) {
	presetOnce.Do(func() {
		resolvedPresets = map[string]Pack{}
		for _, id := range PresetNames() {
			p, e := Load(id, ".")
			if e == nil {
				resolvedPresets[id] = p
			}
		}
	})
	p, ok := resolvedPresets[name]
	if !ok {
		return Pack{}, false
	}
	p.Rules = append([]Rule(nil), p.Rules...)
	p.Suppressions = append([]Suppression(nil), p.Suppressions...)
	p.Imports = append([]Import(nil), p.Imports...)
	for i := range p.Rules {
		r := &p.Rules[i]
		r.Include = append([]string(nil), r.Include...)
		r.Exclude = append([]string(nil), r.Exclude...)
		r.Options = cloneNode(r.Options)
		if r.Enabled != nil {
			enabled := *r.Enabled
			r.Enabled = &enabled
		}
	}
	return p, true
}
func cloneNode(n yaml.Node) yaml.Node {
	n.Content = append([]*yaml.Node(nil), n.Content...)
	for i, child := range n.Content {
		copy := cloneNode(*child)
		n.Content[i] = &copy
	}
	return n
}
