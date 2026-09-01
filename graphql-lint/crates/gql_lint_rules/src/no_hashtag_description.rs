//! Ports `internal/graphql/lint/hashtag.go`'s `no-hashtag-description`
//! rule: forbids a `#` comment immediately preceding a definition, field,
//! argument, or enum value, in favor of a `"..."` or `"""..."""`
//! description.
//!
//! `gqlparser`'s parser groups a "#" comment run immediately before a
//! description-bearing site (after its description, if it has one) into
//! that site's own field; this port has no such grouping available (the
//! validated `Schema` carries no comment trivia at all), so it recovers
//! the same grouping directly from a raw re-lex, using
//! [`crate::descriptions::collect_description_sites`]'s real per-site
//! byte offsets as the anchor to scan backward from.
//!
//! **One structural difference from the Go rule's own scan, needed
//! because this anchors on the site's own *name* token, not its
//! description's start:** for a field, argument, enum value, or a
//! directive's own argument, the name is the true first token of that
//! declaration, so scanning backward from it lands directly on any
//! preceding comment. For a type-kind definition or a directive
//! definition, though, the name is preceded by a keyword ("type",
//! "interface", …, or "directive" + "@") that is *part of* the same
//! declaration, not something separating it from a comment further back.
//! `skip_declaration_keyword` accounts for this by also skipping back over
//! exactly that leading keyword (and, for a directive definition, the "@"
//! after it) before looking for a comment run — otherwise a comment before
//! `type Foo` would be missed because the scan would stop at the `type`
//! token itself, mistaking it for unrelated code.

use crate::descriptions::collect_description_sites;
use apollo_compiler::Schema;
use apollo_parser::{Lexer, Token, TokenKind};
use gql_lint_core::{Violation, rules};

/// Runs the `no-hashtag-description` rule over `schema`.
///
/// # Errors
/// Returns an error if a site's own file fails to read or lex.
pub fn no_hashtag_description(schema: &Schema) -> anyhow::Result<Vec<Violation>> {
    let mut violations = Vec::new();

    for (_file, sites) in collect_description_sites(schema) {
        let source = sites[0].file;
        let text = std::fs::read_to_string(source)
            .map_err(|e| anyhow::anyhow!("read {}: {e}", source.display()))?;
        let tokens = lex_with_trivia(&text).map_err(|e| anyhow::anyhow!("lex {}: {e:?}", source.display()))?;

        for site in &sites {
            // TODO: _comment_offset is the last attached comment's byte
            // offset — see description_style's matching TODO for why
            // line/column resolution from it isn't wired up yet.
            let Some(_comment_offset) = attached_comment_offset(&text, &tokens, site.offset) else {
                continue;
            };
            violations.push(Violation {
                file: source.display().to_string(),
                line: 0,
                column: 0,
                message: format!(
                    "Unexpected GraphQL description as hashtag `#` for {}. \
                     Prefer using \"\"\" for multiline, or \" for a single line description",
                    site.label
                ),
                rule: rules::NO_HASHTAG_DESCRIPTION,
            });
        }
    }

    Ok(violations)
}

/// Lexes `text` into every token, including whitespace and comments
/// (needed for line-adjacency and trailing-comment checks) but excluding
/// EOF.
fn lex_with_trivia(text: &str) -> Result<Vec<Token<'_>>, apollo_parser::Error> {
    let mut tokens = Vec::new();
    for result in Lexer::new(text) {
        let token = result?;
        if token.kind() != TokenKind::Eof {
            tokens.push(token);
        }
    }
    Ok(tokens)
}

/// If a `#` comment run is attached to the declaration whose own name
/// starts at `name_offset` (not separated from it by a blank line, and —
/// for a single-comment run — not itself a trailing comment on the
/// previous line's code), returns that run's last comment's byte offset.
fn attached_comment_offset(text: &str, tokens: &[Token], name_offset: usize) -> Option<usize> {
    let name_idx = tokens.iter().position(|t| t.index() == name_offset)?;
    let anchor = skip_declaration_keyword(tokens, name_idx);

    // Walk backward past whitespace and comments, collecting comment
    // token indices in reverse (nearest-to-declaration first).
    let mut group_rev = Vec::new();
    let mut i = anchor;
    while i > 0 {
        match tokens[i - 1].kind() {
            TokenKind::Whitespace => i -= 1,
            TokenKind::Comment => {
                group_rev.push(i - 1);
                i -= 1;
            }
            _ => break,
        }
    }
    let last_comment = *group_rev.first()?; // nearest to the declaration
    let first_comment = *group_rev.last().unwrap_or(&last_comment);

    // Blank-line check: a group separated from the declaration by a
    // blank line (2+ line breaks in the gap) is not attached.
    let comment_end = tokens[last_comment].index() + tokens[last_comment].data().len();
    let anchor_start = anchor_byte_offset(tokens, anchor, name_offset);
    if text[comment_end..anchor_start].matches('\n').count() >= 2 {
        return None;
    }

    // Trailing-comment check: only applies with exactly one comment (two
    // comment tokens can never share a line, since a "#" comment always
    // runs to end of line, so a multi-comment group's earlier entries can
    // only ever follow another comment on its own line, never a trailing
    // comment on the code before the group).
    if first_comment == last_comment
        && let Some(prev) = tokens[..first_comment]
            .iter()
            .rposition(|t| t.kind() != TokenKind::Whitespace)
        && tokens[prev].kind() != TokenKind::Comment
    {
        let prev_end = tokens[prev].index() + tokens[prev].data().len();
        let comment_start = tokens[first_comment].index();
        if !text[prev_end..comment_start].contains(['\n', '\r']) {
            return None; // same line as the code before it: trailing, not attached.
        }
    }

    Some(tokens[last_comment].index())
}

/// The byte offset `anchor` (a token index) sits at, or `name_offset` if
/// `anchor` is past the end of `tokens` (the declaration's name is the
/// very first token in the file — no keyword to have skipped back over).
fn anchor_byte_offset(tokens: &[Token], anchor: usize, name_offset: usize) -> usize {
    tokens.get(anchor).map_or(name_offset, Token::index)
}

/// Skips backward over the leading keyword (and, for a directive
/// definition, the "@" after it) that precedes a type-kind or
/// directive-definition's own name — see this module's doc comment.
fn skip_declaration_keyword(tokens: &[Token], name_idx: usize) -> usize {
    let mut i = name_idx;
    while i > 0 && tokens[i - 1].kind() == TokenKind::Whitespace {
        i -= 1;
    }
    // "directive @Name": skip the "@".
    if i > 0 && tokens[i - 1].kind() == TokenKind::At {
        i -= 1;
        while i > 0 && tokens[i - 1].kind() == TokenKind::Whitespace {
            i -= 1;
        }
    }
    // A leading keyword ("type", "interface", "union", "enum", "input",
    // "scalar", "directive", "extend"). Declarations never nest, so at
    // most one such keyword can appear here.
    if i > 0 && tokens[i - 1].kind() == TokenKind::Name {
        i -= 1;
    }
    i
}
