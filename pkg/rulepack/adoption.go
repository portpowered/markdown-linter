package rulepack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type baseline struct {
	Version  int      `json:"version"`
	Findings []string `json:"findings"`
}

func fingerprint(root string, d interfaces.Diagnostic) (string, error) {
	relative, e := relativePath(root, d.Path)
	if e != nil {
		return "", e
	}
	source, e := os.ReadFile(d.Path)
	if e != nil {
		return "", e
	}
	lines := strings.Split(string(source), "\n")
	evidence := ""
	if d.Line > 0 && d.Line <= len(lines) {
		evidence = strings.TrimSpace(lines[d.Line-1])
	}
	value := relative + "\x00" + d.RuleID + "\x00" + d.CheckID + "\x00" + d.Message + "\x00" + evidence
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:]), nil
}

// ApplyBaseline consumes counted fingerprints; new copies of existing findings
// are new debt. Source and execution failures cannot be added to this baseline.
func ApplyBaseline(path, root string, findings []interfaces.Diagnostic) ([]interfaces.Diagnostic, int, error) {
	root, e := filepath.Abs(root)
	if e != nil {
		return nil, 0, e
	}
	data, e := os.ReadFile(path)
	if e != nil {
		return nil, 0, e
	}
	var b baseline
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(&b); e != nil {
		return nil, 0, e
	}
	if trailing := decoder.Decode(new(any)); trailing != io.EOF {
		return nil, 0, fmt.Errorf("baseline must contain one JSON document")
	}
	if b.Version != 1 {
		return nil, 0, fmt.Errorf("unsupported baseline version %d", b.Version)
	}
	counts := map[string]int{}
	for _, f := range b.Findings {
		decoded, decodeErr := hex.DecodeString(f)
		if decodeErr != nil || len(decoded) != 32 {
			return nil, 0, fmt.Errorf("invalid baseline fingerprint")
		}
		counts[f]++
	}
	out := []interfaces.Diagnostic{}
	known := 0
	for _, d := range findings {
		key, e := fingerprint(root, d)
		if e != nil {
			return nil, 0, e
		}
		if counts[key] > 0 {
			known++
			counts[key]--
		} else {
			out = append(out, d)
		}
	}
	return out, known, nil
}
func WriteBaseline(path, root string, findings []interfaces.Diagnostic, overwrite bool) error {
	root, e := filepath.Abs(root)
	if e != nil {
		return e
	}
	b := baseline{Version: 1, Findings: []string{}}
	for _, d := range findings {
		key, e := fingerprint(root, d)
		if e != nil {
			return e
		}
		b.Findings = append(b.Findings, key)
	}
	sort.Strings(b.Findings)
	data, e := json.MarshalIndent(b, "", "  ")
	if e != nil {
		return e
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	file, e := os.OpenFile(path, flags, 0644)
	if e != nil {
		return e
	}
	_, e = file.Write(append(data, '\n'))
	closeErr := file.Close()
	if e != nil {
		return e
	}
	return closeErr
}
func BaselineArgs(args []string) ([]string, error) {
	if len(args) == 0 || args[0] != "baseline" {
		return args, nil
	}
	if len(args) < 2 || args[1] != "create" {
		return nil, fmt.Errorf("use baseline create --output <file> <lint options and inputs>")
	}
	rest := append([]string(nil), args[2:]...)
	found := false
	for i, arg := range rest {
		if arg == "--output" {
			rest[i] = "--baseline-write"
			found = true
		}
		if arg == "--overwrite" {
			rest[i] = "--baseline-overwrite"
		}
	}
	if !found {
		rest = append([]string{"--baseline-write", ".lint-baseline.json"}, rest...)
	}
	return rest, nil
}

// SARIF uses the same findings and source locations as ordinary output.
func SARIF(findings []interfaces.Diagnostic) any {
	results := []any{}
	for _, d := range findings {
		level := string(d.Severity)
		if level == "info" {
			level = "note"
		}
		region := map[string]any{}
		if d.Line > 0 {
			region["startLine"] = d.Line
		}
		results = append(results, map[string]any{"ruleId": d.RuleID, "level": level, "message": map[string]string{"text": d.Message}, "locations": []any{map[string]any{"physicalLocation": map[string]any{"artifactLocation": map[string]string{"uri": artifactURI(d.Path)}, "region": region}}}})
	}
	return map[string]any{"version": "2.1.0", "$schema": "https://json.schemastore.org/sarif-2.1.0.json", "runs": []any{map[string]any{"tool": map[string]any{"driver": map[string]string{"name": CommandName()}}, "results": results}}}
}

func artifactURI(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	slash := filepath.ToSlash(absolute)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	return (&url.URL{Scheme: "file", Path: slash}).String()
}
