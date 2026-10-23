// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Honors "# devbase-lint-disable" style suppression comments.

// suppression_comments.go implements devbase's own suppression comments,
// the native counterpart of the "eslint-" ones in eslint_directives.go:
//
//	# devbase-lint-disable-next-line rule[, rule]... [-- reason]
//	# devbase-lint-disable-line rule[, rule]... [-- reason]
//	# devbase-lint-disable rule[, rule]... [-- reason]
//	# devbase-lint-enable [rule[, rule]...]
//
// Unlike the "eslint-" ones, a native directive is strict: it names at
// least one rule, by its bare devbase name, and it gives a reason, either
// inline after "--" or as a "# Why:" comment block directly above a
// disable-next-line or disable directive. A directive that is malformed,
// names an unknown or unsuppressible rule, or has no reason suppresses
// nothing and is reported as invalid-suppression. A valid directive that
// suppresses no violation of a rule it names is reported once for that
// rule as unused-suppression. Both default to "error" and can be lowered
// or turned off in scripts/devbase.yaml.
//
// disable-next-line covers the next line. A description below the
// directive is skipped, because violations are reported on the name's
// line.

package lint

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/lexer"
)

// devbaseDirectivePrefix starts every native suppression comment.
const devbaseDirectivePrefix = "devbase-lint-"

// whyPrefix starts the first line of a reason written above a directive.
const whyPrefix = "Why:"

// suppressionKind identifies which native suppression comment a
// directive is.
type suppressionKind int

// Supported kinds of native suppression comment.
const (
	// suppressDisable silences rules from the line after it until a
	// matching suppressEnable.
	suppressDisable suppressionKind = iota

	// suppressEnable ends a suppressDisable for the rules it names.
	suppressEnable

	// suppressDisableNextLine silences rules on the next line only.
	suppressDisableNextLine

	// suppressDisableLine silences rules on its own line only.
	suppressDisableLine
)

// suppressionKinds maps a directive's keyword, without the prefix, to
// its kind.
//
//nolint:gochecknoglobals // Why: a fixed lookup table, never mutated.
var suppressionKinds = map[string]suppressionKind{
	"disable":           suppressDisable,
	"enable":            suppressEnable,
	"disable-next-line": suppressDisableNextLine,
	"disable-line":      suppressDisableLine,
}

// suppression is one parsed native suppression comment.
type suppression struct {
	kind    suppressionKind
	keyword string
	pos     ast.Position
	rules   []string
	reason  string

	// target is the line a disable-next-line or disable-line directive
	// covers.
	target int

	// problems lists why the directive is invalid, if it is.
	problems []string

	// ownLine is whether the directive is the only thing on its line,
	// not a trailing comment after code.
	ownLine bool

	// used records the rules the directive suppressed a violation of.
	used map[string]bool

	// shadowed records the rules for which another directive also covered
	// a violation the directive would have suppressed.
	shadowed map[string]bool
}

// parseSuppression parses the text of a "#" comment, without the leading
// "#", as a native suppression directive. ok is false for any other
// comment. A comment with the prefix but an unknown keyword is returned
// with a problem so a typo is reported instead of silently ignored.
func parseSuppression(text string) (s suppression, ok bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, devbaseDirectivePrefix) {
		return s, false
	}

	keyword, rest := text, ""
	if i := strings.IndexFunc(text, unicode.IsSpace); i >= 0 {
		keyword, rest = text[:i], text[i:]
	}
	s.keyword = keyword

	kind, known := suppressionKinds[strings.TrimPrefix(keyword, devbaseDirectivePrefix)]
	if !known {
		s.problems = []string{"unknown directive"}
		return s, true
	}
	s.kind = kind

	if loc := eslintReasonSeparator.FindStringIndex(rest); loc != nil {
		s.reason = strings.TrimSpace(rest[loc[1]:])
		rest = rest[:loc[0]]
	}
	for name := range strings.SplitSeq(rest, ",") {
		name = strings.TrimSpace(name)
		if name != "" && !slices.Contains(s.rules, name) {
			s.rules = append(s.rules, name)
		}
	}
	return s, true
}

// acceptsWhy reports whether s may take its reason from a "# Why:" block
// above it: only a disable-next-line or disable directive on its own line.
func (s *suppression) acceptsWhy() bool {
	return s.ownLine && (s.kind == suppressDisableNextLine || s.kind == suppressDisable)
}

