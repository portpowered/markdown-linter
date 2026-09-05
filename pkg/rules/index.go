package rules

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/yuin/goldmark/ast"
)

const (
	// DocumentationIndexPassDataKey is the pass data key used by DocumentationIndexAnalyzer.
	DocumentationIndexPassDataKey interfaces.PassDataKey = "markdown.documentation-index"

	frontmatterDocIDKey     = "doc-id"
	frontmatterProcessIDKey = "process-id"
)

// DocumentationIndexOption configures repository documentation index construction.
type DocumentationIndexOption func(*documentationIndexConfig)

type documentationIndexConfig struct {
	moveMappings map[string]string
	root         string
	skipAssets   bool
}

// WithIndexRoot sets the filesystem boundary and asset discovery root.
func WithIndexRoot(root string) DocumentationIndexOption {
	return func(config *documentationIndexConfig) { config.root = root }
}

// WithoutAssetDiscovery builds a document-only index without scanning the filesystem.
func WithoutAssetDiscovery() DocumentationIndexOption {
	return func(config *documentationIndexConfig) { config.skipAssets = true }
}

// WithMoveMappings registers explicit old-to-new file mappings for moved documents or assets.
func WithMoveMappings(mappings map[string]string) DocumentationIndexOption {
	return func(config *documentationIndexConfig) {
		for oldPath, newPath := range mappings {
			config.moveMappings[normalizeIndexPath(oldPath)] = normalizeIndexPath(newPath)
		}
	}
}

// DocumentationIndexAnalyzer builds a repository documentation index and stores it on the pass.
type DocumentationIndexAnalyzer struct {
	options []DocumentationIndexOption
}

var _ interfaces.Analyzer = (*DocumentationIndexAnalyzer)(nil)

// NewDocumentationIndexAnalyzer creates an analyzer that shares a documentation index with later analyzers.
func NewDocumentationIndexAnalyzer(options ...DocumentationIndexOption) *DocumentationIndexAnalyzer {
	return &DocumentationIndexAnalyzer{options: options}
}

// ID returns the stable documentation-index analyzer identifier.
func (a *DocumentationIndexAnalyzer) ID() string {
	return "markdown.documentation-index"
}

// Analyze builds the index from pass documents.
func (a *DocumentationIndexAnalyzer) Analyze(_ context.Context, pass *interfaces.Pass) {
	options := append(append([]DocumentationIndexOption(nil), a.options...), WithIndexRoot(pass.Root))
	pass.SetData(DocumentationIndexPassDataKey, NewDocumentationIndex(pass.Documents, options...))
}

// DocumentationIndex records repository Markdown files, identifiers, anchors, links, and move mappings.
type DocumentationIndex struct {
	Documents    []*IndexedDocument
	Links        []IndexedLink
	AssetPaths   []string
	MoveMappings map[string]string

	byPath      map[string]*IndexedDocument
	byAssetName map[string][]string
	byDocID     map[string][]*IndexedDocument
	byProcessID map[string][]*IndexedDocument
	byBasename  map[string][]*IndexedDocument
	byFilename  map[string][]*IndexedDocument
	byTitle     map[string][]*IndexedDocument
}

// IndexedDocument describes one Markdown document in the lint surface.
type IndexedDocument struct {
	Document          *interfaces.Document
	Path              string
	Title             string
	DocID             string
	ProcessID         string
	Frontmatter       map[string]string
	FrontmatterFields map[string]IndexedFrontmatterField
	Headings          []IndexedHeading
	Anchors           map[string]struct{}
	InboundLinks      []IndexedLink
}

// IndexedFrontmatterField describes a parsed frontmatter key/value location.
type IndexedFrontmatterField struct {
	Key              string
	Value            string
	Line             int
	KeyStartOffset   int
	KeyEndOffset     int
	ValueStartOffset int
	ValueEndOffset   int
}

// IndexedHeading describes a Markdown heading and its generated anchor.
type IndexedHeading struct {
	Text   string
	Anchor string
	Line   int
	Level  int
}

// IndexedLinkKind classifies local links found in Markdown documents.
type IndexedLinkKind string

