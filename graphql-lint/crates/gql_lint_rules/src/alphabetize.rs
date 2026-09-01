//! Ports `internal/graphql/lint/alphabetize.go`'s `alphabetize` rule:
//! requires alphabetical order among the fields, enum values, and/or
//! arguments its options select.
//!
//! Matches the Go port's own narrowing of `@graphql-eslint`'s option
//! surface to schema-only kinds (`ObjectTypeDefinition`,
//! `InterfaceTypeDefinition`, `InputObjectTypeDefinition` for `fields`;
//! `EnumTypeDefinition` for `values`; field and directive arguments for
//! `arguments`) — this tool only ever lints schema files, never operations,
//! so the operation-only and v4-only options the Go port already dropped
//! stay dropped here too.
//!
//! One difference from the Go port worth calling out, not hiding: the Go
//! version orders a type's base definition and every extension of it
//! together in one alphabetical run, because `gqlparser` merges them
//! before validation. `apollo-compiler` does the same folding (each
//! `ExtendedType` variant's doc comment says so explicitly: "with all
//! information from type extensions folded in"), so this port gets the
//! same merged-run behavior for free, not as separately-ported logic.

use apollo_compiler::Schema;
use apollo_compiler::schema::{ExtendedType, Name};
use gql_lint_core::{Violation, rules};

/// Which of `fields`'s three schema-only kinds are selected. Split out of
/// [`AlphabetizeOptions`] into its own small group (clippy's
/// `struct_excessive_bools` pedantic lint rejects a flat struct with more
/// than 3 bare `bool` fields; grouping by the option key they came from is
/// also just a clearer shape than 6 flat, same-typed fields).
#[derive(Debug, Clone, Copy, Default)]
pub struct FieldKinds {
    pub object: bool,
    pub interface: bool,
    pub input_object: bool,
}

/// Which of `arguments`'s two kinds are selected.
#[derive(Debug, Clone, Copy, Default)]
pub struct ArgumentKinds {
    pub field: bool,
    pub directive: bool,
}

/// Which of `alphabetize`'s options are enabled, decoded from
/// `scripts/devbase.yaml`'s `graphql.lint.rules.alphabetize` options —
/// mirrors the Go port's `alphabetizeOptions`.
#[derive(Debug, Clone, Copy, Default)]
pub struct AlphabetizeOptions {
    pub fields: FieldKinds,
    pub enum_values: bool,
    pub arguments: ArgumentKinds,
}

impl AlphabetizeOptions {
    /// Decodes options from the raw `{fields: [...], values: [...],
    /// arguments: [...]}` mapping `scripts/devbase.yaml` provides, the same
    /// shape the Go port's `parseAlphabetizeOptions` reads.
    #[must_use]
    pub fn from_yaml(options: Option<&serde_yaml::Mapping>) -> Self {
        let Some(options) = options else {
            return Self::default();
        };

        let mut opts = Self::default();
        for key in string_list(options, "fields") {
            match key {
                "ObjectTypeDefinition" => opts.fields.object = true,
                "InterfaceTypeDefinition" => opts.fields.interface = true,
                "InputObjectTypeDefinition" => opts.fields.input_object = true,
                _ => {}
            }
        }
        for key in string_list(options, "values") {
            if key == "EnumTypeDefinition" {
                opts.enum_values = true;
            }
        }
        for key in string_list(options, "arguments") {
            match key {
                "FieldDefinition" => opts.arguments.field = true,
                "DirectiveDefinition" => opts.arguments.directive = true,
                _ => {}
            }
        }
        opts
    }
}

fn string_list<'a>(options: &'a serde_yaml::Mapping, key: &str) -> Vec<&'a str> {
    options
        .get(key)
        .and_then(serde_yaml::Value::as_sequence)
        .map(|seq| seq.iter().filter_map(serde_yaml::Value::as_str).collect())
        .unwrap_or_default()
}

