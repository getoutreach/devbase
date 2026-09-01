//! Ports `internal/graphql/lint/description_style.go`'s `description-style`
//! rule: every description in a schema must use the same quoting style,
//! inline `"..."` or block `"""..."""`.
//!
//! `apollo_compiler::Schema` only exposes a description's *decoded* text
//! (`Option<Node<str>>`), not which quoting style produced it, the same
//! limitation the Go port's own package doc comment describes for
//! `gqlparser`. This re-lexes each file's own raw source with
//! `apollo_parser::Lexer` to recover that, pairing the resulting
//! description-shaped string tokens back up with
//! [`crate::descriptions::collect_description_sites`]'s sites, in the
//! same file-and-position order both are naturally produced in.
//!
//! `descriptionTokens`'s scan has to tell a description string apart from
//! every other string literal SDL allows: a field/argument default value
//! (`= "x"`) and a directive usage argument (`@dir(arg: "x")`), including
//! either nested inside a list or input object literal. Both are only
//! reachable from an `=` token or a `:` inside a directive usage's own
//! argument list, and a description is never preceded by either — so
//! `skip_value`, which walks past one Value production (recursing through
//! matching brackets/braces), excludes them and leaves exactly the
//! description candidates. Ported directly from the Go rule's own
//! `descriptionTokens`/`skipValue`/`skipBalanced`.

use crate::descriptions::{DescriptionSite, collect_description_sites};
use apollo_compiler::Schema;
use apollo_parser::{Lexer, Token, TokenKind};
use gql_lint_core::{Violation, rules};

/// `description-style`'s `scripts/devbase.yaml` options: `true` if every
/// description must be a block string (`"""..."""` — `@graphql-eslint`'s
/// own default), `false` if every description must be inline (`"..."`).
#[derive(Debug, Clone, Copy)]
pub struct DescriptionStyleOptions {
    pub block: bool,
}

impl Default for DescriptionStyleOptions {
    fn default() -> Self {
        Self { block: true }
    }
}

impl DescriptionStyleOptions {
    #[must_use]
    pub fn from_yaml(options: Option<&serde_yaml::Mapping>) -> Self {
        let style = options
            .and_then(|o| o.get("style"))
            .and_then(serde_yaml::Value::as_str);
        Self {
            block: style != Some("inline"),
        }
    }
}

/// Runs the `description-style` rule over `schema`, per `opts`.
///
/// # Errors
/// Returns an error if a file's non-empty-description count in the parsed
/// schema doesn't match its description-like string token count in the
/// raw source — `description_tokens` cannot safely pair the two up in
/// that case. This should never happen for valid SDL; see this module's
/// doc comment for why `description_tokens` is expected to find exactly
/// one token per description.
pub fn description_style(
    schema: &Schema,
    opts: &DescriptionStyleOptions,
) -> anyhow::Result<Vec<Violation>> {
    let mut violations = Vec::new();

    for (_file, sites) in collect_description_sites(schema) {
        let non_empty: Vec<&DescriptionSite> = sites
            .iter()
            .filter(|s| s.description.is_some_and(|d| !d.trim().is_empty()))
            .collect();
        if non_empty.is_empty() {
            continue;
        }

        // Every site in one group shares the same file (see
        // collect_description_sites), and a file's own text is the same
        // regardless of which site we read it through.
        let source = sites[0].file;
        let text = std::fs::read_to_string(source)
            .map_err(|e| anyhow::anyhow!("read {}: {e}", source.display()))?;
        let token_offsets = description_tokens(&text)?;

        if non_empty.len() != token_offsets.len() {
            anyhow::bail!(
                "description-style: {}: {} description(s) in the parsed schema, {} description-like \
                 string token(s) in the raw source",
                source.display(),
                non_empty.len(),
                token_offsets.len()
            );
        }

        for (site, token) in non_empty.iter().zip(token_offsets.iter()) {
            let is_block = token.text.starts_with("\"\"\"");
            if is_block == opts.block {
                continue;
            }
            let found_style = if is_block { "block" } else { "inline" };
            let (line, column) =
                gql_lint_core::line_column_at(&schema.sources, site.file_id, token.offset);
            violations.push(Violation {
                file: source.display().to_string(),
                line,
                column,
                message: format!("Unexpected {found_style} description for {}", site.label),
                rule: rules::DESCRIPTION_STYLE,
            });
        }
    }

    Ok(violations)
}

