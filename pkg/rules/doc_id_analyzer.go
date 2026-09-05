package rules

import (
	"context"
	"fmt"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	docIDUniqueRuleID        = "markdown.doc-id-unique"
	documentIdentifierRuleID = "markdown.document-identifier"
)

var (
	typedDocIDPattern = regexp.MustCompile(`^([A-Z]+)-(\d+)$`)
)

// DocumentIdentifierConfig configures one frontmatter identifier field validation.
type DocumentIdentifierConfig struct {
	// Name labels diagnostics for the configured document or identifier type.
	Name string
	// Field is the frontmatter field to validate, such as doc-id or process-id.
	Field string
	// PathPrefixes restricts validation to matching repository path prefixes. Empty applies to all documents.
	PathPrefixes []string
	// Required reports a diagnostic when the configured field is missing or empty.
	Required bool
	// Format validates non-empty identifier values.
	Format *regexp.Regexp
	// FormatLabel describes the accepted format in diagnostics.
	FormatLabel string
	// Unique reports duplicate canonical identifier values within matching documents.
	Unique bool
	// MatchFilename requires the identifier value to match the Markdown filename without extension.
	MatchFilename bool
}

// DocumentIdentifierAnalyzer validates configured frontmatter identifiers across the lint surface.
type DocumentIdentifierAnalyzer struct {
	ruleID  string
	configs []DocumentIdentifierConfig
}

var _ interfaces.Analyzer = (*DocumentIdentifierAnalyzer)(nil)

// NewDocumentIdentifierAnalyzer creates a configurable identifier analyzer.
func NewDocumentIdentifierAnalyzer(configs ...DocumentIdentifierConfig) *DocumentIdentifierAnalyzer {
	return &DocumentIdentifierAnalyzer{
		ruleID:  documentIdentifierRuleID,
		configs: append([]DocumentIdentifierConfig(nil), configs...),
	}
}

// ID returns the stable document identifier analyzer identifier.
func (a *DocumentIdentifierAnalyzer) ID() string {
	return a.ruleID
}

// Analyze reports required, format, filename-derived, and uniqueness diagnostics.
func (a *DocumentIdentifierAnalyzer) Analyze(_ context.Context, pass *interfaces.Pass) {
	index, ok := documentationIndexFromPass(pass)
	if !ok {
		index = NewDocumentationIndex(pass.Documents, WithoutAssetDiscovery())
	}
	if index == nil {
		return
	}

	for _, config := range a.configs {
		a.analyzeConfig(pass, index, normalizedIdentifierConfig(config))
	}
}

func (a *DocumentIdentifierAnalyzer) analyzeConfig(pass *interfaces.Pass, index *DocumentationIndex, config DocumentIdentifierConfig) {
	matchingDocs := identifierMatchingDocuments(index, config)
	for _, doc := range matchingDocs {
		value := strings.TrimSpace(doc.Frontmatter[strings.ToLower(config.Field)])
		if config.Required && value == "" {
			reportDocumentIdentifier(pass, a.ID(), doc, 1, fmt.Sprintf("%s is missing frontmatter field %s.", doc.Path, config.Field))
			continue
		}
		if value == "" {
			continue
		}
		if config.Format != nil && !config.Format.MatchString(value) {
			line := frontmatterFieldLine(doc, config.Field)
			reportDocumentIdentifier(pass, a.ID(), doc, line, fmt.Sprintf("%s has invalid %s %s %s.", doc.Path, config.FormatLabel, config.Field, value))
		}
		if config.MatchFilename {
			expected := strings.TrimSuffix(filepath.Base(doc.Path), filepath.Ext(doc.Path))
			if value != expected {
				line := frontmatterFieldLine(doc, config.Field)
				reportDocumentIdentifier(pass, a.ID(), doc, line, fmt.Sprintf("%s %s must match filename (%s).", doc.Path, config.Field, expected))
			}
		}
	}
	if config.Unique {
		a.reportDuplicateIdentifiers(pass, index, config, matchingDocs)
	}
}

