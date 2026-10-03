package cli_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSTEOnboardingAndDictionaryFailures(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, ".marklint.yaml")
	file := filepath.Join(root, "guide.md")
	code, _, stderr := command("init", "--preset", "text:ste100", "--output", config)
	if code != 0 {
		t.Fatal(stderr)
	}
	write(t, file, "Use the API.\n")
	code, _, stderr = command("--root", root, file)
	if code != 2 || !strings.Contains(stderr, "STE100 dictionary") {
		t.Fatalf("missing: %d %s", code, stderr)
	}
	write(t, filepath.Join(root, "ste100-dictionary.yaml"), "words: [use, the, api]\nalternatives: {utilize: [use]}\n")
	code, out, stderr := command("--root", root, file)
	if code != 0 {
		t.Fatalf("valid: %d %s %s", code, out, stderr)
	}
	write(t, file, "Utilize the API.\n")
	code, out, stderr = command("--root", root, "--format", "json", file)
	if code != 1 || !strings.Contains(out, "text.ste100.dictionary") || !strings.Contains(out, "consider: use") {
		t.Fatalf("unknown: %d %s %s", code, out, stderr)
	}
	write(t, filepath.Join(root, "ste100-dictionary.yaml"), "unknown: [use]\n")
	code, _, stderr = command("--root", root, file)
	if code != 2 {
		t.Fatalf("malformed: %d %s", code, stderr)
	}
}