// validate records the problems that make s invalid.
func (s *suppression) validate() {
	if s.kind != suppressEnable && len(s.rules) == 0 {
		s.problems = append(s.problems, "names no rule")
	}
	for _, rule := range s.rules {
		switch {
		case configurableRules.Contains(rule):
		case unsuppressibleRules.Contains(rule):
			s.problems = append(s.problems, fmt.Sprintf("rule %q cannot be suppressed", rule))
		default:
			problem := fmt.Sprintf("unknown rule %q", rule)
			if strings.Contains(rule, "--") {
				problem += ` (separate the reason with " -- ")`
			}
			s.problems = append(s.problems, problem)
		}
	}
	if s.kind == suppressEnable || s.reason != "" {
		return
	}
	if s.acceptsWhy() {
		s.problems = append(s.problems, `has no reason (add "-- reason" or a "# Why:" comment directly above)`)
	} else {
		s.problems = append(s.problems, `has no reason (add "-- reason")`)
	}
}

// isTrailing reports whether the comment at tokens[i] follows code on its
// own line.
func isTrailing(tokens []lexer.Token, i int) bool {
	return i > 0 && tokens[i-1].Kind != lexer.Comment && tokens[i-1].Pos.Line == tokens[i].Pos.Line
}

// nextCode returns the index of the first token at or after from that is
// not a comment, or -1.
func nextCode(tokens []lexer.Token, from int) int {
	for i := from; i < len(tokens); i++ {
		if tokens[i].Kind != lexer.Comment {
			return i
		}
	}
	return -1
}

// lineFinder returns a function giving the 1-based line that holds the rune
// at a rune offset in input. A lexer token's Pos.Line is not the line it
// starts on when the token spans lines, as a block string does, so the
// start line comes from the offset. Like the lexer, it ends a line at "\n",
// "\r\n" or a lone "\r".
func lineFinder(input string) func(offset int) int {
	runes := []rune(input)
	starts := []int{0}
	for i, r := range runes {
		if r == '\n' || (r == '\r' && (i+1 == len(runes) || runes[i+1] != '\n')) {
			starts = append(starts, i+1)
		}
	}
	return func(offset int) int {
		line, _ := slices.BinarySearch(starts, offset+1)
		return line
	}
}

// nextLineTarget returns the line a disable-next-line directive at
// tokens[i] covers: the next line, or, when a description starts there,
// the line of the token after the description. lineAt gives the line a
// rune offset is on.
func nextLineTarget(tokens []lexer.Token, i int, lineAt func(int) int) int {
	line := tokens[i].Pos.Line + 1
	j := nextCode(tokens, i+1)
	if j < 0 || (tokens[j].Kind != lexer.String && tokens[j].Kind != lexer.BlockString) ||
		lineAt(tokens[j].Pos.Start) != line {
		return line
	}
	if k := nextCode(tokens, j+1); k >= 0 {
		return tokens[k].Pos.Line
	}
	return line
}

// whyReason returns the reason written as a "# Why:" block directly above
// the directive at tokens[i]: the run of comment-only lines ending on the
// line above it, from the first line whose text starts with "Why:". Another
// directive, a blank line or code ends the run.
func whyReason(tokens []lexer.Token, i int) string {
	var run []string
	line := tokens[i].Pos.Line
	for j := i - 1; j >= 0; j-- {
		t := tokens[j]
		if t.Kind != lexer.Comment || t.Pos.Line != line-1 || isTrailing(tokens, j) {
			break
		}
		text := strings.TrimSpace(strings.TrimPrefix(t.Value, "#"))
		if strings.HasPrefix(text, devbaseDirectivePrefix) {
			break
		}
		run = append(run, text)
		line = t.Pos.Line
	}
	slices.Reverse(run)

	for k, text := range run {
		first, ok := strings.CutPrefix(text, whyPrefix)
		if !ok {
			continue
		}
		parts := slices.Concat([]string{strings.TrimSpace(first)}, run[k+1:])
		return strings.Join(slices.DeleteFunc(parts, func(p string) bool { return p == "" }), " ")
	}
	return ""
}

