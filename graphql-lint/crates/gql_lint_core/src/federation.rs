//! Ports `internal/graphql/lint/federation.go`: synthesizes the small SDL
//! prelude [`parse_and_validate`](crate::parse_and_validate) needs merged
//! in ahead of schema build when `scripts/devbase.yaml` sets
//! `graphql.lint.federation` and/or `graphql.lint.scalars`.
//!
//! A repository using Apollo Federation subgraphs brings spec directives
//! like `@key`/`@shareable` into scope via `extend schema
//! @link(url: ..., import: [...])`, never a `directive @...` declaration
//! of its own — `apollo-compiler` (like `gqlparser`) has no built-in
//! notion of `@link` or anything it imports, so a subgraph's own use of
//! `@key` would otherwise fail this port's existing `known-directives`
//! Tier 1 check the same way an actually-undeclared directive would. This
//! module resolves a repository's own `@link` import list and synthesizes
//! SDL definitions for just the directives it actually imports, honoring
//! an `as` rename — the same targeted fix `federation.go` makes, not a
//! general Federation implementation.
//!
//! **Checked directly against the `apollo-federation` crate (v2.17.0)
//! before writing this module, not assumed:** its
//! `link::federation_spec_definition` module contains the same kind of
//! per-directive SDL-construction logic, but every relevant item is
//! `pub(crate)` — not reachable from outside the crate at all (the one
//! plausible entry point, `Link::from_directive_application_when_link_spec_unknown`,
//! is itself `pub(crate)`). Its actual public surface is a full
//! subgraph/supergraph composition and query-planning pipeline, wildly
//! disproportionate to "render 9 known directive signatures as SDL text."
//! Porting the Go table directly is the correct choice here, not a
//! fallback taken for lack of trying the alternative.
//!
//! Directive signatures below come from the Apollo Federation subgraph
//! specification (`github.com/apollographql/federation`,
//! `docs/source/schema-design/federated-schemas/reference/subgraph-spec.mdx`),
//! the same source `federation.go`'s own table cites. Only the 9
//! directives named in [`FEDERATION_DIRECTIVES`] are supported, and only
//! Federation subgraph spec `v2.3` — matching the Go tool's scope exactly,
//! not extended, since parity is the point of this port.

use crate::Source;
use apollo_compiler::Name;
use apollo_compiler::ast::{Definition, Document, Value};

/// A `@link` import naming a directive this module cannot synthesize a
/// signature for, or a repository's `@link` naming a Federation version
/// other than the one `scripts/devbase.yaml` configures — mirrors
/// `federation.go`'s three sentinel errors, kept distinguishable so a
/// caller can match on which one occurred the way Go's
/// `errors.Is(err, ErrFederationVersionMismatch)` does.
#[derive(Debug, thiserror::Error)]
pub enum FederationError {
    #[error("unsupported federation version: {0}")]
    UnsupportedVersion(String),
    #[error(
        "federation version mismatch: scripts/devbase.yaml sets federation: {configured}, \
         but this links federation {found}"
    )]
    VersionMismatch { configured: String, found: String },
    #[error("imports @{0}, which devbase's federation prelude does not define")]
    UnsupportedDirective(String),
    #[error("parse schema for federation @link directives: {0}")]
    Parse(String),
}

/// Identifies a `@link` as linking against the Apollo Federation subgraph
/// spec itself, versus some other spec a schema might separately `@link`.
const FEDERATION_LINK_URL_PREFIX: &str = "https://specs.apollo.dev/federation/";

/// Federation subgraph spec versions this module ships a directive
/// prelude for — matching `federation.go`'s own `supportedFederationVersions`.
const SUPPORTED_FEDERATION_VERSIONS: &[&str] = &["v2.3"];

