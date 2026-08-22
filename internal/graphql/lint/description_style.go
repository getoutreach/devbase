// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The description-style Tier 3 rule.

// description_style.go implements description-style: every
// description in a schema must use the same quoting style, inline
// "..." or block """...""".
//
// ast.Definition and friends only expose a description's decoded
// text, not which quoting style produced it, so this file recovers
// that by re-lexing each file's own raw source (descriptionTokens) and
// pairing the resulting tokens back up with descriptionSite's
// description-bearing nodes, in the order both appear in that file.
//
// descriptionTokens' scan has to tell a description string apart from
// every other string literal SDL allows: a field/argument default
// value (`= "x"`) and a directive usage argument (`@dir(arg: "x")`),
// including either nested inside a list or input object literal. Both
// are only reachable from a "=" token or a ":" inside a directive
// usage's own argument list, and a description is never preceded by
// either -- so skipValue, which walks past one Value production
// (recursing through matching brackets or braces), can exclude them
// and leave exactly the description candidates.

package lint

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/lexer"
)

// descriptionStyleOptions is description-style's scripts/devbase.yaml
// options, decoded from Rule.Options.
type descriptionStyleOptions struct {
	// block is true if every description must be a block string
	// ("""...""") -- @graphql-eslint's default -- or false if every
	// description must be inline ("...").
	block bool
}

// parseDescriptionStyleOptions decodes opts (Rule.Options) into
// descriptionStyleOptions, defaulting to block style if opts is nil or
// its "style" key isn't the string "inline".
func parseDescriptionStyleOptions(opts map[string]any) descriptionStyleOptions {
	style, _ := opts["style"].(string)
	return descriptionStyleOptions{block: style != "inline"}
}

// descriptionStyleViolations reports a RuleDescriptionStyle violation
// for every description in sites whose quoting style -- read off the
// matching entry in tokens, in order -- disagrees with opts. tokens
// must have exactly one entry per site in sites with a non-empty
// description, in the same order; tier3Descriptions guarantees this
// before calling in.
func descriptionStyleViolations(sites []descriptionSite, tokens []lexer.Token, opts descriptionStyleOptions) []Violation {
	violations := make([]Violation, 0, len(tokens))
	i := 0
	for j := range sites {
		s := &sites[j]
		if strings.TrimSpace(s.description) == "" {
			continue
		}
		tok := tokens[i]
		i++

		isBlock := tok.Kind == lexer.BlockString
		if isBlock == opts.block {
			continue
		}

		foundStyle := "inline"
		if isBlock {
			foundStyle = "block"
		}
		violations = append(violations, Violation{
			err:  gqlerror.ErrorPosf(&tok.Pos, "Unexpected %s description for %s", foundStyle, s.nodeName()),
			Rule: config.RuleDescriptionStyle,
		})
	}
	return violations
}

// descriptionTokens scans src's raw source for every String or
// BlockString token that is a description, in file order, excluding
// default values and directive usage arguments -- see this file's
// package doc comment for how it tells them apart.
func descriptionTokens(src *ast.Source) ([]lexer.Token, error) {
	lx := lexer.New(src)

	var all []lexer.Token
	for {
		tok, err := lx.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("lex %s: %w", src.Name, err)
		}
		if tok.Kind == lexer.EOF {
			break
		}
		if tok.Kind == lexer.Comment {
			continue
		}
		all = append(all, tok)
	}

	// directiveArgsActive tracks whether i is inside a directive usage's
	// "(...)" argument list -- entered on a "@Name(" sequence, exited on
	// the next ParenR. No depth counter is needed: a directive usage's
	// arguments are Values, and no Value production contains "("
	// (lists use "[...]", input objects use "{...}"), so no further
	// ParenL can occur before the one ParenR that closes this list.
	var descriptions []lexer.Token
	directiveArgsActive := false

	for i := 0; i < len(all); {
		switch all[i].Kind { //nolint:exhaustive // Why: every other token kind falls through to the default case unchanged.
		case lexer.ParenL:
			directiveArgsActive = i >= 2 && all[i-1].Kind == lexer.Name && all[i-2].Kind == lexer.At
			i++
		case lexer.ParenR:
			directiveArgsActive = false
			i++
		case lexer.Colon:
			if directiveArgsActive {
				i = skipValue(all, i+1)
			} else {
				i++
			}
		case lexer.Equals:
			i = skipValue(all, i+1)
		case lexer.String, lexer.BlockString:
			descriptions = append(descriptions, all[i])
			i++
		default:
			i++
		}
	}

	// gqlparser/v2's lexer sets a BlockString token's Pos.Line/Pos.Column
	// only after scanning to its closing """, so for one spanning more
	// than one line -- the common case for a real description -- they
	// name the closing line, not the opening one, and the column is
	// often negative. Pos.Start (a rune offset) is unaffected, so
	// recompute Line/Column from that instead. This works around a
	// gqlparser/v2 bug verified against v2.5.36 (go.mod's pinned
	// version); drop it once gqlparser fixes BlockString's own position.
	starts := lineStarts(src.Input)
	for i := range descriptions {
		descriptions[i].Pos.Line, descriptions[i].Pos.Column = linePosition(starts, descriptions[i].Pos.Start)
	}
	return descriptions, nil
}

