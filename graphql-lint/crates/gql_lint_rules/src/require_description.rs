//! Ports `internal/graphql/lint/require_description.go`'s
//! `require-description` rule: a description is required on the kinds of
//! definition its options enable it for.
//!
//! **Correction from this port's own earlier assumption:** this rule does
//! *not* need `apollo-parser`'s raw CST, unlike `description-style` and
//! `no-hashtag-description`. `apollo_compiler::Schema` already carries each
//! definition's decoded description text directly
//! (`Option<Node<str>>` on every type/field/argument/enum-value/directive),
//! and this rule only needs to know whether that text is present once
//! trimmed — it never needs to know which quoting style produced it (that's
//! `description-style`'s job) or what raw comments surrounded it (that's
//! `no-hashtag-description`'s). So this stays in the same
//! `apollo_compiler::Schema`-only style as everything ported before it.

use apollo_compiler::Schema;
use apollo_compiler::schema::ExtendedType;
use gql_lint_core::{Violation, rules};
use std::collections::{HashMap, HashSet};

/// The 6 type-definition kind option keys, matching
/// `@graphql-eslint`'s own AST `Kind` constants.
const SCALAR_TYPE_DEFINITION: &str = "ScalarTypeDefinition";
const OBJECT_TYPE_DEFINITION: &str = "ObjectTypeDefinition";
const INTERFACE_TYPE_DEFINITION: &str = "InterfaceTypeDefinition";
const UNION_TYPE_DEFINITION: &str = "UnionTypeDefinition";
const ENUM_TYPE_DEFINITION: &str = "EnumTypeDefinition";
const INPUT_OBJECT_TYPE_DEFINITION: &str = "InputObjectTypeDefinition";

/// `require-description`'s `scripts/devbase.yaml` options, decoded from
/// the rule's raw options mapping. Field names match `@graphql-eslint`'s
/// own option keys.
///
/// `types`, `DirectiveDefinition`, `FieldDefinition`,
/// `InputValueDefinition`, and `EnumValueDefinition` are collapsed into
/// one `enabled_kinds` set rather than 5 flat bools (clippy's
/// `struct_excessive_bools` pedantic lint rejects more than 3) — they all
/// mean the same thing, "is this named site kind checked at all," so one
/// set reads at least as clearly as 5 near-identical fields.
#[derive(Debug, Clone, Default)]
pub struct RequireDescriptionOptions {
    enabled_kinds: HashSet<&'static str>,
    pub root_field: bool,
    /// An explicit true/false for one of the 6 type-definition kind keys,
    /// overriding the blanket `types` option for that one kind. A kind
    /// absent from this map falls back to whether `"types"` is in
    /// `enabled_kinds`.
    pub per_kind_override: HashMap<&'static str, bool>,
}

impl RequireDescriptionOptions {
    #[must_use]
    pub fn from_yaml(options: Option<&serde_yaml::Mapping>) -> Self {
        let Some(options) = options else {
            return Self::default();
        };
        let is_true = |key: &str| {
            options
                .get(key)
                .and_then(serde_yaml::Value::as_bool)
                .unwrap_or(false)
        };

        let mut per_kind_override = HashMap::new();
        for key in [
            SCALAR_TYPE_DEFINITION,
            OBJECT_TYPE_DEFINITION,
            INTERFACE_TYPE_DEFINITION,
            UNION_TYPE_DEFINITION,
            ENUM_TYPE_DEFINITION,
            INPUT_OBJECT_TYPE_DEFINITION,
        ] {
            if let Some(value) = options.get(key).and_then(serde_yaml::Value::as_bool) {
                per_kind_override.insert(key, value);
            }
        }

        let enabled_kinds = [
            "types",
            "DirectiveDefinition",
            "FieldDefinition",
            "InputValueDefinition",
            "EnumValueDefinition",
        ]
        .into_iter()
        .filter(|key| is_true(key))
        .collect();

        Self {
            enabled_kinds,
            root_field: is_true("rootField"),
            per_kind_override,
        }
    }
}

/// Which kind of description-bearing site a [`Site`] is, mirroring the Go
/// port's `optionKind` tag.
enum SiteKind {
    Type { kind_key: &'static str },
    Field { is_root_field: bool },
    InputValue,
    EnumValue,
    Directive,
}

struct Site<'a> {
    description: Option<&'a str>,
    kind: SiteKind,
    label: String,
    location: Option<apollo_compiler::parser::SourceSpan>,
}

impl RequireDescriptionOptions {
    fn applies(&self, site: &Site) -> bool {
        match &site.kind {
            SiteKind::Type { kind_key } => self
                .per_kind_override
                .get(kind_key)
                .copied()
                .unwrap_or(self.enabled_kinds.contains("types")),
            SiteKind::Field { is_root_field } => {
                self.enabled_kinds.contains("FieldDefinition")
                    || (self.root_field && *is_root_field)
            }
            SiteKind::InputValue => self.enabled_kinds.contains("InputValueDefinition"),
            SiteKind::EnumValue => self.enabled_kinds.contains("EnumValueDefinition"),
            SiteKind::Directive => self.enabled_kinds.contains("DirectiveDefinition"),
        }
    }
}

