package service

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// slugify ports the archive's Python slugify: NFKD, drop combining marks,
// lower-case, runs of [^a-z0-9] become "-", cut to 60 runes.
func slugify(title string) string {
	var b strings.Builder
	pendingDash := false
	for _, r := range norm.NFKD.String(title) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		r = unicode.ToLower(r)
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if pendingDash && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingDash = false
			b.WriteRune(r)
		} else {
			pendingDash = true
		}
	}
	s := []rune(b.String())
	if len(s) > 60 {
		s = s[:60]
	}
	out := strings.Trim(string(s), "-")
	if out == "" {
		return "meeting"
	}
	return out
}