const (
	// IndexedLinkKindMarkdown is a local Markdown link.
	IndexedLinkKindMarkdown IndexedLinkKind = "markdown"
	// IndexedLinkKindImage is a local image link.
	IndexedLinkKindImage IndexedLinkKind = "image"
)

// IndexedLink records a local Markdown or image link destination.
type IndexedLink struct {
	Kind             IndexedLinkKind
	SourceFile       string
	Line             int
	DestinationRange engine.SourceRange
	RawTarget        string
	NormalizedFile   string
	Anchor           string
}

// TargetResolution describes a candidate lookup result from the documentation index.
type TargetResolution struct {
	Target     *IndexedDocument
	Candidates []*IndexedDocument
	Strategy   string
	Ambiguous  bool
}

// AssetResolution describes a candidate non-Markdown asset lookup result.
type AssetResolution struct {
	Target     string
	Candidates []string
	Strategy   string
	Ambiguous  bool
}

// NewDocumentationIndex builds a repository documentation index from parsed Markdown documents.
func NewDocumentationIndex(documents []*interfaces.Document, options ...DocumentationIndexOption) *DocumentationIndex {
	config := documentationIndexConfig{moveMappings: map[string]string{}}
	for _, option := range options {
		option(&config)
	}

	index := &DocumentationIndex{
		MoveMappings: appendStringMap(config.moveMappings),
		byPath:       map[string]*IndexedDocument{},
		byAssetName:  map[string][]string{},
		byDocID:      map[string][]*IndexedDocument{},
		byProcessID:  map[string][]*IndexedDocument{},
		byBasename:   map[string][]*IndexedDocument{},
		byFilename:   map[string][]*IndexedDocument{},
		byTitle:      map[string][]*IndexedDocument{},
	}

	for _, doc := range documents {
		indexed := newIndexedDocument(doc)
		index.Documents = append(index.Documents, indexed)
		index.byPath[indexed.Path] = indexed
		appendIndex(index.byDocID, indexed.DocID, indexed)
		appendIndex(index.byProcessID, indexed.ProcessID, indexed)
		appendIndex(index.byBasename, strings.ToLower(filepath.Base(indexed.Path)), indexed)
		appendIndex(index.byFilename, strings.ToLower(strings.TrimSuffix(filepath.Base(indexed.Path), filepath.Ext(indexed.Path))), indexed)
		appendIndex(index.byTitle, strings.ToLower(indexed.Title), indexed)
	}

	sort.Slice(index.Documents, func(i, j int) bool {
		return index.Documents[i].Path < index.Documents[j].Path
	})

	for _, doc := range documents {
		for _, link := range collectIndexedLinks(doc) {
			index.Links = append(index.Links, link)
			if target := index.byPath[link.NormalizedFile]; target != nil {
				target.InboundLinks = append(target.InboundLinks, link)
			}
		}
	}

	if !config.skipAssets {
		index.indexAssetCandidates(documents, config.root)
	}

	return index
}

// interfaces.Document returns an indexed Markdown document by normalized path.
func (i *DocumentationIndex) Document(path string) (*IndexedDocument, bool) {
	doc, ok := i.byPath[normalizeIndexPath(path)]
	return doc, ok
}

// ResolveDocument resolves a unique candidate document using explicit moves, stable identifiers, and best-effort names.
func (i *DocumentationIndex) ResolveDocument(query string) TargetResolution {
	normalizedQuery := normalizeIndexPath(query)
	if movedPath, ok := i.MoveMappings[normalizedQuery]; ok {
		return i.resolveCandidates("move-mapping", []*IndexedDocument{i.byPath[movedPath]})
	}

	if result := i.resolveCandidates("doc-id", i.byDocID[query]); result.Target != nil || result.Ambiguous {
		return result
	}
	if result := i.resolveCandidates("process-id", i.byProcessID[query]); result.Target != nil || result.Ambiguous {
		return result
	}
	if result := i.resolveCandidates("path", []*IndexedDocument{i.byPath[normalizedQuery]}); result.Target != nil || result.Ambiguous {
		return result
	}
	if result := i.resolveCandidates("basename", i.byBasename[strings.ToLower(filepath.Base(normalizedQuery))]); result.Target != nil || result.Ambiguous {
		return result
	}
	if result := i.resolveCandidates("filename", i.byFilename[strings.ToLower(strings.TrimSuffix(filepath.Base(normalizedQuery), filepath.Ext(normalizedQuery)))]); result.Target != nil || result.Ambiguous {
		return result
	}
	return i.resolveCandidates("title", i.byTitle[strings.ToLower(query)])
}

