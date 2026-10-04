package rulepack

import (
	"context"
	"fmt"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/yuin/goldmark/ast"
	"gopkg.in/yaml.v3"
	"regexp"
	"strings"
)

type matcherOptions struct {
	Patterns           []string          `yaml:"patterns"`
	BannedWords        []string          `yaml:"banned-words"`
	BannedCharacters   string            `yaml:"banned-characters"`
	IgnoreCase         bool              `yaml:"ignore-case"`
	Allow              []string          `yaml:"allow"`
	Scope              string            `yaml:"scope"`
	Message            string            `yaml:"message"`
	CaptureGroup       int               `yaml:"capture-group"`
	PairedConjunctions map[string]string `yaml:"paired-conjunctions"`
}
type matcherCheck struct {
	id       string
	patterns []*regexp.Regexp
	options  matcherOptions
}

func (c matcherCheck) ID() string { return c.id }
func matcherDefaults() matcherOptions {
	return matcherOptions{Scope: "prose", IgnoreCase: true, Message: "review prohibited text"}
}
func matcherFactory(id string, defaults matcherOptions) Factory {
	return func(n yaml.Node) (interfaces.Analyzer, error) {
		if err := validateCheckOptions(id, n); err != nil {
			return nil, err
		}
		options := defaults
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == "paired-conjunctions" {
				options.PairedConjunctions = nil
			}
		}
		if err := DecodeOptions(n, &options); err != nil {
			return nil, err
		}
		if options.Scope != "prose" && options.Scope != "heading" {
			return nil, fmt.Errorf("scope must be prose or heading")
		}
		if strings.TrimSpace(options.Message) == "" {
			return nil, fmt.Errorf("message must be nonempty")
		}
		patterns := append([]string(nil), options.Patterns...)
		for _, word := range options.BannedWords {
			if strings.TrimSpace(word) == "" {
				return nil, fmt.Errorf("banned words must be nonempty")
			}
			parts := strings.Fields(word)
			for i := range parts {
				parts[i] = regexp.QuoteMeta(parts[i])
			}
			patterns = append(patterns, `\b`+strings.Join(parts, `\s+`)+`\b`)
		}
		if options.BannedCharacters != "" {
			characters := []string{}
			for _, character := range options.BannedCharacters {
				characters = append(characters, regexp.QuoteMeta(string(character)))
			}
			patterns = append(patterns, "(?:"+strings.Join(characters, "|")+")")
		}
		if len(patterns) == 0 {
			return nil, fmt.Errorf("matcher requires patterns, banned-words or banned-characters")
		}
		c := matcherCheck{id: id, options: options}
		for _, source := range patterns {
			if options.IgnoreCase {
				source = "(?i)" + source
			}
			pattern, err := regexp.Compile(source)
			if err != nil {
				return nil, fmt.Errorf("matcher pattern: %w", err)
			}
			if pattern.MatchString("") {
				return nil, fmt.Errorf("matcher pattern must not match empty text")
			}
			if options.CaptureGroup < 0 || options.CaptureGroup > pattern.NumSubexp() {
				return nil, fmt.Errorf("capture-group must exist in every pattern")
			}
			c.patterns = append(c.patterns, pattern)
		}
		for first, counterpart := range options.PairedConjunctions {
			if len(strings.Fields(first)) != 1 || len(strings.Fields(counterpart)) != 1 {
				return nil, fmt.Errorf("paired-conjunctions keys and values must be single words")
			}
		}
		return c, nil
	}
}

var strunkURL = regexp.MustCompile(`(?i)(?:[a-z][a-z0-9+.-]*://|mailto:)\S+`)

func (c matcherCheck) Analyze(ctx context.Context, pass *interfaces.Pass) {
	for _, doc := range pass.Documents {
		if ctx.Err() != nil {
			return
		}
		frontEnd := strunkFrontmatterEnd(doc.Source)
		masked := []byte(strings.Repeat(" ", len(doc.Source)))
		_ = ast.Walk(doc.Root, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
			if !enter {
				return ast.WalkContinue, nil
			}
			_, paragraph := n.(*ast.Paragraph)
			_, heading := n.(*ast.Heading)
			if (!paragraph && !heading) || (c.options.Scope == "heading" && !heading) {
				return ast.WalkContinue, nil
			}
			start, end := len(doc.Source), 0
			_ = ast.Walk(n, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
				if !entering {
					return ast.WalkContinue, nil
				}
				switch child.(type) {
				case *ast.CodeSpan, *ast.Image:
					_ = ast.Walk(child, func(protected ast.Node, arriving bool) (ast.WalkStatus, error) {
						if text, ok := protected.(*ast.Text); ok && arriving {
							for i := text.Segment.Start; i < text.Segment.Stop; i++ {
								masked[i] = 0
							}
						}
						return ast.WalkContinue, nil
					})
					return ast.WalkSkipChildren, nil
				case *ast.RawHTML:
					return ast.WalkSkipChildren, nil
				}
				text, ok := child.(*ast.Text)
				if !ok || text.Segment.Start < frontEnd {
					return ast.WalkContinue, nil
				}
				seg := text.Segment
				copy(masked[seg.Start:seg.Stop], doc.Source[seg.Start:seg.Stop])
				if seg.Start < start {
					start = seg.Start
				}
				if seg.Stop > end {
					end = seg.Stop
				}
				return ast.WalkContinue, nil
			})
			if end <= start {
				return ast.WalkSkipChildren, nil
			}
			prose := masked[start:end]
			for _, m := range strunkURL.FindAllIndex(prose, -1) {
				for i := m[0]; i < m[1]; i++ {
					prose[i] = ' '
				}
			}
			for _, pattern := range c.patterns {
				for _, match := range pattern.FindAllSubmatchIndex(prose, -1) {
					if c.options.CaptureGroup > 0 {
						match = match[2*c.options.CaptureGroup : 2*c.options.CaptureGroup+2]
					}
					if match[0] < 0 || match[1] <= match[0] {
						continue
					}
					phrase := string(prose[match[0]:match[1]])
					if allowed(c.options.Allow, strings.Join(strings.Fields(phrase), " ")) || c.pairAlreadyBalanced(phrase) {
						continue
					}
					from, to := start+match[0], start+match[1]
					pass.Report(interfaces.NewDiagnostic(doc.Path, doc.LineForOffset(from), from, to, c.id, fmt.Sprintf("%s (matched %q)", c.options.Message, phrase), interfaces.SeverityWarning))
				}
			}
			return ast.WalkSkipChildren, nil
		})
	}
}

func (c matcherCheck) pairAlreadyBalanced(phrase string) bool {
	for first, counterpart := range c.options.PairedConjunctions {
		if strings.EqualFold(strings.Fields(phrase)[0], first) && regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(counterpart)+`\b`).MatchString(phrase) {
			return true
		}
	}
	return false
}
func strunkFrontmatterEnd(source []byte) int {
	text := string(source)
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return 0
	}
	offset := 0
	for i, line := range strings.SplitAfter(text, "\n") {
		offset += len(line)
		if i > 0 && strings.TrimSpace(line) == "---" {
			return offset
		}
	}
	return len(source)
}
