//! `gql-lint` — a Rust re-implementation of `devbase graphql lint`
//! (`getoutreach/devbase`'s `cmd/devbase/lint_graphql.go`), built to
//! benchmark against the Go binary over the same rule set, config format,
//! and output shape. See the plan for the full build order; this binary
//! currently wires up Tier 1 (via `gql_lint_core`) and one Tier 2 rule
//! (`unique-directive-names-per-location`, via `gql_lint_rules`) end to
//! end, with `--diff` still stubbed out.

mod config;

use clap::Parser;
use gql_lint_core::{Source, Tier1Result, Violation};
use std::path::PathBuf;
use std::process::ExitCode;

/// Run GraphQL schema lint checks — a Rust port of `devbase graphql lint`.
#[derive(Parser)]
struct Cli {
    /// Paths to lint (files or directories); defaults to the current directory.
    paths: Vec<PathBuf>,

    /// Glob pattern to exclude (repeatable, additive with `scripts/devbase.yaml`).
    #[arg(short, long)]
    exclude: Vec<String>,

    /// Report only violations not present at the merge-base of HEAD and --base.
    #[arg(long)]
    diff: bool,

    /// Ref to compute the merge-base against; used with --diff.
    #[arg(long, default_value = "main")]
    base: String,
}

fn main() -> anyhow::Result<ExitCode> {
    let cli = Cli::parse();

    if cli.diff {
        anyhow::bail!(
            "--diff is not implemented yet in gql-lint (base {:?} requested) — \
             see gitdiff.go's Rust port in the plan's build order",
            cli.base
        );
    }

    let cwd = std::env::current_dir()?;
    let (lint_config, config_dir) = config::load(&cwd)?;
    eprintln!(
        "resolved config from {:?} ({} rule override(s))",
        config_dir,
        lint_config.rules.len()
    );

    let excludes = lint_config.merge_excludes(&cli.exclude);
    let paths = if cli.paths.is_empty() {
        vec![PathBuf::from(".")]
    } else {
        cli.paths
    };

    let files = find_graphql_files(&paths, &excludes)?;
    let sources = files
        .iter()
        .map(|path| -> anyhow::Result<Source> {
            Ok(Source {
                name: path.display().to_string(),
                text: std::fs::read_to_string(path)?,
            })
        })
        .collect::<anyhow::Result<Vec<_>>>()?;

    let violations = match gql_lint_core::parse_and_validate(&sources) {
        // `possible-type-extension` rides in on this branch (it's a build-
        // phase error in this port, not a Tier 2 pass — see
        // `gql_lint_core::classify`), but stays config-gated like a real
        // Tier 2/3 rule via `is_always_on`, so it doesn't start failing a
        // repo that never opted into it just because the underlying
        // library changed.
        Tier1Result::Violations(v) => v
            .into_iter()
            .filter(|v| gql_lint_core::is_always_on(v.rule) || lint_config.enabled(v.rule))
            .collect(),
        Tier1Result::Valid(schema) => {
            let mut violations: Vec<Violation> =
                gql_lint_rules::unique_directive_names_per_location(&schema)
                    .into_iter()
                    .filter(|v| lint_config.enabled(v.rule))
                    .collect();

            if lint_config.enabled(gql_lint_core::rules::ALPHABETIZE) {
                let opts = gql_lint_rules::alphabetize::AlphabetizeOptions::from_yaml(
                    lint_config
                        .rules
                        .get(gql_lint_core::rules::ALPHABETIZE)
                        .and_then(|r| r.options.as_ref()),
                );
                violations.extend(gql_lint_rules::alphabetize::alphabetize(&schema, &opts));
            }

            if lint_config.enabled(gql_lint_core::rules::NO_TYPENAME_PREFIX) {
                violations.extend(gql_lint_rules::no_typename_prefix::no_typename_prefix(&schema));
            }

            if lint_config.enabled(gql_lint_core::rules::NO_CASE_INSENSITIVE_ENUM_VALUES_DUPLICATES) {
                violations.extend(
                    gql_lint_rules::no_case_insensitive_enum_values_duplicates::no_case_insensitive_enum_values_duplicates(
                        &schema,
                    ),
                );
            }

            if lint_config.enabled(gql_lint_core::rules::NAMING_CONVENTION) {
                let opts = gql_lint_rules::naming_convention::NamingConventionOptions::from_yaml(
                    lint_config
                        .rules
                        .get(gql_lint_core::rules::NAMING_CONVENTION)
                        .and_then(|r| r.options.as_ref()),
                );
                violations.extend(gql_lint_rules::naming_convention::naming_convention(&schema, &opts));
            }

            violations
        }
    };

    let has_error = report(&violations, &lint_config);
    Ok(if has_error {
        ExitCode::FAILURE
    } else {
        ExitCode::SUCCESS
    })
}

/// Prints each violation and reports whether any resolved to `Error`
/// severity — matching the Go tool's `reportViolations`.
fn report(violations: &[Violation], cfg: &config::Lint) -> bool {
    let mut has_error = false;
    for violation in violations {
        println!("{violation}");
        if cfg.severity_of(violation.rule) == config::Severity::Error {
            has_error = true;
        }
    }
    has_error
}

/// Recursively finds `*.graphql` files under `paths`, skipping anything
/// matching `excludes` — matching the Go tool's `FindGraphQLFiles`
/// (doublestar glob semantics via `globset`, lexically sorted output).
fn find_graphql_files(paths: &[PathBuf], excludes: &[String]) -> anyhow::Result<Vec<PathBuf>> {
    let mut builder = globset::GlobSetBuilder::new();
    for pattern in excludes {
        builder.add(globset::Glob::new(pattern)?);
    }
    let exclude_set = builder.build()?;

    let mut files = Vec::new();
    for root in paths {
        if root.is_file() {
            if !exclude_set.is_match(root) {
                files.push(root.clone());
            }
            continue;
        }

        for entry in walkdir::WalkDir::new(root) {
            let entry = entry?;
            if entry.file_type().is_dir() {
                continue;
            }
            if entry.path().extension().is_none_or(|ext| ext != "graphql") {
                continue;
            }
            if !exclude_set.is_match(entry.path()) {
                files.push(entry.into_path());
            }
        }
    }

    files.sort();
    Ok(files)
}
