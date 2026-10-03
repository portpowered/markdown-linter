package rulepack

import (
	"context"
	"fmt"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/yuin/goldmark/ast"
	"gopkg.in/yaml.v3"
	"regexp"
	"strings"
	"unicode"
)

type markdownOptions struct {
	Indent           int               `yaml:"indent"`
	Style            string            `yaml:"style"`
	Max              int               `yaml:"max"`
	AllowHardBreaks  *bool             `yaml:"allow-hard-breaks"`
	Allow            []string          `yaml:"allow"`
	Terms            map[string]string `yaml:"terms"`
	Dictionary       []string          `yaml:"dictionary"`
	Language         string            `yaml:"language"`
	Scope            string            `yaml:"scope"`
	FrontmatterTitle bool              `yaml:"frontmatter-title"`
}
type markdownCheck struct {
	id      string
	options markdownOptions
}

func (c markdownCheck) ID() string { return c.id }
func markdownFactory(id string) Factory {
	return func(n yaml.Node) (interfaces.Analyzer, error) {
		if e := validateCheckOptions(id, n); e != nil {
			return nil, e
		}
		c := markdownCheck{id: id, options: defaultOptions()}
		if err := DecodeOptions(n, &c.options); err != nil {
			return nil, err
		}
		if c.options.Indent < 1 || c.options.Indent > 8 {
			return nil, fmt.Errorf("indent must be 1 through 8")
		}
		if c.options.Max < 1 {
			return nil, fmt.Errorf("max must be positive")
		}
		if c.options.Style != "one-or-ordered" && c.options.Style != "ordered" && c.options.Style != "one" {
			return nil, fmt.Errorf("unknown list style %q", c.options.Style)
		}
		if c.options.Scope != "" && c.options.Scope != "prose" && c.options.Scope != "heading" {
			return nil, fmt.Errorf("scope must be prose or heading")
		}
		if id == "text.terminology" && len(c.options.Terms) == 0 {
			return nil, fmt.Errorf("terminology requires terms")
		}
		if id == "text.spelling" && (c.options.Language == "" || len(c.options.Dictionary) == 0) {
			return nil, fmt.Errorf("spelling requires language and an offline dictionary")
		}
		return c, nil
	}
}

var referenceDefinition = regexp.MustCompile(`^ {0,3}\[([^\]]+)\]:\s*\S`)
var referenceUse = regexp.MustCompile(`!?\[([^\]]+)\]\[([^\]]*)\]`)
var words = regexp.MustCompile(`[\pL][\pL\pM']*`)

