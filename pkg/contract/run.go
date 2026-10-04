package contract

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/portpowered/markdown-linter/pkg/rulepack"
	"github.com/rivo/uniseg"
	"gopkg.in/yaml.v3"
)

type execution struct {
	program  *Program
	report   *Report
	document *Document
	seen     map[string]bool
	origins  map[string][]int
	samples  map[string][]Sample
	cache    map[string]float64
	ctx      context.Context
}

func (p *Program) Run(ctx context.Context, root string, docs []*Document, report *Report) {
	p.Fixes = nil
	// Run document and corpus adapters once with their complete selected corpus.
	corpus := map[string][]*interfaces.Document{}
	specs := map[string]rulepack.Rule{}
	activateLegacy := func(id string, r Rule, d *Document) {
		if d.Format != "markdown" {
			report.Error("capability.format", r.Check+" does not declare MDX support", location(d.Path, d.Source, 0, 0))
			return
		}
		var node yaml.Node
		_ = node.Encode(r.Options)
		spec := rulepack.Rule{ID: id, Check: r.Check, Severity: interfaces.Severity(r.Severity), Options: node}
		if stock, ok := p.builtin[id]; ok {
			spec = stock
		}
		specs[id] = spec
		absolute := filepath.Join(root, filepath.FromSlash(d.Path))
		for _, old := range corpus[id] {
			if old.Path == absolute {
				return
			}
		}
		legacy := *d.Legacy
		legacy.Path = absolute
		corpus[id] = append(corpus[id], &legacy)
	}
	for _, d := range docs {
		if d.Format == "markdown" {
			syntax, err := p.Registry.Compile(rulepack.Pack{Version: 1, Rules: []rulepack.Rule{{ID: "syntax.fence", Check: "markdown.fence-closed"}, {ID: "syntax.frontmatter", Check: "markdown.frontmatter-valid"}}})
			if err != nil {
				report.Error("execution.syntax", err.Error(), nil)
				return
			}
			copyDoc := *d.Legacy
			copyDoc.Path = filepath.Join(root, filepath.FromSlash(d.Path))
			findings, err := syntax.Run(ctx, root, []*interfaces.Document{&copyDoc})
			if err != nil {
				report.Error("execution.syntax", err.Error(), nil)
				return
			}
			if len(findings) > 0 {
				for _, f := range findings {
					offset := 0
					for line := 1; line < f.Line; line++ {
						end := strings.IndexByte(string(d.Source[offset:]), '\n')
						if end < 0 {
							break
						}
						offset += end + 1
					}
					report.Error("input.parse", f.Message, location(d.Path, d.Source, offset, lineEnd(d.Source, offset)))
				}
				continue
			}
		}
		q, err := p.Policy(d.Path)
		if err != nil {
			report.Error("config.routing", err.Error(), location(d.Path, d.Source, 0, 0))
			continue
		}
		d.Policy = q
		report.Inputs = append(report.Inputs, Input{Path: d.Path, Source: d.SourceKind, Format: d.Format, Language: q.Language, Profile: q.Profile, Sets: q.Sets, Templates: q.Templates, SyntaxOnly: q.SyntaxOnly})
		e := &execution{program: p, report: report, document: d, seen: map[string]bool{}, cache: map[string]float64{}, ctx: ctx}
		for _, b := range q.Bindings {
			if err := ctx.Err(); err != nil {
				report.Error("execution.cancelled", err.Error(), nil)
				return
			}
			r, custom := p.Rules[b.Rule]
			if !custom || !has([]string{"core.limit", "core.match", "core.sequence", "markdown.table-schema", "mermaid.flowchart"}, r.Check) {
				activateLegacy(b.Rule, r, d)
				continue
			}
			kind, _ := bindingKind(b.On)
			targets := []*Target{d.Root}
			scope := b.Scope
			if b.On == "document" {
				scope = "subtree"
			} else {
				targets = d.selectTargets(d.Root, kind, "subtree")
			}
			if len(targets) == 0 {
				report.Summary.ZeroSelectionBindings++
			}
			for _, t := range targets {
				related := []Related{{Role: "definition", Location: p.origin("/rules/"+b.Rule, 0), Message: "Rule definition."}}
				for _, origin := range q.Origins[b.Rule+"|"+b.On+"|"+b.Scope] {
					related = append(related, Related{Role: "application", Location: p.origin(origin, 0), Message: "Rule activation."})
				}
				e.evaluate(b.Rule, r, t, scope, related)
			}
		}
		for _, name := range q.Templates {
			t := p.Templates[name]
			hs := d.selectTargets(d.Root, "heading", "subtree")
			if d.Root.Unknown {
				e.unknown("template."+name+".outline", Rule{Check: "template.outline", Severity: "error"}, d.Root, []Related{{Role: "template", Location: p.origin("/templates/"+name, 0), Message: "Outline definition."}}, "template.unknown-outline", "Dynamic content prevents complete outline validation.")
			}
			for i, h := range t.Headings {
				id := fmt.Sprintf("template.%s.heading.%d", name, i+1)
				origin := []Related{{Role: "template", Location: p.origin("/templates/"+name, h.Line), Message: "Required outline heading."}}
				report.Summary.EvaluatedTargets++
				if i >= len(hs) {
					report.Summary.FailedTargets++
					e.finding(id, Rule{Check: "template.outline", Severity: "error"}, &Target{Kind: "heading", Start: len(d.Source), End: len(d.Source)}, "template.missing-heading", "Required heading is missing (insertion point).", origin, h.Label, nil, nil)
					continue
				}
				actual := normalized(hs[i].Units[0].Text)
				if hs[i].Level != h.Level || !h.Dynamic && actual != h.Label || h.Dynamic && actual == "" {
					report.Summary.FailedTargets++
					e.finding(id, Rule{Check: "template.outline", Severity: "error"}, hs[i], "template.heading", "Heading does not match the required outline.", origin, h.Label, actual, nil)
				}
			}
			for i := len(t.Headings); i < len(hs); i++ {
				report.Summary.FailedTargets++
				e.finding("template."+name+".outline", Rule{Check: "template.outline", Severity: "error"}, hs[i], "template.extra-heading", "Extra heading is not allowed.", nil, len(t.Headings), len(hs), nil)
			}
			for j, rq := range t.Requirements {
				targets, insertion := d.requireTargets(rq, hs)
				id := fmt.Sprintf("template.%s.%d", name, j+1)
				origin := []Related{{Role: "template", Location: p.origin("/templates/"+name, rq.Line), Message: "Template requirement."}}
				if len(targets) == 0 {
					if d.Root.Unknown {
						e.unknown(id, Rule{Check: "template.target", Severity: "error"}, d.Root, origin, "template.unknown-target", "Dynamic content prevents required target selection.")
						continue
					}
					if strings.HasPrefix(rq.Selector, "each ") {
						report.Summary.ZeroSelectionBindings++
						continue
					}
					report.Summary.EvaluatedTargets++
					report.Summary.FailedTargets++
					e.finding(id, Rule{Check: "template.target", Severity: "error"}, &Target{Kind: rq.Selector, Start: insertion, End: insertion}, "template.missing-target", "Required target is missing (insertion point).", origin, rq.Selector, nil, nil)
					continue
				}
				for _, target := range targets {
					scope := rq.Scope
					if target.Kind == "document" {
						scope = "subtree"
					}
					for k, rr := range rq.Rules {
						rid := rq.IDs[k]
						related := append([]Related{}, origin...)
						if rid == "" {
							rid = fmt.Sprintf("%s.%d", id, k+1)
						} else {
							related = append(related, Related{Role: "definition", Location: p.origin("/rules/"+rid, 0), Message: "Rule definition."})
						}
						if !has([]string{"core.limit", "core.match", "core.sequence", "markdown.table-schema", "mermaid.flowchart"}, rr.Check) {
							activateLegacy(rid, rr, d)
						} else {
							e.evaluate(rid, rr, target, scope, related)
						}
					}
				}
			}
		}
	}
	for _, id := range sortedKeys(corpus) {
		spec := specs[id]
		program, err := p.Registry.Compile(rulepack.Pack{Version: 1, Rules: []rulepack.Rule{spec}})
		if err != nil {
			report.Error("config.rule", err.Error(), p.origin("/rules/"+id, 0))
			continue
		}
		results, err := program.Run(ctx, root, corpus[id])
		if err != nil {
			report.Error("execution.check", err.Error(), nil)
			continue
		}
		report.Summary.EvaluatedTargets += len(corpus[id])
		for _, finding := range results {
			rel, err := filepath.Rel(root, finding.Path)
			if err != nil {
				report.Error("execution.path", err.Error(), nil)
				continue
			}
			file := filepath.ToSlash(rel)
			var d *Document
			for _, candidate := range docs {
				if candidate.Path == file {
					d = candidate
					break
				}
			}
			if d == nil {
				continue
			}
			a, b := finding.StartOffset, finding.EndOffset
			if a < 0 || b < a {
				a = 0
				for line := 1; line < finding.Line; line++ {
					end := strings.IndexByte(string(d.Source[a:]), '\n')
					if end < 0 {
						break
					}
					a += end + 1
				}
				b = lineEnd(d.Source, a)
			}
			e := &execution{program: p, document: d, report: report}
			report.Summary.FailedTargets++
			before := len(report.Diagnostics)
			e.finding(id, Rule{Check: spec.Check, Severity: string(spec.Severity)}, &Target{Kind: "document", Start: a, End: b}, "stock."+spec.Check, finding.Message, []Related{}, nil, nil, nil)
			if len(report.Diagnostics) > before {
				p.Fixes = append(p.Fixes, finding)
			}
		}
	}
	report.Finish()
}
func (e *execution) finding(id string, r Rule, t *Target, code, message string, related []Related, expected, actual any, ms []Measurement) {
	loc := location(e.document.Path, e.document.Source, t.Start, t.End)
	if e.document.Format == "markdown" && inlineSuppressed(e.document, id, loc.Start.Line) {
		e.report.Summary.Suppressed++
		return
	}
	for _, s := range e.program.Config.Suppressions {
		if s.Rule == id && Match(s.Path, e.document.Path) && (s.Line == 0 || s.Line == loc.Start.Line) {
			e.report.Summary.Suppressed++
			return
		}
	}
	if related == nil {
		related = []Related{}
	}
	if ms == nil {
		ms = []Measurement{}
	}
	severity := r.Severity
	if severity == "" {
		severity = "error"
	}
	attribution := "region"
	if t.Start == t.End {
		attribution = "insertion"
	}
	parser := "goldmark-1.7.8"
	if e.document.Format == "mdx" {
		parser = "remark-mdx-3.1.1+remark-gfm-4.0.1"
	}
	e.report.Diagnostics = append(e.report.Diagnostics, Diagnostic{Code: code, RuleID: id, CheckID: r.Check, Severity: severity, Result: "fail", Message: message, Target: t.Kind, Attribution: attribution, Location: loc, Related: related, Expected: expected, Actual: actual, Measurements: ms, Provenance: map[string]any{"language": e.document.Policy.Language, "profile": e.document.Policy.Profile, "parser": parser, "segmentation": "uniseg-0.4.7"}, Remediation: "Revise the selected content to satisfy this requirement."})
}
func (e *execution) evaluate(id string, r Rule, t *Target, scope string, related []Related) {
	if scope == "" {
		scope = "direct"
	}
	if t.Kind == "document" {
		scope = "subtree"
	}
	key := fmt.Sprintf("%s|%p|%s", id, t, scope)
	if e.seen[key] {
		for _, index := range e.origins[key] {
			d := &e.report.Diagnostics[index]
			for _, origin := range related {
				duplicate := false
				for _, old := range d.Related {
					if old.Role == origin.Role && old.Location != nil && origin.Location != nil && *old.Location == *origin.Location {
						duplicate = true
					}
				}
				if !duplicate {
					d.Related = append(d.Related, origin)
				}
			}
		}
		return
	}
	e.seen[key] = true
	if e.origins == nil {
		e.origins = map[string][]int{}
	}
	start := len(e.report.Diagnostics)
	defer func() {
		for i := start; i < len(e.report.Diagnostics); i++ {
			e.origins[key] = append(e.origins[key], i)
		}
	}()
	if err := compatible(r, t.Kind, scope); err != nil {
		e.report.Error("capability.target", err.Error(), location(e.document.Path, e.document.Source, t.Start, t.End))
		return
	}
	e.report.Summary.EvaluatedTargets++
	o := r.Options
	view := str(o, "view", "auto")
	fail := false
	code, message := "check.failed", "Requirement failed."
	var expected, actual any
	ms := []Measurement{}
	evidence := t
	if t.Unknown && (r.Check != "core.match" || str(o, "expect", "present") == "absent" || !has([]string{"contains", "word"}, str(o, "mode", ""))) {
		e.report.Complete = false
		e.report.Summary.UnknownTargets++
		severity := r.Severity
		if r.Unknown == "report" {
			severity = "info"
		}
		before := len(e.report.Diagnostics)
		e.finding(id, Rule{Check: r.Check, Severity: severity}, t, "analysis.unknown", "Dynamic content prevents complete analysis.", related, nil, nil, nil)
		if len(e.report.Diagnostics) > before {
			e.report.Diagnostics[len(e.report.Diagnostics)-1].Result = "unknown"
		}
		return
	}
	switch r.Check {
	case "core.limit":
		m := str(o, "measure", "")
		if strings.HasPrefix(m, "readability.") && !strings.HasPrefix(e.document.Policy.Language, "en") {
			e.report.Error("capability.language", "English readability measure does not support this language.", location(e.document.Path, e.document.Source, t.Start, t.End))
			return
		}
		if installed, ok := e.program.Capabilities.lookup(r); ok {
			e.installedMeasure(id, r, t, scope, related, installed)
			return
		}
		if m == "provider.score" {
			e.report.Error("capability.provider", "No installed provider profile: "+str(object(o["parameters"]), "provider", ""), e.program.origin("/rules/"+id, 0))
			return
		}
		cacheKey := fmt.Sprintf("%p|%s|%s|%s|%v", t, m, view, scope, o["parameters"])
		value, ok := e.cache[cacheKey]
		if !ok {
			if strings.HasPrefix(m, "readability.") {
				value, ok = e.readability(t, m, view, scope, object(o["parameters"]))
				if !ok {
					e.report.Complete = false
					e.report.Summary.UnknownTargets++
					severity := r.Severity
					if r.Unknown == "report" {
						severity = "info"
					}
					before := len(e.report.Diagnostics)
					e.finding(id, Rule{Check: r.Check, Severity: severity}, t, "measure.insufficient-sample", "Readability needs an English sample meeting the configured word and sentence floors.", related, o["parameters"], nil, nil)
					if len(e.report.Diagnostics) > before {
						e.report.Diagnostics[len(e.report.Diagnostics)-1].Result = "unknown"
					}
					return
				}
			} else {
				value = e.document.measure(t, m, view, scope)
			}
			e.cache[cacheKey] = value
		}
		actual = value
		expected = map[string]any{"min": o["min"], "max": o["max"]}
		if n, ok := number(o["min"]); ok && value < n {
			fail = true
		}
		if n, ok := number(o["max"]); ok && value > n {
			fail = true
		}
		ms = append(ms, Measurement{Name: m, Value: value, Unit: m, Min: o["min"], Max: o["max"], Inputs: map[string]any{}, Sample: "target"})
		if strings.HasPrefix(m, "readability.") {
			w, s, y := e.readabilityInputs(t, view, scope)
			ms[0].Inputs = map[string]any{"words": w, "sentences": s, "syllables": y, "syllableProvider": "english-vowel-groups", "syllableVersion": "1", "fallbackPolicy": "estimate every ASCII English word"}
			if w < 100 || s < 3 {
				ms[0].Sample = "short"
			}
		}
		code = "limit.out-of-range"
		message = fmt.Sprintf("%s is %g, outside the required bounds.", m, value)
	case "core.match":
		if view == "auto" && (t.Kind == "document" || t.Kind == "section") {
			view = "visible"
		}
		mode := str(o, "mode", "")
		matched := false
		var matchUnit Unit
		var matchStarts, matchEnds []int
		start, end := 0, 0
		for _, u := range e.document.units(t, view, scope) {
			s, starts, ends := comparisonUnit(u, view != "source" && view != "code", str(o, "case", "sensitive") == "fold")
			switch mode {
			case "equals", "contains":
				for _, v := range list(o["values"]) {
					if view != "source" && view != "code" {
						v = normalized(v)
					}
					if str(o, "case", "sensitive") == "fold" {
						v = folded(v)
					}
					index := strings.Index(s, v)
					if mode == "equals" && s != v {
						index = -1
					}
					if index >= 0 {
						matched = true
						start, end = index, index+len(v)
						break
					}
				}
			case "word":
				state, pos := -1, 0
				for s != "" {
					var w string
					w, s, state = uniseg.FirstWordInString(s, state)
					for _, v := range list(o["values"]) {
						v = normalized(v)
						if str(o, "case", "sensitive") == "fold" {
							v = folded(v)
						}
						if w == v {
							matched = true
							start, end = pos, pos+len(w)
							break
						}
					}
					pos += len(w)
					if matched {
						break
					}
				}
			case "pattern":
				patterns, _ := compilePattern(str(o, "pattern", ""))
				if str(o, "case", "sensitive") == "fold" {
					for i, re := range patterns {
						patterns[i] = regexp.MustCompile(folded(re.String()))
					}
				}
				for _, re := range patterns {
					if re.MatchString(s) {
						matched = true
						break
					}
				}
			case "regex":
				pattern := str(o, "regex", "")
				if str(o, "case", "sensitive") == "fold" {
					pattern = "(?i)" + pattern
				}
				if str(o, "anchor", "search") == "full" {
					pattern = "\\A(?:" + pattern + ")\\z"
				}
				re, _ := regexp.Compile(pattern)
				if span := re.FindStringIndex(s); span != nil {
					matched = true
					start, end = span[0], span[1]
				}
			}
			if matched {
				matchUnit = u
				matchStarts, matchEnds = starts, ends
				break
			}
		}
		absent := str(o, "expect", "present") == "absent"
		if t.Unknown && !matched {
			e.unknown(id, r, t, related, "analysis.unknown", "Dynamic content prevents complete analysis.")
			return
		}
		fail = matched == absent
		code = "match.required"
		message = "Required match was not found."
		if absent {
			code = "match.forbidden"
			message = "Forbidden match was found."
			if mode == "word" {
				code = "match.forbidden-word"
			}
			if matched && end > start && end <= len(matchEnds) {
				copyTarget := *t
				copyTarget.Start = matchStarts[start]
				copyTarget.End = matchEnds[end-1]
				evidence = &copyTarget
			}
			_ = matchUnit
		}
		expected = o
		actual = matched
	case "core.sequence":
		xs := []string{}
		for _, b := range e.document.Root.Children {
			if b.Start < t.Start || b.Start >= t.End || b.Kind == "heading" {
				continue
			}
			child := false
			for _, s := range e.document.Targets {
				if s.Kind == "section" && s != t && s.Start >= t.Start && s.Start < t.End && b.Start >= s.Start && b.Start < s.End {
					child = true
					break
				}
			}
			if !child {
				xs = append(xs, b.Kind)
			}
		}
		actual = xs
		if o["sequence"] != nil {
			want := list(o["sequence"])
			expected = want
			fail = strings.Join(xs, "|") != strings.Join(want, "|")
		} else {
			want := list(o["allow"])
			expected = want
			for _, k := range xs {
				if !has(want, k) {
					fail = true
				}
			}
		}
		code = "sequence.order"
		message = "Section blocks do not satisfy the required sequence."
	case "markdown.table-schema":
		want := list(o["header"])
		got := []string{}
		for _, c := range t.Header {
			got = append(got, normalized(c.Units[0].Text))
		}
		for i := range want {
			want[i] = normalized(want[i])
		}
		fail = !slices.Equal(want, got) || t.ExtraCells
		code = "table.header"
		message = "Table header or cell count does not match the schema."
		expected = want
		actual = got
		if !fail {
			for j, h := range want {
				col := object(object(o["columns"])[h])
				seen := map[string]bool{}
				for _, row := range t.Rows {
					cell := &Target{Kind: "cell", Start: t.End, End: t.End, Units: []Unit{{Text: ""}}}
					if j < len(row) {
						cell = row[j]
					}
					text := normalized(cell.Units[0].Text)
					unique, _ := col["unique"].(bool)
					if unique && seen[text] {
						e.report.Summary.FailedTargets++
						e.finding(id, r, cell, "table.duplicate-cell", "Column values must be unique.", related, "unique", text, nil)
					}
					seen[text] = true
					for _, ref := range list(col["rules"]) {
						e.evaluate(ref, e.program.Rules[ref], cell, "direct", append(append([]Related{}, related...), Related{Role: "definition", Location: e.program.origin("/rules/"+ref, 0), Message: "Cell rule."}))
					}
				}
			}
		}
	case "mermaid.flowchart":
		issues := flowchart(t, o)
		if len(issues) == 0 {
			result, err := parseRuntime(e.ctx, "mermaid", t.Code)
			if err != nil {
				e.report.Error("execution.parser", err.Error(), location(e.document.Path, e.document.Source, t.Start, t.End))
				return
			}
			if !result.OK {
				if strings.Contains(result.Error, "DOMPurify") || strings.Contains(result.Error, "Cannot find") {
					e.report.Error("execution.parser", result.Error, location(e.document.Path, e.document.Source, t.Start, t.End))
					return
				}
				issues = append(issues, fmt.Sprintf("Mermaid syntax at payload line %d: %s", result.Line, result.Error))
			}
		}
		fail = len(issues) > 0
		expected = o
		actual = issues
		code = "mermaid.flowchart"
		message = strings.Join(issues, "; ")
	}
	if fail {
		e.report.Summary.FailedTargets++
		e.finding(id, r, evidence, code, message, related, expected, actual, ms)
	}
}
func (e *execution) readability(t *Target, m, view, scope string, params map[string]any) (float64, bool) {
	if !strings.HasPrefix(e.document.Policy.Language, "en") {
		return 0, false
	}
	w, s, y := e.readabilityInputs(t, view, scope)
	if w < 0 {
		return 0, false
	}
	mw, ms := 100.0, 3.0
	if n, ok := number(params["min-words"]); ok {
		mw = n
	}
	if n, ok := number(params["min-sentences"]); ok {
		ms = n
	}
	if float64(w) < mw || float64(s) < ms || w == 0 || s == 0 {
		return 0, false
	}
	if m == "readability.flesch-reading-ease" {
		return 206.835 - 1.015*float64(w)/float64(s) - 84.6*float64(y)/float64(w), true
	}
	return .39*float64(w)/float64(s) + 11.8*float64(y)/float64(w) - 15.59, true
}
func (e *execution) readabilityInputs(t *Target, view, scope string) (int, int, int) {
	w, s, y := 0, 0, 0
	for _, u := range e.document.units(t, view, scope) {
		ws := words(u.Text)
		w += len(ws)
		s += len(sentenceSpans(u.Text))
		for _, word := range ws {
			for _, r := range word {
				if r > 127 {
					return -1, -1, -1
				}
			}
			vowel, last := 0, false
			for _, r := range strings.ToLower(word) {
				is := strings.ContainsRune("aeiouy", r)
				if is && !last {
					vowel++
				}
				last = is
			}
			if strings.HasSuffix(strings.ToLower(word), "e") && vowel > 1 {
				vowel--
			}
			if vowel == 0 {
				vowel = 1
			}
			y += vowel
		}
	}
	return w, s, y
}
func flowchart(t *Target, o map[string]any) []string {
	issues := []string{}
	if t.Language != "mermaid" {
		issues = append(issues, "Requires mermaid language")
	}
	lines := strings.Split(t.Code, "\n")
	first := -1
	seen := map[string]int{}
	for i, l := range lines {
		trim := strings.TrimSpace(l)
		if trim == "" {
			continue
		}
		if first < 0 {
			first = i
			re := regexp.MustCompile(`^flowchart (TB|TD|BT|LR|RL)\s*;?$`)
			m := re.FindStringSubmatch(trim)
			if m == nil {
				issues = append(issues, "First nonblank line must be flowchart DIRECTION")
			} else if direction := str(o, "direction", ""); direction != "" && m[1] != direction {
				issues = append(issues, "Wrong flowchart direction")
			}
			continue
		}
		if n, ok := number(o["source-indent"]); ok {
			indent := len(l) - len(strings.TrimLeft(l, " "))
			if float64(indent) != n || strings.ContainsRune(l, '\t') {
				issues = append(issues, fmt.Sprintf("Wrong indentation on payload line %d", i+1))
			}
		}
		for _, field := range []string{"accTitle", "accDescr"} {
			if strings.HasPrefix(trim, field+":") {
				seen[field]++
				if strings.TrimSpace(strings.TrimPrefix(trim, field+":")) == "" {
					issues = append(issues, "Empty "+field)
				}
			}
		}
		if strings.Contains(trim, "accDescr {") || strings.HasPrefix(trim, "---") || strings.HasPrefix(trim, "%%{") {
			issues = append(issues, "Multiline accessibility/configuration is unsupported")
		}
	}
	if first < 0 {
		issues = append(issues, "Empty Mermaid payload")
	}
	for field, key := range map[string]string{"accTitle": "accessible-title", "accDescr": "accessible-description"} {
		policy := str(o, key, "optional")
		if seen[field] > 1 || policy == "required" && seen[field] != 1 || policy == "forbidden" && seen[field] > 0 {
			issues = append(issues, "Accessibility policy failed for "+field)
		}
	}
	return issues
}
