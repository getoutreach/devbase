//! Ports `internal/graphql/lint/case_insensitive_enum_values_duplicates.go`'s
//! `no-case-insensitive-enum-values-duplicates` rule: forbids two values on
//! the same enum whose names differ only by casing.
//!
//! Like the Go port, this benefits from `apollo-compiler` folding a type
//! extension's enum values into the base definition's before validation:
//! a duplicate split across a base enum and its extension (or across two
//! extensions in different files) is caught the same as one written in a
//! single enum block, which `@graphql-eslint` itself cannot see, since it
//! lints one file's own AST at a time.

use apollo_compiler::Schema;
use apollo_compiler::schema::ExtendedType;
use gql_lint_core::{Violation, rules};
use std::collections::HashSet;

/// Runs the `no-case-insensitive-enum-values-duplicates` rule over `schema`.
///
/// Excludes built-in types the same way [`crate::alphabetize::alphabetize`]
/// does.
#[must_use]
pub fn no_case_insensitive_enum_values_duplicates(schema: &Schema) -> Vec<Violation> {
    let mut violations = Vec::new();

    for ty in gql_lint_core::in_scope_types(schema) {
        let ExtendedType::Enum(enum_type) = ty else {
            continue;
        };

        let mut seen_lower: HashSet<String> = HashSet::with_capacity(enum_type.values.len());
        for value in enum_type.values.keys() {
            if !seen_lower.insert(value.as_str().to_lowercase()) {
                let (file, line, column) =
                    gql_lint_core::resolve_location(&schema.sources, value.location());
                violations.push(Violation {
                    file,
                    line,
                    column,
                    message: format!(
                        "Case-insensitive enum values duplicates are not allowed! Found: `{value}`."
                    ),
                    rule: rules::NO_CASE_INSENSITIVE_ENUM_VALUES_DUPLICATES,
                });
            }
        }
    }

    violations
}
