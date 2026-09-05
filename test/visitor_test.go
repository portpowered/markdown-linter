package markdownlint_test

import (
	"context"
	engine "github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"testing"
)

func TestVisitorRule_LinkAndImageCallbacksReportDestinationTextEdits(t *testing.T) {
	path := writeTempMarkdown(t, "A [doc](old/doc.md) and ![diagram](assets/old.png) stay in one paragraph.\n")
	rule := engine.NewVisitorRule("test.visitor", engine.VisitorCallbacks{
		Link: func(_ context.Context, visit engine.LinkVisit) {
			diagnostic := interfaces.NewDiagnostic(
				visit.Document.Path,
				visit.DestinationRange.Line,
				visit.DestinationRange.StartOffset,
				visit.DestinationRange.EndOffset,
				"test.visitor",
				"rewrite link destination",
				interfaces.SeverityError,
			)
			diagnostic.SuggestedFixes = []interfaces.SuggestedFix{{
				Title:      "Rewrite link destination",
				Confidence: interfaces.FixConfidenceSafe,
				Edits: []interfaces.TextEdit{{
					StartOffset: visit.DestinationRange.StartOffset,
					EndOffset:   visit.DestinationRange.EndOffset,
					Replacement: "new/doc.md",
				}},
			}}
			visit.Pass.Report(diagnostic)
		},
		Image: func(_ context.Context, visit engine.ImageVisit) {
			diagnostic := interfaces.NewDiagnostic(
				visit.Document.Path,
				visit.DestinationRange.Line,
				visit.DestinationRange.StartOffset,
				visit.DestinationRange.EndOffset,
				"test.visitor",
				"rewrite image destination",
				interfaces.SeverityError,
			)
			diagnostic.SuggestedFixes = []interfaces.SuggestedFix{{
				Title:      "Rewrite image destination",
				Confidence: interfaces.FixConfidenceSafe,
				Edits: []interfaces.TextEdit{{
					StartOffset: visit.DestinationRange.StartOffset,
					EndOffset:   visit.DestinationRange.EndOffset,
					Replacement: "assets/new.png",
				}},
			}}
			visit.Pass.Report(diagnostic)
		},
	})
	l := engine.New(engine.WithDiagnosticRules(rule))

	diagnostics, err := l.RunFileDiagnostics(context.Background(), path)
	if err != nil {
		t.Fatalf("RunFileDiagnostics returned error: %v", err)
	}
	if len(diagnostics) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d: %#v", len(diagnostics), diagnostics)
	}

	got := applyTextEdits(string(mustReadFile(t, path)), []interfaces.TextEdit{
		diagnostics[0].SuggestedFixes[0].Edits[0],
		diagnostics[1].SuggestedFixes[0].Edits[0],
	})
	want := "A [doc](new/doc.md) and ![diagram](assets/new.png) stay in one paragraph.\n"
	if got != want {
		t.Fatalf("edited markdown = %q, want %q", got, want)
	}
}

func TestVisitorRule_DenseInlineLinksWithIdenticalDestinationsReportDistinctRanges(t *testing.T) {
	path := writeTempMarkdown(t, "[first](old.md)[second](old.md) ![first](old.png)![second](old.png)\n")
	rule := engine.NewVisitorRule("test.visitor", engine.VisitorCallbacks{
		Link: func(_ context.Context, visit engine.LinkVisit) {
			visit.Pass.Report(interfaces.NewDiagnostic(
				visit.Document.Path,
				visit.DestinationRange.Line,
				visit.DestinationRange.StartOffset,
				visit.DestinationRange.EndOffset,
				"test.visitor",
				"rewrite link destination",
				interfaces.SeverityError,
			))
		},
		Image: func(_ context.Context, visit engine.ImageVisit) {
			visit.Pass.Report(interfaces.NewDiagnostic(
				visit.Document.Path,
				visit.DestinationRange.Line,
				visit.DestinationRange.StartOffset,
				visit.DestinationRange.EndOffset,
				"test.visitor",
				"rewrite image destination",
				interfaces.SeverityError,
			))
		},
	})
	l := engine.New(engine.WithDiagnosticRules(rule))

	diagnostics, err := l.RunFileDiagnostics(context.Background(), path)
	if err != nil {
		t.Fatalf("RunFileDiagnostics returned error: %v", err)
	}
	if len(diagnostics) != 4 {
		t.Fatalf("expected 4 diagnostics, got %d: %#v", len(diagnostics), diagnostics)
	}
	for index := 1; index < len(diagnostics); index++ {
		if diagnostics[index].StartOffset <= diagnostics[index-1].StartOffset {
			t.Fatalf("expected increasing dense destination ranges, got %#v", diagnostics)
		}
	}
}

