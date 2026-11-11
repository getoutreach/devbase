// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Parses conventional commits as conventional-commits-parser
// does with the conventionalcommits preset, and decides the release type
// as @semantic-release/commit-analyzer does.

package release

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/getoutreach/devbase/v2/internal/release/releaserc"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// These patterns come from conventional-commits-parser and
// conventional-changelog-conventionalcommits. They use the jsregexp.go
// fragments where Go's \s, "." or (?i) differ from JavaScript.
var (
	// headerRegex matches a commit header.
	headerRegex = regexp.MustCompile(`^(\w*)(?:\((` + jsDot + `*)\))?!?: (` + jsDot + `*)$`)

	// breakingHeaderRegex matches a commit header with a "!".
	breakingHeaderRegex = regexp.MustCompile(`^(\w*)(?:\((` + jsDot + `*)\))?!: (` + jsDot + `*)$`)

	// noteRegex matches a breaking change note line.
	noteRegex = regexp.MustCompile(`^[` + jsSpaceChars + `|*]*(` + foldASCII("BREAKING CHANGE") + `|` +
		foldASCII("BREAKING-CHANGE") + `)[:` + jsSpaceChars + `]+(` + jsDot + `*)`)

	// fieldRegex matches a "-field-" meta line.
	fieldRegex = regexp.MustCompile(`^-(` + jsDot + `*?)-$`)

	// revertRegex matches a revert commit and captures the reverted
	// header and hash.
	revertRegex = regexp.MustCompile(`^(?:` + foldASCII("Revert") + `|` + foldASCII("revert:") + `)` + jsSpace +
		`"?([\s\S]+?)"?` + jsSpace + `*` + foldASCII("This reverts commit ") + `(\w*)\.`)

	// gpgRegex matches a GPG signature line, which the parser drops.
	gpgRegex = regexp.MustCompile(`^` + jsSpace + `*gpg:`)

	// urlRegex matches a URL; a sentence with a URL has no references.
	urlRegex = regexp.MustCompile(`\b(?:https?)://(?:www\.)?([-a-zA-Z0-9@:%_+.~#?&/=])+\b`)

	// skipReleaseRegex matches a commit that opts out of the release
	// analysis, as semantic-release's /\[skip\s+release]|\[release\s+skip]/i.
	skipReleaseRegex = regexp.MustCompile(`\[` + foldASCII("skip") + jsSpace + `+` + foldASCII("release") + `]|\[` +
		foldASCII("release") + jsSpace + `+` + foldASCII("skip") + `]`)
)

// referenceActions are the reference keywords that
// release-notes-generator gives the parser for github.com, in the order
// that the parser's alternation tries them. The analyzer does not use
// references. Another forge with other keywords needs this to become a
// parser option.
//
//nolint:gochecknoglobals // Why: read-only list.
var referenceActions = []string{"close", "closes", "closed", "fix", "fixes", "fixed", "resolve", "resolves", "resolved"}

// releaseFalse is the release value of a rule that stops a release.
const releaseFalse = "false"

// defaultRules are the commit-analyzer default rules that can match a
// conventionalcommits commit. The rules for the emoji, tag and component
// fields of other presets never match, so they are left out.
//
//nolint:gochecknoglobals // Why: read-only list.
var defaultRules = []releaserc.Rule{
	{Breaking: true, Release: string(BumpMajor)},
	{Revert: true, Release: string(BumpPatch)},
	{Type: "feat", Release: string(BumpMinor)},
	{Type: "fix", Release: string(BumpPatch)},
	{Type: "perf", Release: string(BumpPatch)},
	{Type: "FEAT", Release: string(BumpMinor)},
	{Type: "FIX", Release: string(BumpPatch)},
}

