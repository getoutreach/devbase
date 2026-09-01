//! Shared library surface for the `gql-lint` binary (`src/main.rs`) and
//! its `criterion` benchmark suite (`benches/lint.rs`) — config loading,
//! file discovery, and the Tier 1/2/3 dispatch pipeline, split out of
//! `main.rs` so the benchmarks can call the exact same code path the CLI
//! runs, not a re-implementation of it.

pub mod config;
pub mod gitdiff;

use gql_lint_core::{Source, Tier1Result, Violation};
use std::path::{Path, PathBuf};

/// Runs Tier 1 validation over `sources`, then every Tier 2/3 rule
/// `lint_config` enables if Tier 1 passed. Merges in `lint_config`'s
/// federation/scalars prelude (see `gql_lint_core::federation`) before
/// Tier 1 runs, the same layering `federation.go`'s `preludeSources` uses
/// ahead of `parseAndValidate`.
///
/// `parallel` selects which of [`run_tier2_and_tier3_rules`]/
/// [`run_tier2_and_tier3_rules_parallel`] runs the Tier 2/3 pass — see the
/// latter's own doc comment for what "parallel" means here and why it's a
/// runtime choice rather than two copies of the calling code.
///
/// # Errors
/// See [`gql_lint_core::federation::FederationError`]'s variants.
pub fn lint_sources(
    sources: &[Source],
    lint_config: &config::Lint,
    parallel: bool,
) -> anyhow::Result<Vec<Violation>> {
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
        Tier1Result::Valid(schema) => {
            if parallel {
                run_tier2_and_tier3_rules_parallel(&schema, lint_config)
            } else {
                run_tier2_and_tier3_rules(&schema, lint_config)
            }
        }
    })
}

/// Runs every Tier 2/3 rule `lint_config` enables against `schema`,
/// collecting their violations. Split out of `main` because clippy's
/// `too_many_lines` pedantic lint (rightly) objects to bolting each new
/// rule's config lookup directly into `main` as the rule count grows.
#[must_use]
pub fn run_tier2_and_tier3_rules(
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

/// The `rayon`-parallel twin of [`run_tier2_and_tier3_rules`]: same rule
/// set, same config gating, same per-rule logic — the only difference is
/// that each enabled rule's own computation runs as an independent
/// `rayon` job instead of one after another.
///
/// **Parallelizes at rule level, matching the Go tool's own concurrency
/// model** (`internal/graphql/lint/tier3.go`'s `tier3`, which runs each of
/// its ~8 independent Tier 3 rules as a goroutine via `wg.Go`, over the
/// one already-built schema) — not at file level, which the Go tool
/// itself never does (`Files`/`FilesFromSources` builds one combined
/// schema from all files sequentially first). Every rule here already
/// reads only the shared `&Valid<Schema>` and writes nothing but its own
/// `Vec<Violation>`, the same independence `tier3`'s doc comment relies
/// on to justify unordered concurrent execution: `rayon`'s parallel
/// `collect` preserves each job's original position in the output
/// regardless of which one finishes first, the same "fixed order, not
/// finish order" guarantee `tier3`'s own comment calls out.
#[must_use]
pub fn run_tier2_and_tier3_rules_parallel(
    schema: &apollo_compiler::validation::Valid<apollo_compiler::Schema>,
    lint_config: &config::Lint,
) -> Vec<Violation> {
    use rayon::prelude::*;

    build_tier2_and_tier3_jobs(schema, lint_config)
        .into_par_iter()
        .map(|job| job())
        .flatten()
        .collect()
}

/// Builds one job per enabled Tier 2/3 rule for
/// [`run_tier2_and_tier3_rules_parallel`] to run — split out on its own so
/// that function stays under `clippy::pedantic`'s line-count cap.
fn build_tier2_and_tier3_jobs<'a>(
    schema: &'a apollo_compiler::validation::Valid<apollo_compiler::Schema>,
    lint_config: &'a config::Lint,
) -> Vec<Box<dyn Fn() -> Vec<Violation> + Sync + Send + 'a>> {
    let mut jobs: Vec<Box<dyn Fn() -> Vec<Violation> + Sync + Send + '_>> = Vec::new();

    jobs.push(Box::new(move || {
        gql_lint_rules::unique_directive_names_per_location(schema)
            .into_iter()
            .filter(|v| lint_config.enabled(v.rule))
            .collect()
    }));

    if lint_config.enabled(gql_lint_core::rules::ALPHABETIZE) {
        let opts = gql_lint_rules::alphabetize::AlphabetizeOptions::from_yaml(
            lint_config
                .rules
                .get(gql_lint_core::rules::ALPHABETIZE)
                .and_then(|r| r.options.as_ref()),
        );
        jobs.push(Box::new(move || {
            gql_lint_rules::alphabetize::alphabetize(schema, &opts)
        }));
    }

    if lint_config.enabled(gql_lint_core::rules::NO_TYPENAME_PREFIX) {
        jobs.push(Box::new(move || {
            gql_lint_rules::no_typename_prefix::no_typename_prefix(schema)
        }));
    }

    if lint_config.enabled(gql_lint_core::rules::NO_CASE_INSENSITIVE_ENUM_VALUES_DUPLICATES) {
        jobs.push(Box::new(move || {
            gql_lint_rules::no_case_insensitive_enum_values_duplicates::no_case_insensitive_enum_values_duplicates(
                schema,
            )
        }));
    }

    if lint_config.enabled(gql_lint_core::rules::NAMING_CONVENTION) {
        let opts = gql_lint_rules::naming_convention::NamingConventionOptions::from_yaml(
            lint_config
                .rules
                .get(gql_lint_core::rules::NAMING_CONVENTION)
                .and_then(|r| r.options.as_ref()),
        );
        jobs.push(Box::new(move || {
            gql_lint_rules::naming_convention::naming_convention(schema, &opts)
        }));
    }

    if lint_config.enabled(gql_lint_core::rules::NO_UNREACHABLE_TYPES) {
        jobs.push(Box::new(move || {
            gql_lint_rules::no_unreachable_types::no_unreachable_types(schema)
        }));
    }

    if lint_config.enabled(gql_lint_core::rules::REQUIRE_DEPRECATION_REASON) {
        jobs.push(Box::new(move || {
            gql_lint_rules::deprecation::require_deprecation_reason(schema)
        }));
    }

    if lint_config.enabled(gql_lint_core::rules::REQUIRE_DEPRECATION_DATE) {
        let opts = gql_lint_rules::deprecation::RequireDeprecationDateOptions::from_yaml(
            lint_config
                .rules
                .get(gql_lint_core::rules::REQUIRE_DEPRECATION_DATE)
                .and_then(|r| r.options.as_ref()),
        );
        let today = chrono::Local::now().date_naive();
        jobs.push(Box::new(move || {
            gql_lint_rules::deprecation::require_deprecation_date(schema, &opts, today)
        }));
    }

    jobs.extend(build_description_group_jobs(schema, lint_config));
    jobs
}

