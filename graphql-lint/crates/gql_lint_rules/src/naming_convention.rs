//! Ports `internal/graphql/lint/naming_convention.go`'s `naming-convention`
//! rule: requires names to follow the casing, prefix/suffix, and
//! underscore conventions its options configure per kind of definition.
//!
//! Like the Go port, this narrows `@graphql-eslint`'s own option surface
//! (which accepts any AST node kind, including operation-only kinds that
//! never appear in a standalone schema file) to the kinds that can occur
//! in one: the 6 type-definition kinds (via a shared `types` fallback and
//! per-kind overrides), `FieldDefinition`, `InputValueDefinition` (an
//! argument or an input object field), `EnumValueDefinition`, and
//! `DirectiveDefinition` — plus the 3 root-operation-type field selectors
//! from `@graphql-eslint`'s own documented "recommended schema" config
//! (`FieldDefinition[parent.name.value=Query|Mutation|Subscription]`), so a
//! repo can forbid a `get` prefix on `Query` fields specifically.

use apollo_compiler::Schema;
use apollo_compiler::schema::{ExtendedType, Name};
use gql_lint_core::{Violation, rules};
use regex::Regex;
use std::collections::HashMap;
use std::sync::LazyLock;

/// The 4 casing styles `@graphql-eslint` recognizes, with the exact
/// patterns its `StyleToRegex` uses.
static NAMING_STYLE_REGEXES: LazyLock<HashMap<&'static str, Regex>> = LazyLock::new(|| {
    [
        ("camelCase", r"^[a-z][0-9A-Za-z]*$"),
        ("PascalCase", r"^[A-Z][0-9A-Za-z]*$"),
        ("snake_case", r"^[a-z][0-9_a-z]*[0-9a-z]*$"),
        ("UPPER_CASE", r"^[A-Z][0-9A-Z_]*[0-9A-Z]*$"),
    ]
    .into_iter()
    .map(|(name, pattern)| (name, Regex::new(pattern).expect("built-in pattern")))
    .collect()
});

const SCALAR_TYPE_DEFINITION: &str = "ScalarTypeDefinition";
const OBJECT_TYPE_DEFINITION: &str = "ObjectTypeDefinition";
const INTERFACE_TYPE_DEFINITION: &str = "InterfaceTypeDefinition";
const UNION_TYPE_DEFINITION: &str = "UnionTypeDefinition";
const ENUM_TYPE_DEFINITION: &str = "EnumTypeDefinition";
const INPUT_OBJECT_TYPE_DEFINITION: &str = "InputObjectTypeDefinition";
const FIELD_DEFINITION: &str = "FieldDefinition";
const INPUT_VALUE_DEFINITION: &str = "InputValueDefinition";
const DIRECTIVE_DEFINITION: &str = "DirectiveDefinition";
const ENUM_VALUE_DEFINITION: &str = "EnumValueDefinition";

fn root_field_selector(role: &str) -> String {
    format!("{FIELD_DEFINITION}[parent.name.value={role}]")
}

/// One selector's casing/prefix/suffix requirement, decoded from either
/// the short form (a bare style string) or the long form (an options
/// mapping) of a `naming-convention` selector value.
#[derive(Debug, Clone, Default)]
pub struct NamingRule {
    pub style: Option<String>,
    pub prefix: Option<String>,
    pub suffix: Option<String>,
    pub forbidden_prefixes: Vec<String>,
    pub forbidden_suffixes: Vec<String>,
    pub ignore_regex: Option<Regex>,
}

impl NamingRule {
    fn from_yaml(value: &serde_yaml::Value) -> Option<Self> {
        if let Some(style) = value.as_str() {
            return Some(Self {
                style: Some(style.to_string()),
                ..Self::default()
            });
        }
        let mapping = value.as_mapping()?;
        let string_opt = |key: &str| {
            mapping
                .get(key)
                .and_then(serde_yaml::Value::as_str)
                .map(String::from)
        };
        let string_list_opt = |key: &str| -> Vec<String> {
            mapping
                .get(key)
                .and_then(serde_yaml::Value::as_sequence)
                .map(|seq| {
                    seq.iter()
                        .filter_map(serde_yaml::Value::as_str)
                        .map(String::from)
                        .collect()
                })
                .unwrap_or_default()
        };
        Some(Self {
            style: string_opt("style"),
            prefix: string_opt("prefix"),
            suffix: string_opt("suffix"),
            forbidden_prefixes: string_list_opt("forbiddenPrefixes"),
            forbidden_suffixes: string_list_opt("forbiddenSuffixes"),
            ignore_regex: string_opt("ignorePattern").and_then(|p| Regex::new(&p).ok()),
        })
    }
}

