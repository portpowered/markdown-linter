package rulepack

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/yuin/goldmark/ast"
	"gopkg.in/yaml.v3"
)

type steOptions struct {
	Dictionary        []string          `yaml:"dictionary"`
	DictionaryFile    string            `yaml:"dictionary-file"`
	TechnicalTerms    []string          `yaml:"technical-terms"`
	Scope             string            `yaml:"scope"`
	MaxSentenceWords  int               `yaml:"max-sentence-words"`
	ForbiddenPatterns map[string]string `yaml:"forbidden-patterns"`
}

type steCheck struct {
	id       string
	options  steOptions
	patterns map[string]*regexp.Regexp
}

func (c steCheck) ID() string { return c.id }

func registerSTE(r *Registry) error {
	for _, id := range []string{"text.ste100.dictionary", "text.ste100.grammar"} {
		if err := r.Register(id, steFactory(id)); err != nil {
			return err
		}
	}
	return nil
}

func steDefaults() steOptions { return steOptions{Scope: "prose", MaxSentenceWords: 25} }

func steFactory(id string) Factory {
	return func(n yaml.Node) (interfaces.Analyzer, error) {
		if err := validateCheckOptions(id, n); err != nil {
			return nil, err
		}
		c := steCheck{id: id, options: steDefaults(), patterns: map[string]*regexp.Regexp{}}
		if err := DecodeOptions(n, &c.options); err != nil {
			return nil, err
		}
		if c.options.Scope != "prose" && c.options.Scope != "heading" {
			return nil, fmt.Errorf("scope must be prose or heading")
		}
		if id == "text.ste100.dictionary" {
			if len(c.options.Dictionary) == 0 && c.options.DictionaryFile == "" {
				return nil, fmt.Errorf("STE100 requires dictionary or dictionary-file")
			}
			if c.options.DictionaryFile != "" {
				if err := validatePattern(c.options.DictionaryFile); err != nil {
					return nil, err
				}
				if strings.ContainsAny(c.options.DictionaryFile, "*?[:") {
					return nil, fmt.Errorf("dictionary-file must be a literal root-relative path")
				}
			}
			if err := validateSTEEntries(c.options.Dictionary, c.options.TechnicalTerms); err != nil {
				return nil, err
			}
		} else {
			if c.options.MaxSentenceWords < 1 {
				return nil, fmt.Errorf("max-sentence-words must be positive")
			}
			for name, pattern := range c.options.ForbiddenPatterns {
				if strings.TrimSpace(name) == "" {
					return nil, fmt.Errorf("grammar pattern names must be nonempty")
				}
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("grammar pattern %s: %w", name, err)
				}
				if re.MatchString("") {
					return nil, fmt.Errorf("grammar pattern %s matches empty text", name)
				}
				c.patterns[name] = re
			}
		}
		return c, nil
	}
}

// Exact surface forms are intentional: no implicit stemming or spelling fallback.
var steWord = regexp.MustCompile(`[\pL][\pL\pM\pN]*(?:['’\-][\pL\pM\pN]+)*`)
var steSentence = regexp.MustCompile(`[^.!?]+[.!?]*`)

func validateSTEEntries(dictionary, terms []string) error {
	for _, word := range dictionary {
		if steWord.FindString(word) != word || word == "" {
			return fmt.Errorf("dictionary entry must be one word: %q", word)
		}
	}
	for _, term := range terms {
		if strings.TrimSpace(term) == "" || strings.Join(steWord.FindAllString(term, -1), " ") != term {
			return fmt.Errorf("technical term must contain words separated by single spaces: %q", term)
		}
	}
	return nil
}

type steDictionary struct {
	Words          []string            `yaml:"words"`
	TechnicalTerms []string            `yaml:"technical-terms"`
	Entries        []steEntry          `yaml:"entries"`
	Alternatives   map[string][]string `yaml:"alternatives"`
}

type steEntry struct {
	Word         string   `yaml:"word"`
	Forms        []string `yaml:"forms"`
	PartOfSpeech string   `yaml:"part-of-speech"`
	Meaning      string   `yaml:"meaning"`
}
type steVocabulary struct {
	words        map[string]bool
	phrases      [][]string
	alternatives map[string][]string
}

