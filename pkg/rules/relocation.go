package rules

import (
	"context"
	"fmt"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const linkRelocationRuleID = "markdown.link-relocation"

// LinkRelocationAnalyzer reports safe relative-link rewrites for moved Markdown and image targets.
type LinkRelocationAnalyzer struct{}

var _ interfaces.Analyzer = (*LinkRelocationAnalyzer)(nil)

// NewLinkRelocationAnalyzer creates an analyzer that emits relocation fixes from the shared documentation index.
func NewLinkRelocationAnalyzer() *LinkRelocationAnalyzer {
	return &LinkRelocationAnalyzer{}
}

// ID returns the stable link-relocation analyzer identifier.
func (a *LinkRelocationAnalyzer) ID() string {
	return linkRelocationRuleID
}

// Analyze reports diagnostics and safe fixes for local links whose targets moved.
func (a *LinkRelocationAnalyzer) Analyze(ctx context.Context, pass *interfaces.Pass) {
	value, ok := pass.Data(DocumentationIndexPassDataKey)
	if !ok {
		return
	}
	index, ok := value.(*DocumentationIndex)
	if !ok || index == nil {
		return
	}

	links := append([]IndexedLink(nil), index.Links...)
	sort.SliceStable(links, func(i, j int) bool {
		if links[i].SourceFile != links[j].SourceFile {
			return links[i].SourceFile < links[j].SourceFile
		}
		if links[i].DestinationRange.StartOffset != links[j].DestinationRange.StartOffset {
			return links[i].DestinationRange.StartOffset < links[j].DestinationRange.StartOffset
		}
		return links[i].RawTarget < links[j].RawTarget
	})

	for _, link := range links {
		if err := ctx.Err(); err != nil {
			pass.Report(a.diagnosticWithoutFix(link, interfaces.DiagnosticCategoryRelocationOperational, err.Error()))
			return
		}
		if link.DestinationRange.StartOffset < 0 || link.DestinationRange.EndOffset < link.DestinationRange.StartOffset {
			pass.Report(a.diagnosticWithoutFix(link, interfaces.DiagnosticCategoryRelocationUnavailableRange, "local link destination range is unavailable for relocation"))
			continue
		}

		if err := interfaces.CheckPathRoot(pass.Root, link.NormalizedFile); err != nil {
			pass.Report(a.diagnosticWithoutFix(link, interfaces.DiagnosticCategoryRelocationOperational, "local link target is outside document root or inaccessible"))
			continue
		}
		if target, ok := index.MoveMappings[normalizeIndexPath(link.NormalizedFile)]; ok {
			if err := interfaces.CheckPathRoot(pass.Root, target); err != nil {
				pass.Report(a.diagnosticWithoutFix(link, interfaces.DiagnosticCategoryRelocationMissingMappedTarget, "mapped target is outside document root or inaccessible"))
				continue
			}
		}

		switch link.Kind {
		case IndexedLinkKindMarkdown:
			a.analyzeMarkdownLink(pass, index, link)
		case IndexedLinkKindImage:
			a.analyzeImageLink(pass, index, link)
		}
	}
}

func (a *LinkRelocationAnalyzer) analyzeMarkdownLink(pass *interfaces.Pass, index *DocumentationIndex, link IndexedLink) {
	if _, exists := index.Document(link.NormalizedFile); exists {
		return
	}

	target, message := a.resolveMarkdownTarget(index, link)
	if target == nil {
		pass.Report(a.diagnosticWithoutFix(link, relocationDiagnosticCategory(message), message))
		return
	}
	if link.Anchor != "" {
		if _, ok := target.Anchors[link.Anchor]; !ok {
			pass.Report(a.diagnosticWithoutFix(link, interfaces.DiagnosticCategoryRelocationInvalidAnchor, fmt.Sprintf("moved Markdown target %s does not contain anchor #%s", target.Path, link.Anchor)))
			return
		}
	}

	replacement := relativeLinkDestination(link.SourceFile, target.Path, link.RawTarget)
	pass.Report(a.diagnosticWithReplacement(link, replacement, fmt.Sprintf("rewrite moved Markdown link target to %s", replacement)))
}

func (a *LinkRelocationAnalyzer) analyzeImageLink(pass *interfaces.Pass, index *DocumentationIndex, link IndexedLink) {
	if fileExists(link.NormalizedFile) {
		return
	}

	movedPath, ok := index.MoveMappings[normalizeIndexPath(link.NormalizedFile)]
	if ok {
		targetPath := normalizeIndexPath(movedPath)
		if !fileExists(targetPath) {
			pass.Report(a.diagnosticWithoutFix(link, interfaces.DiagnosticCategoryRelocationMissingMappedTarget, fmt.Sprintf("mapped image target does not exist: %s", targetPath)))
			return
		}

		replacement := relativeLinkDestination(link.SourceFile, targetPath, link.RawTarget)
		pass.Report(a.diagnosticWithReplacement(link, replacement, fmt.Sprintf("rewrite moved image link target to %s", replacement)))
		return
	}

	resolution := index.ResolveAssetExactFilename(link.NormalizedFile)
	if resolution.Target == "" {
		if resolution.Ambiguous {
			pass.Report(a.diagnosticWithoutFix(link, interfaces.DiagnosticCategoryRelocationAmbiguous, fmt.Sprintf("local image target is ambiguous: %s", filepath.Base(link.NormalizedFile))))
			return
		}
		pass.Report(a.diagnosticWithoutFix(link, interfaces.DiagnosticCategoryRelocationUnresolved, "local image target could not be resolved uniquely"))
		return
	}

	replacement := relativeLinkDestination(link.SourceFile, resolution.Target, link.RawTarget)
	pass.Report(a.diagnosticWithReplacement(link, replacement, fmt.Sprintf("rewrite moved image link target to %s", replacement)))
}

func (a *LinkRelocationAnalyzer) resolveMarkdownTarget(index *DocumentationIndex, link IndexedLink) (*IndexedDocument, string) {
	normalizedTarget := normalizeIndexPath(link.NormalizedFile)
	if movedPath, ok := index.MoveMappings[normalizedTarget]; ok {
		targetPath := normalizeIndexPath(movedPath)
		target, exists := index.Document(targetPath)
		if !exists {
			return nil, fmt.Sprintf("mapped Markdown target is not in lint surface: %s", targetPath)
		}
		return target, ""
	}

	resolution := index.ResolveExactFilename(normalizedTarget)
	if resolution.Target != nil {
		return resolution.Target, ""
	}
	if resolution.Ambiguous {
		return nil, fmt.Sprintf("local Markdown target is ambiguous: %s", filepath.Base(normalizedTarget))
	}
	return nil, "local Markdown target could not be resolved uniquely"
}

func (a *LinkRelocationAnalyzer) diagnosticWithReplacement(link IndexedLink, replacement string, message string) interfaces.Diagnostic {
	diagnostic := a.diagnosticWithoutFix(link, interfaces.DiagnosticCategoryGeneral, message)
	diagnostic.SuggestedFixes = []interfaces.SuggestedFix{
		{
			Title:      "Rewrite moved local link target",
			Confidence: interfaces.FixConfidenceSafe,
			Edits: []interfaces.TextEdit{
				{
					StartOffset: link.DestinationRange.StartOffset,
					EndOffset:   link.DestinationRange.EndOffset,
					Replacement: replacement,
				},
			},
		},
	}
	return diagnostic
}

func (a *LinkRelocationAnalyzer) diagnosticWithoutFix(link IndexedLink, category interfaces.DiagnosticCategory, message string) interfaces.Diagnostic {
	diagnostic := interfaces.NewDiagnostic(
		link.SourceFile,
		link.Line,
		link.DestinationRange.StartOffset,
		link.DestinationRange.EndOffset,
		a.ID(),
		message,
		interfaces.SeverityError,
	)
	diagnostic.Category = category
	return diagnostic
}

func relocationDiagnosticCategory(message string) interfaces.DiagnosticCategory {
	if strings.Contains(message, "ambiguous") {
		return interfaces.DiagnosticCategoryRelocationAmbiguous
	}
	if strings.Contains(message, "mapped Markdown target") {
		return interfaces.DiagnosticCategoryRelocationMissingMappedTarget
	}
	return interfaces.DiagnosticCategoryRelocationUnresolved
}

func relativeLinkDestination(sourceFile string, targetPath string, rawTarget string) string {
	sourceDir := filepath.Dir(filepath.FromSlash(normalizeIndexPath(sourceFile)))
	target := filepath.FromSlash(normalizeIndexPath(targetPath))
	relative, err := filepath.Rel(sourceDir, target)
	if err != nil {
		relative = target
	}
	return filepath.ToSlash(relative) + rawFragmentSuffix(rawTarget)
}

func rawFragmentSuffix(rawTarget string) string {
	index := strings.Index(rawTarget, "#")
	if index < 0 {
		return ""
	}
	return rawTarget[index:]
}

func fileExists(path string) bool {
	info, err := os.Stat(filepath.FromSlash(normalizeIndexPath(path)))
	return err == nil && !info.IsDir()
}