// Commit is a parsed conventional commit. Empty strings stand for the
// parser's null values.
type Commit struct {
	Hash    string
	Message string
	Header  string
	Type    string
	Scope   string
	Subject string
	Body    string
	Footer  string

	// Notes are the breaking change notes.
	Notes []Note

	// References are the issue references in the header, body and footer.
	References []Reference

	// Revert is set when the commit reverts another commit.
	Revert *Revert
}

// Note is a breaking change note.
type Note struct {
	Title string
	Text  string
}

// Reference is an issue reference such as "closes owner/repo#12".
type Reference struct {
	Action     string
	Owner      string
	Repository string
	Prefix     string
	Issue      string
	Raw        string
}

// Revert identifies the commit that a revert commit reverts.
type Revert struct {
	Header string
	Hash   string
}

// ParseCommit parses a commit message.
func ParseCommit(raw RawCommit) Commit {
	c := Commit{Hash: raw.Hash, Message: raw.Message}
	var lines []string
	for l := range strings.SplitSeq(trimNewLines(raw.Message), "\n") {
		l = strings.TrimSuffix(l, "\r")
		if !gpgRegex.MatchString(l) {
			lines = append(lines, l)
		}
	}
	p := &commitParser{lines: lines, c: &c}

	if p.available() {
		c.Header = p.next()
		m := breakingHeaderRegex.FindStringSubmatch(c.Header)
		if m == nil {
			m = headerRegex.FindStringSubmatch(c.Header)
		}
		if m != nil {
			c.Type, c.Scope, c.Subject = m[1], m[2], m[3]
		}
		c.References = parseReferences(c.Header)
	}

	isBody := true
	for p.available() {
		p.parseMeta()
		if p.parseNotes() {
			isBody = false
		}
		isBody = p.parseBodyAndFooter(isBody)
	}

	if len(c.Notes) == 0 {
		if m := breakingHeaderRegex.FindStringSubmatch(c.Header); m != nil {
			c.Notes = []Note{{Title: "BREAKING CHANGE", Text: m[3]}}
		}
	}
	if m := revertRegex.FindStringSubmatch(raw.Message); m != nil {
		c.Revert = &Revert{Header: m[1], Hash: m[2]}
	}
	c.cleanup()
	return c
}

// cleanup trims newlines and removes duplicate references, as the
// parser's cleanupCommit does.
func (c *Commit) cleanup() {
	c.Body, c.Footer = trimNewLines(c.Body), trimNewLines(c.Footer)
	for i := range c.Notes {
		c.Notes[i].Text = trimNewLines(c.Notes[i].Text)
	}
	// The parser uses toLocaleLowerCase, which keeps the dot of "İ". A
	// Caser is not safe for concurrent use, so each call makes its own.
	lower := cases.Lower(language.Und)
	seen := map[string]bool{}
	refs := c.References[:0]
	for _, r := range c.References {
		key := lower.String(r.Action + " " + r.Raw)
		if !seen[key] {
			seen[key] = true
			refs = append(refs, r)
		}
	}
	c.References = refs
}

// commitParser holds the state of one parse, as the JavaScript parser does.
type commitParser struct {
	lines []string
	i     int
	c     *Commit

	// metaHash is set once a "-hash-" meta line replaced the hash.
	metaHash bool
}

// available reports whether a line is left.
func (p *commitParser) available() bool {
	return p.i < len(p.lines)
}

// current returns the current line.
func (p *commitParser) current() string {
	return p.lines[p.i]
}

// next returns the current line and moves to the next one.
func (p *commitParser) next() string {
	l := p.current()
	p.i++
	return l
}

// parseMeta consumes "-field-" lines and the lines after them, which
// the parser appends to the commit field of that name.
func (p *commitParser) parseMeta() bool {
	field, parsed := "", false
	for p.available() {
		if m := fieldRegex.FindStringSubmatch(p.current()); m != nil {
			field = m[1]
			p.next()
			continue
		}
		if field == "" {
			break
		}
		parsed = true
		p.setField(field, p.next())
	}
	return parsed
}

