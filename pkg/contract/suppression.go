package contract

import "strings"

// Inline exceptions apply only to an entire comment line outside code.
func inlineSuppressed(d *Document, id string, line int) bool {
	if line < 2 {
		return false
	}
	lines := strings.Split(string(d.Source), "\n")
	if line-2 >= len(lines) {
		return false
	}
	comment := strings.TrimSpace(lines[line-2])
	const prefix = "<!-- marklint-disable-next-line "
	if !strings.HasPrefix(comment, prefix) || !strings.HasSuffix(comment, "-->") {
		return false
	}
	declaration := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(comment, prefix), "-->"))
	rule, reason, found := strings.Cut(declaration, " reason:")
	if !found || strings.TrimSpace(rule) != id || strings.TrimSpace(reason) == "" {
		return false
	}
	offset := 0
	for _, text := range lines[:line-2] {
		offset += len(text) + 1
	}
	for _, target := range d.Targets {
		if target.Kind == "codeblock" && offset >= target.Start && offset < target.End {
			return false
		}
	}
	return true
}
