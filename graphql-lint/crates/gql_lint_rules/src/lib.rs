//! Tier 2 gap-fill and Tier 3 custom rules, run against a validated
//! [`Schema`] (Tier 1 has already passed by the time these run — see
//! `gql_lint_core::parse_and_validate`).
//!
//! Two rules are implemented so far: [`unique_directive_names_per_location`]
//! (Tier 2) and [`alphabetize::alphabetize`] (Tier 3). `possible-type-extension`,
//! the Go port's other Tier 2 rule, is not here — verified against a real
//! fixture to be a Tier 1 concern in this Rust port instead, since
//! `apollo-compiler`'s schema builder already rejects an orphan type
//! extension outright (`gql_lint_core`'s `classify` handles it). The
//! remaining 9 Tier 3 rules are the next slice of work — see the plan's
//! build order. Each new rule should follow the same shape: a free function
//! taking `&Schema` (plus whatever raw source text a Tier 3 rule needs for
//! trivia like `#` comments, not carried on the validated `Schema` itself)
//! and returning `Vec<Violation>`, so `gql_lint_cli` can call whichever
//! subset `scripts/devbase.yaml` enables without any of them needing to know
//! about each other — matching the independence (not the goroutine
//! mechanism) of the Go tool's `tier3` rules, each of which only reads the
//! shared parsed schema and writes its own result.

pub mod alphabetize;
pub mod deprecation;
pub mod description_style;
pub mod descriptions;
pub mod naming_convention;
pub mod no_case_insensitive_enum_values_duplicates;
pub mod no_hashtag_description;
pub mod no_typename_prefix;
pub mod no_unreachable_types;
pub mod require_description;

use apollo_compiler::Schema;
use apollo_compiler::schema::{Component, Directive, ExtendedType};
use apollo_compiler::validation::Valid;
use gql_lint_core::{Violation, rules};
use std::collections::HashSet;

/// Ports `internal/graphql/lint/directives.go`'s
/// `gapFillDirectivesPerLocation`: reports a violation for every
/// non-repeatable directive used more than once at one location (the
/// schema definition, or a single named type — `apollo-compiler`, like
/// `gqlparser`, already merges a type's base definition with all of its
/// extensions before validation, so iterating `schema.types` gives exactly
/// the grouping the Go rule reads from `parsed.schema.Types`).
///
/// Excludes built-in types via [`ExtendedType::is_built_in`], the same way
/// [`alphabetize::alphabetize`] does and for the same reason: a repo's own
/// directive usages are the only ones that should ever be reported, not
/// `apollo-compiler`'s pre-populated introspection schema (which carries
/// no directive usages in practice, but should not be trusted to stay
/// that way across a dependency upgrade without this filter).
#[must_use]
pub fn unique_directive_names_per_location(schema: &Valid<Schema>) -> Vec<Violation> {
    let mut violations = nonrepeatable_duplicates(schema, &schema.schema_definition.directives);

    for ty in schema.types.values().filter(|ty| !ty.is_built_in()) {
        let directives = type_directives(ty);
        violations.extend(nonrepeatable_duplicates(schema, directives));
    }

    violations
}

/// Returns `ty`'s own directive list, whichever [`ExtendedType`] variant it
/// is. `apollo-compiler` does not expose one generic accessor for this
/// across variants (only per-concrete-type struct fields), so this matches
/// once on the caller's behalf.
fn type_directives(ty: &ExtendedType) -> &[Component<Directive>] {
    match ty {
        ExtendedType::Scalar(t) => &t.directives.0,
        ExtendedType::Object(t) => &t.directives.0,
        ExtendedType::Interface(t) => &t.directives.0,
        ExtendedType::Union(t) => &t.directives.0,
        ExtendedType::Enum(t) => &t.directives.0,
        ExtendedType::InputObject(t) => &t.directives.0,
    }
}

/// Reports a violation for every directive in `directives` past the first
/// use of its name, whose definition (looked up in `schema`) is not
/// repeatable. `directives` is assumed to all come from one location, per
/// [`unique_directive_names_per_location`].
fn nonrepeatable_duplicates(
    schema: &Schema,
    directives: &[Component<Directive>],
) -> Vec<Violation> {
    if directives.len() < 2 {
        return Vec::new();
    }

    let mut seen: HashSet<&str> = HashSet::with_capacity(directives.len());
    directives
        .iter()
        .filter_map(|directive| {
            let def = schema.directive_definitions.get(directive.name.as_str())?;
            if def.repeatable {
                return None;
            }
            if !seen.insert(directive.name.as_str()) {
                let (file, line, column) =
                    gql_lint_core::resolve_location(&schema.sources, directive.name.location());
                return Some(Violation {
                    file,
                    line,
                    column,
                    message: format!(
                        "The directive \"@{}\" can only be used once at this location.",
                        directive.name
                    ),
                    rule: rules::UNIQUE_DIRECTIVE_NAMES_PER_LOCATION,
                });
            }
            None
        })
        .collect()
}
