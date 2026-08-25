// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tier 3 no-hashtag-description rule.

// hashtag.go implements 1 (of 10) Tier 3 custom rules:
// no-hashtag-description, which forbids a "#" comment immediately
// preceding a definition, field, argument, or enum value, in favor of
// a "..." or """...""" description.
//
// gqlparser's parser already groups every "#" comment immediately
// before a description-bearing site, after its description if it has
// one, into that site's own AfterDescriptionComment
// (descriptionSite.afterDescriptionComment, populated by
// descriptions.go's groupDescriptionSites). That is why a hashtag
// comment right before a "..." description is never flagged: it was
// never grouped against the site at all, gqlparser attributes it to
// BeforeDescriptionComment instead. What that grouping does not
// preserve is line adjacency: it collects every consecutive "#" line
// with no regard for blank lines between them, or for whether the
// first one is actually a trailing comment on the code before it (for
// example "type Query { # trailing"). noHashtagDescriptionViolations
// recovers both distinctions the same way @graphql-eslint's own rule
// does: by comparing line numbers, using trailingCommentStarts for the
// second one.
//
// Unlike require-description, no-hashtag-description has no per-kind
// options: it applies unconditionally to every site groupDescriptionSites
// produces. It also never applies to a type extension (`extend type Foo
// { ... }`): gqlparser rejects a description before "extend" outright, so
// there is no `"..."` alternative to suggest in its place, and
// groupDescriptionSites accordingly never builds a site for an
// extension's own leading comment (only for the fields an extension
// without a base definition adds, which it already walks like any other
// field).

package lint

import (
	"fmt"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/lexer"
)

// trailingCommentStarts returns the rune offsets (ast.Position.Start) of
// every "#" comment token in src that begins on the same line as the
// token immediately before it, for example the "# trailing" in
// "type Query { # trailing", as opposed to one that starts on its own
// line. noHashtagDescriptionViolations never treats such a comment as
// attached to the site that follows it, matching @graphql-eslint's own
// `line !== prev.line` check on the raw token stream.
func trailingCommentStarts(src *ast.Source) (map[int]bool, error) {
	lx := lexer.New(src)

	trailing := make(map[int]bool)
	var prev lexer.Token
	havePrev := false
	for {
		tok, err := lx.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("lex %s: %w", src.Name, err)
		}
		if tok.Kind == lexer.EOF {
			break
		}
		if tok.Kind == lexer.Comment && havePrev && prev.Kind != lexer.Comment && prev.Pos.Line == tok.Pos.Line {
			trailing[tok.Pos.Start] = true
		}
		prev, havePrev = tok, true
	}
	return trailing, nil
}

// noHashtagDescriptionViolations reports a RuleNoHashtagDescription
// violation for every site in sites whose afterDescriptionComment is
// attached to it with no blank line in between. Two comment tokens can
// never share a line, since a "#" comment always runs to end of line, so
// a multi-comment group's last entry can only ever be on the same line
// as the second-to-last one's, never a trailing comment on the code
// before the group. trailing only needs consulting when the group has
// exactly one comment.
func noHashtagDescriptionViolations(sites []descriptionSite, trailing map[int]bool) []Violation {
	violations := make([]Violation, 0, len(sites))
	for i := range sites {
		s := &sites[i]
		group := s.afterDescriptionComment
		if group == nil || len(group.List) == 0 {
			continue
		}

		last := group.List[len(group.List)-1]
		if len(group.List) == 1 && trailing[last.Position.Start] {
			continue
		}
		if s.pos.Line-last.Position.Line >= 2 {
			continue
		}

		violations = append(violations, Violation{
			err: gqlerror.ErrorPosf(last.Position,
				"Unexpected GraphQL description as hashtag `#` for %s. "+
					`Prefer using """ for multiline, or " for a single line description`, s.nodeName()),
			Rule: config.RuleNoHashtagDescription,
		})
	}
	return violations
}

// tier3NoHashtagDescription runs no-hashtag-description against every
// file in fileSources, using sitesByFile to find each file's own
// description-bearing sites. It does not run unless cfg enables it
// (config.Lint.Enabled).
func tier3NoHashtagDescription(fileSources []*ast.Source, sitesByFile map[*ast.Source][]descriptionSite,
	cfg *config.Lint,
) ([]Violation, error) {
	if !cfg.Enabled(config.RuleNoHashtagDescription) {
		return nil, nil
	}

	var violations []Violation
	for _, src := range fileSources {
		trailing, err := trailingCommentStarts(src)
		if err != nil {
			return nil, err
		}
		violations = append(violations, noHashtagDescriptionViolations(sitesByFile[src], trailing)...)
	}
	return violations, nil
}