// setField appends a meta line to a commit field. Only the string
// fields that the notes use are supported; other fields are ignored.
func (p *commitParser) setField(field, line string) {
	c := p.c
	var dst *string
	switch field {
	case "hash":
		// The parsed commit has no hash of its own, so the first meta
		// line replaces the git hash.
		if !p.metaHash {
			c.Hash, p.metaHash = "", true
		}
		dst = &c.Hash
	case "header":
		dst = &c.Header
	case "type":
		dst = &c.Type
	case "scope":
		dst = &c.Scope
	case "subject":
		dst = &c.Subject
	case "body":
		dst = &c.Body
	case "footer":
		dst = &c.Footer
	default:
		return
	}
	*dst = appendLine(*dst, line)
}

// parseNotes parses a breaking change note and the lines after it.
func (p *commitParser) parseNotes() bool {
	if !p.available() {
		return false
	}
	m := noteRegex.FindStringSubmatch(p.current())
	if m == nil {
		return false
	}
	n := len(p.c.Notes)
	p.c.Notes = append(p.c.Notes, Note{Title: m[1], Text: m[2]})
	p.c.Footer = appendLine(p.c.Footer, p.current())
	p.next()
	for p.available() {
		if p.parseMeta() || p.parseNotes() {
			return true
		}
		refs := parseReferences(p.current())
		if len(refs) > 0 {
			p.c.References = append(p.c.References, refs...)
		} else {
			p.c.Notes[n].Text = appendLine(p.c.Notes[n].Text, p.current())
		}
		p.c.Footer = appendLine(p.c.Footer, p.current())
		p.next()
		if len(refs) > 0 {
			break
		}
	}
	return true
}

// parseBodyAndFooter adds the current line to the body, or to the
// footer once a line has references.
func (p *commitParser) parseBodyAndFooter(isBody bool) bool {
	if !p.available() {
		return isBody
	}
	refs := parseReferences(p.current())
	stillBody := len(refs) == 0 && isBody
	if stillBody {
		p.c.Body = appendLine(p.c.Body, p.current())
	} else {
		p.c.References = append(p.c.References, refs...)
		p.c.Footer = appendLine(p.c.Footer, p.current())
	}
	p.next()
	return stillBody
}

// parseReferences finds the issue references in one line. A line with
// a reference keyword followed by whitespace is split into one sentence
// per keyword; any other line is one sentence with no action.
func parseReferences(line string) []Reference {
	// Each reference has the "#" issue prefix.
	if strings.IndexByte(line, '#') < 0 {
		return nil
	}
	var refs []Reference
	for _, s := range referenceSentences(line) {
		if urlRegex.MatchString(s.text) {
			continue
		}
		for _, r := range referencesIn(s.text) {
			r.Action = s.action
			refs = append(refs, r)
		}
	}
	return refs
}

// referencesIn returns every reference in a sentence, as the parser's
// repeated referenceParts match does.
func referencesIn(text string) []Reference {
	var refs []Reference
	for pos := 0; ; {
		r, end, ok := nextReference(text, pos)
		if !ok {
			return refs
		}
		refs = append(refs, r)
		pos = end
	}
}

// sentence is a part of a line that follows one reference keyword.
type sentence struct {
	action, text string
}

// referenceSentences splits a line as the parser's references regex
// /(close|closes|...)(?:\s+(.*?))(?=(?:close|closes|...)|$)/gi does. When
// that regex does not match, the parser uses /()(.+)/g, which makes each
// run of characters between line terminators a sentence with no action.
func referenceSentences(line string) []sentence {
	lower := asciiLower(line)
	var out []sentence
	for pos := 0; pos < len(line); {
		start, kwEnd := keywordWithSpace(line, lower, pos)
		if start < 0 {
			break
		}
		textStart := skipSpace(line, kwEnd)
		end, ok := sentenceEnd(line, lower, textStart)
		if !ok {
			// The regex fails at this keyword and tries the next position.
			pos = start + 1
			continue
		}
		out = append(out, sentence{action: line[start:kwEnd], text: line[textStart:end]})
		pos = end
	}
	if out == nil {
		for _, text := range strings.FieldsFunc(line, isLineTerminator) {
			out = append(out, sentence{text: text})
		}
	}
	return out
}

