//! Ports `internal/graphql/lint/typename_prefix.go`'s `no-typename-prefix`
//! rule: forbids a field on an object or interface type from starting with
//! that type's own name, case-insensitively.

use apollo_compiler::Schema;
use apollo_compiler::schema::{ExtendedType, FieldDefinition, Name};
use gql_lint_core::{Violation, rules};

/// Runs the `no-typename-prefix` rule over `schema`.
///
/// Excludes built-in types the same way [`crate::alphabetize::alphabetize`]
/// does — see that function's doc comment for why this filter is load-
/// bearing, not decorative.
#[must_use]
pub fn no_typename_prefix(schema: &Schema) -> Vec<Violation> {
    let mut violations = Vec::new();

    for ty in gql_lint_core::in_scope_types(schema) {
        match ty {
            ExtendedType::Object(t) => {
                check_fields(schema, &mut violations, &t.name, t.fields.values());
            }
            ExtendedType::Interface(t) => {
                check_fields(schema, &mut violations, &t.name, t.fields.values());
            }
            _ => {}
        }
    }

    violations
}

fn check_fields<'a>(
    schema: &Schema,
    violations: &mut Vec<Violation>,
    type_name: &Name,
    fields: impl Iterator<Item = &'a apollo_compiler::schema::Component<FieldDefinition>>,
) {
    let lower_type_name = type_name.as_str().to_lowercase();
    for field in fields {
        if field
            .name
            .as_str()
            .to_lowercase()
            .starts_with(&lower_type_name)
        {
            let (file, line, column) =
                gql_lint_core::resolve_location(&schema.sources, field.name.location());
            violations.push(Violation {
                file,
                line,
                column,
                message: format!(
                    "Field \"{}\" starts with the name of the parent type \"{type_name}\"",
                    field.name
                ),
                rule: rules::NO_TYPENAME_PREFIX,
            });
        }
    }
}
