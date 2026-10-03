package engine

import (
	"testing"
)

func TestDestinationScannerMalformedAndEscapedInputs(t *testing.T) {
	for _, raw := range []string{`[unclosed`, `[closed]`, `[x]()`, `[x](<unclosed)`, `[x](target`, `\[escaped](target)`} {
		if got := scanInlineDestinationRanges([]byte(raw), 0, 1); len(got) != 0 {
			t.Fatalf("%q: %+v", raw, got)
		}
	}
	raw := []byte(`[x]( <folder name.md> "title")`)
	got := scanInlineDestinationRanges(raw, 10, 2)
	if len(got) != 1 || got[0].destination != "folder name.md" || got[0].destinationRange.StartOffset != 16 {
		t.Fatalf("%+v", got)
	}
	ranges := inlineDestinationRanges{ranges: got}
	if match := ranges.next(false, 2, "folder name.md"); match.destinationRange.StartOffset < 0 {
		t.Fatal(match)
	}
	if match := ranges.next(false, 2, "folder name.md"); match.destinationRange.StartOffset != -1 {
		t.Fatalf("reused %+v", match)
	}
	// Malformed reference definitions and usages must not capture surrounding prose.
	refs := map[string]referenceDestinationRange{}
	for _, raw := range []string{"[unclosed", "[x]", "[ ]: target", "[x]:", "[x]: <unclosed"} {
		scanReferenceDefinitionRange([]byte(raw), 0, 1, refs)
	}
	if len(refs) != 0 {
		t.Fatal(refs)
	}
	scanReferenceDefinitionRange([]byte(" [x]: <target.md>"), 0, 1, refs)
	scanReferenceDefinitionRange([]byte("[x]: other.md"), 0, 2, refs)
	if len(refs) != 1 || refs["x"].destination != "target.md" {
		t.Fatal(refs)
	}
	for _, raw := range []string{"[unclosed", "[x]", "[x][unclosed", "[x][absent]", `\[x][x]`} {
		if got := scanReferenceUsageDestinationRanges([]byte(raw), 0, 1, refs); len(got) != 0 {
			t.Fatalf("%q: %+v", raw, got)
		}
	}
	if got := scanReferenceUsageDestinationRanges([]byte("[x][] ![x][x]"), 0, 1, refs); len(got) != 2 || got[0].image || !got[1].image {
		t.Fatalf("%+v", got)
	}
	if got := scanReferenceDefinitionRanges([]byte("```\n[x]: code.md\n```\n[y]: prose.md")); len(got) != 1 || got["y"].destination != "prose.md" {
		t.Fatalf("%+v", got)
	}
	for _, raw := range []string{"  ```", "\t~~~", "~~~"} {
		if !isFenceLine([]byte(raw)) {
			t.Fatalf("fence %q", raw)
		}
	}
	if isFenceLine([]byte("`~`")) {
		t.Fatal("mixed fence accepted")
	}
	if got := bytesIndexByteUnescaped([]byte(`\]literal]`), ']'); got != 9 {
		t.Fatalf("escaped bracket=%d", got)
	}
}