// ResolveExactFilename resolves a document by an exact unique Markdown basename.
func (i *DocumentationIndex) ResolveExactFilename(query string) TargetResolution {
	normalizedQuery := normalizeIndexPath(query)
	return i.resolveCandidates("exact-filename", i.byBasename[strings.ToLower(filepath.Base(normalizedQuery))])
}

// ResolveAssetExactFilename resolves a non-Markdown asset by an exact unique basename.
func (i *DocumentationIndex) ResolveAssetExactFilename(query string) AssetResolution {
	normalizedQuery := normalizeIndexPath(query)
	return i.resolveAssetCandidates("exact-asset-filename", i.byAssetName[strings.ToLower(filepath.Base(normalizedQuery))])
}

func (i *DocumentationIndex) resolveCandidates(strategy string, candidates []*IndexedDocument) TargetResolution {
	filtered := compactIndexedDocuments(candidates)
	switch len(filtered) {
	case 0:
		return TargetResolution{Strategy: strategy}
	case 1:
		return TargetResolution{Target: filtered[0], Candidates: filtered, Strategy: strategy}
	default:
		return TargetResolution{Candidates: filtered, Strategy: strategy, Ambiguous: true}
	}
}

func (i *DocumentationIndex) resolveAssetCandidates(strategy string, candidates []string) AssetResolution {
	filtered := compactStrings(candidates)
	switch len(filtered) {
	case 0:
		return AssetResolution{Strategy: strategy}
	case 1:
		return AssetResolution{Target: filtered[0], Candidates: filtered, Strategy: strategy}
	default:
		return AssetResolution{Candidates: filtered, Strategy: strategy, Ambiguous: true}
	}
}

func (i *DocumentationIndex) indexAssetCandidates(documents []*interfaces.Document, boundary string) {
	docPaths := make([]string, 0, len(documents))
	documentPathSet := map[string]struct{}{}
	for _, doc := range documents {
		normalized := normalizeIndexPath(doc.Path)
		docPaths = append(docPaths, normalized)
		documentPathSet[normalized] = struct{}{}
	}

	roots := documentationAssetRoots(docPaths)
	if boundary != "" {
		roots = []string{boundary}
	}
	for _, root := range roots {
		_ = filepath.WalkDir(filepath.FromSlash(root), func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			if err := interfaces.CheckPathRoot(boundary, path); err != nil {
				return nil
			}
			normalized := normalizeIndexPath(path)
			if _, isDocument := documentPathSet[normalized]; isDocument || isMarkdownPath(normalized) {
				return nil
			}
			if _, err := os.Stat(filepath.FromSlash(normalized)); err != nil {
				return nil
			}
			i.AssetPaths = append(i.AssetPaths, normalized)
			i.byAssetName[strings.ToLower(filepath.Base(normalized))] = append(i.byAssetName[strings.ToLower(filepath.Base(normalized))], normalized)
			return nil
		})
	}
	i.AssetPaths = compactStrings(i.AssetPaths)
	sort.Strings(i.AssetPaths)
}

func documentationAssetRoots(paths []string) []string {
	roots := map[string]struct{}{}
	if common := commonDirectory(paths); common != "" {
		roots[common] = struct{}{}
	}
	for _, path := range paths {
		if docsRoot := docsAncestor(path); docsRoot != "" {
			roots[docsRoot] = struct{}{}
		}
	}

	result := make([]string, 0, len(roots))
	for root := range roots {
		result = append(result, root)
	}
	sort.Strings(result)
	return result
}

