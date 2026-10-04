package contract

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Requirement struct {
	Selector, Scope string
	Rules           []Rule
	IDs             []string
	Heading, Line   int
}
type TemplateHeading struct {
	Level   int
	Label   string
	Dynamic bool
	Line    int
}
type Template struct {
	Headings     []TemplateHeading
	Requirements []Requirement
}

var selectorRE = regexp.MustCompile(`^(document|heading|section|each[ \t]+(?:heading|section|paragraph|sentence|table|codeblock|source-line)|(?:heading|section|paragraph|sentence|table|codeblock|source-line)\[[1-9][0-9]*\](?:\.sentence\[[1-9][0-9]*\])?)`)
var assertionRE = regexp.MustCompile(`^([a-z][a-z0-9-]*)[ \t]*(<=|>=|=)[ \t]*`)
var integerRE = regexp.MustCompile(`^(0|[1-9][0-9]*)(?:\.\.(0|[1-9][0-9]*))?`)

func parseDirective(s string, p *Program, heading, line int) (Requirement, error) {
	r := Requirement{Heading: heading, Line: line, Rules: []Rule{}, IDs: []string{}}
	s = strings.TrimSpace(s)
	sel := selectorRE.FindString(s)
	if sel == "" {
		return r, fmt.Errorf("line %d: invalid target", line)
	}
	if strings.Contains(sel, ".sentence[") && !strings.HasPrefix(sel, "paragraph[") {
		return r, fmt.Errorf("only paragraph[n].sentence[m] is a chained path")
	}
	r.Selector = strings.Join(strings.Fields(sel), " ")
	s = s[len(sel):]
	if s == "" || s[0] != ' ' && s[0] != '\t' {
		return r, fmt.Errorf("directive needs assertions")
	}
	kind := r.Selector
	kind = strings.TrimPrefix(kind, "each ")
	if i := strings.IndexByte(kind, '['); i >= 0 {
		kind = kind[:i]
	}
	if strings.Contains(r.Selector, ".sentence[") {
		kind = "sentence"
	}
	seen := map[string]bool{}
	limits := map[string][2]*float64{}
	predicates := 0
	for strings.TrimSpace(s) != "" {
		s = strings.TrimLeft(s, " \t")
		m := assertionRE.FindStringSubmatch(s)
		if m == nil {
			return r, fmt.Errorf("line %d: invalid assertion near %q", line, s)
		}
		s = s[len(m[0]):]
		key, op := m[1], m[2]
		if s == "" {
			return r, fmt.Errorf("missing assertion value")
		}
		if s[0] == '"' {
			if op != "=" {
				return r, fmt.Errorf("string assertions require =")
			}
			end := 1
			escape := false
			for ; end < len(s); end++ {
				if escape {
					escape = false
					continue
				}
				if s[end] == '\\' {
					escape = true
					continue
				}
				if s[end] == '"' {
					break
				}
			}
			if end == len(s) {
				return r, fmt.Errorf("unterminated JSON string")
			}
			raw := s[:end+1]
			if !utf8.ValidString(raw) || !validEscapes(raw) {
				return r, fmt.Errorf("invalid Unicode scalar string")
			}
			var value string
			if err := json.Unmarshal([]byte(raw), &value); err != nil {
				return r, err
			}
			s = s[end+1:]
			token := key + op
			if key == "rule" {
				token += value
			}
			if seen[token] {
				return r, fmt.Errorf("duplicate assertion %s", key)
			}
			seen[token] = true
			if key == "scope" {
				if !has([]string{"direct", "subtree"}, value) || r.Selector == "document" || r.Selector == "heading" {
					return r, fmt.Errorf("scope not allowed on this target")
				}
				r.Scope = value
				continue
			}
			predicates++
			if key == "rule" {
				rule, ok := p.Rules[value]
				if !ok {
					return r, fmt.Errorf("unknown template rule %s", value)
				}
				r.Rules = append(r.Rules, rule)
				r.IDs = append(r.IDs, value)
				continue
			}
			allowed := map[string]string{"equals": "heading paragraph sentence source-line", "contains": "document section heading paragraph sentence source-line", "not-contains": "document section heading paragraph sentence source-line", "has-word": "document section heading paragraph sentence source-line", "no-word": "document section heading paragraph sentence source-line", "language": "codeblock"}
			if !has(strings.Fields(allowed[key]), kind) {
				return r, fmt.Errorf("property %s does not accept %s", key, kind)
			}
			mode, expect, view := "contains", "present", "auto"
			switch key {
			case "equals":
				mode = "equals"
			case "not-contains":
				expect = "absent"
			case "has-word":
				mode = "word"
			case "no-word":
				mode = "word"
				expect = "absent"
			case "language":
				mode = "equals"
				view = "language"
			}
			rule := Rule{Check: "core.match", Severity: "error", Unknown: "fail", Options: map[string]any{"mode": mode, "expect": expect, "view": view, "values": []string{value}}}
			if err := p.validateRule(rule); err != nil {
				return r, err
			}
			r.Rules = append(r.Rules, rule)
			r.IDs = append(r.IDs, "")
		} else {
			m2 := integerRE.FindStringSubmatch(s)
			if m2 == nil {
				return r, fmt.Errorf("integer or JSON string required")
			}
			s = s[len(m2[0]):]
			if seen[key+op] {
				return r, fmt.Errorf("duplicate numeric assertion")
			}
			seen[key+op] = true
			min, max := -1.0, -1.0
			a, err := strconv.ParseUint(m2[1], 10, 53)
			if err != nil {
				return r, err
			}
			switch op {
			case "=":
				min, max = float64(a), float64(a)
			case "<=":
				max = float64(a)
			case ">=":
				min = float64(a)
			}
			if m2[2] != "" {
				if op != "=" {
					return r, fmt.Errorf("ranges require =")
				}
				b, err := strconv.ParseUint(m2[2], 10, 53)
				if err != nil {
					return r, err
				}
				max = float64(b)
			}
			if min >= 0 && max >= 0 && min > max {
				return r, fmt.Errorf("reverse range")
			}
			if _, ok := measures[key]; !ok || strings.Contains(key, ".") {
				return r, fmt.Errorf("unknown inline measure %s", key)
			}
			pair := limits[key]
			if min >= 0 && (pair[0] == nil || min > *pair[0]) {
				v := min
				pair[0] = &v
			}
			if max >= 0 && (pair[1] == nil || max < *pair[1]) {
				v := max
				pair[1] = &v
			}
			if pair[0] != nil && pair[1] != nil && *pair[0] > *pair[1] {
				return r, fmt.Errorf("contradictory numeric assertions")
			}
			limits[key] = pair
			predicates++
		}
		if s != "" && s[0] != ' ' && s[0] != '\t' {
			return r, fmt.Errorf("assertions need whitespace separation")
		}
	}
	if predicates == 0 {
		return r, fmt.Errorf("scope-only or empty directive")
	}
	if r.Selector == "document" && heading >= 0 || (r.Selector == "section" || r.Selector == "heading") && heading < 0 {
		return r, fmt.Errorf("invalid directive context")
	}
	for _, m := range sortedKeys(limits) {
		pair := limits[m]
		o := map[string]any{"measure": m}
		if pair[0] != nil {
			o["min"] = *pair[0]
		}
		if pair[1] != nil {
			o["max"] = *pair[1]
		}
		r.Rules = append(r.Rules, Rule{Check: "core.limit", Severity: "error", Unknown: "fail", Options: o})
		r.IDs = append(r.IDs, "")
	}
	for _, rr := range r.Rules {
		if err := compatible(rr, kind, r.Scope); err != nil {
			return r, err
		}
	}
	return r, nil
}
func validEscapes(raw string) bool {
	for i := 0; i+5 < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		i++
		if raw[i] != 'u' {
			continue
		}
		v, err := strconv.ParseUint(raw[i+1:i+5], 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if v >= 0xD800 && v <= 0xDBFF {
			if i+6 >= len(raw) || raw[i+1:i+3] != "\\u" {
				return false
			}
			w, err := strconv.ParseUint(raw[i+3:i+7], 16, 16)
			if err != nil || w < 0xDC00 || w > 0xDFFF {
				return false
			}
			i += 6
		} else if v >= 0xDC00 && v <= 0xDFFF {
			return false
		}
	}
	return true
}
func directiveEnd(s string) (int, error) {
	quoted, escape := false, false
	for i := 2; i < len(s)-1; i++ {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		if quoted && c == '\\' {
			escape = true
			continue
		}
		if c == '"' {
			quoted = !quoted
			continue
		}
		if !quoted && s[i:i+2] == "}}" {
			return i, nil
		}
	}
	return 0, fmt.Errorf("unclosed directive")
}
func parseTemplate(s string, p *Program) (*Template, error) {
	t := &Template{}
	heading := -1
	fence := ""
	comment := false
	for i, line := range strings.Split(s, "\n") {
		line = strings.TrimSuffix(line, "\r")
		trim := strings.TrimSpace(line)
		if fence != "" {
			if len(trim) >= len(fence) && strings.TrimRight(trim, string(fence[0])) == "" {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~") {
			n := 0
			for n < len(trim) && trim[n] == trim[0] {
				n++
			}
			fence = trim[:n]
			continue
		}
		if comment {
			if strings.Contains(trim, "-->") {
				comment = false
			}
			continue
		}
		if strings.HasPrefix(trim, "<!--") {
			comment = !strings.Contains(trim, "-->")
			continue
		}
		if strings.Contains(trim, "`") { // Inline code is inert; remove complete code spans before context validation.
			trim = regexp.MustCompile("`+[^`]*`+").ReplaceAllString(trim, "")
		}
		prefix := 0
		for prefix < len(trim) && trim[prefix] == '#' {
			prefix++
		}
		isHeading := prefix > 0 && prefix <= 6 && prefix < len(trim) && trim[prefix] == ' '
		content := trim
		if isHeading {
			if heading < 0 && prefix != 1 || heading >= 0 && (prefix == 1 || prefix > t.Headings[heading].Level+1) {
				return nil, fmt.Errorf("line %d: invalid heading outline", i+1)
			}
			heading++
			content = strings.TrimSpace(trim[prefix:])
			t.Headings = append(t.Headings, TemplateHeading{Level: prefix, Line: i + 1})
		}
		at := strings.Index(content, "{{")
		if at >= 0 && (at == 0 || content[at-1] != '\\') {
			if !isHeading && at != 0 {
				return nil, fmt.Errorf("line %d: directive must occupy a line", i+1)
			}
			end, err := directiveEnd(content[at:])
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			end += at
			if strings.TrimSpace(content[end+2:]) != "" {
				return nil, fmt.Errorf("line %d: trailing directive content", i+1)
			}
			r, err := parseDirective(content[at+2:end], p, heading, i+1)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			if isHeading && r.Selector != "heading" {
				return nil, fmt.Errorf("inline heading directive requires heading target")
			}
			t.Requirements = append(t.Requirements, r)
			if isHeading {
				label := visibleHeadingLabel(strings.TrimSpace(content[:at]))
				t.Headings[heading].Label = normalized(label)
				t.Headings[heading].Dynamic = label == ""
			}
		} else if isHeading {
			t.Headings[heading].Label = visibleHeadingLabel(content)
			if t.Headings[heading].Label == "" {
				return nil, fmt.Errorf("empty heading")
			}
		}
	}
	if len(t.Headings) == 0 || t.Headings[0].Level != 1 {
		return nil, fmt.Errorf("template requires exactly one H1")
	}
	return t, nil
}

func visibleHeadingLabel(s string) string {
	d := Parse("template.md", []byte("# "+s+"\n"), "memory")
	for _, target := range d.Targets {
		if target.Kind == "heading" && len(target.Units) > 0 {
			return normalized(target.Units[0].Text)
		}
	}
	return ""
}
func (d *Document) requireTargets(r Requirement, headings []*Target) ([]*Target, int) {
	context := d.Root
	if r.Heading >= 0 {
		if r.Heading >= len(headings) {
			return nil, len(d.Source)
		}
		h := headings[r.Heading]
		context = nil
		for _, s := range d.Targets {
			if s.Kind == "section" && s.Heading == h {
				context = s
				break
			}
		}
		if context == nil {
			return nil, h.End
		}
		if r.Selector == "heading" {
			return []*Target{h}, h.Start
		}
		if r.Selector == "section" {
			return []*Target{context}, context.End
		}
	}
	if r.Selector == "document" {
		return []*Target{d.Root}, 0
	}
	sel := r.Selector
	if strings.HasPrefix(sel, "each ") {
		return d.selectTargets(context, sel[5:], r.Scope), context.End
	}
	parts := strings.Split(sel, ".")
	var result *Target
	for _, part := range parts {
		bracket := strings.IndexByte(part, '[')
		kind := part[:bracket]
		n, _ := strconv.Atoi(part[bracket+1 : len(part)-1])
		xs := d.selectTargets(context, kind, r.Scope)
		if n > len(xs) {
			return nil, context.End
		}
		result = xs[n-1]
		context = result
	}
	return []*Target{result}, context.End
}
