// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Helpers that give Go regular expressions and string scans
// the character rules of JavaScript regular expressions without the "u"
// flag, which conventional-commits-parser and its presets use.

package release

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/getoutreach/devbase/v2/internal/release/internal/js"
)

// These are pattern fragments for the JavaScript character classes that
// differ from the Go ones.
const (
	// jsLineTerminators are the JavaScript line terminators, for use
	// inside a character class.
	jsLineTerminators = `\n\r\x{2028}\x{2029}`

	// jsSpaceChars are the characters of JavaScript's \s, for use inside
	// a character class: the line terminators, the ASCII spaces, U+FEFF
	// and the Zs category. Go's \s has only the ASCII ones.
	jsSpaceChars = jsLineTerminators + `\t\v\f\x{feff}\p{Zs}`

	// jsSpace matches one character of JavaScript's \s.
	jsSpace = `[` + jsSpaceChars + `]`

	// jsDot matches one character of JavaScript's ".", which does not
	// match a line terminator. Go's "." only leaves out "\n".
	jsDot = `[^` + jsLineTerminators + `]`
)

// foldASCII returns a pattern that matches s with the ASCII letters in
// either case, as the JavaScript "i" flag does. Go's (?i) also folds
// "ſ" to "s" and the Kelvin sign to "k", which JavaScript does not.
func foldASCII(s string) string {
	var b strings.Builder
	for _, r := range s {
		// r|0x20 is the lowercase form of an ASCII letter, and no other
		// rune maps into a-z.
		if l := r | 0x20; l >= 'a' && l <= 'z' {
			b.WriteString("[" + string(l) + string(l&^0x20) + "]")
		} else {
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	return b.String()
}

// isLineTerminator reports whether r is one of jsLineTerminators.
func isLineTerminator(r rune) bool {
	return r == '\n' || r == '\r' || r == 0x2028 || r == 0x2029
}

// spaceAt returns the length of the JavaScript whitespace character at
// s[i:], or 0.
func spaceAt(s string, i int) int {
	// At the end of s, DecodeRuneInString returns RuneError, which is
	// not a space.
	r, n := utf8.DecodeRuneInString(s[i:])
	if js.IsSpace(r) {
		return n
	}
	return 0
}

// skipSpace returns the index after the JavaScript whitespace at s[i:].
func skipSpace(s string, i int) int {
	return len(s) - len(strings.TrimLeftFunc(s[i:], js.IsSpace))
}