/// The 9 Federation subgraph directives this module knows how to
/// synthesize: (name, SDL with `$NAME` standing in for an `as` rename,
/// whether the SDL references the `FieldSet` scalar). Same 9 directives,
/// same signatures, same source citation as `federation.go`'s own
/// `federationDirectives` table.
const FEDERATION_DIRECTIVES: &[(&str, &str, bool)] = &[
    (
        "key",
        "directive @$NAME(fields: FieldSet!, resolvable: Boolean = true) repeatable on OBJECT | INTERFACE",
        true,
    ),
    (
        "shareable",
        "directive @$NAME repeatable on OBJECT | FIELD_DEFINITION",
        false,
    ),
    (
        "override",
        "directive @$NAME(from: String!) on FIELD_DEFINITION",
        false,
    ),
    (
        "inaccessible",
        "directive @$NAME on FIELD_DEFINITION | OBJECT | INTERFACE | UNION | ARGUMENT_DEFINITION \
         | SCALAR | ENUM | ENUM_VALUE | INPUT_OBJECT | INPUT_FIELD_DEFINITION",
        false,
    ),
    (
        "external",
        "directive @$NAME on FIELD_DEFINITION | OBJECT",
        false,
    ),
    (
        "requires",
        "directive @$NAME(fields: FieldSet!) on FIELD_DEFINITION",
        true,
    ),
    (
        "provides",
        "directive @$NAME(fields: FieldSet!) on FIELD_DEFINITION",
        true,
    ),
    (
        "tag",
        "directive @$NAME(name: String!) repeatable on FIELD_DEFINITION | INTERFACE | OBJECT | UNION \
         | ARGUMENT_DEFINITION | SCALAR | ENUM | ENUM_VALUE | INPUT_OBJECT | INPUT_FIELD_DEFINITION",
        false,
    ),
    ("extends", "directive @$NAME on OBJECT | INTERFACE", false),
];

/// One entry from a `@link` import list, resolved to the directive name
/// it refers to and the name it is imported as (equal to the name unless
/// renamed with `as`).
struct Import {
    name: String,
    alias: String,
}

/// Builds the extra [`Source`]s [`crate::parse_and_validate`] should merge
/// in ahead of schema build, for `scripts/devbase.yaml`'s
/// `graphql.lint.federation`/`graphql.lint.scalars` settings. Returns an
/// empty `Vec` if `federation` is `None` and `scalars` is empty —
/// matching `preludeSources`'s no-op case.
///
/// # Errors
/// See [`FederationError`]'s variants.
pub fn prelude_sources(
    sources: &[Source],
    federation: Option<&str>,
    scalars: &[String],
) -> Result<Vec<Source>, FederationError> {
    let mut extra = Vec::new();

    if let Some(version) = federation {
        let prelude = federation_prelude(sources, version)?;
        if !prelude.is_empty() {
            extra.push(Source {
                name: "<federation prelude>".to_string(),
                text: prelude,
            });
        }
    }

    if !scalars.is_empty() {
        extra.push(Source {
            name: "<scalars prelude>".to_string(),
            text: scalars_prelude(scalars),
        });
    }

    Ok(extra)
}

/// Renders `names` as one `scalar X` declaration per line — unconditional,
/// no `@link` involved, matching `federation.go`'s `scalarsPrelude`.
fn scalars_prelude(names: &[String]) -> String {
    use std::fmt::Write as _;

    names.iter().fold(String::new(), |mut prelude, name| {
        let _ = writeln!(prelude, "scalar {name}");
        prelude
    })
}

