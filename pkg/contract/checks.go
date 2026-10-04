package contract

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/portpowered/markdown-linter/pkg/rulepack"
	"github.com/rivo/uniseg"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"
)

var measures = map[string]string{
	"words": "document section heading paragraph sentence source-line cell", "graphemes": "document section heading paragraph sentence source-line cell codeblock", "sentences": "document section paragraph cell",
	"paragraphs": "document section", "tables": "document section", "codeblocks": "document section", "lists": "document section", "blockquotes": "document section", "blocks": "document section", "headings": "document", "sections": "document", "subsections": "section", "max-heading-level": "document", "level": "heading", "rows": "table", "columns": "table", "lines": "codeblock", "source-lines": "document section", "bytes": "document section",
	"readability.flesch-reading-ease": "document section paragraph sentence", "readability.flesch-kincaid-grade": "document section paragraph sentence", "provider.score": "document section paragraph sentence",
}

func normalized(s string) string { return strings.Join(strings.Fields(norm.NFC.String(s)), " ") }
func folded(s string) string     { return cases.Fold().String(s) }
func words(s string) []string {
	out := []string{}
	state := -1
	for s != "" {
		var word string
		word, s, state = uniseg.FirstWordInString(s, state)
		for _, r := range word {
			if unicode.IsLetter(r) || unicode.IsNumber(r) {
				out = append(out, word)
				break
			}
		}
	}
	return out
}
func sentenceSpans(s string) [][2]int {
	out := [][2]int{}
	state, pos := -1, 0
	for s != "" {
		var part string
		part, s, state = uniseg.FirstSentenceInString(s, state)
		a, b := pos, pos+len(part)
		pos = b
		left := strings.TrimLeftFunc(part, unicode.IsSpace)
		a += len(part) - len(left)
		right := strings.TrimRightFunc(left, unicode.IsSpace)
		b = a + len(right)
		if b > a {
			out = append(out, [2]int{a, b})
		}
	}
	return out
}
func fields(o map[string]any, allowed string) error {
	for k, v := range o {
		if !has(strings.Fields(allowed), k) {
			return fmt.Errorf("unknown option %s", k)
		}
		if v == nil {
			return fmt.Errorf("null option %s", k)
		}
	}
	return nil
}
func stringField(o map[string]any, key string) error {
	if v, ok := o[key]; ok {
		if _, ok := v.(string); !ok {
			return fmt.Errorf("%s must be a string", key)
		}
	}
	return nil
}
func stringsField(v any, nonempty bool) error {
	if v == nil && !nonempty {
		return nil
	}
	xs, ok := v.([]any)
	if !ok {
		if a, ok := v.([]string); ok {
			xs = make([]any, len(a))
			for i, s := range a {
				xs[i] = s
			}
		} else {
			return fmt.Errorf("expected string array")
		}
	}
	if nonempty && len(xs) == 0 {
		return fmt.Errorf("array must not be empty")
	}
	for _, x := range xs {
		if s, ok := x.(string); !ok || normalized(s) == "" {
			return fmt.Errorf("expected nonblank string")
		}
	}
	return nil
}
func (p *Program) validateRule(r Rule) error {
	o := r.Options
	switch r.Check {
	case "core.limit":
		if err := fields(o, "measure min max view parameters"); err != nil {
			return err
		}
		for _, k := range []string{"measure", "view"} {
			if err := stringField(o, k); err != nil {
				return err
			}
		}
		m := str(o, "measure", "")
		_, installed := p.Capabilities.lookup(r)
		if _, ok := measures[m]; !ok && !installed {
			return fmt.Errorf("unknown measure %s", m)
		}
		if o["min"] == nil && o["max"] == nil {
			return fmt.Errorf("at least one bound is required")
		}
		for _, k := range []string{"min", "max"} {
			if v, ok := o[k]; ok {
				n, ok := number(v)
				if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
					return fmt.Errorf("invalid bound %s", k)
				}
				if !strings.Contains(m, ".") && (n < 0 || n != math.Trunc(n)) {
					return fmt.Errorf("count bounds must be nonnegative integers")
				}
			}
		}
		a, okA := number(o["min"])
		b, okB := number(o["max"])
		if okA && okB && a > b {
			return fmt.Errorf("min exceeds max")
		}
		if !has(strings.Fields("auto prose visible source code language cell"), str(o, "view", "auto")) {
			return fmt.Errorf("invalid view")
		}
		if v, ok := o["parameters"]; ok && object(v) == nil {
			return fmt.Errorf("parameters must be an object")
		}
		params := object(o["parameters"])
		if params == nil {
			params = map[string]any{}
		}
		if installed {
			ext, _ := p.Capabilities.lookup(r)
			values := params
			if m == "provider.score" {
				values = map[string]any{}
				aggregation := str(params, "aggregation", "target")
				if aggregation == "each-window" && ext.Descriptor.Granularity != "window" || aggregation == "target" && ext.Descriptor.Granularity != "target" {
					return fmt.Errorf("aggregation does not match native provider granularity")
				}
				if str(params, "context", "target") == "paragraph" && !ext.Descriptor.SentenceAttribution {
					return fmt.Errorf("paragraph provider context requires an installed sentence attribution adapter")
				}
			}
			if err := validateJSON(ext.Descriptor.ParametersSchema, values); err != nil {
				return fmt.Errorf("measure parameters: %w", err)
			}
		}
		if strings.HasPrefix(m, "readability.") {
			if err := fields(params, "min-words min-sentences"); err != nil {
				return err
			}
			for k, v := range params {
				n, ok := number(v)
				if !ok || n < 1 || n != math.Trunc(n) {
					return fmt.Errorf("%s must be a positive integer", k)
				}
			}
		} else if m == "provider.score" {
			if err := fields(params, "provider measure aggregation context"); err != nil {
				return err
			}
			if str(params, "provider", "") == "" || str(params, "measure", "") == "" {
				return fmt.Errorf("provider and measure required")
			}
			if !has([]string{"target", "each-window"}, str(params, "aggregation", "target")) || !has([]string{"target", "paragraph"}, str(params, "context", "target")) {
				return fmt.Errorf("invalid provider parameters")
			}
		} else if len(params) > 0 && !installed {
			return fmt.Errorf("count measure accepts no parameters")
		}
	case "core.match":
		if err := fields(o, "mode expect values pattern regex anchor view case"); err != nil {
			return err
		}
		for _, k := range []string{"mode", "expect", "pattern", "regex", "anchor", "view", "case"} {
			if err := stringField(o, k); err != nil {
				return err
			}
		}
		mode := str(o, "mode", "")
		if !has(strings.Fields("equals contains word pattern regex"), mode) {
			return fmt.Errorf("invalid match mode")
		}
		if !has([]string{"present", "absent"}, str(o, "expect", "present")) || !has([]string{"sensitive", "fold"}, str(o, "case", "sensitive")) {
			return fmt.Errorf("invalid expectation or case")
		}
		if !has(strings.Fields("auto prose visible source code language cell"), str(o, "view", "auto")) {
			return fmt.Errorf("invalid view")
		}
		payload := "values"
		if mode == "pattern" {
			payload = "pattern"
		}
		if mode == "regex" {
			if str(o, "case", "sensitive") != "sensitive" {
				return fmt.Errorf("regex folding is expressed with regex flags")
			}
			payload = "regex"
		}
		for _, k := range []string{"values", "pattern", "regex"} {
			if k != payload && o[k] != nil {
				return fmt.Errorf("%s payload is forbidden for %s", k, mode)
			}
		}
		if mode != "regex" && o["anchor"] != nil {
			return fmt.Errorf("anchor requires regex")
		}
		if payload == "values" {
			if err := stringsField(o["values"], true); err != nil {
				return err
			}
			seen := map[string]bool{}
			for _, v := range list(o["values"]) {
				v = normalized(v)
				if str(o, "case", "sensitive") == "fold" {
					v = folded(v)
				}
				if seen[v] {
					return fmt.Errorf("duplicate operand")
				}
				seen[v] = true
				if tokens := words(v); mode == "word" && (len(tokens) != 1 || tokens[0] != v) {
					return fmt.Errorf("word operand must be one token")
				}
			}
		}
		if mode == "pattern" {
			if _, err := compilePattern(str(o, "pattern", "")); err != nil {
				return err
			}
		}
		if mode == "regex" {
			s := str(o, "regex", "")
			if len(s) > 4096 {
				return fmt.Errorf("regex exceeds 4096 bytes")
			}
			re, err := regexp.Compile(s)
			if err != nil {
				return err
			}
			if re.MatchString("") {
				return fmt.Errorf("regex must not match empty text")
			}
			if !has([]string{"full", "search"}, str(o, "anchor", "search")) {
				return fmt.Errorf("invalid regex anchor")
			}
		}
	case "core.sequence":
		if err := fields(o, "sequence allow"); err != nil {
			return err
		}
		if (o["sequence"] == nil) == (o["allow"] == nil) {
			return fmt.Errorf("exactly one of sequence or allow required")
		}
		key := "sequence"
		if o["allow"] != nil {
			key = "allow"
		}
		if err := stringsField(o[key], true); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, k := range list(o[key]) {
			if !has(strings.Fields("paragraph table codeblock list blockquote thematic-break component html opaque"), k) || key == "allow" && seen[k] {
				return fmt.Errorf("invalid block kind %s", k)
			}
			seen[k] = true
		}
	case "markdown.table-schema":
		if err := fields(o, "header columns"); err != nil {
			return err
		}
		if err := stringsField(o["header"], true); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, h := range list(o["header"]) {
			h = normalized(h)
			if seen[h] {
				return fmt.Errorf("duplicate normalized header")
			}
			seen[h] = true
		}
		if v, ok := o["columns"]; ok && object(v) == nil {
			return fmt.Errorf("columns must be an object")
		}
		columnNames := map[string]bool{}
		for k, v := range object(o["columns"]) {
			if k != normalized(k) || columnNames[normalized(k)] {
				return fmt.Errorf("column keys must be normalized and unique")
			}
			columnNames[normalized(k)] = true
			if !seen[normalized(k)] {
				return fmt.Errorf("unknown column %s", k)
			}
			col := object(v)
			if col == nil {
				return fmt.Errorf("column must be an object")
			}
			if err := fields(col, "unique rules"); err != nil {
				return err
			}
			if v, ok := col["unique"]; ok {
				if _, ok := v.(bool); !ok {
					return fmt.Errorf("unique must be boolean")
				}
			}
			if err := stringsField(col["rules"], false); err != nil {
				return err
			}
		}
	case "mermaid.flowchart":
		if err := fields(o, "direction accessible-title accessible-description source-indent"); err != nil {
			return err
		}
		if o["direction"] != nil && !has(strings.Fields("TB TD BT LR RL"), str(o, "direction", "")) {
			return fmt.Errorf("invalid direction")
		}
		for _, k := range []string{"accessible-title", "accessible-description"} {
			if !has(strings.Fields("optional required forbidden"), str(o, k, "optional")) {
				return fmt.Errorf("invalid accessibility policy")
			}
		}
		if v, ok := o["source-indent"]; ok {
			n, ok := number(v)
			if !ok || n < 0 || n > 8 || n != math.Trunc(n) {
				return fmt.Errorf("source-indent must be 0..8")
			}
		}
	default:
		if _, ok := p.Registry.Describe(r.Check); !ok {
			return fmt.Errorf("unknown check %s", r.Check)
		}
		if _, ok := o["dictionary-file"]; ok {
			return fmt.Errorf("secondary policy files are forbidden in version 2; inline the vocabulary")
		}
		var node yaml.Node
		if err := node.Encode(o); err != nil {
			return err
		}
		_, err := p.Registry.Compile(rulepack.Pack{Version: 1, Rules: []rulepack.Rule{{ID: "validate", Check: r.Check, Options: node}}})
		return err
	}
	return nil
}
func compatible(r Rule, kind, scope string) error {
	if r.Check == "core.limit" && str(object(r.Options["parameters"]), "context", "target") == "paragraph" && kind != "sentence" {
		return fmt.Errorf("paragraph context requires sentence target")
	}
	v := str(r.Options, "view", "auto")
	switch r.Check {
	case "core.limit":
		m := str(r.Options, "measure", "")
		if r.Measure != nil {
			if !has(r.Measure.Targets, kind) || !has(r.Measure.Views, v) {
				return fmt.Errorf("installed measure does not accept target/view")
			}
			return nil
		}
		if !has(strings.Fields(measures[m]), kind) {
			return fmt.Errorf("measure %s does not accept %s", m, kind)
		}
		text := has([]string{"words", "graphemes", "sentences"}, m) || strings.Contains(m, ".")
		switch {
		case kind == "codeblock":
			if m == "lines" {
				if v != "auto" && v != "code" {
					return fmt.Errorf("lines requires code view")
				}
			} else if m != "graphemes" || v != "code" && v != "language" {
				return fmt.Errorf("codeblock text count requires graphemes with code/language view")
			}
		case kind == "cell":
			if v != "auto" && v != "cell" {
				return fmt.Errorf("cell measure requires cell view")
			}
		case m == "bytes" || m == "source-lines":
			if v != "auto" && v != "source" {
				return fmt.Errorf("source count requires source view")
			}
		case text:
			if !has([]string{"auto", "prose", "visible"}, v) {
				return fmt.Errorf("text measure incompatible view %s", v)
			}
			if strings.Contains(m, ".") && v == "visible" {
				return fmt.Errorf("score requires prose view")
			}
		default:
			if v != "auto" {
				return fmt.Errorf("structural count accepts auto view only")
			}
		}
	case "core.match":
		mode := str(r.Options, "mode", "")
		if !has(strings.Fields("document section heading paragraph sentence source-line codeblock cell"), kind) {
			return fmt.Errorf("matcher does not accept %s", kind)
		}
		if (v == "code" || v == "language") && kind != "codeblock" || v == "cell" && kind != "cell" {
			return fmt.Errorf("view %s does not accept %s", v, kind)
		}
		if kind == "codeblock" && v != "code" && v != "language" && v != "source" {
			return fmt.Errorf("codeblock matcher requires explicit view")
		}
		if kind == "cell" && v != "auto" && v != "cell" {
			return fmt.Errorf("cell matcher requires cell view")
		}
		if (mode == "word" || mode == "pattern") && has([]string{"source", "code", "language"}, v) {
			return fmt.Errorf("word/pattern mode incompatible view")
		}
		if (kind == "document" || kind == "section") && v != "source" && (mode == "equals" || mode == "pattern" || mode == "regex" && str(r.Options, "anchor", "search") == "full") {
			return fmt.Errorf("whole match is invalid on aggregate prose")
		}
	case "core.sequence":
		if kind != "section" || scope == "subtree" {
			return fmt.Errorf("sequence requires direct section")
		}
	case "markdown.table-schema":
		if kind != "table" {
			return fmt.Errorf("table-schema requires table")
		}
	case "mermaid.flowchart":
		if kind != "codeblock" {
			return fmt.Errorf("flowchart requires codeblock")
		}
	default:
		if kind != "document" {
			return fmt.Errorf("legacy check %s accepts document only", r.Check)
		}
	}
	return nil
}

