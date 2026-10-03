package rulepack

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/yuin/goldmark/ast"
	"gopkg.in/yaml.v3"
)

type strunkOptions struct {
	Allow []string `yaml:"allow"`
	Scope string   `yaml:"scope"`
}

type strunkSpec struct {
	name, pattern, guidance string
}

// These finite lexical signals invite editorial review. They are not a grammar parser.
var strunkSpecs = []strunkSpec{
	{"needless-words", `(?i)\b(?:in\s+order\s+to|due\s+to\s+the\s+fact\s+that|at\s+this\s+point\s+in\s+time|in\s+the\s+event\s+that|for\s+the\s+purpose\s+of|the\s+question\s+as\s+to\s+whether|in\s+a\s+manner\s+that|on\s+a\s+daily\s+basis)\b`, "consider a shorter expression while preserving meaning"},
	{"fancy-words", `(?i)\b(?:utilize|utilization|aforementioned|heretofore|henceforth)\b`, "consider a familiar word such as use, earlier, or from now on"},
	{"qualifiers", `(?i)\b(?:very|really|rather|quite)\b`, "review this qualifier; a precise description may be stronger"},
	{"negative-phrases", `(?i)\b(?:not\s+uncommon|not\s+impossible|not\s+unlikely|not\s+without)\b`, "consider a positive statement if the nuance permits it"},
	{"passive-voice", `(?i)\b(?:is|are|was|were|be|been|being)\s+(?:(?:carefully|automatically|explicitly)\s+)?(?:approved|created|deleted|modified|sent|written|read|processed|returned|requested|performed|used|made|done|seen|given|taken|built|stored|generated)\s+by\b`, "possible agent-bearing passive; consider naming the actor first"},
	{"usage", `(?i)\b(?:irregardless|(?:could|should|would|might|must)\s+of)\b`, "review conventional usage: regardless, or a modal followed by have"},
	{"exclamations", `!+`, "review emphatic punctuation; factual prose usually needs a period"},
	{"existential-openings", `(?im)(?:^|[.!?]\s+)(there\s+(?:is|are|was|were))\b`, "consider opening with the subject instead of there"},
	{"redundant-pairs", `(?i)\b(?:advance\s+planning|basic\s+fundamentals|final\s+outcome|each\s+and\s+every|free\s+gift|past\s+history)\b`, "review this pair for duplicated meaning"},
	{"correlative-pairs", `(?i)\b(?:both\b[^.!?;]*?\bor|either\b[^.!?;]*?\band|neither\b[^.!?;]*?\bor)\b`, "review the paired construction: both/and, either/or, or neither/nor"},
}

type strunkCheck struct {
	id      string
	spec    strunkSpec
	pattern *regexp.Regexp
	options strunkOptions
}

func (c strunkCheck) ID() string { return c.id }

func registerStrunkWhite(r *Registry) error {
	for _, spec := range strunkSpecs {
		id := "text.strunk-white." + spec.name
		if err := r.Register(id, strunkFactory(id, spec)); err != nil {
			return err
		}
	}
	return nil
}

func strunkFactory(id string, spec strunkSpec) Factory {
	return func(n yaml.Node) (interfaces.Analyzer, error) {
		if err := validateCheckOptions(id, n); err != nil {
			return nil, err
		}
		options := strunkOptions{Scope: "prose"}
		if err := DecodeOptions(n, &options); err != nil {
			return nil, err
		}
		if options.Scope != "prose" && options.Scope != "heading" {
			return nil, fmt.Errorf("scope must be prose or heading")
		}
		return strunkCheck{id: id, spec: spec, pattern: regexp.MustCompile(spec.pattern), options: options}, nil
	}
}

var strunkURL = regexp.MustCompile(`(?i)(?:[a-z][a-z0-9+.-]*://|mailto:)\S+`)
var strunkConjunction = map[string]*regexp.Regexp{
	"both":    regexp.MustCompile(`(?i)\band\b`),
	"either":  regexp.MustCompile(`(?i)\bor\b`),
	"neither": regexp.MustCompile(`(?i)\bnor\b`),
}

func (c strunkCheck) Analyze(ctx context.Context, pass *interfaces.Pass) {
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
			for _, match := range c.pattern.FindAllSubmatchIndex(prose, -1) {
				if c.spec.name == "existential-openings" {
					match = match[2:4]
				}
				phrase := string(prose[match[0]:match[1]])
				if allowed(c.options.Allow, strings.Join(strings.Fields(phrase), " ")) || c.pairAlreadyBalanced(phrase) {
					continue
				}
				from, to := start+match[0], start+match[1]
				pass.Report(interfaces.NewDiagnostic(doc.Path, doc.LineForOffset(from), from, to, c.id, c.spec.guidance, interfaces.SeverityWarning))
			}
			return ast.WalkSkipChildren, nil
		})
	}
}

func (c strunkCheck) pairAlreadyBalanced(phrase string) bool {
	if c.spec.name != "correlative-pairs" {
		return false
	}
	for first, counterpart := range strunkConjunction {
		if strings.EqualFold(strings.Fields(phrase)[0], first) && counterpart.MatchString(phrase) {
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
