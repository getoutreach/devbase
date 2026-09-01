//! `gql-lint` — a Rust re-implementation of `devbase graphql lint`
//! (`getoutreach/devbase`'s `cmd/devbase/lint_graphql.go`), built to
//! benchmark against the Go binary over the same rule set, config format,
//! and output shape. See the plan for the full build order.

mod config;
mod gitdiff;

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

    let mut violations = lint_sources(&sources, &lint_config)?;

    if cli.diff {
        let merge_base_content = gitdiff::merge_base_files(&cwd, &cli.base, &files)?;
        let merge_base_sources: Vec<Source> = files
            .iter()
            .filter_map(|file| {
                merge_base_content.get(file).map(|text| Source {
                    name: file.display().to_string(),
                    text: text.clone(),
                })
            })
            .collect();
        let merge_base_violations = lint_sources(&merge_base_sources, &lint_config)?;
        violations = gql_lint_core::diff::new(&merge_base_violations, &violations);
    }

    let has_error = report(&violations, &lint_config);
    Ok(if has_error {
        ExitCode::FAILURE
    } else {
        ExitCode::SUCCESS
    })
}

/// Runs Tier 1 validation over `sources`, then every Tier 2/3 rule
/// `lint_config` enables if Tier 1 passed — the same pipeline for both the
/// working-tree pass and, under `--diff`, the merge-base pass. Merges in
/// `lint_config`'s federation/scalars prelude (see
/// `gql_lint_core::federation`) before Tier 1 runs, the same layering
/// `federation.go`'s `preludeSources` uses ahead of `parseAndValidate`.
fn lint_sources(sources: &[Source], lint_config: &config::Lint) -> anyhow::Result<Vec<Violation>> {
    let federation_sources = gql_lint_core::federation::prelude_sources(
        sources,
        lint_config.federation.as_deref(),
        &lint_config.scalars,
    )?;
    let all_sources = sources.iter().chain(federation_sources.iter());

    Ok(match gql_lint_core::parse_and_validate(all_sources) {
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
        Tier1Result::Valid(schema) => run_tier2_and_tier3_rules(&schema, lint_config),
    })
}

/// Runs every Tier 2/3 rule `lint_config` enables against `schema`,
/// collecting their violations. Split out of `main` because clippy's
/// `too_many_lines` pedantic lint (rightly) objects to bolting each new
/// rule's config lookup directly into `main` as the rule count grows.
fn run_tier2_and_tier3_rules(
    schema: &apollo_compiler::validation::Valid<apollo_compiler::Schema>,
    lint_config: &config::Lint,
) -> Vec<Violation> {
    let mut violations: Vec<Violation> =
        gql_lint_rules::unique_directive_names_per_location(schema)
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
        violations.extend(gql_lint_rules::alphabetize::alphabetize(schema, &opts));
    }

    if lint_config.enabled(gql_lint_core::rules::NO_TYPENAME_PREFIX) {
        violations.extend(gql_lint_rules::no_typename_prefix::no_typename_prefix(
            schema,
        ));
    }

    if lint_config.enabled(gql_lint_core::rules::NO_CASE_INSENSITIVE_ENUM_VALUES_DUPLICATES) {
        violations.extend(
            gql_lint_rules::no_case_insensitive_enum_values_duplicates::no_case_insensitive_enum_values_duplicates(
                schema,
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
        violations.extend(gql_lint_rules::naming_convention::naming_convention(
            schema, &opts,
        ));
    }

    if lint_config.enabled(gql_lint_core::rules::NO_UNREACHABLE_TYPES) {
        violations.extend(gql_lint_rules::no_unreachable_types::no_unreachable_types(
            schema,
        ));
    }

    if lint_config.enabled(gql_lint_core::rules::REQUIRE_DEPRECATION_REASON) {
        violations.extend(gql_lint_rules::deprecation::require_deprecation_reason(
            schema,
        ));
    }

    if lint_config.enabled(gql_lint_core::rules::REQUIRE_DEPRECATION_DATE) {
        let opts = gql_lint_rules::deprecation::RequireDeprecationDateOptions::from_yaml(
            lint_config
                .rules
                .get(gql_lint_core::rules::REQUIRE_DEPRECATION_DATE)
                .and_then(|r| r.options.as_ref()),
        );
        violations.extend(gql_lint_rules::deprecation::require_deprecation_date(
            schema,
            &opts,
            chrono::Local::now().date_naive(),
        ));
    }

    if lint_config.enabled(gql_lint_core::rules::REQUIRE_DESCRIPTION) {
        let opts = gql_lint_rules::require_description::RequireDescriptionOptions::from_yaml(
            lint_config
                .rules
                .get(gql_lint_core::rules::REQUIRE_DESCRIPTION)
                .and_then(|r| r.options.as_ref()),
        );
        violations.extend(gql_lint_rules::require_description::require_description(
            schema, &opts,
        ));
    }

    if lint_config.enabled(gql_lint_core::rules::DESCRIPTION_STYLE) {
        let opts = gql_lint_rules::description_style::DescriptionStyleOptions::from_yaml(
            lint_config
                .rules
                .get(gql_lint_core::rules::DESCRIPTION_STYLE)
                .and_then(|r| r.options.as_ref()),
        );
        match gql_lint_rules::description_style::description_style(schema, &opts) {
            Ok(v) => violations.extend(v),
            Err(e) => eprintln!("description-style: {e}"),
        }
    }

    if lint_config.enabled(gql_lint_core::rules::NO_HASHTAG_DESCRIPTION) {
        match gql_lint_rules::no_hashtag_description::no_hashtag_description(schema) {
            Ok(v) => violations.extend(v),
            Err(e) => eprintln!("no-hashtag-description: {e}"),
        }
    }

    violations
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
            // `walkdir` preserves a literal `./` prefix when `root` is
            // `.`, unlike Go's `filepath.WalkDir`/`filepath.Join`, which
            // clean it away — an anchored exclude pattern like
            // `codegen-templates/**` (no leading `**/`) would otherwise
            // never match `./codegen-templates/...`, silently including
            // files `scripts/devbase.yaml` says to skip. Cleaning here
            // keeps both the exclude match and the reported file path
            // (via `Source::name`, downstream) parity with Go's own
            // cleaned paths.
            let path = clean_current_dir_prefix(&entry.into_path());
            if !exclude_set.is_match(&path) {
                files.push(path);
            }
        }
    }

    files.sort();
    Ok(files)
}

/// Removes every `.` (current-directory) component from `path`, the same
/// cleanup `filepath.Join`/`filepath.Clean` apply implicitly in Go —
/// `walkdir` does not do this on its own.
fn clean_current_dir_prefix(path: &std::path::Path) -> PathBuf {
    path.components()
        .filter(|c| !matches!(c, std::path::Component::CurDir))
        .collect()
}
