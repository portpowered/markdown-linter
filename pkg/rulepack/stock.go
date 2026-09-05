package rulepack

import (
	"context"
	"fmt"
	"regexp"

	"github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/portpowered/markdown-linter/pkg/rules"
	"gopkg.in/yaml.v3"
)

// AdaptRule makes a document check available as a repository analyzer.
func AdaptRule(rule interfaces.Rule) interfaces.Analyzer { return ruleAdapter{rule: rule} }

type ruleAdapter struct{ rule interfaces.Rule }

var _ interfaces.Analyzer = ruleAdapter{}

func (a ruleAdapter) ID() string { return a.rule.ID() }
func (a ruleAdapter) Analyze(ctx context.Context, pass *interfaces.Pass) {
	linter := engine.New(engine.WithRules(a.rule))
	for _, diagnostic := range linter.RunDiagnosticsForDocuments(ctx, pass.Documents) {
		pass.Report(diagnostic)
	}
}

// RegisterStock uses exactly the same factory registry available to customer checks.
func RegisterStock(registry *Registry) error {
	simple := map[string]func() interfaces.Rule{
		"markdown.formatting":    func() interfaces.Rule { return rules.NewDocsCheckFormattingRule() },
		"markdown.heading-order": func() interfaces.Rule { return rules.NewHeadingOrderRule() },
		"markdown.ordered-list":  func() interfaces.Rule { return rules.NewOrderedListRule() },
	}
	for name, constructor := range simple {
		factory := constructor
		if err := registry.Register(name, func(node yaml.Node) (interfaces.Analyzer, error) {
			if err := DecodeOptions(node, &struct{}{}); err != nil {
				return nil, err
			}
			return AdaptRule(factory()), nil
		}); err != nil {
			return err
		}
	}
	if err := registry.Register("markdown.local-links", func(node yaml.Node) (interfaces.Analyzer, error) {
		var options struct {
			AllowDirectories bool `yaml:"allow-directories"`
		}
		if err := DecodeOptions(node, &options); err != nil {
			return nil, err
		}
		return localLinks{allowDirectories: options.AllowDirectories}, nil
	}); err != nil {
		return err
	}
	factories := map[string]Factory{
		"markdown.required-heading":    newRequiredHeading,
		"markdown.document-identifier": newIdentifier,
		"markdown.document-structure":  newStructure,
		"markdown.link-relocation":     newRelocation,
		"markdown.doc-id-unique": func(node yaml.Node) (interfaces.Analyzer, error) {
			if err := DecodeOptions(node, &struct{}{}); err != nil {
				return nil, err
			}
			return rules.NewDocIDUniquenessAnalyzer(), nil
		},
	}
	for name, factory := range factories {
		if err := registry.Register(name, factory); err != nil {
			return err
		}
	}
	return nil
}

type requiredHeading struct {
	Heading string `yaml:"heading"`
	Level   int    `yaml:"level"`
}

var _ interfaces.Analyzer = requiredHeading{}

func newRequiredHeading(node yaml.Node) (interfaces.Analyzer, error) {
	var check requiredHeading
	if err := DecodeOptions(node, &check); err != nil {
		return nil, err
	}
	if check.Heading == "" || check.Level < 1 || check.Level > 6 {
		return nil, fmt.Errorf("heading and level (1 through 6) are required")
	}
	return check, nil
}
func (r requiredHeading) ID() string { return "markdown.required-heading" }
func (r requiredHeading) Analyze(_ context.Context, pass *interfaces.Pass) {
	index := rules.NewDocumentationIndex(pass.Documents, rules.WithoutAssetDiscovery())
	for _, doc := range index.Documents {
		found := false
		for _, heading := range doc.Headings {
			if heading.Text == r.Heading && heading.Level == r.Level {
				found = true
				break
			}
		}
		if !found {
			pass.Report(interfaces.NewDiagnostic(doc.Path, 1, -1, -1, r.ID(), fmt.Sprintf("missing level %d heading %q", r.Level, r.Heading), interfaces.SeverityError))
		}
	}
}