// compilePattern lowers the bounded surface language to Go's linear-time RE2 engine.
func compilePattern(s string) ([]*regexp.Regexp, error) {
	if len(s) > 65536 {
		return nil, fmt.Errorf("pattern exceeds 64 KiB")
	}
	out := []*regexp.Regexp{}
	for _, line := range strings.Split(s, "\n") {
		line = normalized(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(out) == 64 || utf8.RuneCountInString(line) > 4096 {
			return nil, fmt.Errorf("pattern limit exceeded")
		}
		var b strings.Builder
		b.WriteString("(?s)^")
		names := map[string]bool{}
		literal := false
		previousCapture := false
		separator := ""
		for i := 0; i < len(line); {
			switch line[i] {
			case '\\':
				if i+1 == len(line) || !strings.ContainsRune("{}\\#", rune(line[i+1])) {
					return nil, fmt.Errorf("invalid pattern escape")
				}
				b.WriteString(regexp.QuoteMeta(line[i+1 : i+2]))
				literal = true
				separator += line[i+1 : i+2]
				i += 2
			case '{':
				end := strings.IndexByte(line[i:], '}')
				if end < 0 {
					return nil, fmt.Errorf("unclosed capture")
				}
				end += i
				name := line[i+1 : end]
				if !regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`).MatchString(name) || names[name] || len(names) == 8 {
					return nil, fmt.Errorf("invalid, duplicate or excess capture")
				}
				if previousCapture && strings.TrimSpace(separator) == "" {
					return nil, fmt.Errorf("neighboring captures need literal separator")
				}
				names[name] = true
				b.WriteString("(.+?)")
				previousCapture = true
				separator = ""
				i = end + 1
			case '}':
				return nil, fmt.Errorf("unescaped closing brace")
			default:
				_, n := utf8.DecodeRuneInString(line[i:])
				piece := line[i : i+n]
				b.WriteString(regexp.QuoteMeta(piece))
				if strings.TrimSpace(piece) != "" {
					literal = true
				}
				separator += piece
				i += n
			}
		}
		if !literal {
			return nil, fmt.Errorf("pattern needs literal text")
		}
		b.WriteString("$")
		re, err := regexp.Compile(b.String())
		if err != nil {
			return nil, err
		}
		out = append(out, re)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("pattern has no alternatives")
	}
	return out, nil
}