// lineStarts returns the rune offset of the first rune of each line in
// input; line i (1-indexed) starts at starts[i-1]. A line break is a
// "\r", optionally followed by "\n", or a lone "\n" -- the same 3 forms
// gqlparser/v2's own lexer recognizes, so this numbering matches its
// Pos.Line elsewhere. Offsets are in runes, matching ast.Position.Start;
// walking input as UTF-8 directly, rather than converting it to a
// []rune first, avoids copying the whole file.
func lineStarts(input string) []int {
	starts := make([]int, 1, len(input)/40+1)
	runeIdx := 0
	for i := 0; i < len(input); {
		r, size := utf8.DecodeRuneInString(input[i:])
		i += size
		runeIdx++
		switch r {
		case '\r':
			// '\n' is single-byte ASCII and never a continuation byte of a
			// multi-byte UTF-8 sequence, so it's safe to check input[i]
			// directly without decoding another rune.
			if i < len(input) && input[i] == '\n' {
				i++
				runeIdx++
			}
			starts = append(starts, runeIdx)
		case '\n':
			starts = append(starts, runeIdx)
		}
	}
	return starts
}

// linePosition returns the 1-based (line, column) for the rune offset
// start, given starts (from lineStarts) -- the same Line/Column
// convention gqlparser/v2's own lexer uses everywhere else.
func linePosition(starts []int, start int) (line, col int) {
	i := sort.SearchInts(starts, start+1) - 1
	if i < 0 {
		i = 0
	}
	return i + 1, start - starts[i] + 1
}

// skipValue returns the index just past the one Value production
// (spec grammar) starting at tokens[i]: a scalar/enum-value token
// (i+1), or -- for a list or input object literal -- the index past
// its matching closing bracket or brace, however deeply nested.
func skipValue(tokens []lexer.Token, i int) int {
	if i >= len(tokens) {
		return i
	}
	switch tokens[i].Kind { //nolint:exhaustive // Why: every scalar/enum-value token kind falls through to the default case unchanged.
	case lexer.BracketL:
		return skipBalanced(tokens, i, lexer.BracketL, lexer.BracketR)
	case lexer.BraceL:
		return skipBalanced(tokens, i, lexer.BraceL, lexer.BraceR)
	default:
		return i + 1
	}
}

// skipBalanced returns the index just past the token that closes the
// openTok/closeTok pair starting at tokens[i] (itself an openTok
// token), counting nested pairs of the same kind.
func skipBalanced(tokens []lexer.Token, i int, openTok, closeTok lexer.Type) int {
	depth := 0
	for i < len(tokens) {
		switch tokens[i].Kind { //nolint:exhaustive // Why: every other token kind is part of the value being skipped and needs no handling.
		case openTok:
			depth++
		case closeTok:
			depth--
		}
		i++
		if depth == 0 {
			break
		}
	}
	return i
}