/// Names of `schema`'s root operation types (`Query`, `Mutation`, and/or
/// `Subscription` — whatever it actually declares), for the `rootField`
/// option.
fn root_type_names(schema: &Schema) -> HashSet<&str> {
    let def = &schema.schema_definition;
    [&def.query, &def.mutation, &def.subscription]
        .into_iter()
        .filter_map(|name| name.as_ref().map(|n| n.as_str()))
        .collect()
}

/// Runs the `require-description` rule over `schema`, per `opts`.
#[must_use]
pub fn require_description(schema: &Schema, opts: &RequireDescriptionOptions) -> Vec<Violation> {
    let root_types = root_type_names(schema);

    let mut violations = Vec::new();
    for site in collect_sites(schema, &root_types) {
        if !opts.applies(&site) {
            continue;
        }
        if site.description.is_some_and(|d| !d.trim().is_empty()) {
            continue;
        }
        let (file, line, column) = gql_lint_core::resolve_location(&schema.sources, site.location);
        violations.push(Violation {
            file,
            line,
            column,
            message: format!("Description is required for {}", site.label),
            rule: rules::REQUIRE_DESCRIPTION,
        });
    }
    violations
}

fn collect_sites<'a>(
    schema: &'a Schema,
    root_types: &std::collections::HashSet<&str>,
) -> Vec<Site<'a>> {
    let mut sites = Vec::new();

    for ty in gql_lint_core::in_scope_types(schema) {
        let (kind_key, kind_label, ty_name) = match ty {
            ExtendedType::Scalar(t) => (SCALAR_TYPE_DEFINITION, "scalar", &t.name),
            ExtendedType::Object(t) => (OBJECT_TYPE_DEFINITION, "type", &t.name),
            ExtendedType::Interface(t) => (INTERFACE_TYPE_DEFINITION, "interface", &t.name),
            ExtendedType::Union(t) => (UNION_TYPE_DEFINITION, "union", &t.name),
            ExtendedType::Enum(t) => (ENUM_TYPE_DEFINITION, "enum", &t.name),
            ExtendedType::InputObject(t) => (INPUT_OBJECT_TYPE_DEFINITION, "input", &t.name),
        };
        sites.push(Site {
            description: ty.description().map(|d| &**d),
            kind: SiteKind::Type { kind_key },
            label: format!("{kind_label} \"{ty_name}\""),
            location: ty_name.location(),
        });

        let object_or_interface_fields = match ty {
            ExtendedType::Object(t) => Some(&t.fields),
            ExtendedType::Interface(t) => Some(&t.fields),
            _ => None,
        };
        if let Some(fields) = object_or_interface_fields {
            let is_root = root_types.contains(ty_name.as_str());
            for field in fields.values() {
                sites.push(Site {
                    description: field.description.as_deref(),
                    kind: SiteKind::Field {
                        is_root_field: is_root,
                    },
                    label: format!("field \"{}\" in {kind_label} \"{ty_name}\"", field.name),
                    location: field.name.location(),
                });
                for arg in &field.arguments {
                    sites.push(Site {
                        description: arg.description.as_deref(),
                        kind: SiteKind::InputValue,
                        label: format!("input value \"{}\" in field \"{}\"", arg.name, field.name),
                        location: arg.name.location(),
                    });
                }
            }
        }

        match ty {
            ExtendedType::InputObject(t) => {
                for field in t.fields.values() {
                    sites.push(Site {
                        description: field.description.as_deref(),
                        kind: SiteKind::InputValue,
                        label: format!("input value \"{}\" in input \"{ty_name}\"", field.name),
                        location: field.name.location(),
                    });
                }
            }
            ExtendedType::Enum(t) => {
                for value in t.values.values() {
                    sites.push(Site {
                        description: value.description.as_deref(),
                        kind: SiteKind::EnumValue,
                        label: format!("enum value \"{}\" in enum \"{ty_name}\"", value.value),
                        location: value.value.location(),
                    });
                }
            }
            // Object and Interface are already handled above, alongside
            // their fields' arguments; Union and Scalar carry no
            // description-bearing sites of their own.
            ExtendedType::Object(_)
            | ExtendedType::Interface(_)
            | ExtendedType::Union(_)
            | ExtendedType::Scalar(_) => {}
        }
    }

    for directive in gql_lint_core::in_scope_directive_definitions(schema) {
        sites.push(Site {
            description: directive.description.as_deref(),
            kind: SiteKind::Directive,
            label: format!("directive \"{}\"", directive.name),
            location: directive.name.location(),
        });
        for arg in &directive.arguments {
            sites.push(Site {
                description: arg.description.as_deref(),
                kind: SiteKind::InputValue,
                label: format!(
                    "input value \"{}\" in directive \"{}\"",
                    arg.name, directive.name
                ),
                location: arg.name.location(),
            });
        }
    }

    sites
}
