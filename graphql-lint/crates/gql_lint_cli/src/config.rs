//! Loads `scripts/devbase.yaml`, the same per-repo configuration file the
//! Go `devbase graphql lint` tool reads (`internal/graphql/config/config.go`)
//! — same schema, same discovery rule (walk up to the enclosing git root),
//! so a repo's existing config file needs no changes to be read by this
//! tool too.

use serde::Deserialize;
use std::collections::HashMap;
use std::path::{Path, PathBuf};

const SCRIPTS_DEVBASE_YAML: &str = "scripts/devbase.yaml";

#[derive(Debug, Clone, Copy, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Severity {
    Off,
    Warn,
    Error,
}

/// A single rule override, decoded from either the short form
/// (`rule-name: warn`) or the long form
/// (`rule-name: [error, { someOption: true }]`) — matching Go's `Rule`
/// type and its custom `UnmarshalYAML` exactly.
#[derive(Debug, Clone)]
pub struct Rule {
    pub severity: Severity,
    pub options: Option<serde_yaml::Mapping>,
}

impl<'de> Deserialize<'de> for Rule {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        #[derive(Deserialize)]
        #[serde(untagged)]
        enum RuleForm {
            Short(Severity),
            Long(Vec<serde_yaml::Value>),
        }

        match RuleForm::deserialize(deserializer)? {
            RuleForm::Short(severity) => Ok(Rule {
                severity,
                options: None,
            }),
            RuleForm::Long(values) => {
                if values.is_empty() || values.len() > 2 {
                    return Err(serde::de::Error::custom(format!(
                        "sequence form must have 1 or 2 elements, got {}",
                        values.len()
                    )));
                }
                let severity: Severity = serde_yaml::from_value(values[0].clone())
                    .map_err(serde::de::Error::custom)?;
                let options = match values.get(1) {
                    Some(v) => Some(
                        serde_yaml::from_value(v.clone()).map_err(serde::de::Error::custom)?,
                    ),
                    None => None,
                };
                Ok(Rule { severity, options })
            }
        }
    }
}

/// The `graphql.lint` section of `scripts/devbase.yaml`.
#[derive(Debug, Clone, Default, Deserialize)]
pub struct Lint {
    #[serde(default)]
    pub exclude: Vec<String>,
    #[serde(default)]
    pub rules: HashMap<String, Rule>,
    /// Apollo Federation subgraph spec version this repo's schema links
    /// against, if any (only `"v2.3"` is supported — see
    /// `gql_lint_core::federation`). Wired into schema validation via
    /// `federation::prelude_sources`, called from `main.rs`'s
    /// `lint_sources` before Tier 1 runs.
    pub federation: Option<String>,
    /// Custom scalar names declared outside this repo's `.graphql` files.
    #[serde(default)]
    pub scalars: Vec<String>,
}

impl Lint {
    /// A rule is enabled only once configured to a severity other than
    /// `Off` — matching `@graphql-eslint`'s "off by default" behavior, per
    /// the Go tool's `Enabled` doc comment. Never consulted for Tier 1
    /// rules, which always run regardless of config.
    #[must_use]
    pub fn enabled(&self, rule: &str) -> bool {
        self.rules
            .get(rule)
            .is_some_and(|r| r.severity != Severity::Off)
    }

    /// Resolves the severity that applies to a violation tagged `rule`.
    /// Defaults to `Error` for any rule absent from the config (a Tier 1
    /// rule, or an unclassified `apollo-compiler` diagnostic) — matching
    /// the Go tool's `SeverityOf`.
    #[must_use]
    pub fn severity_of(&self, rule: &str) -> Severity {
        self.rules.get(rule).map_or(Severity::Error, |r| r.severity)
    }

    #[must_use]
    pub fn merge_excludes(&self, extra: &[String]) -> Vec<String> {
        self.exclude.iter().chain(extra).cloned().collect()
    }
}

#[derive(Debug, Deserialize)]
struct GraphQlSection {
    lint: Lint,
}

#[derive(Debug, Deserialize)]
struct FileConfig {
    graphql: GraphQlSection,
}

/// Discovers and parses `scripts/devbase.yaml` for the repository
/// containing `start_dir`, walking up until either the file is found or
/// the enclosing git repository's top-level directory is reached —
/// matching the Go tool's `discover`/`Load` exactly, including never
/// crossing a git repository boundary.
///
/// Returns `(Lint::default(), None)` if no config file is found, the same
/// "built-in defaults" fallback the Go tool uses.
pub fn load(start_dir: &Path) -> anyhow::Result<(Lint, Option<PathBuf>)> {
    let mut dir = start_dir.canonicalize()?;
    loop {
        let candidate = dir.join(SCRIPTS_DEVBASE_YAML);
        if candidate.is_file() {
            let text = std::fs::read_to_string(&candidate)?;
            let config: FileConfig = serde_yaml::from_str(&text)?;
            return Ok((config.graphql.lint, Some(dir)));
        }

        if dir.join(".git").exists() {
            return Ok((Lint::default(), None));
        }

        match dir.parent() {
            Some(parent) => dir = parent.to_path_buf(),
            None => return Ok((Lint::default(), None)),
        }
    }
}
