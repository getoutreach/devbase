// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: JavaScript string rules that the ports of the Node.js
// release tools need.

// Package js has the JavaScript string rules that the release command
// ports need, where Go's rules differ: whitespace, trim and URI
// encoding.
package js

import (
	"strings"
	"unicode"
)

// These are the bytes that the JavaScript URI functions keep, besides
// the ASCII letters and digits.
const (
	// URIComponentKeep is what encodeURIComponent keeps.
	URIComponentKeep = "-_.!~*'()"

	// URIKeep is what encodeURI keeps.
	URIKeep = URIComponentKeep + ";,/?:@&=+$#"

	// URIAuthKeep is what Node's url.format keeps in the credentials of
	// a URL: what encodeURIComponent keeps, and ":".
	URIAuthKeep = URIComponentKeep + ":"
)

// IsSpace reports whether r matches JavaScript's \s: unicode.IsSpace has
// U+0085, which JavaScript does not, and does not have U+FEFF, which
// JavaScript does.
func IsSpace(r rune) bool {
	return r == 0xfeff || (r != 0x85 && unicode.IsSpace(r))
}

// Trim removes leading and trailing JavaScript whitespace, as
// String.prototype.trim does.
func Trim(s string) string {
	return strings.TrimFunc(s, IsSpace)
}

// ASCIILower lowercases the ASCII letters of s and keeps its length, as
// the "i" flag of a JavaScript regular expression without the "u" flag
// folds case: no other letter folds to an ASCII one.
func ASCIILower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// EncodeURIComponent percent-encodes s as encodeURIComponent does.
func EncodeURIComponent(s string) string {
	return PercentEncode(s, URIComponentKeep)
}

// EncodeURI percent-encodes s as encodeURI does.
func EncodeURI(s string) string {
	return PercentEncode(s, URIKeep)
}

// PercentEncode percent-encodes the UTF-8 bytes of s, except ASCII
// letters, digits and the bytes in keep.
func PercentEncode(s, keep string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := range len(s) {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte(keep, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}