func commonDirectory(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	parts := strings.Split(normalizeIndexPath(filepath.Dir(paths[0])), "/")
	for _, path := range paths[1:] {
		candidate := strings.Split(normalizeIndexPath(filepath.Dir(path)), "/")
		limit := min(len(parts), len(candidate))
		index := 0
		for index < limit && parts[index] == candidate[index] {
			index++
		}
		parts = parts[:index]
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "/")
}

func docsAncestor(path string) string {
	parts := strings.Split(normalizeIndexPath(path), "/")
	for index, part := range parts {
		if part == "docs" {
			return strings.Join(parts[:index+1], "/")
		}
	}
	return ""
}

func newIndexedDocument(doc *interfaces.Document) *IndexedDocument {
	frontmatterFields := parseFrontmatterFields(doc.Source)
	frontmatter := frontmatterValues(frontmatterFields)
	headings, anchors := collectIndexedHeadings(doc)
	title := firstHeadingTitleFromIndex(headings)
	if title == "" {
		title = frontmatter["title"]
	}

	return &IndexedDocument{
		Document:          doc,
		Path:              normalizeIndexPath(doc.Path),
		Title:             title,
		DocID:             frontmatter[frontmatterDocIDKey],
		ProcessID:         frontmatter[frontmatterProcessIDKey],
		Frontmatter:       frontmatter,
		FrontmatterFields: frontmatterFields,
		Headings:          headings,
		Anchors:           anchors,
	}
}

func collectIndexedLinks(doc *interfaces.Document) []IndexedLink {
	var links []IndexedLink
	callbacks := engine.VisitorCallbacks{
		Link: func(_ context.Context, visit engine.LinkVisit) {
			if link, ok := indexedLocalLink(doc, IndexedLinkKindMarkdown, visit.Destination, visit.DestinationRange, visit.Range.Line); ok {
				links = append(links, link)
			}
		},
		Image: func(_ context.Context, visit engine.ImageVisit) {
			if link, ok := indexedLocalLink(doc, IndexedLinkKindImage, visit.Destination, visit.DestinationRange, visit.Range.Line); ok {
				links = append(links, link)
			}
		},
	}
	engine.WalkVisitorCallbacks(context.Background(), interfaces.NewPass([]*interfaces.Document{doc}), doc, callbacks)
	return links
}

func indexedLocalLink(doc *interfaces.Document, kind IndexedLinkKind, rawTarget string, destinationRange engine.SourceRange, line int) (IndexedLink, bool) {
	target, ok := parseLocalLinkTarget(strings.TrimSpace(rawTarget))
	if !ok {
		return IndexedLink{}, false
	}

	normalizedFile := normalizeIndexPath(doc.Path)
	if target.path != "" {
		normalizedFile = normalizeIndexPath(filepath.Join(filepath.Dir(doc.Path), filepath.FromSlash(target.path)))
	}

	return IndexedLink{
		Kind:             kind,
		SourceFile:       normalizeIndexPath(doc.Path),
		Line:             line,
		DestinationRange: destinationRange,
		RawTarget:        rawTarget,
		NormalizedFile:   normalizedFile,
		Anchor:           target.anchor,
	}, true
}

func collectIndexedHeadings(doc *interfaces.Document) ([]IndexedHeading, map[string]struct{}) {
	var headings []IndexedHeading
	anchors := make(map[string]struct{})
	seen := make(map[string]int)

	_ = doc.Walk(func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || node.Kind() != ast.KindHeading {
			return ast.WalkContinue, nil
		}

		heading := node.(*ast.Heading)
		text := string(node.Text(doc.Source))
		baseAnchor := normalizeAnchor(text)
		if baseAnchor == "" {
			return ast.WalkContinue, nil
		}

		index := seen[baseAnchor]
		seen[baseAnchor] = index + 1

		anchor := baseAnchor
		if index > 0 {
			anchor = baseAnchor + "-" + strconv.Itoa(index)
		}

		headings = append(headings, IndexedHeading{
			Text:   text,
			Anchor: anchor,
			Line:   doc.NodeLine(node),
			Level:  heading.Level,
		})
		anchors[anchor] = struct{}{}
		return ast.WalkContinue, nil
	})

	return headings, anchors
}

