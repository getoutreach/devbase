//! Ports `internal/graphql/lint/deprecation.go`'s `require-deprecation-reason`
//! and `require-deprecation-date` rules: every applied `@deprecated` usage
//! must have a non-empty `reason` argument and a valid, not-yet-passed
//! deletion date.
//!
//! **Load-bearing correctness point, not a style choice:** both rules read
//! a usage site's own *applied* directive arguments
//! (`directive.arguments`, only what was actually written at that call
//! site), never [`apollo_compiler::ast::Directive::argument_by_name`] or
//! any schema-level default-value resolution. `apollo-compiler`'s built-in
//! `@deprecated` directive definition gives `reason` a default value
//! (`"No longer supported"`), so resolving defaults would make
//! `require-deprecation-reason` never fire for a bare `@deprecated` with
//! no explicit reason — exactly the case this rule exists to catch. This
//! mirrors the Go port's own reasoning (its doc comment explains reading
//! applied arguments directly, not the resolved directive definition, for
//! a different but related reason: a re-declared `@deprecated` signature
//! that `gqlparser` silently keeps the first-seen definition for).
//!
//! **Message text is an approximation, not yet verified against
//! `@graphql-eslint`/the Go port byte-for-byte:** the Go port's messages
//! embed a full `kindLabel "name" in parentLabel` chain built by a shared
//! walk (`descriptions.go`) this port hasn't built. The rule *logic*
//! (finding every applied `@deprecated` usage, checking `reason`/date) is
//! the correctness-critical part and is verified end-to-end; the label
//! text is close but not exact — revisit once/if exact message parity is
//! needed.

use apollo_compiler::Schema;
use apollo_compiler::ast::Value;
use apollo_compiler::schema::{Directive, ExtendedType};
use chrono::NaiveDate;
use gql_lint_core::{Violation, rules};

const DEPRECATED: &str = "deprecated";

/// One place in the schema an `@deprecated` directive can be applied,
/// paired with a label for messages. `directive` is resolved eagerly at
/// collection time (see [`deprecatable_sites`]) rather than stored as a
/// raw directive list, because a type's own applied directives use a
/// different (but equally `.get()`-able) list type than a field's,
/// argument's, or enum value's — `apollo-compiler`'s data model, not a
/// port artifact.
struct DeprecatableSite<'a> {
    directive: Option<&'a Directive>,
    label: String,
    location: Option<apollo_compiler::parser::SourceSpan>,
}

/// Runs `require-deprecation-reason` over `schema`.
#[must_use]
pub fn require_deprecation_reason(schema: &Schema) -> Vec<Violation> {
    let mut violations = Vec::new();
    for site in deprecatable_sites(schema) {
        let Some(deprecated) = site.directive else {
            continue;
        };
        if reason_is_present(deprecated) {
            continue;
        }
        violations.push(violation(
            schema,
            rules::REQUIRE_DEPRECATION_REASON,
            site.location,
            &format!("Deprecation reason is required for {}", site.label),
        ));
    }
    violations
}

/// `require-deprecation-date`'s options: which `@deprecated` argument
/// carries the deletion date, defaulting to `"deletionDate"`.
#[derive(Debug, Clone)]
pub struct RequireDeprecationDateOptions {
    pub argument_name: String,
}

impl Default for RequireDeprecationDateOptions {
    fn default() -> Self {
        Self {
            argument_name: "deletionDate".to_string(),
        }
    }
}

impl RequireDeprecationDateOptions {
    #[must_use]
    pub fn from_yaml(options: Option<&serde_yaml::Mapping>) -> Self {
        let argument_name = options
            .and_then(|o| o.get("argumentName"))
            .and_then(serde_yaml::Value::as_str)
            .filter(|s| !s.is_empty())
            .map_or_else(|| "deletionDate".to_string(), String::from);
        Self { argument_name }
    }
}

/// Runs `require-deprecation-date` over `schema`, per `opts`. `today` is
/// passed in (rather than read from the clock inside this function) so
/// tests can pin it.
#[must_use]
pub fn require_deprecation_date(
    schema: &Schema,
    opts: &RequireDeprecationDateOptions,
    today: NaiveDate,
) -> Vec<Violation> {
    let mut violations = Vec::new();
    for site in deprecatable_sites(schema) {
        let Some(deprecated) = site.directive else {
            continue;
        };

        let Some(arg) = deprecated
            .arguments
            .iter()
            .find(|a| a.name.as_str() == opts.argument_name)
        else {
            violations.push(violation(
                schema,
                rules::REQUIRE_DEPRECATION_DATE,
                site.location,
                &format!(
                    "Directive \"@deprecated\" must have a deletion date for {}",
                    site.label
                ),
            ));
            continue;
        };

        let Value::String(raw) = &*arg.value else {
            violations.push(violation(
                schema,
                rules::REQUIRE_DEPRECATION_DATE,
                site.location,
                &format!(
                    "Deletion date must be in format \"DD/MM/YYYY\" for {}",
                    site.label
                ),
            ));
            continue;
        };

        match parse_deletion_date(raw) {
            Some(date) if today > date => {
                violations.push(violation(
                    schema,
                    rules::REQUIRE_DEPRECATION_DATE,
                    site.location,
                    &format!("{} can be removed", site.label),
                ));
            }
            Some(_) => {}
            None if DATE_SHAPE_RE.is_match(raw) => {
                violations.push(violation(
                    schema,
                    rules::REQUIRE_DEPRECATION_DATE,
                    site.location,
                    &format!("Invalid \"{raw}\" deletion date for {}", site.label),
                ));
            }
            None => {
                violations.push(violation(
                    schema,
                    rules::REQUIRE_DEPRECATION_DATE,
                    site.location,
                    &format!(
                        "Deletion date must be in format \"DD/MM/YYYY\" for {}",
                        site.label
                    ),
                ));
            }
        }
    }
    violations
}