/// Runs the `alphabetize` rule over `schema`, per `opts`.
///
/// `fields` and `arguments` are independent options, matching the Go
/// port's own structure (two separate `if` conditions, not one nested
/// inside the other) — a repo can enable `arguments: [FieldDefinition]`
/// without also enabling `fields: [ObjectTypeDefinition]`, and each
/// object/interface type's field order and its fields' argument order
/// must be checked independently, not gated on each other.
///
/// **Caught by running this against a real fixture, not by inspection:**
/// `Schema::builder()` pre-populates built-in scalars and the reserved
/// `__`-prefixed introspection types (`__Schema`, `__Type`, `__Field`, and
/// so on). An early version of this function walked `schema.types`
/// without excluding them and reported dozens of spurious violations
/// against GraphQL's own introspection schema, alongside the one real
/// violation in the fixture. The Go port avoids this by restricting every
/// Tier 3 walk to `inScope`, definitions whose source file identity
/// matches one of the repository's own `*.graphql` files
/// (`internal/graphql/lint/tier3.go`). `apollo-compiler` exposes a
/// simpler, direct equivalent: [`ExtendedType::is_built_in`].
#[must_use]
pub fn alphabetize(schema: &Schema, opts: &AlphabetizeOptions) -> Vec<Violation> {
    let mut violations = Vec::new();

    for ty in schema.types.values().filter(|ty| !ty.is_built_in()) {
        match ty {
            ExtendedType::Object(t) => {
                if opts.fields.object {
                    check_order(schema, &mut violations, t.fields.keys());
                }
                if opts.arguments.field {
                    for field in t.fields.values() {
                        check_order(
                            schema,
                            &mut violations,
                            field.arguments.iter().map(|a| &a.name),
                        );
                    }
                }
            }
            ExtendedType::Interface(t) => {
                if opts.fields.interface {
                    check_order(schema, &mut violations, t.fields.keys());
                }
                if opts.arguments.field {
                    for field in t.fields.values() {
                        check_order(
                            schema,
                            &mut violations,
                            field.arguments.iter().map(|a| &a.name),
                        );
                    }
                }
            }
            ExtendedType::InputObject(t) if opts.fields.input_object => {
                check_order(schema, &mut violations, t.fields.keys());
            }
            ExtendedType::Enum(t) if opts.enum_values => {
                check_order(schema, &mut violations, t.values.keys());
            }
            _ => {}
        }
    }

    if opts.arguments.directive {
        for directive in schema
            .directive_definitions
            .values()
            .filter(|d| !d.is_built_in())
        {
            check_order(
                schema,
                &mut violations,
                directive.arguments.iter().map(|a| &a.name),
            );
        }
    }

    violations
}

/// Reports a violation for every name in `names` that sorts after its
/// successor per [`locale_compare`] — the Rust port of the Go rule's
/// `checkAlphaOrder`.
fn check_order<'a>(
    schema: &Schema,
    violations: &mut Vec<Violation>,
    names: impl Iterator<Item = &'a Name>,
) {
    let mut prev: Option<&Name> = None;
    for curr in names {
        if let Some(prev) = prev
            && locale_compare(prev.as_str(), curr.as_str()) == std::cmp::Ordering::Greater
        {
            let (file, line, column) =
                gql_lint_core::resolve_location(&schema.sources, curr.location());
            violations.push(Violation {
                file,
                line,
                column,
                message: format!("`{curr}` should be before `{prev}`."),
                rule: rules::ALPHABETIZE,
            });
        }
        prev = Some(curr);
    }
}

/// Mimics JavaScript's default `String.prototype.localeCompare` closely
/// enough for `alphabetize`'s own comparisons, matching
/// `@graphql-eslint`'s ordering exactly: case-insensitive at the primary
/// level (so names group by base letter regardless of case), with a
/// lowercase-before-uppercase tiebreak when two names are
/// case-insensitively equal — the opposite of plain byte order, where an
/// uppercase letter sorts before its lowercase counterpart. A direct port
/// of the Go rule's own `localeCompare`, kept because the ordering it
/// produces is part of this rule's observable behavior, not an
/// implementation detail free to pick idiomatically.
fn locale_compare(a: &str, b: &str) -> std::cmp::Ordering {
    let (lower_a, lower_b) = (a.to_lowercase(), b.to_lowercase());
    if lower_a != lower_b {
        return lower_a.cmp(&lower_b);
    }

    for (ca, cb) in a.chars().zip(b.chars()) {
        if ca == cb {
            continue;
        }
        let (a_lower, b_lower) = (ca.is_lowercase(), cb.is_lowercase());
        if a_lower != b_lower {
            return if a_lower {
                std::cmp::Ordering::Less
            } else {
                std::cmp::Ordering::Greater
            };
        }
        return ca.cmp(&cb);
    }

    a.chars().count().cmp(&b.chars().count())
}

#[cfg(test)]
mod tests {
    use super::locale_compare;
    use std::cmp::Ordering;

    #[test]
    fn case_insensitive_primary_order() {
        assert_eq!(locale_compare("apple", "Banana"), Ordering::Less);
        assert_eq!(locale_compare("Banana", "apple"), Ordering::Greater);
    }

    #[test]
    fn lowercase_before_uppercase_tiebreak() {
        // Case-insensitively equal ("id" == "id"), differ only by case:
        // lowercase must sort first, the reverse of plain byte order.
        assert_eq!(locale_compare("id", "Id"), Ordering::Less);
        assert_eq!(locale_compare("Id", "id"), Ordering::Greater);
    }

    #[test]
    fn equal_names() {
        assert_eq!(locale_compare("name", "name"), Ordering::Equal);
    }
}
