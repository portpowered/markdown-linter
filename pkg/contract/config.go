// Package contract implements the single-file version-2 policy contract.
package contract

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/portpowered/markdown-linter/pkg/rulepack"
	"gopkg.in/yaml.v3"
)

type Rule struct {
	Check       string         `yaml:"check,omitempty" json:"check"`
	Extends     string         `yaml:"extends,omitempty" json:"extends,omitempty"`
	Options     map[string]any `yaml:"options,omitempty" json:"options"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty"`
	Severity    string         `yaml:"severity,omitempty" json:"severity"`
	Unknown     string         `yaml:"on-unknown,omitempty" json:"onUnknown"`
	Measure     *Descriptor    `yaml:"-" json:"-"`
}
type Binding struct {
	Rule  string `yaml:"rule" json:"rule"`
	On    string `yaml:"on" json:"on"`
	Scope string `yaml:"scope,omitempty" json:"scope"`
}
type Set struct {
	Description string    `yaml:"description,omitempty" json:"description,omitempty"`
	Use         []string  `yaml:"use,omitempty" json:"use"`
	Rules       []Binding `yaml:"rules,omitempty" json:"rules"`
	Templates   []string  `yaml:"templates,omitempty" json:"templates"`
}
type Apply struct {
	Files    []string `yaml:"files,omitempty"`
	Exclude  []string `yaml:"exclude,omitempty"`
	Use      []string `yaml:"use"`
	Language string   `yaml:"language,omitempty"`
	Profile  string   `yaml:"profile,omitempty"`
}
type Config struct {
	Version      int                    `yaml:"version"`
	Language     string                 `yaml:"language,omitempty"`
	Profile      string                 `yaml:"profile,omitempty"`
	Components   map[string]string      `yaml:"components,omitempty"`
	Rules        map[string]Rule        `yaml:"rules,omitempty"`
	Templates    map[string]string      `yaml:"templates,omitempty"`
	Sets         map[string]Set         `yaml:"sets,omitempty"`
	Apply        []Apply                `yaml:"apply"`
	Exclude      []string               `yaml:"exclude,omitempty"`
	Suppressions []rulepack.Suppression `yaml:"suppressions,omitempty"`
}
type Policy struct {
	Redundant  []string            `json:"redundantCompatibleLimits"`
	Sets       []string            `json:"sets"`
	Templates  []string            `json:"templates"`
	Bindings   []Binding           `json:"bindings"`
	Language   string              `json:"language"`
	Profile    string              `json:"profile"`
	Origins    map[string][]string `json:"origins"`
	SyntaxOnly bool                `json:"syntaxOnly"`
}
type Program struct {
	Config       Config
	Rules        map[string]Rule
	Templates    map[string]*Template
	Registry     *rulepack.Registry
	Source       []byte
	Path         string
	tree         yaml.Node
	builtin      map[string]rulepack.Rule
	bundledSets  map[string][]Binding
	Capabilities *Capabilities
	Network      []string
	Fixes        []interfaces.Diagnostic
}

var nameRE = regexp.MustCompile(`^[a-z][a-z0-9.-]*$`)
var kinds = strings.Fields("heading section paragraph sentence table codeblock source-line")

