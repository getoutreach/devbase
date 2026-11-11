// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The close actions of issue-parser 7 with its "github"
// configuration, which success.js uses to find the issues that a commit
// message or a pull request body closes.

package github

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// closeKeywords are the "close" action keywords of the "github"
// configuration of issue-parser (lib/hosts-config.js).
//
//nolint:gochecknoglobals // Why: read-only list.
var closeKeywords = []string{"close", "closes", "closed", "fix", "fixes", "fixed", "resolve", "resolves", "resolved"}

// issueRefRegexp is the regular expression of issue-parser for the
// "github" configuration, with its "gim" flags. The actions are the close
// keywords and "Duplicate of"; the delimiter is ":"; the host is
// "https://github.com/"; the issue prefixes are "#", "gh-", "/issues/"
// and "/pull/"; the mention prefix is "@".
//
// The JavaScript expression ends a reference with (?!\w) after \d+,
// which is the same as \b there. The "i" flag folds only ASCII letters
// without the "u" flag, so the keywords are folded by hand, and the
// expression has no (?i). The "m" flag makes ^ match after "\n", "\r",
// U+2028 and U+2029; scanText turns "\r" into "\n" so that Go's (?m)^
// sees the same line starts, and a match after U+2028 or U+2029 can
// start one character earlier, where [^\w\n\v\r] matches them.
var issueRefRegexp = regexp.MustCompile(func() string {
	actions := append(append([]string{}, closeKeywords...), "Duplicate of")
	for i, a := range actions {
		actions[i] = foldASCII(a)
	}
	prefixes := []string{"#", "gh-", "/issues/", "/pull/"}
	for i, p := range prefixes {
		prefixes[i] = foldASCII(p)
	}
	ref := `(?:(?:[^\w\n\v\r]|^)+(` + strings.Join(actions, "|") + `))?` +
		`(?:[^\w\n\v\r]|^|(?: |\t)*(?: |\t|:)(?: |\t)*)` +
		`(?:` + foldASCII("https://github.com/") + `)?` +
		`((?:(?:[\w\-.]+)/)+(?:[\w\-.]+))?` +
		`(` + strings.Join(prefixes, "|") + `)` +
		`(\d+)\b`
	mention := `((?:(?:[^\w\n\v\r]|^)+(?:@)[\w\-.]+[^\W])+)`
	return `(?m)(?:` + ref + `|` + mention + `)`
}())

// htmlCommentRegexp matches an HTML comment, as HTML_COMMENT_REGEXP.
var htmlCommentRegexp = regexp.MustCompile(`<!--[\s\S]*?-->`)

// closeAction is an issue that a text closes with a close keyword.
type closeAction struct {
	// slug is the "owner/repo" before the issue number, or empty.
	slug string

	// issue is the issue number.
	issue string
}

// parseCloseActions returns the close actions of text, as
// issueParser("github")(text).actions.close does. As issue-parser, it
// first removes fenced code blocks, inline code, HTML code elements and
// HTML comments.
func parseCloseActions(text string) []closeAction {
	text = removeFencedCode(text)
	text = reverseString(removeInlineCode(reverseString(text)))
	// Upstream removes the <code> elements while the expression matches
	// from its lastIndex, which is once when it matches at all.
	if issueRefRegexp.MatchString(scanText(text)) {
		text = removeHTMLCode(text)
	}
	text = htmlCommentRegexp.ReplaceAllString(text, " ")

	var actions []closeAction
	for _, m := range issueRefRegexp.FindAllStringSubmatch(scanText(text), -1) {
		if m[1] == "" || !isCloseKeyword(m[1]) {
			continue
		}
		actions = append(actions, closeAction{slug: m[2], issue: m[4]})
	}
	return actions
}

// isCloseKeyword reports whether s is a close keyword in any case.
func isCloseKeyword(s string) bool {
	for _, k := range closeKeywords {
		if strings.EqualFold(s, k) {
			return true
		}
	}
	return false
}

