//! Shared vocabulary and Tier 1 (spec-validation) support for `gql-lint`.
//!
//! Mirrors `devbase`'s Go implementation at
//! `internal/graphql/{config,lint}` rule-for-rule, not necessarily
//! line-for-line: see this crate's `rules` module for the 22 rule names,
//! grouped into the same three tiers the Go version settled on (10 / 2 / 10,
//! not the RFC's original 9 / 3 / 10 — `unique-enum-value-names` moved into
//! Tier 1 once the shipped Go tool found `gqlparser` already rejects a
//! duplicate enum value on its own).

pub mod rules;

use apollo_compiler::Schema;
use apollo_compiler::diagnostic::ToCliReport;
use apollo_compiler::parser::{FileId, SourceMap, SourceSpan};
use apollo_compiler::validation::Valid;
use std::fmt;

/// A single lint finding, formatted as `<file>:<line>:<col>: <message> [<rule>]`
/// — the exact format `devbase`'s Go tool uses, so output stays
/// drop-in-comparable between the two implementations.
#[derive(Debug, Clone)]
pub struct Violation {
    pub file: String,
    pub line: usize,
    pub column: usize,
    pub message: String,
    pub rule: &'static str,
}

impl fmt::Display for Violation {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "{}:{}:{}: {} [{}]",
            self.file, self.line, self.column, self.message, self.rule
        )
    }
}

/// Resolves `location` against `sources` into `(file, line, column)`,
/// ready to drop straight into a [`Violation`]. Every rule that has a
/// `&Schema` (or `&Valid<Schema>`, which derefs to it) in scope can call
/// this with `&schema.sources` and any `Name`/`Node<T>`'s own
/// `.location()`. Returns all-empty/zero if `location` is `None` (a
/// synthetic node with no source position) or if `sources` doesn't
/// recognize the location's file — this should not happen in practice,
/// but a missing position is a worse failure mode than a panic here.
#[must_use]
pub fn resolve_location(
    sources: &SourceMap,
    location: Option<SourceSpan>,
) -> (String, usize, usize) {
    let Some(location) = location else {
        return (String::new(), 0, 0);
    };
    let Some(file) = sources.get(&location.file_id()) else {
        return (String::new(), 0, 0);
    };
    let path = file.path().display().to_string();
    match location.line_column(sources) {
        Some(lc) => (path, lc.line, lc.column),
        None => (path, 0, 0),
    }
}

/// Resolves a raw byte offset within `file_id`'s own source text into
/// `(line, column)`, for a site that has no [`SourceSpan`] to give
/// [`resolve_location`] — a raw `apollo_parser::Token::index()` offset from
/// a fresh re-lex, as `description_style` and `no_hashtag_description` both
/// need. `SourceFile::get_line_column` already does exactly this
/// conversion (and already counts `column` by Unicode Scalar Value, not
/// byte, matching [`apollo_compiler::parser::LineColumn`]'s convention), so
/// this is a thin, panic-free wrapper rather than a hand-rolled line
/// scanner. Returns `(0, 0)` if `sources` doesn't recognize `file_id` or
/// `offset` is out of bounds.
#[must_use]
pub fn line_column_at(sources: &SourceMap, file_id: FileId, offset: usize) -> (usize, usize) {
    sources
        .get(&file_id)
        .and_then(|file| file.get_line_column(offset))
        .map_or((0, 0), |lc| (lc.line, lc.column))
}

/// Tag used for a schema-build error `apollo-compiler` raises that doesn't
/// map to one of the 10 named Tier 1 rules below — the Rust equivalent of
/// the Go tool's `UnclassifiedRule = "gqlparser"` constant.
pub const UNCLASSIFIED_RULE: &str = "apollo-compiler";

/// Reports whether a rule tagging a [`Tier1Result::Violations`] entry
/// always runs, regardless of `scripts/devbase.yaml` — true for every
/// genuine Tier 1 rule, false for `possible-type-extension`. That one
/// comes out of the same build phase (see `classify`'s doc comment) but
/// stays a user-configurable, off-by-default rule in the Go tool it is
/// meant to match: a caller should suppress it here when
/// `scripts/devbase.yaml` has it disabled, the same way it filters a
/// genuine Tier 2/3 rule, rather than always failing every repo with an
/// orphan extension `gqlparser` used to accept silently.
#[must_use]
pub fn is_always_on(rule: &str) -> bool {
    rule != rules::POSSIBLE_TYPE_EXTENSION
}