func normLabel(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
func allowed(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

func (c markdownCheck) Analyze(ctx context.Context, pass *interfaces.Pass) {
	for _, doc := range pass.Documents {
		if ctx.Err() != nil {
			return
		}
		report := func(start, end int, message string, fix string) {
			d := interfaces.NewDiagnostic(doc.Path, doc.LineForOffset(start), start, end, c.id, message, interfaces.SeverityError)
			if fix != "" {
				d.SuggestedFixes = []interfaces.SuggestedFix{{Title: message, Confidence: interfaces.FixConfidenceSafe, Edits: []interfaces.TextEdit{{StartOffset: start, EndOffset: end, Replacement: fix}}}}
			}
			pass.Report(d)
		}
		code := make([]bool, len(doc.Source)+1)
		frontEnd := 0
		if strings.HasPrefix(string(doc.Source), "---\n") || strings.HasPrefix(string(doc.Source), "---\r\n") {
			offset := 0
			for i, line := range strings.SplitAfter(string(doc.Source), "\n") {
				offset += len(line)
				if i > 0 && strings.TrimSpace(line) == "---" {
					frontEnd = offset
					break
				}
			}
		}
		ast.Walk(doc.Root, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
			if !enter {
				return ast.WalkContinue, nil
			}
			switch n.(type) {
			case *ast.CodeSpan:
				for child := n.FirstChild(); child != nil; child = child.NextSibling() {
					if text, ok := child.(*ast.Text); ok {
						for j := text.Segment.Start; j < text.Segment.Stop && j < len(code); j++ {
							code[j] = true
						}
					}
				}
			case *ast.CodeBlock, *ast.FencedCodeBlock:
				for i := 0; i < n.Lines().Len(); i++ {
					seg := n.Lines().At(i)
					for j := seg.Start; j < seg.Stop && j < len(code); j++ {
						code[j] = true
					}
				}
			}
			return ast.WalkContinue, nil
		})
		start := 0
		h1 := 0
		lastWord := ""
		lastWordEnd := 0
		lastWordStart := 0
		headings := map[string]bool{}
		parents := make([]string, 7)
		switch c.id {
		case "markdown.final-newline":
			source := string(doc.Source)
			if source != "" && (!strings.HasSuffix(source, "\n") || strings.HasSuffix(source, "\n\n") || strings.HasSuffix(source, "\r\n\r\n")) {
				end := len(source)
				trim := strings.TrimRight(source, "\r\n")
				nl := "\n"
				if strings.Contains(source, "\r\n") {
					nl = "\r\n"
				}
				report(len(trim), end, "end file with one newline", nl)
			}
		case "markdown.fence-closed", "markdown.formatting":
			pattern := regexp.MustCompile("^ *(?:> *)*(?:[-+*] +|[0-9]+[.)] +)?(`{3,}|~{3,})(.*)$")
			open := ""
			openAt := 0
			for _, line := range strings.SplitAfter(string(doc.Source), "\n") {
				m := pattern.FindStringSubmatch(strings.TrimRight(line, "\r\n"))
				if m != nil {
					if open == "" && code[start] {
						start += len(line)
						continue
					}
					if open == "" {
						open = m[1]
						openAt = start
					} else if m[1][0] == open[0] && len(m[1]) >= len(open) && strings.TrimSpace(m[2]) == "" {
						open = ""
					}
				}
				start += len(line)
			}
			if open != "" {
				report(openAt, openAt, "close fenced code block", "")
			}
		case "markdown.reference-definitions":
			defs := map[string]bool{}
			start = 0
			for _, line := range strings.SplitAfter(string(doc.Source), "\n") {
				if start >= frontEnd && !code[start] {
					if m := referenceDefinition.FindStringSubmatch(line); m != nil {
						key := normLabel(m[1])
						if defs[key] {
							report(start, start+len(line), "duplicate reference definition "+m[1], "")
						}
						defs[key] = true
					}
				}
				start += len(line)
			}
			start = 0
			for _, line := range strings.SplitAfter(string(doc.Source), "\n") {
				if start >= frontEnd && !code[start] {
					for _, m := range referenceUse.FindAllStringSubmatchIndex(line, -1) {
						if code[start+m[0]] {
							continue
						}
						label := line[m[4]:m[5]]
						if label == "" {
							label = line[m[2]:m[3]]
						}
						if !defs[normLabel(label)] {
							report(start+m[0], start+m[1], "undefined reference "+label, "")
						}
					}
				}
				start += len(line)
			}
		case "markdown.frontmatter-valid":
			if strings.HasPrefix(string(doc.Source), "---\n") || strings.HasPrefix(string(doc.Source), "---\r\n") {
				if frontEnd == 0 {
					report(0, 0, "close YAML frontmatter", "")
				} else {
					lines := strings.Split(string(doc.Source[:frontEnd]), "\n")
					body := strings.Join(lines[1:len(lines)-2], "\n")
					var fields map[string]any
					if e := yaml.Unmarshal([]byte(body), &fields); e != nil {
						report(0, 0, "invalid YAML frontmatter: "+e.Error(), "")
					}
				}
			}
		}
		if c.id == "markdown.trailing-whitespace" || c.id == "markdown.formatting" || c.id == "markdown.line-length" {
			start = 0
			for _, raw := range strings.SplitAfter(string(doc.Source), "\n") {
				line := strings.TrimRight(raw, "\r\n")
				if !code[start] && start >= frontEnd {
					if c.id == "markdown.line-length" {
						if len([]rune(line)) > c.options.Max && !strings.Contains(line, "|") && !strings.Contains(line, "://") {
							report(start, start+len(line), fmt.Sprintf("line exceeds %d characters", c.options.Max), "")
						}
					} else {
						trim := strings.TrimRight(line, " \t")
						hard := strings.HasSuffix(line, "  ") && !strings.HasSuffix(line, "   ")
						permit := c.options.AllowHardBreaks == nil || *c.options.AllowHardBreaks
						if len(trim) != len(line) && !(hard && permit) {
							d := interfaces.NewDiagnostic(doc.Path, doc.LineForOffset(start), start+len(trim), start+len(line), c.id, "remove trailing whitespace", interfaces.SeverityError)
							d.SuggestedFixes = []interfaces.SuggestedFix{{Title: "Remove trailing whitespace", Confidence: interfaces.FixConfidenceSafe, Edits: []interfaces.TextEdit{{StartOffset: d.StartOffset, EndOffset: d.EndOffset, Replacement: ""}}}}
							pass.Report(d)
						}
					}
				}
				start += len(raw)
			}
		}
		ast.Walk(doc.Root, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
			if !enter {
				return ast.WalkContinue, nil
			}
			if ctx.Err() != nil {
				return ast.WalkStop, ctx.Err()
			}
			if _, ok := n.(*ast.CodeSpan); ok {
				return ast.WalkSkipChildren, nil
			}
			if _, ok := n.(*ast.AutoLink); ok {
				return ast.WalkSkipChildren, nil
			}
			if n.Type() == ast.TypeBlock && n.Lines().Len() > 0 && n.Lines().At(0).Start < frontEnd {
				return ast.WalkSkipChildren, nil
			}
			switch v := n.(type) {
			case *ast.Heading:
				text := string(v.Text(doc.Source))
				if v.Level == 1 {
					h1++
				}
				if c.id == "markdown.heading-duplicates" {
					key := strings.Join(parents[:v.Level], "/") + "/" + text
					if headings[key] {
						report(v.Lines().At(0).Start, v.Lines().At(0).Stop, "duplicate sibling heading "+text, "")
					}
					headings[key] = true
				}
				parents[v.Level] = text
				for i := v.Level + 1; i < 7; i++ {
					parents[i] = ""
				}
			case *ast.List:
				if c.id == "markdown.ordered-list" && v.IsOrdered() {
					position := 1
					allOne, ordered := true, true
					locations := []int{}
					actual := []int{}
					re := regexp.MustCompile(`^\s*(\d+)[.)]\s`)
					for item := v.FirstChild(); item != nil; item = item.NextSibling() {
						offset := offsetForNode(item)
						m := re.FindStringSubmatch(doc.SourceLine(doc.LineForOffset(offset)))
						number := position
						if m != nil {
							fmt.Sscan(m[1], &number)
						}
						allOne = allOne && number == 1
						ordered = ordered && number == position
						locations = append(locations, offset)
						actual = append(actual, number)
						position++
					}
					valid := ordered
					if c.options.Style == "one" {
						valid = allOne
					}
					if c.options.Style == "one-or-ordered" {
						valid = allOne || ordered
					}
					if !valid {
						for i, offset := range locations {
							expected := i + 1
							if c.options.Style == "one" {
								expected = 1
							}
							if actual[i] != expected {
								report(offset, offset, "ordered list numbering does not match "+c.options.Style, "")
							}
						}
					}
				}
				if c.id == "markdown.list-style" && !v.IsOrdered() {
					marker := byte('-')
					if len(c.options.Allow) > 0 && len(c.options.Allow[0]) == 1 {
						marker = c.options.Allow[0][0]
					}
					parentIndent := 0
					nested := false
					for parent := v.Parent(); parent != nil; parent = parent.Parent() {
						if _, ok := parent.(*ast.ListItem); ok {
							nested = true
							line := doc.SourceLine(doc.LineForOffset(offsetForNode(parent)))
							m := regexp.MustCompile(`^( *)(?:[-+*]|[0-9]+[.)])\s`).FindStringSubmatch(line)
							if m != nil {
								parentIndent = len(m[1])
							}
							break
						}
					}
					expected := 0
					if nested {
						expected = parentIndent + c.options.Indent
					}
					for item := v.FirstChild(); item != nil; item = item.NextSibling() {
						line := doc.SourceLine(doc.LineForOffset(offsetForNode(item)))
						m := regexp.MustCompile(`^( *)(?:[-+*]|[0-9]+[.)])\s`).FindStringSubmatch(line)
						if m != nil && len(m[1]) != expected {
							report(offsetForNode(item), offsetForNode(item), fmt.Sprintf("list indentation should be %d spaces", expected), "")
						}
					}
					if v.Marker != marker {
						report(offsetForNode(v), offsetForNode(v), fmt.Sprintf("use %c for unordered list markers", marker), "")
					}
				}
			case *ast.RawHTML:
				if c.id == "markdown.html-policy" {
					for _, tag := range regexp.MustCompile(`</?([A-Za-z][A-Za-z0-9]*)`).FindAllStringSubmatch(string(v.Text(doc.Source)), -1) {
						if !allowed(c.options.Allow, tag[1]) {
							report(offsetForNode(v.Parent()), offsetForNode(v.Parent()), "HTML tag not allowed: "+tag[1], "")
						}
					}
				}
			case *ast.HTMLBlock:
				if c.id == "markdown.html-policy" {
					for _, tag := range regexp.MustCompile(`</?([A-Za-z][A-Za-z0-9]*)`).FindAllStringSubmatch(string(v.Text(doc.Source)), -1) {
						if !allowed(c.options.Allow, tag[1]) {
							report(offsetForNode(v), offsetForNode(v), "HTML tag not allowed: "+tag[1], "")
						}
					}
				}
			case *ast.FencedCodeBlock:
				if c.id == "markdown.fence-language" && (v.Info == nil || strings.TrimSpace(string(v.Info.Text(doc.Source))) == "") {
					offset := 0
					if v.Lines().Len() > 0 {
						offset = v.Lines().At(0).Start
					}
					report(offset, offset, "specify code fence language (use text for plain text)", "")
				}
				return ast.WalkSkipChildren, nil
			case *ast.CodeBlock:
				return ast.WalkSkipChildren, nil
			case *ast.Image:
				if c.id == "markdown.image-alt" && strings.TrimSpace(string(v.Text(doc.Source))) == "" && !allowed(c.options.Allow, string(v.Destination)) {
					offset := 0
					if v.Parent() != nil {
						offset = offsetForNode(v.Parent())
					}
					report(offset, offset, "provide image alt text or an explicit decorative-image exception", "")
				}
			case *ast.Link:
				if c.id == "markdown.link-text" {
					label := strings.TrimSpace(string(v.Text(doc.Source)))
					if label == "" || strings.EqualFold(label, "click here") {
						report(offsetForNode(v.Parent()), offsetForNode(v.Parent()), "use descriptive link text", "")
					}
				}
			case *ast.Text:
				seg := v.Segment
				if seg.Start < frontEnd {
					return ast.WalkContinue, nil
				}
				s := string(seg.Value(doc.Source))
				for _, m := range regexp.MustCompile(`(?i)(?:[a-z][a-z0-9+.-]*://|mailto:)\S+`).FindAllStringIndex(s, -1) {
					s = s[:m[0]] + strings.Repeat(" ", m[1]-m[0]) + s[m[1]:]
				}
				if c.options.Scope == "heading" {
					heading := false
					for p := n.Parent(); p != nil; p = p.Parent() {
						if _, ok := p.(*ast.Heading); ok {
							heading = true
						}
					}
					if !heading {
						return ast.WalkContinue, nil
					}
				}
				switch c.id {
				case "text.no-dashes":
					for i, r := range s {
						if r == '-' || unicode.Is(unicode.Dash, r) {
							report(seg.Start+i, seg.Start+i+len(string(r)), "hyphens and dashes are not permitted in prose", "")
						}
					}
				case "text.no-load-bearing":
					wordRanges := words.FindAllStringIndex(s, -1)
					if len(wordRanges) > 0 {
						first := wordRanges[0]
						if strings.EqualFold(lastWord, "load") && strings.EqualFold(s[first[0]:first[1]], "bearing") && strings.Trim(string(doc.Source[lastWordEnd:seg.Start+first[0]]), " \t\r\n*_~") == "" {
							report(lastWordStart, seg.Start+first[1], "avoid load bearing in prose", "")
						}
						last := wordRanges[len(wordRanges)-1]
						lastWord = s[last[0]:last[1]]
						lastWordStart = seg.Start + last[0]
						lastWordEnd = seg.Start + last[1]
					}
					re := regexp.MustCompile(`(?i)\bload[\s\p{Pd}]+bearing\b`)
					for _, m := range re.FindAllStringIndex(s, -1) {
						report(seg.Start+m[0], seg.Start+m[1], "avoid load bearing in prose", "")
					}
				case "text.terminology":
					for bad, preferred := range c.options.Terms {
						re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(bad) + `\b`)
						for _, m := range re.FindAllStringIndex(s, -1) {
							if !allowed(c.options.Allow, s[m[0]:m[1]]) {
								report(seg.Start+m[0], seg.Start+m[1], fmt.Sprintf("use %q instead of %q", preferred, bad), "")
							}
						}
					}
				case "text.spelling", "text.repeated-word":
					for _, m := range words.FindAllStringIndex(s, -1) {
						word := s[m[0]:m[1]]
						if c.id == "text.spelling" && !allowed(c.options.Dictionary, word) && !allowed(c.options.Allow, word) {
							report(seg.Start+m[0], seg.Start+m[1], "word not in "+c.options.Language+" vocabulary: "+word, "")
						}
						if c.id == "text.repeated-word" && strings.EqualFold(lastWord, word) && strings.TrimSpace(string(doc.Source[lastWordEnd:seg.Start+m[0]])) == "" && !allowed(c.options.Allow, word) {
							report(seg.Start+m[0], seg.Start+m[1], "repeated word "+word, "")
						}
						lastWord = word
						lastWordEnd = seg.Start + m[1]
					}
				}
			case *ast.Paragraph:
				lastWord = ""
			}
			return ast.WalkContinue, nil
		})
		if c.id == "markdown.blank-lines" {
			ast.Walk(doc.Root, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
				if !enter || n.Parent() != doc.Root {
					return ast.WalkContinue, nil
				}
				switch n.(type) {
				case *ast.Heading, *ast.List, *ast.FencedCodeBlock:
					if n.PreviousSibling() != nil && !n.HasBlankPreviousLines() {
						report(offsetForNode(n), offsetForNode(n), "separate top-level blocks with blank lines", "")
					}
				}
				return ast.WalkContinue, nil
			})
		}
		if c.id == "markdown.single-title" && h1 != 1 {
			hasTitle := false
			if c.options.FrontmatterTitle && frontEnd > 0 {
				hasTitle = strings.Contains(string(doc.Source[:frontEnd]), "title:")
			}
			if !hasTitle {
				report(0, 0, "full document must have exactly one H1 title", "")
			}
		}
	}
}
func offsetForNode(n ast.Node) int {
	if n != nil && n.Type() == ast.TypeBlock && n.Lines().Len() == 0 && n.FirstChild() != nil {
		return offsetForNode(n.FirstChild())
	}
	for n != nil {
		if n.Type() == ast.TypeBlock && n.Lines().Len() > 0 {
			return n.Lines().At(0).Start
		}
		n = n.Parent()
	}
	return 0
}

func defaultOptions() markdownOptions {
	return markdownOptions{Style: "one-or-ordered", Max: 120, Indent: 2}
}