func (c steCheck) vocabulary(root string) (steVocabulary, error) {
	v := steVocabulary{words: map[string]bool{}, alternatives: map[string][]string{}}
	entries := append([]string(nil), c.options.Dictionary...)
	terms := append([]string(nil), c.options.TechnicalTerms...)
	if c.options.DictionaryFile != "" {
		filename := filepath.Join(root, filepath.FromSlash(c.options.DictionaryFile))
		if err := interfaces.CheckPathRoot(root, filename); err != nil {
			return v, err
		}
		f, err := os.Open(filename)
		if err != nil {
			return v, err
		}
		defer func() { _ = f.Close() }()
		var dictionary steDictionary
		decoder := yaml.NewDecoder(f)
		decoder.KnownFields(true)
		if err := decoder.Decode(&dictionary); err != nil {
			return v, err
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return v, fmt.Errorf("dictionary must contain exactly one YAML document")
		}
		if err := validateSTEEntries(dictionary.Words, dictionary.TechnicalTerms); err != nil {
			return v, err
		}
		for _, entry := range dictionary.Entries {
			forms := append([]string{entry.Word}, entry.Forms...)
			if err := validateSTEEntries(forms, nil); err != nil {
				return v, err
			}
			if entry.PartOfSpeech != "" && !allowed([]string{"noun", "verb", "adjective", "adverb", "pronoun", "preposition", "conjunction", "determiner", "interjection"}, entry.PartOfSpeech) {
				return v, fmt.Errorf("unknown part-of-speech %q", entry.PartOfSpeech)
			}
			entries = append(entries, forms...)
		}
		v.alternatives = dictionary.Alternatives
		entries = append(entries, dictionary.Words...)
		terms = append(terms, dictionary.TechnicalTerms...)
	}
	if len(entries) == 0 && len(terms) == 0 {
		return v, fmt.Errorf("STE100 dictionary must not be empty")
	}
	for _, word := range entries {
		v.words[strings.ToLower(word)] = true
	}
	for _, term := range terms {
		v.phrases = append(v.phrases, strings.Fields(strings.ToLower(term)))
	}
	normalized := map[string][]string{}
	for rejected, replacements := range v.alternatives {
		if err := validateSTEEntries([]string{rejected}, nil); err != nil {
			return v, err
		}
		key := strings.ToLower(rejected)
		if v.words[key] || allowed(terms, rejected) || len(replacements) == 0 || normalized[key] != nil {
			return v, fmt.Errorf("alternative key %q must be unique, rejected, and have replacements", rejected)
		}
		for _, replacement := range replacements {
			if err := validateSTEEntries(nil, []string{replacement}); err != nil {
				return v, err
			}
			for _, word := range strings.Fields(replacement) {
				if !v.words[strings.ToLower(word)] && !allowed(terms, replacement) {
					return v, fmt.Errorf("alternative %q is not approved", replacement)
				}
			}
		}
		normalized[key] = replacements
	}
	v.alternatives = normalized
	return v, nil
}