func TestVisitorRule_ReferenceStyleLinksAndImagesReportDefinitionDestinationRanges(t *testing.T) {
	path := writeTempMarkdown(t, "See [target][doc-ref] and ![diagram][image-ref].\n\n[doc-ref]: old/doc.md \"Doc\"\n[image-ref]: assets/old.png\n")
	rule := engine.NewVisitorRule("test.visitor", engine.VisitorCallbacks{
		Link: func(_ context.Context, visit engine.LinkVisit) {
			diagnostic := interfaces.NewDiagnostic(
				visit.Document.Path,
				visit.DestinationRange.Line,
				visit.DestinationRange.StartOffset,
				visit.DestinationRange.EndOffset,
				"test.visitor",
				"rewrite reference link destination",
				interfaces.SeverityError,
			)
			diagnostic.SuggestedFixes = []interfaces.SuggestedFix{{
				Title:      "Rewrite reference link destination",
				Confidence: interfaces.FixConfidenceSafe,
				Edits: []interfaces.TextEdit{{
					StartOffset: visit.DestinationRange.StartOffset,
					EndOffset:   visit.DestinationRange.EndOffset,
					Replacement: "new/doc.md",
				}},
			}}
			visit.Pass.Report(diagnostic)
		},
		Image: func(_ context.Context, visit engine.ImageVisit) {
			diagnostic := interfaces.NewDiagnostic(
				visit.Document.Path,
				visit.DestinationRange.Line,
				visit.DestinationRange.StartOffset,
				visit.DestinationRange.EndOffset,
				"test.visitor",
				"rewrite reference image destination",
				interfaces.SeverityError,
			)
			diagnostic.SuggestedFixes = []interfaces.SuggestedFix{{
				Title:      "Rewrite reference image destination",
				Confidence: interfaces.FixConfidenceSafe,
				Edits: []interfaces.TextEdit{{
					StartOffset: visit.DestinationRange.StartOffset,
					EndOffset:   visit.DestinationRange.EndOffset,
					Replacement: "assets/new.png",
				}},
			}}
			visit.Pass.Report(diagnostic)
		},
	})
	l := engine.New(engine.WithDiagnosticRules(rule))

	diagnostics, err := l.RunFileDiagnostics(context.Background(), path)
	if err != nil {
		t.Fatalf("RunFileDiagnostics returned error: %v", err)
	}
	if len(diagnostics) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d: %#v", len(diagnostics), diagnostics)
	}

	got := applyTextEdits(string(mustReadFile(t, path)), []interfaces.TextEdit{
		diagnostics[0].SuggestedFixes[0].Edits[0],
		diagnostics[1].SuggestedFixes[0].Edits[0],
	})
	want := "See [target][doc-ref] and ![diagram][image-ref].\n\n[doc-ref]: new/doc.md \"Doc\"\n[image-ref]: assets/new.png\n"
	if got != want {
		t.Fatalf("edited markdown = %q, want %q", got, want)
	}
}

func TestVisitorRule_CommonNodeCallbacksReceiveSourceRanges(t *testing.T) {
	path := writeTempMarkdown(t, "# Title\n\n1. Item text\n")
	var visited []string
	rule := engine.NewVisitorRule("test.visitor", engine.VisitorCallbacks{
		Heading: func(_ context.Context, visit engine.HeadingVisit) {
			if visit.Range.Line != 1 || visit.Range.StartOffset < 0 || visit.Range.EndOffset <= visit.Range.StartOffset {
				t.Fatalf("heading range = %#v, want concrete range on line 1", visit.Range)
			}
			visited = append(visited, "heading")
		},
		List: func(_ context.Context, visit engine.ListVisit) {
			if visit.Range.Line != 3 {
				t.Fatalf("list line = %d, want 3", visit.Range.Line)
			}
			visited = append(visited, "list")
		},
		Text: func(_ context.Context, visit engine.TextVisit) {
			if visit.Value == "Item text" {
				if visit.Range.Line != 3 || visit.Range.StartOffset < 0 || visit.Range.EndOffset <= visit.Range.StartOffset {
					t.Fatalf("text range = %#v, want concrete range on line 3", visit.Range)
				}
				visited = append(visited, "text")
			}
		},
	})
	l := engine.New(engine.WithAnalyzers(rule))

	_, err := l.RunFilesDiagnostics(context.Background(), []string{path})
	if err != nil {
		t.Fatalf("RunFilesDiagnostics returned error: %v", err)
	}

	for _, want := range []string{"heading", "list", "text"} {
		if !containsString(visited, want) {
			t.Fatalf("expected callback %q in visited callbacks %#v", want, visited)
		}
	}
}

func applyTextEdits(source string, edits []interfaces.TextEdit) string {
	for left := 0; left < len(edits)-1; left++ {
		for right := left + 1; right < len(edits); right++ {
			if edits[left].StartOffset < edits[right].StartOffset {
				edits[left], edits[right] = edits[right], edits[left]
			}
		}
	}

	for _, edit := range edits {
		source = source[:edit.StartOffset] + edit.Replacement + source[edit.EndOffset:]
	}
	return source
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