/// One `.graphql` file's path and content, ready to hand to
/// [`parse_and_validate`]. Mirrors `gqlparser.ast.Source` / the Go tool's
/// `[]*ast.Source` — a (name, text) pair, nothing more.
pub struct Source {
    pub name: String,
    pub text: String,
}

/// The result of running Tier 1 validation over a combined set of sources.
pub enum Tier1Result {
    /// The combined schema parsed and validated cleanly. Carries the
    /// [`Valid<Schema>`] Tier 2/3 rules need.
    Valid(Valid<Schema>),
    /// At least one Tier 1 violation was found.
    ///
    /// **Deliberate divergence from the Go tool, not an oversight:**
    /// `gqlparser`'s schema validation stops at the first spec violation it
    /// finds, so the Go tool can only ever report one Tier 1 violation per
    /// run (see `internal/graphql/lint/lint.go`'s package doc comment).
    /// `apollo-compiler`'s `Schema::parse_and_validate` collects a whole
    /// `DiagnosticList` instead — confirmed by reading
    /// `apollo-compiler-1.32.0/src/validation/mod.rs`'s `WithErrors<T>` and
    /// `DiagnosticList` directly, not assumed. This variant therefore
    /// carries every Tier 1 violation found, not just the first. Decide at
    /// the CLI layer whether to truncate to one (for output parity with the
    /// Go tool) or keep all of them (a real improvement) — this crate
    /// reports what apollo-compiler actually found either way.
    Violations(Vec<Violation>),
}

/// Parses and validates `sources` as one combined GraphQL schema — the
/// Rust equivalent of the Go tool's `lint.FilesFromSources` Tier 1 step
/// (`internal/graphql/lint/lint.go`'s `parseAndValidate`).
///
/// Unlike the Go version, this does not yet merge a federation- or
/// scalars-synthesized prelude (`internal/graphql/lint/federation.go`'s
/// job) — federation support is scoped separately; see the plan.
#[must_use]
pub fn parse_and_validate(sources: &[Source]) -> Tier1Result {
    // apollo-compiler's `Schema::parse_and_validate` takes one source string
    // plus a path; to validate multiple files together (so a type defined
    // in one file resolves when referenced from another, matching the Go
    // tool's combined-schema behavior) go through `Schema::builder()` and
    // add each file individually instead of concatenating source text
    // ourselves, which would corrupt reported line/column numbers.
    let mut builder = Schema::builder();
    for source in sources {
        builder = builder.parse(&source.text, &source.name);
    }

    let schema = match builder.build() {
        Ok(schema) => schema,
        Err(with_errors) => return Tier1Result::Violations(violations_from(&with_errors)),
    };

    match schema.validate() {
        Ok(valid) => Tier1Result::Valid(valid),
        Err(with_errors) => Tier1Result::Violations(violations_from(&with_errors)),
    }
}

/// Converts every diagnostic in `with_errors` into a [`Violation`], via
/// [`classify`].
fn violations_from<T>(with_errors: &apollo_compiler::validation::WithErrors<T>) -> Vec<Violation> {
    with_errors
        .errors
        .iter()
        .map(|diagnostic| {
            classify(
                &diagnostic.error.to_string(),
                diagnostic.sources,
                diagnostic.error.location(),
            )
        })
        .collect()
}