static DATE_SHAPE_RE: std::sync::LazyLock<regex::Regex> =
    std::sync::LazyLock::new(|| regex::Regex::new(r"^\d{2}/\d{2}/\d{4}$").expect("valid pattern"));

/// Parses `raw` as a `DD/MM/YYYY` calendar date, matching the Go port's
/// `deletionDateLayout` — `None` for a wrong shape or a shape-correct but
/// non-existent calendar date (for example `30/02/2026`).
fn parse_deletion_date(raw: &str) -> Option<NaiveDate> {
    if !DATE_SHAPE_RE.is_match(raw) {
        return None;
    }
    let mut parts = raw.split('/');
    let day: u32 = parts.next()?.parse().ok()?;
    let month: u32 = parts.next()?.parse().ok()?;
    let year: i32 = parts.next()?.parse().ok()?;
    NaiveDate::from_ymd_opt(year, month, day)
}

fn violation(
    schema: &Schema,
    rule: &'static str,
    location: Option<apollo_compiler::parser::SourceSpan>,
    message: &str,
) -> Violation {
    let (file, line, column) = gql_lint_core::resolve_location(&schema.sources, location);
    Violation {
        file,
        line,
        column,
        message: message.to_string(),
        rule,
    }
}

/// Present per `@graphql-eslint`'s own check: a `reason` argument whose
/// literal value, trimmed, is non-empty. A non-string literal (for
/// example `reason: 0`) still counts as present, matching the Go port's
/// `String(value).trim()` equivalent behavior.
fn reason_is_present(directive: &Directive) -> bool {
    let Some(arg) = directive.arguments.iter().find(|a| a.name == "reason") else {
        return false;
    };
    match &*arg.value {
        Value::String(s) => !s.trim().is_empty(),
        Value::Null => false,
        _ => true,
    }
}

/// Collects every place in `schema` an `@deprecated` directive can be
/// applied: object/interface fields (and their arguments), input object
/// fields, directive definition arguments, and enum values. Excludes
/// built-in types — see [`crate::alphabetize::alphabetize`]'s doc comment
/// for why.
fn deprecatable_sites(schema: &Schema) -> Vec<DeprecatableSite<'_>> {
    let mut sites = Vec::new();

    for ty in gql_lint_core::in_scope_types(schema) {
        let object_or_interface_fields = match ty {
            ExtendedType::Object(t) => Some((&t.name, &t.fields)),
            ExtendedType::Interface(t) => Some((&t.name, &t.fields)),
            _ => None,
        };
        if let Some((type_name, fields)) = object_or_interface_fields {
            for field in fields.values() {
                sites.push(DeprecatableSite {
                    directive: ast_deprecated(&field.directives),
                    label: format!("field \"{}\" in type \"{type_name}\"", field.name),
                    location: field.name.location(),
                });
                for arg in &field.arguments {
                    sites.push(DeprecatableSite {
                        directive: ast_deprecated(&arg.directives),
                        label: format!(
                            "argument \"{}\" of field \"{}\" in type \"{type_name}\"",
                            arg.name, field.name
                        ),
                        location: arg.name.location(),
                    });
                }
            }
        }

        match ty {
            ExtendedType::InputObject(t) => {
                for field in t.fields.values() {
                    sites.push(DeprecatableSite {
                        directive: ast_deprecated(&field.directives),
                        label: format!("input value \"{}\" in input \"{}\"", field.name, t.name),
                        location: field.name.location(),
                    });
                }
            }
            ExtendedType::Enum(t) => {
                for value in t.values.values() {
                    sites.push(DeprecatableSite {
                        directive: ast_deprecated(&value.directives),
                        label: format!("enum value \"{}\" in enum \"{}\"", value.value, t.name),
                        location: value.value.location(),
                    });
                }
            }
            // Object and Interface are already handled above, alongside
            // their field arguments; Union and Scalar carry no
            // deprecatable sites of their own.
            ExtendedType::Object(_)
            | ExtendedType::Interface(_)
            | ExtendedType::Union(_)
            | ExtendedType::Scalar(_) => {}
        }
    }

    for directive in gql_lint_core::in_scope_directive_definitions(schema) {
        for arg in &directive.arguments {
            sites.push(DeprecatableSite {
                directive: ast_deprecated(&arg.directives),
                label: format!(
                    "argument \"{}\" of directive \"@{}\"",
                    arg.name, directive.name
                ),
                location: arg.name.location(),
            });
        }
    }

    sites
}

/// Looks up an applied `@deprecated` directive in an `ast::DirectiveList`
/// (the list type a field, argument, or enum value's own `directives`
/// carries — distinct from `schema::DirectiveList`, which a type or
/// schema definition's own applied directives use instead).
fn ast_deprecated(directives: &apollo_compiler::ast::DirectiveList) -> Option<&Directive> {
    directives.get(DEPRECATED).map(|node| &**node)
}