func has(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
func strictTree(n *yaml.Node) error {
	if n.Anchor != "" || n.Kind == yaml.AliasNode || n.Style&yaml.TaggedStyle != 0 {
		return fmt.Errorf("line %d: tags, anchors and aliases are forbidden", n.Line)
	}
	if n.Tag == "!!float" {
		var v float64
		if err := n.Decode(&v); err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("line %d: nonfinite number", n.Line)
		}
	}
	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Tag != "!!str" || k.Value == "<<" || seen[k.Value] {
				return fmt.Errorf("line %d: duplicate, merge, or nonstring key %q", k.Line, k.Value)
			}
			seen[k.Value] = true
		}
	}
	for _, c := range n.Content {
		if err := strictTree(c); err != nil {
			return err
		}
	}
	return nil
}
func Decode(source []byte, filename string, registry *rulepack.Registry) (*Program, error) {
	return DecodeWithCapabilities(source, filename, registry, NewCapabilities())
}
func DecodeWithCapabilities(source []byte, filename string, registry *rulepack.Registry, capabilities *Capabilities) (*Program, error) {
	p := &Program{Source: source, Path: filename, Registry: registry, Rules: map[string]Rule{}, Templates: map[string]*Template{}, builtin: map[string]rulepack.Rule{}}
	if capabilities == nil {
		capabilities = NewCapabilities()
	}
	p.Capabilities = capabilities
	d := yaml.NewDecoder(bytes.NewReader(source))
	if err := d.Decode(&p.tree); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("configuration must contain exactly one YAML document")
	}
	if err := strictTree(&p.tree); err != nil {
		return nil, err
	}
	var raw any
	if err := p.tree.Decode(&raw); err != nil {
		return nil, err
	}
	if err := validateJSON(ConfigSchemaWithCapabilities(registry, capabilities), raw); err != nil {
		return nil, fmt.Errorf("configuration schema: %w", err)
	}
	d = yaml.NewDecoder(bytes.NewReader(source))
	d.KnownFields(true)
	if err := d.Decode(&p.Config); err != nil {
		return nil, err
	}
	c := &p.Config
	if c.Version != 2 {
		return nil, fmt.Errorf("--config requires version 2; migrate version-1 packs explicitly")
	}
	if c.Apply == nil {
		return nil, fmt.Errorf("apply is required (use [] for syntax-only)")
	}
	if c.Language == "" {
		c.Language = "en"
	}
	if c.Profile == "" {
		c.Profile = "unicode-v1"
	}
	if err := analysis(c.Language, c.Profile); err != nil {
		return nil, err
	}
	for k, v := range c.Components {
		if k == "" || !has([]string{"inline", "block", "opaque"}, v) {
			return nil, fmt.Errorf("invalid component %q", k)
		}
	}
	p.installBaseline()
	for _, id := range sortedKeys(c.Rules) {
		if !nameRE.MatchString(id) {
			return nil, fmt.Errorf("invalid rule name %q", id)
		}
		if _, ok := p.builtin[id]; ok {
			return nil, fmt.Errorf("reserved rule name %q", id)
		}
	}
	visiting := map[string]bool{}
	var resolve func(string) (Rule, error)
	resolve = func(id string) (Rule, error) {
		if r, ok := p.Rules[id]; ok {
			return r, nil
		}
		r, ok := c.Rules[id]
		if !ok {
			return r, fmt.Errorf("unknown rule %q", id)
		}
		if visiting[id] {
			return r, fmt.Errorf("rule inheritance cycle at %s", id)
		}
		visiting[id] = true
		defer delete(visiting, id)
		if (r.Check == "") == (r.Extends == "") {
			return r, fmt.Errorf("rule %s requires exactly one of check and extends", id)
		}
		if r.Extends != "" {
			parent, err := resolve(r.Extends)
			if err != nil {
				return r, err
			}
			patch := r.Options
			r.Check = parent.Check
			r.Options = map[string]any{}
			for k, v := range parent.Options {
				r.Options[k] = v
			}
			for k, v := range patch {
				r.Options[k] = v
			}
			if r.Severity == "" {
				r.Severity = parent.Severity
			}
			if r.Unknown == "" {
				r.Unknown = parent.Unknown
			}
		}
		if r.Severity == "" {
			r.Severity = "error"
		}
		// Explicit options override the canonical installed defaults. Inheritance
		// has already resolved its parent and patch before applying this baseline.
		if stock, ok := p.builtin[r.Check]; ok {
			defaults := map[string]any{}
			_ = stock.Options.Decode(&defaults)
			for key, value := range r.Options {
				defaults[key] = value
			}
			r.Options = defaults
		}
		if !has([]string{"error", "warning", "info"}, r.Severity) {
			return r, fmt.Errorf("invalid severity for %s", id)
		}
		if r.Unknown == "" {
			r.Unknown = "fail"
		}
		if !has([]string{"fail", "report"}, r.Unknown) {
			return r, fmt.Errorf("invalid on-unknown for %s", id)
		}
		if err := p.validateRule(r); err != nil {
			return r, fmt.Errorf("rule %s: %w", id, err)
		}
		if installed, ok := p.Capabilities.lookup(r); ok {
			d := installed.Descriptor
			r.Measure = &d
		}
		p.Rules[id] = r
		return r, nil
	}
	for _, id := range sortedKeys(c.Rules) {
		if _, err := resolve(id); err != nil {
			return nil, err
		}
	}
	for id, r := range p.Rules {
		if r.Check == "markdown.table-schema" {
			for _, col := range object(r.Options["columns"]) {
				for _, ref := range list(object(col)["rules"]) {
					rr, ok := p.Rules[ref]
					if !ok {
						return nil, fmt.Errorf("rule %s: unknown cell rule %s", id, ref)
					}
					if err := compatible(rr, "cell", "direct"); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	for _, id := range sortedKeys(c.Templates) {
		if !nameRE.MatchString(id) {
			return nil, fmt.Errorf("invalid template name %s", id)
		}
		t, err := parseTemplate(c.Templates[id], p)
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", id, err)
		}
		p.Templates[id] = t
	}
	for id, s := range c.Sets {
		if !nameRE.MatchString(id) || p.bundled(id) {
			return nil, fmt.Errorf("invalid or reserved set %s", id)
		}
		for _, b := range s.Rules {
			r, ok := p.Rules[b.Rule]
			if !ok {
				return nil, fmt.Errorf("unknown rule %s", b.Rule)
			}
			kind, err := bindingKind(b.On)
			if err != nil {
				return nil, err
			}
			if b.Scope != "" && b.Scope != "direct" && b.Scope != "subtree" {
				return nil, fmt.Errorf("invalid scope")
			}
			if b.On == "document" && b.Scope != "" {
				return nil, fmt.Errorf("document scope is always subtree")
			}
			if err := compatible(r, kind, b.Scope); err != nil {
				return nil, err
			}
		}
		for _, t := range s.Templates {
			if p.Templates[t] == nil {
				return nil, fmt.Errorf("unknown template %s", t)
			}
		}
		if _, _, _, err := p.expand([]string{id}); err != nil {
			return nil, err
		}
	}
	for _, a := range c.Apply {
		if a.Use == nil {
			return nil, fmt.Errorf("apply.use is required")
		}
		if a.Files != nil && len(a.Files) == 0 {
			return nil, fmt.Errorf("files must be nonempty")
		}
		if a.Language != "" || a.Profile != "" {
			l, pr := a.Language, a.Profile
			if l == "" {
				l = c.Language
			}
			if pr == "" {
				pr = c.Profile
			}
			if err := analysis(l, pr); err != nil {
				return nil, err
			}
		}
		if _, _, _, err := p.expand(a.Use); err != nil {
			return nil, err
		}
		for _, pat := range append(append([]string{}, a.Files...), a.Exclude...) {
			if err := Pattern(pat); err != nil {
				return nil, err
			}
		}
	}
	for _, pat := range c.Exclude {
		if err := Pattern(pat); err != nil {
			return nil, err
		}
	}
	generated := map[string]bool{}
	for name, template := range p.Templates {
		generated["template."+name+".outline"] = true
		for i := range template.Headings {
			generated[fmt.Sprintf("template.%s.heading.%d", name, i+1)] = true
		}
		for i, requirement := range template.Requirements {
			id := fmt.Sprintf("template.%s.%d", name, i+1)
			generated[id] = true
			for j, ref := range requirement.IDs {
				if ref == "" {
					generated[fmt.Sprintf("%s.%d", id, j+1)] = true
				}
			}
		}
	}
	for _, s := range c.Suppressions {
		_, ok := p.Rules[s.Rule]
		_, stock := p.builtin[s.Rule]
		if !ok && !stock && !generated[s.Rule] {
			return nil, fmt.Errorf("unknown suppressed rule %s", s.Rule)
		}
		if strings.TrimSpace(s.Reason) == "" || s.Line < 0 {
			return nil, fmt.Errorf("suppression requires reason and nonnegative line")
		}
		if err := Pattern(s.Path); err != nil {
			return nil, err
		}
	}
	return p, nil
}
func Default(registry *rulepack.Registry) *Program {
	p, err := Decode([]byte("version: 2\napply:\n  - use: [\"markdown:recommended\"]\n"), "", registry)
	if err != nil {
		panic(err)
	}
	return p
}
func analysis(language, profile string) error {
	if profile != "unicode-v1" {
		return fmt.Errorf("capability.profile: unavailable profile %s", profile)
	}
	if !regexp.MustCompile(`^[A-Za-z]{2,8}(-[A-Za-z0-9]{1,8})*$`).MatchString(language) {
		return fmt.Errorf("invalid language tag %s", language)
	}
	if has([]string{"zh", "ja", "th", "lo", "km", "my"}, strings.Split(language, "-")[0]) {
		return fmt.Errorf("capability.segmentation: %s requires an installed segmentation profile", language)
	}
	return nil
}
func Pattern(pat string) error {
	if pat == "" || path.IsAbs(pat) || strings.ContainsAny(pat, "\\:{}") || path.Clean(pat) != pat || pat == ".." || strings.HasPrefix(pat, "../") || strings.Contains(pat, "/../") {
		return fmt.Errorf("invalid root-relative pattern %q", pat)
	}
	if strings.Contains(pat, "**") && (!strings.HasSuffix(pat, "/**") || strings.Contains(strings.TrimSuffix(pat, "/**"), "**")) {
		return fmt.Errorf("only trailing /** supported")
	}
	_, err := path.Match(pat, "")
	return err
}
func Match(pat, file string) bool {
	if strings.HasSuffix(pat, "/**") {
		p := strings.TrimSuffix(pat, "/**")
		return file == p || strings.HasPrefix(file, p+"/")
	}
	ok, _ := path.Match(pat, file)
	return ok
}
func Matches(pats []string, file string) bool {
	for _, p := range pats {
		if Match(p, file) {
			return true
		}
	}
	return false
}
func bindingKind(on string) (string, error) {
	if on == "document" {
		return on, nil
	}
	if strings.HasPrefix(on, "each ") && has(kinds, on[5:]) {
		return on[5:], nil
	}
	return "", fmt.Errorf("invalid binding selector %q", on)
}
func (p *Program) bundled(id string) bool {
	_, ok := p.bundledSets[id]
	return ok
}

func (p *Program) installBaseline() {
	p.bundledSets = map[string][]Binding{}
	for _, definition := range RuleDefinitions() {
		id := definition.Descriptor["id"].(string)
		if _, installed := p.Registry.Describe(id); !installed {
			continue
		}
		var options yaml.Node
		_ = options.Encode(definition.Defaults)
		p.builtin[id] = rulepack.Rule{ID: id, Check: id, Severity: interfaces.Severity(definition.RecommendedSeverity), Options: options}
		for _, preset := range definition.Presets {
			p.bundledSets[preset] = append(p.bundledSets[preset], Binding{Rule: id, On: "document"})
		}
		for _, instance := range definition.BundledRules {
			defaults := map[string]any{}
			for key, value := range definition.Defaults {
				defaults[key] = value
			}
			for key, value := range instance.Options {
				defaults[key] = value
			}
			var options yaml.Node
			_ = options.Encode(defaults)
			p.builtin[instance.ID] = rulepack.Rule{ID: instance.ID, Check: instance.Check, Severity: interfaces.Severity(instance.Severity), Options: options}
			for _, preset := range instance.Presets {
				p.bundledSets[preset] = append(p.bundledSets[preset], Binding{Rule: instance.ID, On: "document"})
			}
		}
	}
}
func (p *Program) expand(ids []string) ([]string, []Binding, []string, error) {
	sets, bindings, templates := []string{}, []Binding{}, []string{}
	seen, active := map[string]bool{}, map[string]bool{}
	var walk func(string) error
	walk = func(id string) error {
		if active[id] {
			return fmt.Errorf("set cycle at %s", id)
		}
		if seen[id] {
			return nil
		}
		if has([]string{"portos-defaults", "portos:internal", "portos-default"}, id) {
			return fmt.Errorf("set %s was replaced by portos in version 2", id)
		}
		active[id] = true
		defer delete(active, id)
		if p.bundled(id) {
			bindings = append(bindings, p.bundledSets[id]...)
		} else {
			s, ok := p.Config.Sets[id]
			if !ok {
				return fmt.Errorf("unknown set %s", id)
			}
			for _, ref := range s.Use {
				if err := walk(ref); err != nil {
					return err
				}
			}
			bindings = append(bindings, s.Rules...)
			templates = append(templates, s.Templates...)
		}
		seen[id] = true
		sets = append(sets, id)
		return nil
	}
	for _, id := range ids {
		if err := walk(id); err != nil {
			return nil, nil, nil, err
		}
	}
	return sets, bindings, templates, nil
}
func (p *Program) Policy(file string) (Policy, error) {
	q := Policy{Language: p.Config.Language, Profile: p.Config.Profile, Sets: []string{}, Templates: []string{}, Bindings: []Binding{}, Origins: map[string][]string{}}
	matched := len(p.Config.Apply) == 0
	explicitL, explicitP := "", ""
	for i, a := range p.Config.Apply {
		if (a.Files != nil && !Matches(a.Files, file)) || Matches(a.Exclude, file) {
			continue
		}
		matched = true
		if a.Language != "" {
			if explicitL != "" && explicitL != a.Language {
				return q, fmt.Errorf("conflicting languages for %s", file)
			}
			explicitL = a.Language
			q.Language = a.Language
		}
		if a.Profile != "" {
			if explicitP != "" && explicitP != a.Profile {
				return q, fmt.Errorf("conflicting profiles for %s", file)
			}
			explicitP = a.Profile
			q.Profile = a.Profile
		}
		ss, bs, ts, err := p.expand(a.Use)
		if err != nil {
			return q, err
		}
		for _, s := range ss {
			if !has(q.Sets, s) {
				q.Sets = append(q.Sets, s)
			}
		}
		for _, t := range ts {
			if !has(q.Templates, t) {
				q.Templates = append(q.Templates, t)
			}
		}
		for _, b := range bs {
			key := b.Rule + "|" + b.On + "|" + b.Scope
			q.Origins[key] = append(q.Origins[key], fmt.Sprintf("/apply/%d", i))
			exists := false
			for _, old := range q.Bindings {
				if old == b {
					exists = true
				}
			}
			if !exists {
				q.Bindings = append(q.Bindings, b)
			}
		}
	}
	if !matched {
		return q, fmt.Errorf("input.unmatched-policy: %s", file)
	}
	if len(q.Templates) > 1 {
		return q, fmt.Errorf("conflicting templates for %s", file)
	}
	q.SyntaxOnly = len(q.Bindings) == 0 && len(q.Templates) == 0
	q.Redundant = []string{}
	for i, a := range q.Bindings {
		ra, ok := p.Rules[a.Rule]
		if !ok || ra.Check != "core.limit" {
			continue
		}
		for _, b := range q.Bindings[:i] {
			rb, ok := p.Rules[b.Rule]
			if !ok || rb.Check != "core.limit" || a.On != b.On || a.Scope != b.Scope || str(ra.Options, "measure", "") != str(rb.Options, "measure", "") || str(ra.Options, "view", "auto") != str(rb.Options, "view", "auto") || fmt.Sprint(ra.Options["parameters"]) != fmt.Sprint(rb.Options["parameters"]) {
				continue
			}
			q.Redundant = append(q.Redundant, a.Rule+" and "+b.Rule+" constrain the same measurement independently")
		}
	}
	if err := p.contradictions(q.Bindings); err != nil {
		return q, err
	}
	return q, nil
}
func (p *Program) contradictions(bs []Binding) error {
	bounds := map[string][2]float64{}
	for _, b := range bs {
		r, ok := p.Rules[b.Rule]
		if !ok || r.Check != "core.limit" {
			continue
		}
		scope := b.Scope
		if scope == "" {
			scope = "direct"
		}
		if b.On == "document" {
			scope = "subtree"
		}
		key := b.On + "|" + scope + "|" + str(r.Options, "measure", " ") + "|" + str(r.Options, "view", "auto") + "|" + fmt.Sprint(r.Options["parameters"])
		v, ok := bounds[key]
		if !ok {
			v = [2]float64{math.Inf(-1), math.Inf(1)}
		}
		if n, ok := number(r.Options["min"]); ok && n > v[0] {
			v[0] = n
		}
		if n, ok := number(r.Options["max"]); ok && n < v[1] {
			v[1] = n
		}
		if v[0] > v[1] {
			return fmt.Errorf("contradictory active bounds on %s", key)
		}
		bounds[key] = v
	}
	return nil
}
func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func list(v any) []string {
	a, _ := v.([]any)
	out := []string{}
	for _, x := range a {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	if a, ok := v.([]string); ok {
		return a
	}
	return out
}
func str(m map[string]any, k, def string) string {
	if s, ok := m[k].(string); ok {
		return s
	}
	return def
}
func number(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}