// keywordWithSpace returns the start and end of the first reference
// keyword at or after pos that is followed by whitespace, or -1.
func keywordWithSpace(line, lower string, pos int) (start, end int) {
	for i := pos; i < len(line); i++ {
		for _, kw := range referenceActions {
			if strings.HasPrefix(lower[i:], kw) && spaceAt(line, i+len(kw)) > 0 {
				return i, i + len(kw)
			}
		}
	}
	return -1, -1
}

// sentenceEnd returns where the lazy (.*?) of a sentence stops: at the
// next keyword or at the end of the line. It returns false when a line
// terminator comes first, because "." does not match it.
func sentenceEnd(line, lower string, i int) (int, bool) {
	for i < len(line) {
		if startsWithKeyword(lower[i:]) {
			return i, true
		}
		r, n := utf8.DecodeRuneInString(line[i:])
		if isLineTerminator(r) {
			return 0, false
		}
		i += n
	}
	return i, true
}

// startsWithKeyword reports whether s starts with a reference keyword.
func startsWithKeyword(s string) bool {
	for _, kw := range referenceActions {
		if strings.HasPrefix(s, kw) {
			return true
		}
	}
	return false
}

// nextReference finds the next reference in s at or after pos, as the
// parser's referenceParts regex
// /(?:.*?)??\s*([\w-\.\/]*?)??(#)([\w-]+)(?=\s|$|[,;)\]])/gi does. It
// returns the reference and the end of the match.
func nextReference(s string, pos int) (Reference, int, bool) {
	// start is where the match starts. The lazy (.*?) prefix cannot
	// cross a line terminator, so after one the match starts again.
	start := pos
	for skip := pos; skip < len(s); {
		p := skipSpace(s, skip)
		// The repository group is optional and lazy: try no repository
		// first, then longer and longer repository names.
		for repoLen := 0; p+repoLen < len(s); repoLen++ {
			if repoLen > 0 && !isRepoChar(s[p+repoLen-1]) {
				break
			}
			hashAt := p + repoLen
			if s[hashAt] != '#' {
				continue
			}
			issue, ok := matchIssue(s, hashAt+1)
			if !ok {
				continue
			}
			r := Reference{Prefix: "#", Issue: issue, Raw: s[start : hashAt+1+len(issue)]}
			if repo := s[p:hashAt]; repo != "" {
				if owner, name, found := strings.Cut(repo, "/"); found {
					r.Owner, repo = owner, name
				}
				r.Repository = repo
			}
			return r, hashAt + 1 + len(issue), true
		}
		r, n := utf8.DecodeRuneInString(s[skip:])
		skip += n
		if isLineTerminator(r) {
			start = skip
		}
	}
	return Reference{}, 0, false
}

// matchIssue matches [\w-]+ at start, greedy with backtracking, followed
// by whitespace, the end, or one of ",;)]".
func matchIssue(s string, start int) (string, bool) {
	end := start
	for end < len(s) && isIssueChar(s[end]) {
		end++
	}
	for ; end > start; end-- {
		if end == len(s) || spaceAt(s, end) > 0 || strings.IndexByte(",;)]", s[end]) >= 0 {
			return s[start:end], true
		}
	}
	return "", false
}

