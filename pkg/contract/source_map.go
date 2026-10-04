package contract

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// comparisonUnit carries both ends of a normalized scalar back to original bytes.
// Case expansion, NFC composition, whitespace collapse and formatting splits keep
// source evidence separate from the normalized comparison string.
func comparisonUnit(u Unit, normalize, fold bool) (string, []int, []int) {
	var b strings.Builder
	starts, ends := []int{}, []int{}
	appendPart := func(s string, a, z int) {
		if fold {
			s = folded(s)
		}
		b.WriteString(s)
		for range []byte(s) {
			starts = append(starts, a)
			ends = append(ends, z)
		}
	}
	bounds := func(a, z int) (int, int) {
		if len(u.Offsets) == len(u.Text) && z > a {
			end := u.Offsets[z-1] + 1
			if len(u.EndOffsets) == len(u.Text) {
				end = u.EndOffsets[z-1]
			}
			return u.Offsets[a], end
		}
		return u.Start + a, u.Start + z
	}
	if !normalize {
		for i := 0; i < len(u.Text); {
			_, n := utf8.DecodeRuneInString(u.Text[i:])
			a, z := bounds(i, i+n)
			appendPart(u.Text[i:i+n], a, z)
			i += n
		}
		return b.String(), starts, ends
	}
	var it norm.Iter
	it.InitString(norm.NFC, u.Text)
	pos := 0
	pendingSpace := false
	spaceA, spaceZ := 0, 0
	for !it.Done() {
		part := string(it.Next())
		next := it.Pos()
		a, z := bounds(pos, next)
		pos = next
		for _, r := range part {
			if unicode.IsSpace(r) {
				if b.Len() > 0 {
					if !pendingSpace {
						spaceA = a
					}
					spaceZ = z
					pendingSpace = true
				}
				continue
			}
			if pendingSpace {
				appendPart(" ", spaceA, spaceZ)
				pendingSpace = false
			}
			appendPart(string(r), a, z)
		}
	}
	return b.String(), starts, ends
}
