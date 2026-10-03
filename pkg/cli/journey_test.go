package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestPortosOnboardingBaselineAndSARIF(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, ".marklint.yaml")
	file := filepath.Join(root, "guide.md")
	baseline := filepath.Join(root, "baseline.json")
	code, _, stderr := command("init", "--preset", "portos-defaults", "--output", config)
	if code != 0 {
		t.Fatal(stderr)
	}
	write(t, file, "# Guide\n\nA dash - in prose. `safe-code` [link](https://site.test/a-b)\n")
	code, out, stderr := command("--root", root, file)
	if code != 1 || !strings.Contains(out, "text.no-dashes") {
		t.Fatalf("discovery: %d %s %s", code, out, stderr)
	}
	code, _, stderr = command("baseline", "create", "--output", baseline, "--root", root, file)
	if code != 0 {
		t.Fatal(stderr)
	}
	code, out, stderr = command("--baseline", baseline, "--root", root, "--format", "json", file)
	if code != 0 || !strings.Contains(stderr, "1 known") {
		t.Fatalf("baseline: %d %s %s", code, out, stderr)
	}
	write(t, file, "# Guide\n\nA dash - in prose. `safe-code` [link](https://site.test/a-b)\n\nNew - debt.\n")
	code, out, stderr = command("--baseline", baseline, "--root", root, "--format", "sarif", file)
	var sarif map[string]any
	if code != 1 || json.Unmarshal([]byte(out), &sarif) != nil || sarif["version"] != "2.1.0" {
		t.Fatalf("SARIF: %d %s %s", code, out, stderr)
	}
	code, out, stderr = command("config", "explain", "--rules", config, "--root", root, "--path", "guide.md")
	if code != 0 || !strings.Contains(out, "text.no-dashes") || !strings.Contains(out, "preset:portos:internal") {
		t.Fatalf("explain: %d %s %s", code, out, stderr)
	}
}