// isWordChar reports whether c matches \w.
func isWordChar(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isIssueChar reports whether c matches [\w-].
func isIssueChar(c byte) bool {
	return isWordChar(c) || c == '-'
}

// isRepoChar reports whether c matches [\w-./].
func isRepoChar(c byte) bool {
	return isIssueChar(c) || c == '.' || c == '/'
}

// trimNewLines removes leading and trailing newlines.
func trimNewLines(s string) string {
	return strings.Trim(s, "\r\n")
}

// appendLine joins src and line with a newline, as the parser does.
func appendLine(src, line string) string {
	if src == "" {
		return line
	}
	return src + "\n" + line
}

// ParseCommits parses the raw commits that have a message.
func ParseCommits(raws []RawCommit) []Commit {
	commits := make([]Commit, 0, len(raws))
	for _, r := range raws {
		if jsTrim(r.Message) != "" {
			commits = append(commits, ParseCommit(r))
		}
	}
	return commits
}

// filterReverted returns the commits without each pair of a revert
// commit and the commit it reverts, as conventional-commits-filter
// does. Commits are newest first, so a revert comes before its commit.
func filterReverted(commits []Commit) []Commit {
	drop := map[int]bool{}
	var held []int
	for i := range commits {
		for k, j := range held {
			rv := commits[j].Revert
			if jsTrim(commits[i].Header) == jsTrim(rv.Header) &&
				jsTrim(commits[i].Hash) == jsTrim(rv.Hash) {
				drop[i], drop[j] = true, true
				held = slices.Delete(held, k, k+1)
				break
			}
		}
		if !drop[i] && commits[i].Revert != nil {
			held = append(held, i)
		}
	}
	out := make([]Commit, 0, len(commits)-len(drop))
	for i := range commits {
		if !drop[i] {
			out = append(out, commits[i])
		}
	}
	return out
}

// DropSkipped returns the commits without the ones marked
// [skip release]. semantic-release does this before the analyzer runs,
// and only for the analyzer: the notes keep these commits.
func DropSkipped(commits []Commit) []Commit {
	return slices.DeleteFunc(slices.Clone(commits), func(c Commit) bool {
		return skipReleaseRegex.MatchString(c.Message)
	})
}

// Analyze returns the release type for the commits from ParseCommits,
// as commit-analyzer does: reverted commits are left out, then custom
// rules are checked first; a commit that matches no custom rule uses the
// default rules.
func Analyze(commits []Commit, rules []releaserc.Rule) Bump {
	commits = filterReverted(commits)
	result := BumpNone
	for i := range commits {
		c := &commits[i]
		t, matched := matchRules(c, rules)
		if !matched {
			t, _ = matchRules(c, defaultRules)
		}
		result = maxBump(result, t)
		if result == BumpMajor {
			break
		}
	}
	return result
}

// matchRules returns the release type of the rules that match c, and
// whether any rule matched. As in commit-analyzer, the matching rules
// are checked in order: a rule wins when no type is set yet, when the
// current type is "false", or when its type is higher. A "false" rule
// always wins, so a later "false" rule cancels an earlier type.
func matchRules(c *Commit, rules []releaserc.Rule) (Bump, bool) {
	result, matched := "", false
	for _, r := range rules {
		if (r.Breaking && len(c.Notes) == 0) || (r.Revert && c.Revert == nil) {
			continue
		}
		if !releaserc.MatchGlob(r.Type, c.Type) || !releaserc.MatchGlob(r.Scope, c.Scope) ||
			!releaserc.MatchGlob(r.Subject, c.Subject) {
			continue
		}
		if !matched || outranks(r.Release, result) {
			result = r.Release
		}
		matched = true
		if result == string(BumpMajor) {
			break
		}
	}
	if result == releaseFalse {
		return BumpNone, matched
	}
	return Bump(result), matched
}

// outranks reports whether release type next replaces current, as
// commit-analyzer's compareReleaseTypes does. "false" stands for the
// false and null release values: it is replaced by anything, and it
// replaces anything, because its index in the release type list is -1.
func outranks(next, current string) bool {
	return current == releaseFalse || next == releaseFalse || Bump(next).rank() > Bump(current).rank()
}
