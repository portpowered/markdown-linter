package rules

import (
	"context"
	"fmt"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"strings"
)

const typedDocumentStructureRuleID = "markdown.typed-doc-structure"

// TypedDocumentStructureAnalyzer validates docs-check typed-document frontmatter and headings.
type TypedDocumentStructureAnalyzer struct{ configs []DocumentStructureConfig }

// DocumentStructureConfig describes a customer's document convention.
type DocumentStructureConfig struct {
	PathPrefixes       []string `yaml:"paths"`
	RequiredFields     []string `yaml:"required-fields"`
	RequiredHeadings   []string `yaml:"required-headings"`
	AnyHeadings        []string `yaml:"any-headings"`
	AlternativeMessage string   `yaml:"alternative-message"`
}

var _ interfaces.Analyzer = (*TypedDocumentStructureAnalyzer)(nil)

// NewTypedDocumentStructureAnalyzer creates an analyzer for docs-check typed-document structure.
func NewTypedDocumentStructureAnalyzer(configs ...DocumentStructureConfig) *TypedDocumentStructureAnalyzer {
	return &TypedDocumentStructureAnalyzer{configs: append([]DocumentStructureConfig(nil), configs...)}
}

// ID returns the stable typed-document structure analyzer identifier.
func (a *TypedDocumentStructureAnalyzer) ID() string {
	return typedDocumentStructureRuleID
}

// Analyze reports missing frontmatter, required frontmatter fields, and required headings.
func (a *TypedDocumentStructureAnalyzer) Analyze(_ context.Context, pass *interfaces.Pass) {
	index, ok := documentationIndexFromPass(pass)
	if !ok {
		index = NewDocumentationIndex(pass.Documents, WithoutAssetDiscovery())
	}

	for _, doc := range index.Documents {
		docType, ok := a.classifyDocument(doc.Path)
		if !ok {
			continue
		}
		a.analyzeDocument(pass, doc, docType)
	}
}

type typedDocumentType struct {
	requiredFields   []string
	requiredHeadings []string
	alternativeGroup headingAlternativeGroup
}

type headingAlternativeGroup struct {
	options []string
	message string
}

func (a *TypedDocumentStructureAnalyzer) analyzeDocument(pass *interfaces.Pass, doc *IndexedDocument, docType typedDocumentType) {
	if !hasFrontmatterBlock(doc.Document.Source) {
		reportTypedStructure(pass, doc, 1, fmt.Sprintf("%s is missing YAML frontmatter.", doc.Path))
		return
	}

	for _, field := range docType.requiredFields {
		if strings.TrimSpace(doc.Frontmatter[field]) == "" {
			reportTypedStructure(pass, doc, 1, fmt.Sprintf("%s is missing frontmatter field %s.", doc.Path, field))
		}
	}

	for _, heading := range docType.requiredHeadings {
		if !hasLevelTwoHeading(doc, heading) {
			reportTypedStructure(pass, doc, 1, fmt.Sprintf("%s is missing required heading %q.", doc.Path, heading))
		}
	}

	if len(docType.alternativeGroup.options) > 0 && !hasAnyLevelTwoHeading(doc, docType.alternativeGroup.options) {
		reportTypedStructure(pass, doc, 1, fmt.Sprintf("%s %s", doc.Path, docType.alternativeGroup.message))
	}
}

func documentationIndexFromPass(pass *interfaces.Pass) (*DocumentationIndex, bool) {
	value, ok := pass.Data(DocumentationIndexPassDataKey)
	if !ok {
		return nil, false
	}
	index, ok := value.(*DocumentationIndex)
	return index, ok
}

func (a *TypedDocumentStructureAnalyzer) classifyDocument(path string) (typedDocumentType, bool) {
	for _, config := range a.configs {
		applies := len(config.PathPrefixes) == 0
		for _, prefix := range config.PathPrefixes {
			if pathInDocumentationArea(path, prefix) {
				applies = true
				break
			}
		}
		if applies {
			return typedDocumentType{requiredFields: config.RequiredFields, requiredHeadings: config.RequiredHeadings,
				alternativeGroup: headingAlternativeGroup{options: config.AnyHeadings, message: config.AlternativeMessage}}, true
		}
	}
	return typedDocumentType{}, false
}

func pathInDocumentationArea(path string, area string) bool {
	normalizedPath := normalizeIndexPath(path)
	normalizedArea := normalizeIndexPath(area)
	return strings.HasPrefix(normalizedPath, normalizedArea+"/") || strings.Contains(normalizedPath, "/"+normalizedArea+"/")
}

func hasFrontmatterBlock(source []byte) bool {
	lines := strings.SplitAfter(string(source), "\n")
	_, _, ok := frontmatterBlockStart(lines)
	return ok
}

func hasLevelTwoHeading(doc *IndexedDocument, text string) bool {
	for _, heading := range doc.Headings {
		if heading.Level == 2 && heading.Text == text {
			return true
		}
	}
	return false
}

func hasAnyLevelTwoHeading(doc *IndexedDocument, headings []string) bool {
	for _, heading := range headings {
		if hasLevelTwoHeading(doc, heading) {
			return true
		}
	}
	return false
}

func frontmatterFieldLine(doc *IndexedDocument, field string) int {
	if indexedField, ok := doc.FrontmatterFields[field]; ok {
		return indexedField.Line
	}
	return 1
}

func reportTypedStructure(pass *interfaces.Pass, doc *IndexedDocument, line int, message string) {
	pass.Report(interfaces.NewDiagnostic(
		doc.Document.Path,
		line,
		-1,
		-1,
		typedDocumentStructureRuleID,
		message,
		interfaces.SeverityError,
	))
}