/// The `require-description`/`description-style`/`no-hashtag-description`
/// third of [`build_tier2_and_tier3_jobs`]'s jobs, split out on its own
/// (mirroring Go's own `descGroupEnabled` split in `tier3.go`) so that
/// function stays under `clippy::pedantic`'s line-count cap.
fn build_description_group_jobs<'a>(
    schema: &'a apollo_compiler::validation::Valid<apollo_compiler::Schema>,
    lint_config: &'a config::Lint,
) -> Vec<Box<dyn Fn() -> Vec<Violation> + Sync + Send + 'a>> {
    let mut jobs: Vec<Box<dyn Fn() -> Vec<Violation> + Sync + Send + '_>> = Vec::new();

    if lint_config.enabled(gql_lint_core::rules::REQUIRE_DESCRIPTION) {
        let opts = gql_lint_rules::require_description::RequireDescriptionOptions::from_yaml(
            lint_config
                .rules
                .get(gql_lint_core::rules::REQUIRE_DESCRIPTION)
                .and_then(|r| r.options.as_ref()),
        );
        jobs.push(Box::new(move || {
            gql_lint_rules::require_description::require_description(schema, &opts)
        }));
    }

    if lint_config.enabled(gql_lint_core::rules::DESCRIPTION_STYLE) {
        let opts = gql_lint_rules::description_style::DescriptionStyleOptions::from_yaml(
            lint_config
                .rules
                .get(gql_lint_core::rules::DESCRIPTION_STYLE)
                .and_then(|r| r.options.as_ref()),
        );
        jobs.push(Box::new(
            move || match gql_lint_rules::description_style::description_style(schema, &opts) {
                Ok(v) => v,
                Err(e) => {
                    eprintln!("description-style: {e}");
                    Vec::new()
                }
            },
        ));
    }

    if lint_config.enabled(gql_lint_core::rules::NO_HASHTAG_DESCRIPTION) {
        jobs.push(Box::new(move || {
            match gql_lint_rules::no_hashtag_description::no_hashtag_description(schema) {
                Ok(v) => v,
                Err(e) => {
                    eprintln!("no-hashtag-description: {e}");
                    Vec::new()
                }
            }
        }));
    }

    jobs
}

/// Prints each violation and reports whether any resolved to `Error`
/// severity — matching the Go tool's `reportViolations`.
#[must_use]
pub fn report(violations: &[Violation], cfg: &config::Lint) -> bool {
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
///
/// # Errors
/// If an `excludes` pattern is not a valid glob, or a directory under
/// `paths` cannot be walked.
pub fn find_graphql_files(paths: &[PathBuf], excludes: &[String]) -> anyhow::Result<Vec<PathBuf>> {
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
fn clean_current_dir_prefix(path: &Path) -> PathBuf {
    path.components()
        .filter(|c| !matches!(c, std::path::Component::CurDir))
        .collect()
}