func parseFrontmatter(source []byte) map[string]string {
	return frontmatterValues(parseFrontmatterFields(source))
}

func frontmatterValues(fields map[string]IndexedFrontmatterField) map[string]string {
	frontmatter := map[string]string{}
	for key, field := range fields {
		frontmatter[key] = field.Value
	}
	return frontmatter
}

func parseFrontmatterFields(source []byte) map[string]IndexedFrontmatterField {
	frontmatter := map[string]IndexedFrontmatterField{}
	text := string(source)
	lines := strings.SplitAfter(text, "\n")
	startLine, startOffset, ok := frontmatterBlockStart(lines)
	if !ok {
		return frontmatter
	}

	offset := startOffset
	for index, rawLine := range lines[startLine:] {
		lineStartOffset := offset
		offset += len(rawLine)
		if index == 0 {
			continue
		}

		lineWithoutNewline := strings.TrimSuffix(strings.TrimSuffix(rawLine, "\n"), "\r")
		trimmedLine := strings.TrimSpace(lineWithoutNewline)
		if trimmedLine == "---" {
			return frontmatter
		}
		key, value, ok := strings.Cut(lineWithoutNewline, ":")
		if !ok {
			continue
		}
		keyStartColumn := len(key) - len(strings.TrimLeft(key, " \t"))
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		valueStartColumn := len(key) + 1 + len(value) - len(strings.TrimLeft(value, " \t"))
		trimmedValue := strings.TrimSpace(value)
		unquotedValue, quoteOffset := trimFrontmatterValueQuotes(trimmedValue)
		valueStartColumn += quoteOffset

		normalizedKey := strings.ToLower(trimmedKey)
		frontmatter[normalizedKey] = IndexedFrontmatterField{
			Key:              normalizedKey,
			Value:            unquotedValue,
			Line:             startLine + index + 1,
			KeyStartOffset:   lineStartOffset + keyStartColumn,
			KeyEndOffset:     lineStartOffset + keyStartColumn + len(trimmedKey),
			ValueStartOffset: lineStartOffset + valueStartColumn,
			ValueEndOffset:   lineStartOffset + valueStartColumn + len(unquotedValue),
		}
	}

	return frontmatter
}

func frontmatterBlockStart(lines []string) (int, int, bool) {
	if len(lines) == 0 {
		return 0, 0, false
	}
	if strings.TrimSpace(lines[0]) == "---" {
		return 0, 0, true
	}
	if len(lines) < 3 {
		return 0, 0, false
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[0]), "# ") || strings.TrimSpace(lines[1]) != "" || strings.TrimSpace(lines[2]) != "---" {
		return 0, 0, false
	}
	return 2, len(lines[0]) + len(lines[1]), true
}

func trimFrontmatterValueQuotes(value string) (string, int) {
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1], 1
		}
	}
	return strings.Trim(value, `"'`), 0
}

func firstHeadingTitleFromIndex(headings []IndexedHeading) string {
	for _, heading := range headings {
		if heading.Level == 1 {
			return heading.Text
		}
	}
	if len(headings) > 0 {
		return headings[0].Text
	}
	return ""
}

func appendIndex(index map[string][]*IndexedDocument, key string, doc *IndexedDocument) {
	if key == "" {
		return
	}
	index[key] = append(index[key], doc)
}

func appendStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func compactStrings(values []string) []string {
	seen := map[string]struct{}{}
	var filtered []string
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		filtered = append(filtered, value)
	}
	sort.Strings(filtered)
	return filtered
}

func compactIndexedDocuments(documents []*IndexedDocument) []*IndexedDocument {
	seen := map[string]struct{}{}
	var filtered []*IndexedDocument
	for _, doc := range documents {
		if doc == nil {
			continue
		}
		if _, exists := seen[doc.Path]; exists {
			continue
		}
		seen[doc.Path] = struct{}{}
		filtered = append(filtered, doc)
	}
	return filtered
}

func normalizeIndexPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func isMarkdownPath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".md")
}
