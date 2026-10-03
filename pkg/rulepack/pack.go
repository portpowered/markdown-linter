// Package rulepack configures built-in and customer analyzers through one registry.
package rulepack

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"gopkg.in/yaml.v3"
)

// Pack is the versioned customer rule configuration.
type Pack struct {
	Version      int           `yaml:"version"`
	Extends      []string      `yaml:"extends,omitempty"`
	Overrides    []Override    `yaml:"overrides,omitempty"`
	Imports      []Import      `yaml:"-"`
	Rules        []Rule        `yaml:"rules"`
	Suppressions []Suppression `yaml:"suppressions,omitempty"`
}

// Rule assigns an identity, severity, scope and options to a registered check.
type Rule struct {
	ID          string              `yaml:"id"`
	Enabled     *bool               `yaml:"enabled,omitempty"`
	Origin      string              `yaml:"-"`
	Check       string              `yaml:"check"`
	Description string              `yaml:"description,omitempty"`
	Severity    interfaces.Severity `yaml:"severity,omitempty"`
	Include     []string            `yaml:"include,omitempty"`
	Exclude     []string            `yaml:"exclude,omitempty"`
	Options     yaml.Node           `yaml:"options,omitempty"`
}

// Suppression explicitly silences a configured rule at a file and optional line.
type Suppression struct {
	Rule   string `yaml:"rule"`
	Path   string `yaml:"path"`
	Line   int    `yaml:"line,omitempty"`
	Reason string `yaml:"reason"`
}

// Factory constructs a check using validated customer options.
type Factory func(yaml.Node) (interfaces.Analyzer, error)

// Registry holds factories registered by either the application or its extensions.
type Registry struct {
	factories   map[string]Factory
	descriptors map[string]Descriptor
}

func NewRegistry() *Registry { return &Registry{factories: make(map[string]Factory)} }

// Register rejects conflicting check names instead of replacing built-in behavior.
func (r *Registry) Register(name string, factory Factory) error {
	if strings.TrimSpace(name) == "" || factory == nil {
		return fmt.Errorf("check name and factory are required")
	}
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("duplicate check %q", name)
	}
	r.factories[name] = factory
	return nil
}

// Decode rejects unknown fields and multiple YAML documents.
func Decode(reader io.Reader) (Pack, error) {
	var pack Pack
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)
	if err := decoder.Decode(&pack); err != nil {
		return pack, fmt.Errorf("rule pack: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return pack, fmt.Errorf("rule pack must contain exactly one YAML document")
	}
	if pack.Version != 1 {
		return pack, fmt.Errorf("unsupported rule pack version %d", pack.Version)
	}
	return pack, nil
}

// DecodeOptions gives third-party checks the same strict option validation as stock checks.
func DecodeOptions(node yaml.Node, target any) error {
	if node.Kind == 0 {
		node = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("options must be a mapping")
	}
	content, err := yaml.Marshal(&node)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	return decoder.Decode(target)
}

type compiledRule struct {
	spec     Rule
	analyzer interfaces.Analyzer
}

// Program is a validated rule pack ready to run on parsed documents.
type Program struct {
	rules        []compiledRule
	suppressions []Suppression
}

// Compile validates the complete pack, including rules not selected by a later CLI filter.
func (r *Registry) Compile(pack Pack) (*Program, error) {
	if pack.Version != 1 {
		return nil, fmt.Errorf("unsupported rule pack version %d", pack.Version)
	}
	if len(pack.Extends) > 0 || len(pack.Overrides) > 0 {
		return nil, fmt.Errorf("unresolved pack: use rulepack.Load before Compile")
	}
	program := &Program{suppressions: append([]Suppression(nil), pack.Suppressions...)}
	seen := map[string]bool{}
	for _, spec := range pack.Rules {
		if strings.TrimSpace(spec.ID) == "" || seen[spec.ID] {
			return nil, fmt.Errorf("missing or duplicate rule ID %q", spec.ID)
		}
		seen[spec.ID] = true
		if spec.Severity == "" {
			spec.Severity = interfaces.SeverityError
		}
		switch spec.Severity {
		case interfaces.SeverityError, interfaces.SeverityWarning, interfaces.SeverityInfo:
		default:
			return nil, fmt.Errorf("rule %s: invalid severity %q", spec.ID, spec.Severity)
		}
		for _, pattern := range append(append([]string(nil), spec.Include...), spec.Exclude...) {
			if err := validatePattern(pattern); err != nil {
				return nil, fmt.Errorf("rule %s: %w", spec.ID, err)
			}
		}
		factory, ok := r.factories[spec.Check]
		if !ok {
			return nil, fmt.Errorf("rule %s: unknown check %q", spec.ID, spec.Check)
		}
		analyzer, err := factory(spec.Options)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", spec.ID, err)
		}
		if analyzer == nil {
			return nil, fmt.Errorf("rule %s: factory returned no analyzer", spec.ID)
		}
		if spec.Enabled == nil || *spec.Enabled {
			program.rules = append(program.rules, compiledRule{spec, analyzer})
		}
	}
	for _, s := range program.suppressions {
		if !seen[s.Rule] || s.Line < 0 || strings.TrimSpace(s.Reason) == "" {
			return nil, fmt.Errorf("invalid suppression for rule %q: existing rule, reason and nonnegative line required", s.Rule)
		}
		if err := validatePattern(s.Path); err != nil {
			return nil, err
		}
	}
	return program, nil
}

