//! Shared `DescriptionSite`/`collect_description_sites` machinery for
//! `description_style` and `no_hashtag_description` — the Rust port of
//! `internal/graphql/lint/descriptions.go`'s `descriptionSite`/
//! `groupDescriptionSites`. `require_description` does not need this: it
//! only needs whether a description is present, which
//! `apollo_compiler::Schema` gives directly (see that module's own doc
//! comment for why).
//!
//! Unlike every rule ported before it, this module resolves **real
//! per-site file and byte-offset position** via `apollo_compiler`'s
//! `Node::location()`/`Name::location()` plus `schema.sources`. It has to:
//! `description_style` and `no_hashtag_description` both need to re-lex
//! each site's own source file with `apollo_parser::Lexer` and pair raw
//! tokens back up with schema-level sites *in file order*, so "which
//! file, at what byte offset" is load-bearing here, not a cosmetic detail
//! deferred like every other rule's `Violation.file`/`.line`/`.column`
//! TODO.

use apollo_compiler::Schema;
use apollo_compiler::parser::FileId;
use apollo_compiler::schema::ExtendedType;
use std::path::Path;

/// One node in a schema that can carry a description: a type definition,
/// field, argument, enum value, or directive definition.
pub struct DescriptionSite<'a> {
    pub description: Option<&'a str>,
    pub label: String,
    pub file: &'a Path,
    /// This site's own file, as a [`FileId`] rather than just a path —
    /// lets a caller resolve any other raw byte offset from the same file
    /// (for example a re-lexed comment token's own offset) back to a real
    /// line/column via `gql_lint_core::line_column_at`, without needing a
    /// [`apollo_compiler::parser::SourceSpan`] for that offset.
    pub file_id: FileId,
    /// Byte offset of this site's own name (or, for a directive
    /// definition, effectively the same anchor point) — used both to
    /// order sites within a file and as the point `no_hashtag_description`
    /// scans backward from.
    pub offset: usize,
}

/// Pushes a [`DescriptionSite`] onto `sites`, resolving `location` to a
/// file and byte offset via `schema.sources` — or doing nothing if
/// `location` is absent (a synthetic/introspection node, for example an
/// injected `__schema` meta-field, never written in any file's SDL, so
/// there is nothing to check) or if it belongs to a synthesized prelude
/// source (`gql_lint_core::federation::prelude_sources`'
/// `<federation prelude>`/`<scalars prelude>`, by the same `<...>`
/// bracketed-name convention `federation.go` itself uses for the same
/// purpose) rather than a real on-disk file — this module's two callers,
/// `description_style` and `no_hashtag_description`, both re-read a
/// site's own file from disk to recover raw source text/trivia the
/// validated `Schema` doesn't carry, and a synthesized prelude has no
/// file to read. Silently skipping these sites here, at the one place
/// both rules' sites are collected, is far safer than letting each rule
/// hit a file-not-found error on its own: a prelude's own directive/scalar
/// declarations never carry hand-written descriptions or `#` comments
/// worth checking anyway, so nothing real is lost, and skipping here
/// keeps the position-ordered, per-real-file grouping both rules depend
/// on from ever containing a group with no file to read — a group that
/// used to fail their whole run (bailing out through a `HashMap`'s
/// unordered iteration, silently dropping violations for whichever real
/// files happened to be visited after it) rather than just that one
/// synthetic group.
///
/// A free function, not a closure capturing `sites`: a closure here hits
/// a real borrow-checker limitation (mutable references are invariant, so
/// a closure capturing `&mut Vec<DescriptionSite<'a>>` can't be called
/// with a fresh per-call lifetime for `description` the way this needs).
fn push_site<'a>(
    schema: &'a Schema,
    sites: &mut Vec<DescriptionSite<'a>>,
    description: Option<&'a str>,
    label: String,
    location: Option<apollo_compiler::parser::SourceSpan>,
) {
    let Some(location) = location else { return };
    let Some(file) = schema.sources.get(&location.file_id()) else {
        return;
    };
    if file.path().to_str().is_some_and(|p| p.starts_with('<')) {
        return;
    }
    sites.push(DescriptionSite {
        description,
        label,
        file: file.path(),
        file_id: location.file_id(),
        offset: location.offset(),
    });
}

