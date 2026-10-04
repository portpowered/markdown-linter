package rulepack

import (
	"path/filepath"
	"testing"
)

func TestScopeValidationAndMatching(t *testing.T) {
	for _, pattern := range []string{"", "../escape", "/root", `docs\file`, "docs/../file", "**/file", "docs/**/file", "["} {
		if err := validatePattern(pattern); err == nil {
			t.Fatalf("accepted scope %q", pattern)
		}
	}
	for _, tc := range []struct {
		pattern, file string
		want          bool
	}{{"docs/**", "docs", true}, {"docs/**", "docs/nested/file", true}, {"docs/**", "other/file", false}, {"*.md", "guide.md", true}, {"*.md", "nested/guide.md", false}} {
		if got := matches(tc.pattern, tc.file); got != tc.want {
			t.Fatalf("%+v got=%v", tc, got)
		}
	}
	if anyMatch([]string{"a.md"}, "b.md") {
		t.Fatal("unexpected match")
	}
	if _, err := relativePath(t.TempDir(), filepath.Join(t.TempDir(), "file.md")); err == nil {
		t.Fatal("outside relative path accepted")
	}
}