/// Parses `sources`' raw `extend schema @link(...)` directives and
/// returns the SDL to merge in for `want_version`. Returns `""` if
/// `sources` contain no `@link` to the Federation subgraph spec —
/// `scripts/devbase.yaml` opted in, but nothing in this schema uses it
/// yet, matching Go's same no-op case.
fn federation_prelude(sources: &[Source], want_version: &str) -> Result<String, FederationError> {
    if !SUPPORTED_FEDERATION_VERSIONS.contains(&want_version) {
        return Err(FederationError::UnsupportedVersion(
            want_version.to_string(),
        ));
    }

    // Only sources containing the substring "link" are parsed here: a
    // `@link(...)` application cannot appear without it. This avoids a
    // syntax-only re-parse of every file in a large repo that never
    // mentions federation at all — the same prefilter federation.go uses
    // (`sourcesContaining`).
    let candidates: Vec<&Source> = sources.iter().filter(|s| s.text.contains("link")).collect();
    if candidates.is_empty() {
        return Ok(String::new());
    }

    let mut needs_field_set = false;
    let mut directive_defs = Vec::new();
    let mut seen_link = false;

    for source in candidates {
        // Syntax-only parse (no schema validation) — exactly what's needed
        // to walk `extend schema @link(...)` applications; a `Schema`
        // gives no way back to a not-yet-declared directive's raw
        // arguments the way this needs.
        let doc = Document::parse(&source.text, &source.name)
            .map_err(|e| FederationError::Parse(e.errors.to_string()))?;

        for definition in &doc.definitions {
            let Definition::SchemaExtension(schema_extension) = definition else {
                continue;
            };
            for directive in schema_extension.directives.get_all("link") {
                let Some(got_version) = federation_link_version(directive) else {
                    continue; // links to some other, unrelated spec.
                };
                if got_version != want_version {
                    return Err(FederationError::VersionMismatch {
                        configured: want_version.to_string(),
                        found: got_version,
                    });
                }
                seen_link = true;

                for import in federation_link_imports(directive) {
                    let Some(&(_, sdl, needs_fs)) = FEDERATION_DIRECTIVES
                        .iter()
                        .find(|(name, _, _)| *name == import.name)
                    else {
                        return Err(FederationError::UnsupportedDirective(import.name));
                    };
                    directive_defs.push(sdl.replace("$NAME", &import.alias));
                    needs_field_set |= needs_fs;
                }
            }
        }
    }

    if !seen_link {
        return Ok(String::new());
    }

    let mut prelude = String::from(
        "directive @link(url: String!, as: String, for: link__Purpose, import: [link__Import]) \
         repeatable on SCHEMA\n\
         scalar link__Import\n\
         enum link__Purpose { SECURITY EXECUTION }\n",
    );
    if needs_field_set {
        prelude.push_str("scalar FieldSet\n");
    }
    for def in directive_defs {
        prelude.push_str(&def);
        prelude.push('\n');
    }
    Ok(prelude)
}

/// Extracts the Federation subgraph spec version named by `directive`'s
/// `url` argument, or `None` if `url` doesn't reference the Federation
/// spec (a schema may separately `@link` some other spec).
fn federation_link_version(directive: &apollo_compiler::ast::Directive) -> Option<String> {
    let url_arg = directive.arguments.iter().find(|a| a.name == "url")?;
    let Value::String(url) = &*url_arg.value else {
        return None;
    };
    url.strip_prefix(FEDERATION_LINK_URL_PREFIX)
        .map(str::to_string)
}

/// Extracts `directive`'s `import` list, if any, as the directive imports
/// it names. An entry that can't be classified as a directive import
/// (naming a type, not a directive, or a malformed shape) is silently
/// skipped rather than erroring — `federation.go` only classifies `@name`
/// entries as directive imports too, but never actually needs its own
/// error path for the string/object shapes it does recognize; the
/// `FederationError::UnsupportedDirective` case is reserved for a
/// well-formed directive import naming a directive this table doesn't
/// define, not a malformed import entry.
fn federation_link_imports(directive: &apollo_compiler::ast::Directive) -> Vec<Import> {
    let Some(import_arg) = directive.arguments.iter().find(|a| a.name == "import") else {
        return Vec::new();
    };
    let Value::List(entries) = &*import_arg.value else {
        return Vec::new();
    };

    entries
        .iter()
        .filter_map(|entry| parse_import_entry(entry))
        .collect()
}

/// Classifies one `@link` import-list entry as a directive import: either
/// a bare `"@name"` string, or a `{name: "@name", as: "@alias"}` object.
/// Returns `None` for an entry naming a type (no leading `@`) or any other
/// shape this doesn't recognize.
fn parse_import_entry(entry: &Value) -> Option<Import> {
    match entry {
        Value::String(s) => {
            let name = s.strip_prefix('@')?.to_string();
            Some(Import {
                alias: name.clone(),
                name,
            })
        }
        Value::Object(fields) => {
            let name = object_field_str(fields, "name")?
                .strip_prefix('@')?
                .to_string();
            let alias = object_field_str(fields, "as")
                .and_then(|s| s.strip_prefix('@'))
                .map_or_else(|| name.clone(), str::to_string);
            Some(Import { name, alias })
        }
        _ => None,
    }
}

/// Reads `key`'s string value out of an `ObjectValue`'s field list.
fn object_field_str<'a>(
    fields: &'a [(Name, apollo_compiler::Node<Value>)],
    key: &str,
) -> Option<&'a str> {
    fields
        .iter()
        .find(|(name, _)| name.as_str() == key)
        .and_then(|(_, value)| match &**value {
            Value::String(s) => Some(s.as_str()),
            _ => None,
        })
}