/// `naming-convention`'s `scripts/devbase.yaml` options, decoded from the
/// rule's raw options mapping.
#[derive(Debug, Clone, Default)]
pub struct NamingConventionOptions {
    /// If false (the default, matching `@graphql-eslint`'s own default),
    /// a leading underscore is its own violation, independent of whatever
    /// selector rule also applies to that name.
    pub allow_leading_underscore: bool,
    pub allow_trailing_underscore: bool,
    /// Fallback rule for a type-definition site whose own kind key has no
    /// selector entry of its own.
    pub types: Option<NamingRule>,
    /// Every other selector key present in the config, as-is (for example
    /// `"FieldDefinition"` or `"FieldDefinition[parent.name.value=Query]"`).
    pub selectors: HashMap<String, NamingRule>,
}

impl NamingConventionOptions {
    #[must_use]
    pub fn from_yaml(options: Option<&serde_yaml::Mapping>) -> Self {
        let Some(options) = options else {
            return Self::default();
        };

        let mut opts = Self {
            allow_leading_underscore: options
                .get("allowLeadingUnderscore")
                .and_then(serde_yaml::Value::as_bool)
                .unwrap_or(false),
            allow_trailing_underscore: options
                .get("allowTrailingUnderscore")
                .and_then(serde_yaml::Value::as_bool)
                .unwrap_or(false),
            types: None,
            selectors: HashMap::new(),
        };

        for (key, value) in options {
            let Some(key) = key.as_str() else { continue };
            if key == "allowLeadingUnderscore" || key == "allowTrailingUnderscore" {
                continue;
            }
            let Some(rule) = NamingRule::from_yaml(value) else {
                continue;
            };
            if key == "types" {
                opts.types = Some(rule);
            } else {
                opts.selectors.insert(key.to_string(), rule);
            }
        }

        opts
    }

    /// Resolves the rule that applies to `site`: its own root-field
    /// selector (if any) or base selector, whichever the config actually
    /// set, falling back to the shared `types` rule only for a
    /// type-definition site whose own kind key was not configured
    /// individually.
    fn rule_for<'a>(&'a self, site: &NamingSite) -> Option<&'a NamingRule> {
        if let Some(root_selector) = &site.root_selector
            && let Some(rule) = self.selectors.get(root_selector)
        {
            return Some(rule);
        }
        if let Some(rule) = self.selectors.get(site.base_selector) {
            return Some(rule);
        }
        if site.type_def_kind_key.is_some() {
            return self.types.as_ref();
        }
        None
    }
}

/// One name in a schema that `naming-convention` checks.
struct NamingSite<'a> {
    name: &'a Name,
    kind_label: &'static str,
    base_selector: &'static str,
    root_selector: Option<String>,
    type_def_kind_key: Option<&'static str>,
}

/// Runs the `naming-convention` rule over `schema`, per `opts`.
#[must_use]
pub fn naming_convention(schema: &Schema, opts: &NamingConventionOptions) -> Vec<Violation> {
    let roles = root_type_roles(schema);
    let sites = collect_sites(schema, &roles);

    let mut violations = Vec::with_capacity(sites.len());
    for site in &sites {
        if let Some(rule) = opts.rule_for(site)
            && let Some(msg) = naming_rule_message(site.name.as_str(), rule)
        {
            violations.push(violation(schema, site, &msg));
        }
        if !opts.allow_leading_underscore && site.name.as_str().starts_with('_') {
            violations.push(violation(
                schema,
                site,
                "Leading underscores are not allowed",
            ));
        }
        if !opts.allow_trailing_underscore && site.name.as_str().ends_with('_') {
            violations.push(violation(
                schema,
                site,
                "Trailing underscores are not allowed",
            ));
        }
    }
    violations
}

fn violation(schema: &Schema, site: &NamingSite, message: &str) -> Violation {
    let (file, line, column) =
        gql_lint_core::resolve_location(&schema.sources, site.name.location());
    Violation {
        file,
        line,
        column,
        message: format!("{} \"{}\" {}", site.kind_label, site.name, message),
        rule: rules::NAMING_CONVENTION,
    }
}

/// Maps each of `schema`'s root operation type names to its role
/// ("Query", "Mutation", "Subscription"), for [`root_field_selector`].
fn root_type_roles(schema: &Schema) -> HashMap<&str, &'static str> {
    let mut roles = HashMap::with_capacity(3);
    let def = &schema.schema_definition;
    if let Some(name) = &def.query {
        roles.insert(name.as_str(), "Query");
    }
    if let Some(name) = &def.mutation {
        roles.insert(name.as_str(), "Mutation");
    }
    if let Some(name) = &def.subscription {
        roles.insert(name.as_str(), "Subscription");
    }
    roles
}

