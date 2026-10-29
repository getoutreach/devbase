// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Honors "# eslint-disable" style suppression comments.

// eslint_directives.go lets a schema silence Tier 2 and Tier 3
// violations with the same comments @graphql-eslint honors, so a
// repository that already carries them for ESLint needs no changes:
//
//	# eslint-disable [rule[, rule]...] [-- reason]
//	# eslint-enable [rule[, rule]...]
//	# eslint-disable-next-line [rule[, rule]...]
//
// A rule is named either as @graphql-eslint writes it
// ("@graphql-eslint/naming-convention") or by its bare devbase name; a
// directive naming no rule covers every suppressible rule. eslint-disable
// runs from its own line to the matching eslint-enable, or to the end of
// the file. eslint-disable-line is not supported.
//
// Tier 1 violations and gqlparser's unclassified ones are never
// suppressed: they are syntax and schema validity errors, and hiding one
// would defer the failure to the next consumer of the schema.
//
// A directive is a real "#" comment token, so text inside a description
// block string is never one.

package lint

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/getoutreach/gobox/pkg/set"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/lexer"
)

// eslintRulePrefix is the namespace @graphql-eslint gives its rules in
// directive comments.
const eslintRulePrefix = "@graphql-eslint/"

// eslintReasonSeparator splits a directive's rule list from its
// optional reason, as ESLint does.
var eslintReasonSeparator = regexp.MustCompile(`\s-{2,}\s`)

// eslintDirectiveKind identifies which suppression comment a directive is.
type eslintDirectiveKind int

// Supported kinds of suppression comment.
const (
	// eslintDisable silences rules from its own line on.
	eslintDisable eslintDirectiveKind = iota

	// eslintEnable ends an eslintDisable for the rules it names.
	eslintEnable

	// eslintDisableNextLine silences rules on the next line only.
	eslintDisableNextLine
)

// eslintDirective is one parsed suppression comment. A nil rules slice
// means the directive names no rule and so covers all of them.
type eslintDirective struct {
	kind  eslintDirectiveKind
	line  int
	rules []string
}

// parseEslintDirective parses the text of a "#" comment, without the
// leading "#", as a suppression directive. ok is false for any other
// comment, including one that merely starts with a directive's name.
func parseEslintDirective(text string) (kind eslintDirectiveKind, rules []string, ok bool) {
	text = strings.TrimSpace(text)

	var rest string
	// The longest name is tried first so that "eslint-disable-next-line" is
	// not read as "eslint-disable" followed by "-next-line".
	for _, name := range []struct {
		prefix string
		kind   eslintDirectiveKind
	}{
		{"eslint-disable-next-line", eslintDisableNextLine},
		{"eslint-disable", eslintDisable},
		{"eslint-enable", eslintEnable},
	} {
		if after, found := strings.CutPrefix(text, name.prefix); found {
			kind, rest, ok = name.kind, after, true
			break
		}
	}
	if !ok {
		return 0, nil, false
	}
	if first, _ := utf8.DecodeRuneInString(rest); rest != "" && !unicode.IsSpace(first) {
		return 0, nil, false
	}

	if loc := eslintReasonSeparator.FindStringIndex(rest); loc != nil {
		rest = rest[:loc[0]]
	}
	for name := range strings.SplitSeq(rest, ",") {
		if name = strings.TrimPrefix(strings.TrimSpace(name), eslintRulePrefix); name != "" {
			rules = append(rules, name)
		}
	}
	return kind, rules, true
}

// eslintDirectives lexes src for suppression comments, in source order.
func eslintDirectives(src *ast.Source) ([]eslintDirective, error) {
	lx := lexer.New(src)

	var directives []eslintDirective
	for {
		tok, err := lx.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("lex %s: %w", src.Name, err)
		}
		if tok.Kind == lexer.EOF {
			return directives, nil
		}
		if tok.Kind != lexer.Comment {
			continue
		}
		if kind, rules, ok := parseEslintDirective(strings.TrimPrefix(tok.Value, "#")); ok {
			directives = append(directives, eslintDirective{kind: kind, line: tok.Pos.Line, rules: rules})
		}
	}
}

// suppresses reports whether directives silence rule on line.
func suppresses(directives []eslintDirective, rule string, line int) bool {
	allDisabled := false
	byRule := make(map[string]bool)
	for _, d := range directives {
		if d.kind == eslintDisableNextLine {
			if d.line+1 == line && (d.rules == nil || slices.Contains(d.rules, rule)) {
				return true
			}
			continue
		}
		if d.line >= line {
			continue
		}

		disabled := d.kind == eslintDisable
		if d.rules == nil {
			allDisabled = disabled
			clear(byRule)
			continue
		}
		for _, r := range d.rules {
			byRule[r] = disabled
		}
	}
	if disabled, ok := byRule[rule]; ok {
		return disabled
	}
	return allDisabled
}

// unsuppressibleRules are the rules a directive comment never silences.
// Tier 1 violations return from FilesFromSources before suppression runs,
// so for those this guard only protects against that changing. The two
// suppression diagnostics are added before the "eslint-" directives are
// applied, so this set is what keeps them visible.
//
//nolint:gochecknoglobals // Why: built once, never mutated.
var unsuppressibleRules = set.Of(append(config.Tier1RuleNames(), UnclassifiedRule,
	config.RuleInvalidSuppression, config.RuleUnusedSuppression)...)

// withoutSuppressed returns violations minus every Tier 2 or Tier 3
// violation silenced by a directive comment in the source it was found
// in. sources must be the sources violations were produced from, and
// violations must not be used after the call, since it is filtered in place.
func withoutSuppressed(sources []*ast.Source, violations []Violation) ([]Violation, error) {
	byFile := make(map[string][]eslintDirective, len(sources))
	for _, src := range sources {
		// Every directive contains this substring, so files without it are
		// not lexed.
		if !strings.Contains(src.Input, "eslint-") {
			continue
		}
		directives, err := eslintDirectives(src)
		if err != nil {
			return nil, err
		}
		byFile[src.Name] = directives
	}
	if len(byFile) == 0 {
		return violations, nil
	}

	return slices.DeleteFunc(violations, func(v Violation) bool {
		return !unsuppressibleRules.Contains(v.Rule) && len(v.err.Locations) > 0 &&
			suppresses(byFile[v.File()], v.Rule, v.err.Locations[0].Line)
	}), nil
}
