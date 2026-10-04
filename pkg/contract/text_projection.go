package contract

import (
	"html"
	"strings"
	"unicode/utf8"
)

func projectText(raw string, start int) Unit {
	u := Unit{Start: start, End: start + len(raw)}
	var b strings.Builder
	appendPart := func(text string, a, z int) {
		b.WriteString(text)
		for range []byte(text) {
			u.Offsets = append(u.Offsets, a)
			u.EndOffsets = append(u.EndOffsets, z)
		}
	}
	for i := 0; i < len(raw); {
		if raw[i] == '&' {
			if end := strings.IndexByte(raw[i:], ';'); end >= 0 && end < 40 {
				encoded := raw[i : i+end+1]
				decoded := html.UnescapeString(encoded)
				if decoded != encoded {
					appendPart(decoded, start+i, start+i+end+1)
					i += end + 1
					continue
				}
			}
		}
		if raw[i] == '\\' && i+1 < len(raw) && strings.ContainsRune(`!"#$%&'()*+,-./:;<=>?@[\]^_`+"`"+`{|}~`, rune(raw[i+1])) {
			appendPart(raw[i+1:i+2], start+i, start+i+2)
			i += 2
			continue
		}
		_, n := utf8.DecodeRuneInString(raw[i:])
		part := raw[i : i+n]
		b.WriteString(part)
		for j := 0; j < n; j++ {
			u.Offsets = append(u.Offsets, start+i+j)
			u.EndOffsets = append(u.EndOffsets, start+i+n)
		}
		i += n
	}
	u.Text = b.String()
	return u
}
