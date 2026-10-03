package rules

import (
	"context"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyChecksReportCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	doc := coverageDocument("guide.md", "# Guide\n\n1. First\n2. Second\n\n[link](other.md)\n")
	for _, rule := range []interfaces.Rule{NewHeadingOrderRule(), NewOrderedListRule(), NewLocalLinkRule()} {
		findings := rule.Check(ctx, doc)
		if len(findings) != 1 || findings[0].Line != 0 || findings[0].Message != context.Canceled.Error() {
			t.Fatalf("%s: %+v", rule.ID(), findings)
		}
	}
}
func TestDirectoryAnchorReadAndMarkerOverflow(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "directory")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	doc := coverageDocument(filepath.Join(root, "guide.md"), "# Guide\n\n[anchor](directory#section)\n")
	got := NewLocalLinkRule(WithDirectoryLinkTargetsAllowed()).Check(context.Background(), doc)
	if len(got) != 1 || !strings.Contains(got[0].Message, "could not be parsed") {
		t.Fatalf("%+v", got)
	}
	if _, ok := orderedListMarker("ordinary text"); ok {
		t.Fatal("ordinary prose marker")
	}
	if _, ok := orderedListMarker(strings.Repeat("9", 100) + ". Item"); ok {
		t.Fatal("overflow marker")
	}
}