/// Collects every [`NamingSite`] in `schema`, excluding built-in types —
/// see [`crate::alphabetize::alphabetize`]'s doc comment for why.
fn collect_sites<'a>(
    schema: &'a Schema,
    roles: &HashMap<&str, &'static str>,
) -> Vec<NamingSite<'a>> {
    let mut sites = Vec::new();

    for ty in gql_lint_core::in_scope_types(schema) {
        let (kind_label, kind_key, name) = match ty {
            ExtendedType::Scalar(t) => ("Scalar", SCALAR_TYPE_DEFINITION, &t.name),
            ExtendedType::Object(t) => ("Type", OBJECT_TYPE_DEFINITION, &t.name),
            ExtendedType::Interface(t) => ("Interface", INTERFACE_TYPE_DEFINITION, &t.name),
            ExtendedType::Union(t) => ("Union", UNION_TYPE_DEFINITION, &t.name),
            ExtendedType::Enum(t) => ("Enumerator", ENUM_TYPE_DEFINITION, &t.name),
            ExtendedType::InputObject(t) => ("Input type", INPUT_OBJECT_TYPE_DEFINITION, &t.name),
        };
        sites.push(NamingSite {
            name,
            kind_label,
            base_selector: kind_key,
            root_selector: None,
            type_def_kind_key: Some(kind_key),
        });

        match ty {
            ExtendedType::Object(t) => {
                add_fields(&mut sites, t.name.as_str(), t.fields.values(), roles);
            }
            ExtendedType::Interface(t) => {
                add_fields(&mut sites, t.name.as_str(), t.fields.values(), roles);
            }
            ExtendedType::InputObject(t) => {
                for field in t.fields.values() {
                    sites.push(NamingSite {
                        name: &field.name,
                        kind_label: "Input property",
                        base_selector: INPUT_VALUE_DEFINITION,
                        root_selector: None,
                        type_def_kind_key: None,
                    });
                }
            }
            ExtendedType::Enum(t) => {
                for value in t.values.values() {
                    sites.push(NamingSite {
                        name: &value.value,
                        kind_label: "Enumeration value",
                        base_selector: ENUM_VALUE_DEFINITION,
                        root_selector: None,
                        type_def_kind_key: None,
                    });
                }
            }
            _ => {}
        }
    }

    for directive in gql_lint_core::in_scope_directive_definitions(schema) {
        sites.push(NamingSite {
            name: &directive.name,
            kind_label: "Directive",
            base_selector: DIRECTIVE_DEFINITION,
            root_selector: None,
            type_def_kind_key: None,
        });
        for arg in &directive.arguments {
            sites.push(NamingSite {
                name: &arg.name,
                kind_label: "Input property",
                base_selector: INPUT_VALUE_DEFINITION,
                root_selector: None,
                type_def_kind_key: None,
            });
        }
    }

    sites
}

fn add_fields<'a>(
    sites: &mut Vec<NamingSite<'a>>,
    type_name: &str,
    fields: impl Iterator<
        Item = &'a apollo_compiler::schema::Component<apollo_compiler::schema::FieldDefinition>,
    >,
    roles: &HashMap<&str, &'static str>,
) {
    let root_selector = roles.get(type_name).map(|role| root_field_selector(role));
    for field in fields {
        sites.push(NamingSite {
            name: &field.name,
            kind_label: "Field",
            base_selector: FIELD_DEFINITION,
            root_selector: root_selector.clone(),
            type_def_kind_key: None,
        });
        for arg in &field.arguments {
            sites.push(NamingSite {
                name: &arg.name,
                kind_label: "Input property",
                base_selector: INPUT_VALUE_DEFINITION,
                root_selector: None,
                type_def_kind_key: None,
            });
        }
    }
}

/// Reports the violation message `naming-convention` should raise for
/// `name` under `rule`, or `None` if `name` satisfies it. Checks, in
/// order (matching `@graphql-eslint`'s own `getError`): a required
/// prefix, a required suffix, every forbidden prefix, every forbidden
/// suffix, then the casing style — stopping at the first one that
/// applies, against `name` with leading/trailing underscores stripped.
/// `ignorePattern`, if it matches, skips all of the above.
fn naming_rule_message(name: &str, rule: &NamingRule) -> Option<String> {
    let trimmed = name.trim_matches('_');

    if let Some(ignore) = &rule.ignore_regex
        && ignore.is_match(trimmed)
    {
        return None;
    }
    if let Some(prefix) = &rule.prefix
        && !trimmed.starts_with(prefix.as_str())
    {
        return Some(format!("should have \"{prefix}\" prefix"));
    }
    if let Some(suffix) = &rule.suffix
        && !trimmed.ends_with(suffix.as_str())
    {
        return Some(format!("should have \"{suffix}\" suffix"));
    }
    for forbidden in &rule.forbidden_prefixes {
        if trimmed.starts_with(forbidden.as_str()) {
            return Some(format!("should not have \"{forbidden}\" prefix"));
        }
    }
    for forbidden in &rule.forbidden_suffixes {
        if trimmed.ends_with(forbidden.as_str()) {
            return Some(format!("should not have \"{forbidden}\" suffix"));
        }
    }
    let style = rule.style.as_deref()?;
    let re = NAMING_STYLE_REGEXES.get(style)?;
    if re.is_match(trimmed) {
        None
    } else {
        Some(format!("should be in {style} format"))
    }
}