/// Collects every [`DescriptionSite`] in `schema`, grouped by the file it
/// was written in and sorted by byte offset within each group — the same
/// per-file, position order `descriptionTokens` (in `description_style`)
/// and a raw re-lex (in `no_hashtag_description`) both need to pair up
/// against. Excludes built-in types, same as every other rule in this
/// crate.
#[must_use]
pub fn collect_description_sites(schema: &Schema) -> Vec<(&Path, Vec<DescriptionSite<'_>>)> {
    let mut sites: Vec<DescriptionSite<'_>> = Vec::new();
    collect_type_sites(schema, &mut sites);
    collect_directive_sites(schema, &mut sites);
    group_by_file(sites)
}

/// Collects every type definition's own site, plus its fields, field
/// arguments, and enum values, into `sites`.
fn collect_type_sites<'a>(schema: &'a Schema, sites: &mut Vec<DescriptionSite<'a>>) {
    for ty in gql_lint_core::in_scope_types(schema) {
        let (kind_label, ty_name) = match ty {
            ExtendedType::Scalar(t) => ("scalar", &t.name),
            ExtendedType::Object(t) => ("type", &t.name),
            ExtendedType::Interface(t) => ("interface", &t.name),
            ExtendedType::Union(t) => ("union", &t.name),
            ExtendedType::Enum(t) => ("enum", &t.name),
            ExtendedType::InputObject(t) => ("input", &t.name),
        };
        push_site(
            schema,
            sites,
            ty.description().map(|d| &**d),
            format!("{kind_label} \"{ty_name}\""),
            ty_name.location(),
        );

        let object_or_interface_fields = match ty {
            ExtendedType::Object(t) => Some(&t.fields),
            ExtendedType::Interface(t) => Some(&t.fields),
            _ => None,
        };
        if let Some(fields) = object_or_interface_fields {
            for field in fields.values() {
                push_site(
                    schema,
                    sites,
                    field.description.as_deref(),
                    format!("field \"{}\" in {kind_label} \"{ty_name}\"", field.name),
                    field.name.location(),
                );
                for arg in &field.arguments {
                    push_site(
                        schema,
                        sites,
                        arg.description.as_deref(),
                        format!("input value \"{}\" in field \"{}\"", arg.name, field.name),
                        arg.name.location(),
                    );
                }
            }
        }

        match ty {
            ExtendedType::InputObject(t) => {
                for field in t.fields.values() {
                    push_site(
                        schema,
                        sites,
                        field.description.as_deref(),
                        format!("input value \"{}\" in input \"{ty_name}\"", field.name),
                        field.name.location(),
                    );
                }
            }
            ExtendedType::Enum(t) => {
                for value in t.values.values() {
                    push_site(
                        schema,
                        sites,
                        value.description.as_deref(),
                        format!("enum value \"{}\" in enum \"{ty_name}\"", value.value),
                        value.value.location(),
                    );
                }
            }
            ExtendedType::Object(_)
            | ExtendedType::Interface(_)
            | ExtendedType::Union(_)
            | ExtendedType::Scalar(_) => {}
        }
    }
}

/// Collects every non-built-in directive definition's own site, plus its
/// arguments, into `sites`.
fn collect_directive_sites<'a>(schema: &'a Schema, sites: &mut Vec<DescriptionSite<'a>>) {
    for directive in gql_lint_core::in_scope_directive_definitions(schema) {
        push_site(
            schema,
            sites,
            directive.description.as_deref(),
            format!("directive \"{}\"", directive.name),
            directive.name.location(),
        );
        for arg in &directive.arguments {
            push_site(
                schema,
                sites,
                arg.description.as_deref(),
                format!(
                    "input value \"{}\" in directive \"{}\"",
                    arg.name, directive.name
                ),
                arg.name.location(),
            );
        }
    }
}

/// Groups `sites` by file, sorting each group by byte offset.
fn group_by_file<'a>(sites: Vec<DescriptionSite<'a>>) -> Vec<(&'a Path, Vec<DescriptionSite<'a>>)> {
    let mut by_file: std::collections::HashMap<&Path, Vec<DescriptionSite<'a>>> =
        std::collections::HashMap::new();
    for site in sites {
        by_file.entry(site.file).or_default().push(site);
    }

    let mut grouped: Vec<(&Path, Vec<DescriptionSite<'_>>)> = by_file.into_iter().collect();
    for (_, group) in &mut grouped {
        group.sort_by_key(|s| s.offset);
    }
    grouped
}