/// Classifies a rendered `apollo-compiler` diagnostic message into one of
/// the 10 Tier 1 rule names, falling back to [`UNCLASSIFIED_RULE`].
///
/// **This is message-text matching, the same fragile approach the Go tool
/// uses (`internal/graphql/lint/lint.go`'s `ruleForMessage`) — not an
/// improvement over it.** `apollo-compiler` does have a structured
/// `DiagnosticData`/`BuildError` enum with named variants that would
/// classify schema-build errors (`SchemaDefinitionCollision`,
/// `DirectiveDefinitionCollision`, `TypeDefinitionCollision`,
/// `OrphanTypeExtension`, `DuplicateRootOperation`, and more — read
/// directly from `apollo-compiler-1.32.0/src/schema/mod.rs`'s `BuildError`),
/// but that enum is `pub(crate)`, not part of the public API. Its public
/// `unstable_error_name()` accessor, which would have been a robust
/// alternative to string matching, explicitly does not cover schema-build
/// errors either (verified by reading its match arms in
/// `validation/mod.rs`: only `ExecutableBuildError` and one `RecursionLimitError`
/// case return `Some(..)`; the `SchemaBuildError` case falls through to
/// `_ => None`). So, for this specific need, string matching against a
/// pinned `apollo-compiler` version is the only externally-visible option
/// — match the Go tool's own tactic of pinning the dependency to an exact
/// version for this reason, and re-verify these patterns on every upgrade
/// the same way `lint_test.go` does for `gqlparser`.
///
/// The patterns below are **verified against real `apollo-compiler` 1.32.0
/// output**, not inferred: each was captured by running this crate's CLI
/// against a small hand-written fixture per case (see the plan's Tier 1
/// acceptance-check step). 8 of the 10 Tier 1 rules are covered this way.
/// Two are not yet verified against a real fixture:
/// `unique-operation-types` (a within-one-`schema{}`-block duplicate root
/// type — the same case the Go tool's own `ruleForMessage` has no mapping
/// for either, per this plan's "Corrections" section) and any input-object-
/// or union-member collision variant of the "defined multiple times"
/// family (only the object-field and enum-value forms were captured).
/// Extend the match arms below once those are exercised, rather than
/// guessing their shape.
///
/// One entry here is not from the RFC/Go tool's own Tier 1 set at all:
/// `possible-type-extension`. In the Go tool it's a Tier 2 gap-fill rule,
/// off by default, because `gqlparser` silently synthesizes an empty
/// placeholder type for an orphan `extend type Foo { ... }` instead of
/// erroring. Verified on a real fixture that `apollo-compiler` does *not*
/// do this — it rejects an orphan extension outright, unconditionally,
/// during schema build. That makes this rule a Tier 1 concern in this
/// port, not Tier 2: it always runs and can't be turned off, the same as
/// every other name in [`rules::TIER1_RULES`]. See [`is_always_on`] for
/// where this distinction actually matters to a caller.
fn classify(message: &str, sources: &SourceMap, location: Option<SourceSpan>) -> Violation {
    let rule = if message.starts_with("must not have multiple `schema` definitions") {
        rules::LONE_SCHEMA_DEFINITION
    } else if message.starts_with("the directive ") && message.contains("is defined multiple times")
    {
        rules::UNIQUE_DIRECTIVE_NAMES
    } else if message.starts_with("the type ") && message.contains("is defined multiple times") {
        rules::UNIQUE_TYPE_NAMES
    } else if message.starts_with("duplicate definitions for the ")
        && message.contains(" field of ")
    {
        rules::UNIQUE_FIELD_DEFINITION_NAMES
    } else if message.starts_with("duplicate definitions for the ")
        && message.contains(" value of enum type ")
    {
        rules::UNIQUE_ENUM_VALUE_NAMES
    } else if message.starts_with("the argument ") && message.contains(" is not supported by ") {
        rules::KNOWN_ARGUMENT_NAMES
    } else if message.starts_with("cannot find directive ") {
        rules::KNOWN_DIRECTIVES
    } else if message.starts_with("cannot find type ") {
        rules::KNOWN_TYPE_NAMES
    } else if message.starts_with("the required argument ") && message.contains(" is not provided")
    {
        rules::PROVIDED_REQUIRED_ARGUMENTS
    } else if message.starts_with("type extension for undefined type ") {
        rules::POSSIBLE_TYPE_EXTENSION
    } else {
        UNCLASSIFIED_RULE
    };

    let (file, line, column) = resolve_location(sources, location);
    Violation {
        file,
        line,
        column,
        message: message.to_string(),
        rule,
    }
}