/// One description-like string token found by [`description_tokens`]: its
/// raw source text (quotes included, so the caller can tell block from
/// inline by checking for a `"""` prefix) and its own byte offset (so the
/// caller can resolve a real line/column via `gql_lint_core::line_column_at`).
struct DescriptionToken<'a> {
    text: &'a str,
    offset: usize,
}

/// Scans `text` for every string token that is a description, in file
/// order, excluding default values and directive usage arguments.
fn description_tokens(text: &str) -> anyhow::Result<Vec<DescriptionToken<'_>>> {
    let all = lex_all(text)?;

    let mut descriptions = Vec::new();
    // Tracks whether we're inside a directive usage's "(...)" argument
    // list — entered on a "@Name(" sequence, exited on the next RParen.
    // No depth counter is needed: a directive usage's arguments are
    // Values, and no Value production contains "(" (lists use "[...]",
    // input objects use "{...}"), so no further LParen can occur before
    // the one RParen that closes this list.
    let mut directive_args_active = false;

    let mut i = 0;
    while i < all.len() {
        match all[i].kind() {
            TokenKind::LParen => {
                directive_args_active = i >= 2
                    && all[i - 1].kind() == TokenKind::Name
                    && all[i - 2].kind() == TokenKind::At;
                i += 1;
            }
            TokenKind::RParen => {
                directive_args_active = false;
                i += 1;
            }
            TokenKind::Colon => {
                i = if directive_args_active {
                    skip_value(&all, i + 1)
                } else {
                    i + 1
                };
            }
            TokenKind::Eq => {
                i = skip_value(&all, i + 1);
            }
            TokenKind::StringValue => {
                descriptions.push(DescriptionToken {
                    text: all[i].data(),
                    offset: all[i].index(),
                });
                i += 1;
            }
            _ => i += 1,
        }
    }

    Ok(descriptions)
}

/// Lexes `text` into every non-whitespace, non-comment token, in order.
fn lex_all(text: &str) -> anyhow::Result<Vec<Token<'_>>> {
    let mut tokens = Vec::new();
    for result in Lexer::new(text) {
        let token = result.map_err(|e| anyhow::anyhow!("lex error: {e:?}"))?;
        match token.kind() {
            TokenKind::Whitespace | TokenKind::Comment | TokenKind::Eof => {}
            _ => tokens.push(token),
        }
    }
    Ok(tokens)
}

/// Returns the index just past the one Value production (spec grammar)
/// starting at `tokens[i]`: a scalar/enum-value token (`i + 1`), or — for
/// a list or input object literal — the index past its matching closing
/// bracket or brace, however deeply nested.
fn skip_value(tokens: &[Token], i: usize) -> usize {
    if i >= tokens.len() {
        return i;
    }
    match tokens[i].kind() {
        TokenKind::LBracket => skip_balanced(tokens, i, TokenKind::LBracket, TokenKind::RBracket),
        TokenKind::LCurly => skip_balanced(tokens, i, TokenKind::LCurly, TokenKind::RCurly),
        _ => i + 1,
    }
}

/// Returns the index just past the token that closes the `open`/`close`
/// pair starting at `tokens[i]` (itself an `open` token), counting nested
/// pairs of the same kind.
fn skip_balanced(tokens: &[Token], mut i: usize, open: TokenKind, close: TokenKind) -> usize {
    let mut depth = 0i32;
    while i < tokens.len() {
        let kind = tokens[i].kind();
        if kind == open {
            depth += 1;
        } else if kind == close {
            depth -= 1;
        }
        i += 1;
        if depth == 0 {
            break;
        }
    }
    i
}