func validatePattern(pattern string) error {
	if strings.Contains(pattern, "**") {
		if !strings.HasSuffix(pattern, "/**") || strings.Contains(strings.TrimSuffix(pattern, "/**"), "**") {
			return fmt.Errorf("only a trailing /** recursive scope is supported: %q", pattern)
		}
	}
	if pattern == "" || path.IsAbs(pattern) || strings.Contains(pattern, "\\") || pattern == ".." || strings.HasPrefix(pattern, "../") || path.Clean(pattern) != pattern {
		return fmt.Errorf("scope must be a nonempty root-relative slash path: %q", pattern)
	}
	_, err := path.Match(pattern, "")
	return err
}

func matches(pattern, file string) bool {
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return file == prefix || strings.HasPrefix(file, prefix+"/")
	}
	matched, _ := path.Match(pattern, file)
	return matched
}

func anyMatch(patterns []string, file string) bool {
	for _, pattern := range patterns {
		if matches(pattern, file) {
			return true
		}
	}
	return false
}

func relativePath(root, file string) (string, error) {
	absolute, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, absolute)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("document %q is outside root", file)
	}
	return rel, nil
}

// Run executes checks with identical scoping, identity, severity and suppression handling.
func (p *Program) Run(ctx context.Context, root string, documents []*interfaces.Document) ([]interfaces.Diagnostic, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	paths := make(map[*interfaces.Document]string, len(documents))
	byPath := map[string]*interfaces.Document{}
	for _, doc := range documents {
		if doc == nil {
			return nil, fmt.Errorf("nil document")
		}
		rel, err := relativePath(root, doc.Path)
		if err != nil {
			return nil, err
		}
		paths[doc] = rel
		byPath[rel] = doc
	}
	diagnostics := []interfaces.Diagnostic{}
	for _, rule := range p.rules {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		selected := []*interfaces.Document{}
		selectedPaths := map[string]bool{}
		for _, doc := range documents {
			rel := paths[doc]
			if (len(rule.spec.Include) == 0 || anyMatch(rule.spec.Include, rel)) && !anyMatch(rule.spec.Exclude, rel) {
				selected = append(selected, doc)
				selectedPaths[rel] = true
			}
		}
		pass := interfaces.NewPass(selected)
		pass.Root = root
		rule.analyzer.Analyze(ctx, pass)
		for _, diagnostic := range pass.Diagnostics() {
			diagnostic.RuleID = rule.spec.ID
			diagnostic.CheckID = rule.spec.Check
			diagnostic.Origin = rule.spec.Origin
			diagnostic.Severity = rule.spec.Severity
			rel, err := relativePath(root, diagnostic.Path)
			if err != nil {
				return nil, err
			}
			if !selectedPaths[rel] {
				return nil, fmt.Errorf("rule %s reported a diagnostic outside its selected documents: %s", rule.spec.ID, diagnostic.Path)
			}
			if !p.suppressed(diagnostic, rel) && !inlineSuppressed(byPath[rel], diagnostic) {
				diagnostics = append(diagnostics, diagnostic)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(diagnostics, func(i, j int) bool {
		a, b := diagnostics[i], diagnostics[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		return a.Message < b.Message
	})
	return diagnostics, nil
}

func (p *Program) suppressed(d interfaces.Diagnostic, rel string) bool {
	for _, s := range p.suppressions {
		if s.Rule == d.RuleID && matches(s.Path, rel) && (s.Line == 0 || s.Line == d.Line) {
			return true
		}
	}
	return false
}

func (r *Rule) setSeverity(s string) error { r.Severity = interfaces.Severity(s); return nil }
