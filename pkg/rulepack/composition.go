package rulepack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Descriptor is available for customer factories as well as stock checks.
type Descriptor struct {
	Defaults map[string]any    `json:"defaults,omitempty"`
	Category string            `json:"category"`
	Severity string            `json:"recommendedSeverity"`
	Fixable  bool              `json:"fixable"`
	Presets  []string          `json:"presets"`
	ID       string            `json:"id"`
	Title    string            `json:"title"`
	Kind     string            `json:"kind"`
	Options  map[string]string `json:"options,omitempty"`
	Guidance string            `json:"guidance"`
}

func (r *Registry) Describe(id string) (Descriptor, bool) {
	if _, ok := r.factories[id]; !ok {
		return Descriptor{}, false
	}
	if d, ok := r.descriptors[id]; ok {
		return d, true
	}
	return Descriptor{ID: id, Title: id, Kind: "custom", Guidance: "Customer check; consult its factory documentation."}, true
}
func (r *Registry) SetDescriptor(d Descriptor) error {
	if d.Kind == "" {
		d.Kind = "custom"
	}
	if _, ok := r.factories[d.ID]; !ok {
		return fmt.Errorf("unknown check %s", d.ID)
	}
	if r.descriptors == nil {
		r.descriptors = map[string]Descriptor{}
	}
	r.descriptors[d.ID] = d
	return nil
}
func (r *Registry) Catalog() []Descriptor {
	out := []Descriptor{}
	for id := range r.factories {
		d, _ := r.Describe(id)
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Override replaces fields, including the entire supplied options map.
type Override struct {
	ID       string    `yaml:"id"`
	Enabled  *bool     `yaml:"enabled,omitempty"`
	Severity *string   `yaml:"severity,omitempty"`
	Include  *[]string `yaml:"include,omitempty"`
	Exclude  *[]string `yaml:"exclude,omitempty"`
	Options  yaml.Node `yaml:"options,omitempty"`
}
type Import struct {
	Name string
	Hash string
}

// Load resolves only embedded and local packs; no network or code is loaded.
func Load(filename, root string) (Pack, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Pack{}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Pack{}, err
	}
	active := map[string]bool{}
	loaded := map[string]bool{}
	var resolve func(string, string) (Pack, error)
	resolve = func(name, base string) (Pack, error) {
		var p Pack
		var identity string
		var content []byte
		if builtin, ok := rawPreset(name); ok {
			p = builtin
			identity = "preset:" + name
			content, err = embeddedPacks.ReadFile("packs/" + packFiles[name])
			if err != nil {
				return p, err
			}
		} else {
			if strings.Contains(name, ":") && !filepath.IsAbs(name) {
				return p, fmt.Errorf("unknown preset %q", name)
			}
			filename := name
			if !filepath.IsAbs(filename) {
				filename = filepath.Join(base, name)
			}
			filename, err = filepath.Abs(filename)
			if err != nil {
				return p, err
			}
			filename, err = filepath.EvalSymlinks(filename)
			if err != nil {
				return p, err
			}
			rel, e := filepath.Rel(root, filename)
			if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return p, fmt.Errorf("pack %q escapes root", name)
			}
			identity = filename
			base = filepath.Dir(filename)
			content, err = os.ReadFile(filename)
			if err != nil {
				return p, err
			}
			p, err = Decode(strings.NewReader(string(content)))
			if err != nil {
				return p, fmt.Errorf("%s: %w", filename, err)
			}
		}
		if active[identity] {
			return p, fmt.Errorf("pack dependency cycle at %s", identity)
		}
		if loaded[identity] {
			return Pack{Version: 1}, nil
		}
		active[identity] = true
		defer delete(active, identity)
		out := Pack{Version: 1, Rules: []Rule{}}
		seen := map[string]bool{}
		appendRules := func(rules []Rule) error {
			for _, r := range rules {
				if seen[r.ID] {
					return fmt.Errorf("duplicate rule %q from %s; use overrides", r.ID, identity)
				}
				seen[r.ID] = true
				out.Rules = append(out.Rules, r)
			}
			return nil
		}
		for _, parent := range p.Extends {
			q, e := resolve(parent, base)
			if e != nil {
				return out, e
			}
			if e = appendRules(q.Rules); e != nil {
				return out, e
			}
			out.Suppressions = append(out.Suppressions, q.Suppressions...)
			out.Imports = append(out.Imports, q.Imports...)
		}
		for i := range p.Rules {
			p.Rules[i].Origin = identity
		}
		if err = appendRules(p.Rules); err != nil {
			return out, err
		}
		touched := map[string]bool{}
		for _, o := range p.Overrides {
			if !seen[o.ID] || touched[o.ID] {
				return out, fmt.Errorf("unknown or duplicate override ID %q", o.ID)
			}
			touched[o.ID] = true
			for i := range out.Rules {
				r := &out.Rules[i]
				if r.ID != o.ID {
					continue
				}
				if o.Enabled != nil {
					r.Enabled = o.Enabled
				}
				if o.Severity != nil {
					if err = r.setSeverity(*o.Severity); err != nil {
						return out, err
					}
				}
				if o.Include != nil {
					r.Include = *o.Include
				}
				if o.Exclude != nil {
					r.Exclude = *o.Exclude
				}
				if o.Options.Kind != 0 {
					r.Options = o.Options
				}
				r.Origin += "; override: " + identity
			}
		}
		out.Suppressions = append(out.Suppressions, p.Suppressions...)
		h := sha256.Sum256(content)
		out.Imports = append(out.Imports, Import{identity, hex.EncodeToString(h[:])})
		seenSuppressions := map[string]bool{}
		dedup := []Suppression{}
		for _, suppression := range out.Suppressions {
			key := fmt.Sprintf("%s|%s|%d|%s", suppression.Rule, suppression.Path, suppression.Line, suppression.Reason)
			if !seenSuppressions[key] {
				dedup = append(dedup, suppression)
				seenSuppressions[key] = true
			}
		}
		out.Suppressions = dedup
		loaded[identity] = true
		return out, nil
	}
	if _, ok := rawPreset(filename); !ok {
		filename, err = filepath.Abs(filename)
		if err != nil {
			return Pack{}, err
		}
	}
	return resolve(filename, root)
}

func (r Rule) MarshalJSON() ([]byte, error) {
	var options any
	if r.Options.Kind != 0 {
		if e := r.Options.Decode(&options); e != nil {
			return nil, e
		}
	}
	severity := string(r.Severity)
	if severity == "" {
		severity = "error"
	}
	enabled := r.Enabled == nil || *r.Enabled
	return json.Marshal(struct {
		ID       string   `json:"id"`
		Check    string   `json:"check"`
		Enabled  bool     `json:"enabled"`
		Severity string   `json:"severity"`
		Include  []string `json:"include,omitempty"`
		Exclude  []string `json:"exclude,omitempty"`
		Options  any      `json:"options,omitempty"`
		Origin   string   `json:"origin"`
	}{r.ID, r.Check, enabled, severity, r.Include, r.Exclude, options, r.Origin})
}
