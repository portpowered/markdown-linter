package contract

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/text/cases"
)

func TestComparisonTextMatchesSourceMappedProjection(t *testing.T) {
	sources := []string{"", " Safe **text** ", "\r\n\tA\u2003B\u00a0", "cafe\u0301 Straße İΣς", "😀 \xff\xfe\u0345"}
	random := rand.New(rand.NewSource(42))
	alphabet := []rune("Ab \r\n\t caféßİΣς😀\u0301\u2003\u0345\u212a")
	for range 1000 {
		var text strings.Builder
		for range random.Intn(80) {
			text.WriteRune(alphabet[random.Intn(len(alphabet))])
		}
		sources = append(sources, text.String())
	}
	for _, source := range sources {
		for _, normalize := range []bool{false, true} {
			for _, fold := range []bool{false, true} {
				want, _, _ := comparisonUnit(Unit{Text: source}, normalize, fold)
				if got := comparisonText(source, normalize, fold); got != want {
					t.Fatalf("projection differs for %q (%v, %v): %q != %q", source, normalize, fold, got, want)
				}
			}
		}
	}
}

func TestFoldedPreservesUnicodeCaseFolding(t *testing.T) {
	var ascii strings.Builder
	for value := 0; value < 128; value++ {
		ascii.WriteByte(byte(value))
	}
	for _, source := range []string{ascii.String(), "", "UPPER lower 123!", "Straße", "İstanbul", "Σσς", "cafe\u0301", "😀 ABC é", "\xffABC"} {
		if got, want := folded(source), cases.Fold().String(source); got != want {
			t.Fatalf("folding %q: %q != %q", source, got, want)
		}
	}
}

func TestPreparedWordsPreserveSourceOrderOffsetsAndPolicyValues(t *testing.T) {
	p := one(t, "core.match", `{mode: word, expect: absent, case: fold, values: [STRASSE, CAFÉ]}`, "document")
	values := []string{"STRASSE", " CAFÉ ", "STRASSE"}
	p.Rules["test"].Options["values"] = values
	for _, test := range []struct {
		source     string
		start, end int
	}{
		{"safe cafe\u0301 then Straße", 5, 11},
		{"safe Straße then café", 5, 12},
	} {
		report := lint(t, p, test.source)
		if len(report.Diagnostics) != 1 {
			t.Fatalf("diagnostics: %+v", report.Diagnostics)
		}
		location := report.Diagnostics[0].Location
		if location.StartOffset != test.start || location.EndOffset != test.end {
			t.Fatalf("source evidence for %q: %+v", test.source, location)
		}
	}
	if !reflect.DeepEqual(values, []string{"STRASSE", " CAFÉ ", "STRASSE"}) {
		t.Fatal("match preparation modified the caller's policy values")
	}
}
