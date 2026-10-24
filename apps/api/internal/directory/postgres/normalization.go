package postgres

import (
	"strings"
	"unicode"
)

// Derive SQL's one-rune translation from the same Go Unicode tables used by
// directory.Normalize. PostgreSQL libc/locale versions must not change matching.
// Whitespace becomes ASCII space before SQL collapses/trims literal spaces.
var normalizationFrom, normalizationTo = normalizationMap()

func normalizationMap() (string, string) {
	var from, to strings.Builder
	for r := rune(0); r <= unicode.MaxRune; r++ {
		lower := unicode.ToLower(r)
		switch {
		case unicode.IsSpace(r):
			from.WriteRune(r)
			to.WriteRune(' ')
		case lower != r:
			from.WriteRune(r)
			to.WriteRune(lower)
		}
	}
	return from.String(), to.String()
}