// foldASCII returns a pattern that matches s with its ASCII letters in
// either case, as the JavaScript "i" flag without "u" does.
func foldASCII(s string) string {
	var b strings.Builder
	for _, r := range s {
		if l := r | 0x20; l >= 'a' && l <= 'z' {
			b.WriteString("[" + string(l) + string(l&^0x20) + "]")
		} else {
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	return b.String()
}

// scanText returns text with "\r" turned into "\n", which keeps the byte
// offsets and gives Go's (?m)^ the JavaScript line starts after "\r".
func scanText(text string) string {
	return strings.ReplaceAll(text, "\r", "\n")
}

// isLineStart reports whether ^ with the "m" flag matches at byte i of s:
// at the start or after a JavaScript line terminator.
func isLineStart(s string, i int) bool {
	if i == 0 {
		return true
	}
	if c := s[i-1]; c == '\n' || c == '\r' {
		return true
	}
	return i >= 3 && (s[i-3:i] == " " || s[i-3:i] == " ")
}

// removeFencedCode removes the fenced code blocks of text, as
// text.replace(FENCE_BLOCK_REGEXP, "") does with
// /^(([ \t]*`{3,4})([^\n]*)([\s\S]+?)(^[ \t]*\2))/gm.
func removeFencedCode(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		if isLineStart(text, i) {
			if end := matchFence(text, i); end >= 0 {
				i = end
				continue
			}
		}
		b.WriteByte(text[i])
		i++
	}
	return b.String()
}

// matchFence returns the end of a fenced code block at line start i, or
// -1. It follows the backtracking order of FENCE_BLOCK_REGEXP: four
// backticks before three; the longest rest of the opening line, so the
// first closing fence after the line; else the closing fence that is
// last inside the line, after a "\r".
func matchFence(s string, i int) int {
	j := i
	for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
		j++
	}
	n := 0
	for j+n < len(s) && s[j+n] == '`' {
		n++
	}
	for t := min(n, 4); t >= 3; t-- {
		fence := s[i : j+t]
		start := j + t
		lineEnd := strings.IndexByte(s[start:], '\n')
		if lineEnd < 0 {
			lineEnd = len(s)
		} else {
			lineEnd += start
		}
		for q := lineEnd + 1; q <= len(s); q++ {
			if end := closingFence(s, q, fence); end >= 0 {
				return end
			}
		}
		for q := lineEnd; q > start; q-- {
			if end := closingFence(s, q, fence); end >= 0 {
				return end
			}
		}
	}
	return -1
}

// closingFence returns the end of ^[ \t]*fence at q, or -1.
func closingFence(s string, q int, fence string) int {
	if q > len(s) || !isLineStart(s, q) {
		return -1
	}
	ws := q
	for ws < len(s) && (s[ws] == ' ' || s[ws] == '\t') {
		ws++
	}
	for r := ws; r >= q; r-- {
		if strings.HasPrefix(s[r:], fence) {
			return r + len(fence)
		}
	}
	return -1
}

// isLineTerminator reports whether r is a JavaScript line terminator,
// which "." does not match.
func isLineTerminator(r rune) bool {
	return r == '\n' || r == '\r' || r == ' ' || r == ' '
}

// removeInlineCode removes inline code from the reversed text s, as
// s.replace(CODE_BLOCK_REGEXP, "") does with
// /(`(?!\\))((?:.(?!\1(?!\\)))*.?)\1/g. In the reversed text, a backtick
// that a backslash follows is escaped.
func removeInlineCode(s string) string {
	r := []rune(s)
	n := len(r)
	tick := func(k int) bool { return k < n && r[k] == '`' }
	unescapedTick := func(k int) bool { return tick(k) && (k+1 >= n || r[k+1] != '\\') }
	var out []rune
	for i := 0; i < n; {
		if !unescapedTick(i) {
			out = append(out, r[i])
			i++
			continue
		}
		// The loop takes each character that no unescaped backtick
		// follows.
		p := i + 1
		for p < n && !isLineTerminator(r[p]) && !unescapedTick(p+1) {
			p++
		}
		end := -1
		if p < n && !isLineTerminator(r[p]) {
			// .? takes r[p] and \1 matches the backtick after it.
			end = p + 2
		} else {
			// Backtracking finds the last backtick that the loop took.
			for k := p - 1; k > i; k-- {
				if r[k] == '`' {
					end = k + 1
					break
				}
			}
		}
		if end < 0 {
			out = append(out, r[i])
			i++
			continue
		}
		i = end
	}
	return string(out)
}

// removeHTMLCode removes the HTML code elements of s, as
// s.replace(HTML_CODE_BLOCK_REGEXP, "") does with
// /(<code)+?((?!(<code|<\/code>)+?)[\S\s])*(<\/code>)+?/gim: from "<code"
// to the next "</code>", when no other "<code" comes before it.
func removeHTMLCode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if end := matchHTMLCode(s, i); end >= 0 {
			i = end
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		b.WriteString(s[i : i+size])
		i += size
	}
	return b.String()
}

// matchHTMLCode returns the end of an HTML code element at i, or -1.
func matchHTMLCode(s string, i int) int {
	if !hasPrefixFold(s[i:], "<code") {
		return -1
	}
	p := i
	for hasPrefixFold(s[p:], "<code") {
		p += len("<code")
	}
	for p < len(s) && !hasPrefixFold(s[p:], "<code") && !hasPrefixFold(s[p:], "</code>") {
		p++
	}
	if !hasPrefixFold(s[p:], "</code>") {
		return -1
	}
	return p + len("</code>")
}

// hasPrefixFold reports whether s starts with the ASCII prefix with its
// ASCII letters in any case. Unlike strings.EqualFold, it does not fold
// "ſ" to "s", as JavaScript without the "u" flag does not.
func hasPrefixFold(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := range len(prefix) {
		a, b := s[i], prefix[i]
		if a >= 'A' && a <= 'Z' {
			a += 'a' - 'A'
		}
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}
		if a != b {
			return false
		}
	}
	return true
}

// reverseString reverses the runes of s.
func reverseString(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}
