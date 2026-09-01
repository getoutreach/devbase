//! The 22 rule names, grouped by tier exactly as the shipped Go tool
//! settled on (`internal/graphql/config/config.go`), not as the original
//! RFC draft described them.

/// Tier 1: enforced by the schema builder itself while parsing SDL. A
/// config file can never override these — see `gql_lint_cli`'s config
/// loader, which rejects an override targeting one of these names the
/// same way the Go tool's `validateNoTier1Overrides` does.
pub const TIER1_RULES: &[&str] = &[
    UNIQUE_DIRECTIVE_NAMES,
    UNIQUE_FIELD_DEFINITION_NAMES,
    UNIQUE_OPERATION_TYPES,
    UNIQUE_TYPE_NAMES,
    KNOWN_ARGUMENT_NAMES,
    KNOWN_DIRECTIVES,
    KNOWN_TYPE_NAMES,
    PROVIDED_REQUIRED_ARGUMENTS,
    LONE_SCHEMA_DEFINITION,
    UNIQUE_ENUM_VALUE_NAMES,
];

/// Tier 2: gap-fill rules, partially covered by the schema builder and
/// needing a small custom pass on top of a validated [`apollo_compiler::Schema`].
pub const TIER2_RULES: &[&str] = &[UNIQUE_DIRECTIVE_NAMES_PER_LOCATION, POSSIBLE_TYPE_EXTENSION];

/// Tier 3: fully custom style/convention rules.
pub const TIER3_RULES: &[&str] = &[
    REQUIRE_DEPRECATION_REASON,
    NO_HASHTAG_DESCRIPTION,
    REQUIRE_DEPRECATION_DATE,
    REQUIRE_DESCRIPTION,
    DESCRIPTION_STYLE,
    ALPHABETIZE,
    NO_UNREACHABLE_TYPES,
    NO_TYPENAME_PREFIX,
    NO_CASE_INSENSITIVE_ENUM_VALUES_DUPLICATES,
    NAMING_CONVENTION,
];

pub const UNIQUE_DIRECTIVE_NAMES: &str = "unique-directive-names";
pub const UNIQUE_FIELD_DEFINITION_NAMES: &str = "unique-field-definition-names";
pub const UNIQUE_OPERATION_TYPES: &str = "unique-operation-types";
pub const UNIQUE_TYPE_NAMES: &str = "unique-type-names";
pub const KNOWN_ARGUMENT_NAMES: &str = "known-argument-names";
pub const KNOWN_DIRECTIVES: &str = "known-directives";
pub const KNOWN_TYPE_NAMES: &str = "known-type-names";
pub const PROVIDED_REQUIRED_ARGUMENTS: &str = "provided-required-arguments";
pub const LONE_SCHEMA_DEFINITION: &str = "lone-schema-definition";
pub const UNIQUE_ENUM_VALUE_NAMES: &str = "unique-enum-value-names";

pub const UNIQUE_DIRECTIVE_NAMES_PER_LOCATION: &str = "unique-directive-names-per-location";
pub const POSSIBLE_TYPE_EXTENSION: &str = "possible-type-extension";

pub const REQUIRE_DEPRECATION_REASON: &str = "require-deprecation-reason";
pub const NO_HASHTAG_DESCRIPTION: &str = "no-hashtag-description";
pub const REQUIRE_DEPRECATION_DATE: &str = "require-deprecation-date";
pub const REQUIRE_DESCRIPTION: &str = "require-description";
pub const DESCRIPTION_STYLE: &str = "description-style";
pub const ALPHABETIZE: &str = "alphabetize";
pub const NO_UNREACHABLE_TYPES: &str = "no-unreachable-types";
pub const NO_TYPENAME_PREFIX: &str = "no-typename-prefix";
pub const NO_CASE_INSENSITIVE_ENUM_VALUES_DUPLICATES: &str =
    "no-case-insensitive-enum-values-duplicates";
pub const NAMING_CONVENTION: &str = "naming-convention";
