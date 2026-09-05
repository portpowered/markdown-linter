package markdownlint_test

import (
	"github.com/portpowered/markdown-linter/pkg/rules"
	"regexp"
)

func customerIdentifierConfigs() []rules.DocumentIdentifierConfig {
	return []rules.DocumentIdentifierConfig{
		{
			Name:         "process doc-id",
			Field:        "doc-id",
			PathPrefixes: []string{"docs/processes"},
			Required:     true,
			Format:       regexp.MustCompile(`^PROC-\d+$`),
			FormatLabel:  "process",
			Unique:       true,
		},
		{
			Name:         "standard doc-id",
			Field:        "doc-id",
			PathPrefixes: []string{"docs/standards"},
			Required:     true,
			Format:       regexp.MustCompile(`^STD-\d+$`),
			FormatLabel:  "standard",
			Unique:       true,
		},
		{
			Name:         "intent doc-id",
			Field:        "doc-id",
			PathPrefixes: []string{"docs/intents"},
			Required:     true,
			Format:       regexp.MustCompile(`^INTENT-\d+$`),
			FormatLabel:  "intent",
			Unique:       true,
		},
		{
			Name:         "architecture doc-id",
			Field:        "doc-id",
			PathPrefixes: []string{"docs/architecture"},
			Required:     true,
			Format:       regexp.MustCompile(`^ARCH-\d{3}$`),
			FormatLabel:  "architecture",
			Unique:       true,
		},
		{
			Name:          "process-id",
			Field:         "process-id",
			PathPrefixes:  []string{"docs/processes"},
			Required:      true,
			Unique:        true,
			MatchFilename: true,
		},
	}
}

func customerStructureConfigs() []rules.DocumentStructureConfig {
	return []rules.DocumentStructureConfig{{PathPrefixes: []string{"docs/processes"}, RequiredFields: []string{"author", "last modified", "doc-id", "process-id"}, RequiredHeadings: []string{"Purpose", "Scope", "Prerequisites", "Procedure"}, AnyHeadings: []string{"Verification", "Checklist"}, AlternativeMessage: "must include a Verification or Checklist section."}, {PathPrefixes: []string{"docs/standards"}, RequiredFields: []string{"author", "last modified", "doc-id"}, RequiredHeadings: []string{"Usage", "References", "Changelog"}, AnyHeadings: []string{"Standard Summary", "Quick Rules"}, AlternativeMessage: "must include a Standard Summary or Quick Rules section."}, {PathPrefixes: []string{"docs/intents"}, RequiredFields: []string{"author", "last modified", "component", "doc-id"}, RequiredHeadings: []string{"Purpose & Vision", "Intended Users", "Core Capabilities", "Intended Workflows", "Boundaries", "Success Indicators", "Related Intents"}, AnyHeadings: []string{}, AlternativeMessage: ""}, {PathPrefixes: []string{"docs/architecture"}, RequiredFields: []string{"author", "last modified", "doc-id"}, RequiredHeadings: []string{"Overview", "System Context", "Design Decisions"}, AnyHeadings: []string{}, AlternativeMessage: ""}}
}