// Prose masks retain byte offsets and keep each paragraph/heading independent.
func steProse(doc *interfaces.Document, scope string, visit func([]byte, int)) {
	front := strunkFrontmatterEnd(doc.Source)
	masked := make([]byte, len(doc.Source))
	for i := range masked {
		masked[i] = ' '
	}
	_ = ast.Walk(doc.Root, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		if !enter {
			return ast.WalkContinue, nil
		}
		_, paragraph := n.(*ast.Paragraph)
		_, heading := n.(*ast.Heading)
		if (!paragraph && !heading) || (scope == "heading" && !heading) {
			return ast.WalkContinue, nil
		}
		touched := [][2]int{}
		start, end := len(doc.Source), 0
		_ = ast.Walk(n, func(child ast.Node, arriving bool) (ast.WalkStatus, error) {
			if !arriving {
				return ast.WalkContinue, nil
			}
			switch child.(type) {
			case *ast.CodeSpan, *ast.Image, *ast.RawHTML, *ast.AutoLink:
				_ = ast.Walk(child, func(protected ast.Node, entering bool) (ast.WalkStatus, error) {
					if text, ok := protected.(*ast.Text); ok && entering {
						touched = append(touched, [2]int{text.Segment.Start, text.Segment.Stop})
						for i := text.Segment.Start; i < text.Segment.Stop; i++ {
							masked[i] = 0
						}
					}
					return ast.WalkContinue, nil
				})
				return ast.WalkSkipChildren, nil
			}
			if text, ok := child.(*ast.Text); ok && text.Segment.Start >= front {
				if text.Segment.Start < start {
					start = text.Segment.Start
				}
				if text.Segment.Stop > end {
					end = text.Segment.Stop
				}
				touched = append(touched, [2]int{text.Segment.Start, text.Segment.Stop})
				copy(masked[text.Segment.Start:text.Segment.Stop], doc.Source[text.Segment.Start:text.Segment.Stop])
			}
			return ast.WalkContinue, nil
		})
		if end <= start {
			return ast.WalkSkipChildren, nil
		}
		prose := masked[start:end]
		for _, url := range strunkURL.FindAllIndex(prose, -1) {
			for i := url[0]; i < url[1]; i++ {
				prose[i] = 0
			}
		}
		visit(prose, start)
		for _, at := range touched {
			for i := at[0]; i < at[1]; i++ {
				masked[i] = ' '
			}
		}
		return ast.WalkSkipChildren, nil
	})
}

func (c steCheck) Analyze(ctx context.Context, pass *interfaces.Pass) {
	if ctx.Err() != nil || len(pass.Documents) == 0 {
		return
	}
	var vocabulary steVocabulary
	if c.id == "text.ste100.dictionary" {
		var err error
		vocabulary, err = c.vocabulary(pass.Root)
		if err != nil {
			pass.ReportError(fmt.Errorf("STE100 dictionary: %w", err))
			return
		}
	}
	for _, doc := range pass.Documents {
		if ctx.Err() != nil {
			return
		}
		steProse(doc, c.options.Scope, func(prose []byte, base int) {
			report := func(start, end int, message string) {
				start += base
				end += base
				pass.Report(interfaces.NewDiagnostic(doc.Path, doc.LineForOffset(start), start, end, c.id, message, interfaces.SeverityError))
			}
			if c.id == "text.ste100.dictionary" {
				matches := steWord.FindAllIndex(prose, -1)
				approved := make([]bool, len(matches))
				for i := range matches {
					for _, phrase := range vocabulary.phrases {
						if i+len(phrase) > len(matches) {
							continue
						}
						match := true
						for j, word := range phrase {
							at := matches[i+j]
							if !strings.EqualFold(string(prose[at[0]:at[1]]), word) {
								match = false
								break
							}
							if j > 0 && strings.TrimSpace(string(prose[matches[i+j-1][1]:at[0]])) != "" {
								match = false
								break
							}
						}
						if match {
							for j := range phrase {
								approved[i+j] = true
							}
						}
					}
				}
				for i, at := range matches {
					word := string(prose[at[0]:at[1]])
					if !approved[i] && !vocabulary.words[strings.ToLower(word)] {
						message := fmt.Sprintf("word %q is not in the approved STE100 dictionary", word)
						if alternatives := vocabulary.alternatives[strings.ToLower(word)]; len(alternatives) > 0 {
							message += "; consider: " + strings.Join(alternatives, ", ")
						}
						report(at[0], at[1], message)
					}
				}
				return
			}
			for _, sentence := range steSentence.FindAllIndex(prose, -1) {
				matches := steWord.FindAllIndex(prose[sentence[0]:sentence[1]], -1)
				if len(matches) > c.options.MaxSentenceWords {
					report(sentence[0]+matches[0][0], sentence[0]+matches[len(matches)-1][1], fmt.Sprintf("sentence has %d words; limit is %d", len(matches), c.options.MaxSentenceWords))
				}
			}
			names := []string{}
			for name := range c.patterns {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				for _, at := range c.patterns[name].FindAllIndex(prose, -1) {
					report(at[0], at[1], "STE100 grammar: "+name)
				}
			}
		})
	}
}
