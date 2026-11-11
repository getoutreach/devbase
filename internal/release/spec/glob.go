// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Release rule globs: the subset of micromatch syntax that
// path.Match reads the same way, and micromatch's matching rules.

package spec

import (
	"fmt"
	"path"
	"strings"
)

// errEmptyGlob is returned for an empty rule glob, which makes micromatch
// throw.
var errEmptyGlob = fmt.Errorf("%w: empty pattern", ErrUnsupported)

// errUnsupportedGlob is returned for glob syntax outside the allowlist of
// ValidateGlob.
var errUnsupportedGlob = fmt.Errorf("%w: unsupported glob syntax", ErrUnsupported)

// ValidateGlob checks that a rule glob uses only syntax that path.Match
// and micromatch read the same way: literal characters, "*", "?", a
// "[...]" or "[^...]" class of letters, digits, ranges, "_" and ".", and
// "\" before punctuation. Everything else fails closed, because
// micromatch has more syntax (negation, braces, extglobs, globstars,
// POSIX classes) that path.Match reads in another way.
func ValidateGlob(p string) error {
	if p == "" {
		return errEmptyGlob
	}
	if strings.HasPrefix(p, "./") {
		return errUnsupportedGlob // micromatch removes a leading "./".
	}
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c == '*' && strings.HasPrefix(p[i+1:], "*"):
			return errUnsupportedGlob // a globstar
		case c == '*' || c == '?':
		case c == '\\':
			i++
			if i == len(p) || isGlobAlnum(p[i]) || p[i] < '!' || p[i] > '~' {
				return errUnsupportedGlob
			}
		case c == '[':
			end := globClassEnd(p, i)
			if end < 0 {
				return errUnsupportedGlob
			}
			i = end
		case isGlobAlnum(c) || c >= 0x80 || strings.IndexByte(" -_.,:;/#'%&=~$^<>+@", c) >= 0:
		default:
			return errUnsupportedGlob
		}
	}
	return nil
}

// MatchGlob reports whether value matches a rule glob as micromatch.isMatch
// does, for the patterns that ValidateGlob accepts. An empty pattern means
// that the rule has no such key, and it matches anything. micromatch
// never matches an empty or missing value, and a path segment pattern
// that starts with "*" or "?" does not match a segment that starts with
// a dot.
func MatchGlob(pattern, value string) bool {
	if pattern == "" {
		return true
	}
	if value == "" {
		return false
	}
	if ok, err := path.Match(pattern, value); err != nil || !ok {
		return false
	}
	// path.Match matched, so both have the same number of segments.
	for value != "" {
		var seg, p string
		seg, value, _ = strings.Cut(value, "/")
		p, pattern, _ = strings.Cut(pattern, "/")
		if strings.HasPrefix(seg, ".") && (strings.HasPrefix(p, "*") || strings.HasPrefix(p, "?")) {
			return false
		}
	}
	return true
}

// globClassEnd returns the index of the "]" that closes the class that
// starts at p[start], or -1 if the class is not in the allowlist of
// ValidateGlob.
func globClassEnd(p string, start int) int {
	i := start + 1
	if i < len(p) && p[i] == '^' {
		i++
	}
	first := i
	for ; i < len(p) && p[i] != ']'; i++ {
		c := p[i]
		switch {
		case isGlobAlnum(c) && i+2 < len(p) && p[i+1] == '-' && p[i+2] != ']':
			if !isGlobAlnum(p[i+2]) || p[i+2] < c {
				return -1
			}
			i += 2
		case isGlobAlnum(c) || c == '_' || c == '.':
		default:
			return -1
		}
	}
	if i == len(p) || i == first {
		return -1
	}
	return i
}

// isGlobAlnum reports whether c is an ASCII letter or digit.
func isGlobAlnum(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