// DefaultPack provides general checks without application-specific conventions.
func DefaultPack() Pack {
	return Pack{Version: 1, Rules: []Rule{
		{ID: "markdown.formatting", Check: "markdown.formatting"},
		{ID: "markdown.heading-order", Check: "markdown.heading-order"},
		{ID: "markdown.local-links", Check: "markdown.local-links"},
	}}
}

// AnalysisGroup composes public analyzers sharing one pass (for example index and relocation).
type AnalysisGroup struct {
	Name      string
	Analyzers []interfaces.Analyzer
}

func (g AnalysisGroup) ID() string { return g.Name }
func (g AnalysisGroup) Analyze(ctx context.Context, pass *interfaces.Pass) {
	for _, a := range g.Analyzers {
		a.Analyze(ctx, pass)
	}
}

var _ interfaces.Analyzer = AnalysisGroup{}

type identifierOptions struct {
	Configs []struct {
		Name          string   `yaml:"name"`
		Field         string   `yaml:"field"`
		Paths         []string `yaml:"paths"`
		Required      bool     `yaml:"required"`
		Pattern       string   `yaml:"pattern"`
		FormatLabel   string   `yaml:"format-label"`
		Unique        bool     `yaml:"unique"`
		MatchFilename bool     `yaml:"match-filename"`
	} `yaml:"identifiers"`
}

func newIdentifier(node yaml.Node) (interfaces.Analyzer, error) {
	var options identifierOptions
	if err := DecodeOptions(node, &options); err != nil {
		return nil, err
	}
	if len(options.Configs) == 0 {
		return nil, fmt.Errorf("identifiers must not be empty")
	}
	configs := []rules.DocumentIdentifierConfig{}
	for _, c := range options.Configs {
		if c.Field == "" {
			return nil, fmt.Errorf("identifier field is required")
		}
		var pattern *regexp.Regexp
		if c.Pattern != "" {
			var err error
			pattern, err = regexp.Compile(c.Pattern)
			if err != nil {
				return nil, err
			}
		}
		configs = append(configs, rules.DocumentIdentifierConfig{Name: c.Name, Field: c.Field, PathPrefixes: c.Paths, Required: c.Required, Format: pattern, FormatLabel: c.FormatLabel, Unique: c.Unique, MatchFilename: c.MatchFilename})
	}
	return rules.NewDocumentIdentifierAnalyzer(configs...), nil
}
func newStructure(node yaml.Node) (interfaces.Analyzer, error) {
	var options struct {
		Types []rules.DocumentStructureConfig `yaml:"types"`
	}
	if err := DecodeOptions(node, &options); err != nil {
		return nil, err
	}
	if len(options.Types) == 0 {
		return nil, fmt.Errorf("types must not be empty")
	}
	return rules.NewTypedDocumentStructureAnalyzer(options.Types...), nil
}
func newRelocation(node yaml.Node) (interfaces.Analyzer, error) {
	var options struct {
		Moves map[string]string `yaml:"moves"`
	}
	if err := DecodeOptions(node, &options); err != nil {
		return nil, err
	}
	return AnalysisGroup{Name: "markdown.link-relocation", Analyzers: []interfaces.Analyzer{
		rules.NewDocumentationIndexAnalyzer(rules.WithMoveMappings(options.Moves)), rules.NewLinkRelocationAnalyzer(),
	}}, nil
}

// localLinks constructs the same public rule with the pass filesystem policy.
type localLinks struct{ allowDirectories bool }

func (a localLinks) ID() string { return "markdown.local-links" }
func (a localLinks) Analyze(ctx context.Context, pass *interfaces.Pass) {
	options := []rules.LocalLinkRuleOption{rules.WithLinkRoot(pass.Root)}
	if a.allowDirectories {
		options = append(options, rules.WithDirectoryLinkTargetsAllowed())
	}
	AdaptRule(rules.NewLocalLinkRule(options...)).Analyze(ctx, pass)
}