// suppressionsIn lexes src for native suppression comments, in source
// order.
func suppressionsIn(src *ast.Source) ([]*suppression, error) {
	lx := lexer.New(src)

	var tokens []lexer.Token
	for {
		tok, err := lx.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("lex %s: %w", src.Name, err)
		}
		if tok.Kind == lexer.EOF {
			break
		}
		tokens = append(tokens, tok)
	}

	lineAt := lineFinder(src.Input)

	var out []*suppression
	for i, tok := range tokens {
		if tok.Kind != lexer.Comment {
			continue
		}
		s, ok := parseSuppression(strings.TrimPrefix(tok.Value, "#"))
		if !ok {
			continue
		}
		s.pos = tok.Pos
		s.ownLine = !isTrailing(tokens, i)
		s.used = make(map[string]bool)
		s.shadowed = make(map[string]bool)

		if len(s.problems) == 0 {
			switch s.kind {
			case suppressDisableNextLine:
				s.target = nextLineTarget(tokens, i, lineAt)
			case suppressDisableLine:
				s.target = tok.Pos.Line
			case suppressDisable, suppressEnable:
			}
			if s.reason == "" && s.acceptsWhy() {
				s.reason = whyReason(tokens, i)
			}
			s.validate()
		}
		out = append(out, &s)
	}
	return out, nil
}

// suppressedBy reports whether a valid directive in ss silences rule on
// line. The first directive that covers it is recorded as used for rule,
// a line directive before a range one, and any others as shadowed.
func suppressedBy(ss []*suppression, rule string, line int) bool {
	if len(ss) == 0 {
		return false
	}

	var covering []*suppression
	activeBy := make(map[string]*suppression)
	for _, s := range ss {
		if len(s.problems) > 0 {
			continue
		}
		switch s.kind {
		case suppressDisableNextLine, suppressDisableLine:
			if s.target == line && slices.Contains(s.rules, rule) {
				covering = append(covering, s)
			}
		case suppressDisable:
			if s.pos.Line < line {
				for _, r := range s.rules {
					activeBy[r] = s
				}
			}
		case suppressEnable:
			if s.pos.Line >= line {
				continue
			}
			if len(s.rules) == 0 {
				clear(activeBy)
			}
			for _, r := range s.rules {
				delete(activeBy, r)
			}
		}
	}

	if active := activeBy[rule]; active != nil {
		covering = append(covering, active)
	}
	if len(covering) == 0 {
		return false
	}
	covering[0].used[rule] = true
	for _, s := range covering[1:] {
		s.shadowed[rule] = true
	}
	return true
}

// suppressionDiagnostics reports the invalid directives in ss, and the
// valid ones that suppressed nothing for a rule they name, as violations
// positioned at the directive. cfg can turn either diagnostic off.
func suppressionDiagnostics(ss []*suppression, cfg *config.Lint) []Violation {
	var out []Violation
	report := func(s *suppression, rule, format string, args ...any) {
		pos := s.pos
		out = append(out, Violation{err: gqlerror.ErrorPosf(&pos, format, args...), Rule: rule})
	}

	for _, s := range ss {
		if len(s.problems) > 0 {
			if !cfg.Disabled(config.RuleInvalidSuppression) {
				report(s, config.RuleInvalidSuppression, "Invalid suppression comment %s: %s",
					s.keyword, strings.Join(s.problems, "; "))
			}
			continue
		}
		if s.kind == suppressEnable || cfg.Disabled(config.RuleUnusedSuppression) {
			continue
		}
		for _, rule := range s.rules {
			if s.used[rule] {
				continue
			}
			hint := ""
			if s.shadowed[rule] {
				hint = " (another directive already covers it)"
			}
			report(s, config.RuleUnusedSuppression, "Unused suppression comment %s: rule %q suppressed no violation%s",
				s.keyword, rule, hint)
		}
	}
	return out
}

// withNativeSuppression returns violations minus every Tier 2 or Tier 3
// violation silenced by a valid native directive in the source it was found
// in, plus the diagnostics about the directives themselves. sources must be
// the sources violations were produced from, and violations must not be used
// after the call, since it is filtered in place.
func withNativeSuppression(sources []*ast.Source, violations []Violation, cfg *config.Lint) ([]Violation, error) {
	byFile := make(map[string][]*suppression, len(sources))
	for _, src := range sources {
		// Every directive contains the prefix, so files without it are not
		// lexed.
		if !strings.Contains(src.Input, devbaseDirectivePrefix) {
			continue
		}
		ss, err := suppressionsIn(src)
		if err != nil {
			return nil, err
		}
		if len(ss) > 0 {
			byFile[src.Name] = ss
		}
	}
	if len(byFile) == 0 {
		return violations, nil
	}

	kept := slices.DeleteFunc(violations, func(v Violation) bool {
		return !unsuppressibleRules.Contains(v.Rule) && len(v.err.Locations) > 0 &&
			suppressedBy(byFile[v.File()], v.Rule, v.err.Locations[0].Line)
	})
	for _, src := range sources {
		kept = append(kept, suppressionDiagnostics(byFile[src.Name], cfg)...)
	}
	return kept, nil
}