func (a *DocumentIdentifierAnalyzer) reportDuplicateIdentifiers(pass *interfaces.Pass, index *DocumentationIndex, config DocumentIdentifierConfig, docs []*IndexedDocument) {
	duplicates := duplicatedIdentifiers(docs, config.Field)
	ids := make([]string, 0, len(duplicates))
	for id := range duplicates {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	reservedDocIDs := map[string]struct{}{}
	for _, id := range ids {
		duplicateDocs := duplicates[id]
		sort.SliceStable(duplicateDocs, func(i, j int) bool {
			return duplicateDocs[i].Path < duplicateDocs[j].Path
		})
		fix, hasFix := safeIdentifierReassignmentFix(index, config, duplicateDocs, reservedDocIDs)
		diagnosticDoc := duplicateDocs[0]
		if hasFix {
			diagnosticDoc = duplicateDocs[1]
		}
		field := diagnosticDoc.FrontmatterFields[strings.ToLower(config.Field)]
		diagnostic := interfaces.NewDiagnostic(
			diagnosticDoc.Path,
			field.Line,
			field.ValueStartOffset,
			field.ValueEndOffset,
			a.ID(),
			fmt.Sprintf("duplicate %s %s declared in: %s", config.Field, id, joinedIndexedDocumentPaths(duplicateDocs)),
			interfaces.SeverityError,
		)
		if hasFix {
			diagnostic.SuggestedFixes = []interfaces.SuggestedFix{fix}
			reservedDocIDs[canonicalIdentifier(fix.Edits[0].Replacement)] = struct{}{}
		}
		pass.Report(diagnostic)
	}
}

func normalizedIdentifierConfig(config DocumentIdentifierConfig) DocumentIdentifierConfig {
	config.Field = strings.ToLower(strings.TrimSpace(config.Field))
	config.Name = strings.TrimSpace(config.Name)
	if config.Name == "" {
		config.Name = config.Field
	}
	config.FormatLabel = strings.TrimSpace(config.FormatLabel)
	if config.FormatLabel == "" {
		config.FormatLabel = config.Name
	}
	for index, prefix := range config.PathPrefixes {
		config.PathPrefixes[index] = normalizeIndexPath(prefix)
	}
	return config
}

func identifierMatchingDocuments(index *DocumentationIndex, config DocumentIdentifierConfig) []*IndexedDocument {
	var docs []*IndexedDocument
	for _, doc := range index.Documents {
		if identifierConfigAppliesToPath(config, doc.Path) {
			docs = append(docs, doc)
		}
	}
	return docs
}

func identifierConfigAppliesToPath(config DocumentIdentifierConfig, path string) bool {
	if len(config.PathPrefixes) == 0 {
		return true
	}
	for _, prefix := range config.PathPrefixes {
		if pathInDocumentationArea(path, prefix) {
			return true
		}
	}
	return false
}

func duplicatedIdentifiers(docs []*IndexedDocument, field string) map[string][]*IndexedDocument {
	byCanonicalID := map[string][]*IndexedDocument{}
	for _, doc := range docs {
		canonicalID := canonicalIdentifier(doc.Frontmatter[strings.ToLower(field)])
		if canonicalID == "" {
			continue
		}
		byCanonicalID[canonicalID] = append(byCanonicalID[canonicalID], doc)
	}

	duplicates := map[string][]*IndexedDocument{}
	for id, docs := range byCanonicalID {
		if len(docs) > 1 {
			duplicates[id] = docs
		}
	}
	return duplicates
}

func canonicalIdentifier(id string) string {
	return strings.ToUpper(strings.TrimSpace(id))
}

func reportDocumentIdentifier(pass *interfaces.Pass, ruleID string, doc *IndexedDocument, line int, message string) {
	pass.Report(interfaces.NewDiagnostic(
		doc.Document.Path,
		line,
		-1,
		-1,
		ruleID,
		message,
		interfaces.SeverityError,
	))
}

// DocIDUniquenessAnalyzer reports duplicate frontmatter doc-id values across the lint surface.
type DocIDUniquenessAnalyzer struct {
	*DocumentIdentifierAnalyzer
}

var _ interfaces.Analyzer = (*DocIDUniquenessAnalyzer)(nil)

// NewDocIDUniquenessAnalyzer creates an analyzer that emits duplicate doc-id diagnostics.
func NewDocIDUniquenessAnalyzer() *DocIDUniquenessAnalyzer {
	analyzer := NewDocumentIdentifierAnalyzer(DocumentIdentifierConfig{
		Name:   "doc-id",
		Field:  frontmatterDocIDKey,
		Unique: true,
	})
	analyzer.ruleID = docIDUniqueRuleID
	return &DocIDUniquenessAnalyzer{DocumentIdentifierAnalyzer: analyzer}
}

func safeIdentifierReassignmentFix(index *DocumentationIndex, config DocumentIdentifierConfig, duplicateDocs []*IndexedDocument, reservedDocIDs map[string]struct{}) (interfaces.SuggestedFix, bool) {
	if len(duplicateDocs) != 2 {
		return interfaces.SuggestedFix{}, false
	}
	if config.Field != frontmatterDocIDKey {
		return interfaces.SuggestedFix{}, false
	}

	eligibleDoc := duplicateDocs[1]
	field, ok := eligibleDoc.FrontmatterFields[frontmatterDocIDKey]
	if !ok || field.ValueStartOffset < 0 || field.ValueEndOffset < field.ValueStartOffset {
		return interfaces.SuggestedFix{}, false
	}

	replacement, ok := nextUnusedTypedDocID(index, duplicateDocs[0].DocID, reservedDocIDs)
	if !ok {
		return interfaces.SuggestedFix{}, false
	}

	return interfaces.SuggestedFix{
		Title:      fmt.Sprintf("Reassign duplicate doc-id to %s", replacement),
		Confidence: interfaces.FixConfidenceSafe,
		Edits: []interfaces.TextEdit{
			{
				StartOffset: field.ValueStartOffset,
				EndOffset:   field.ValueEndOffset,
				Replacement: replacement,
			},
		},
	}, true
}

func nextUnusedTypedDocID(index *DocumentationIndex, duplicateID string, reservedDocIDs map[string]struct{}) (string, bool) {
	prefix, width, ok := parseTypedDocID(canonicalIdentifier(duplicateID))
	if !ok {
		return "", false
	}

	usedNumbers := map[int]struct{}{}
	maxNumber := 0
	for _, doc := range index.Documents {
		docPrefix, _, ok := parseTypedDocID(canonicalIdentifier(doc.DocID))
		if !ok || docPrefix != prefix {
			continue
		}
		number, err := typedDocIDNumber(canonicalIdentifier(doc.DocID))
		if err != nil {
			continue
		}
		usedNumbers[number] = struct{}{}
		if number > maxNumber {
			maxNumber = number
		}
	}
	for reservedID := range reservedDocIDs {
		docPrefix, _, ok := parseTypedDocID(reservedID)
		if !ok || docPrefix != prefix {
			continue
		}
		number, err := typedDocIDNumber(reservedID)
		if err != nil {
			continue
		}
		usedNumbers[number] = struct{}{}
		if number > maxNumber {
			maxNumber = number
		}
	}

	nextNumber := maxNumber + 1
	for {
		if _, exists := usedNumbers[nextNumber]; !exists {
			return fmt.Sprintf("%s-%0*d", prefix, width, nextNumber), true
		}
		nextNumber++
	}
}

func parseTypedDocID(id string) (string, int, bool) {
	matches := typedDocIDPattern.FindStringSubmatch(id)
	if matches == nil {
		return "", 0, false
	}
	return matches[1], len(matches[2]), true
}

func typedDocIDNumber(id string) (int, error) {
	matches := typedDocIDPattern.FindStringSubmatch(id)
	if matches == nil {
		return 0, fmt.Errorf("doc-id %q is not typed", id)
	}
	return strconv.Atoi(matches[2])
}

func joinedIndexedDocumentPaths(docs []*IndexedDocument) string {
	paths := make([]string, 0, len(docs))
	for _, doc := range docs {
		paths = append(paths, doc.Path)
	}
	sort.Strings(paths)
	return strings.Join(paths, ", ")
}
