//! `gql-lint` — a Rust re-implementation of `devbase graphql lint`
//! (`getoutreach/devbase`'s `cmd/devbase/lint_graphql.go`), built to
//! benchmark against the Go binary over the same rule set, config format,
//! and output shape. See the plan for the full build order. The actual
//! pipeline lives in this crate's own library (`src/lib.rs`), reused as-is
//! by `benches/lint.rs` — this file is only argument parsing and I/O.

use clap::Parser;
use gql_lint_cli::config;
use gql_lint_core::Source;
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

    /// Run Tier 2/3 rules as parallel `rayon` jobs instead of sequentially
    /// — see `gql_lint_cli::run_tier2_and_tier3_rules_parallel`'s doc
    /// comment for what "parallel" means here. A benchmarking knob, not a
    /// correctness-affecting one: output is identical either way.
    #[arg(long)]
    parallel: bool,
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

    let files = gql_lint_cli::find_graphql_files(&paths, &excludes)?;
    let sources = files
        .iter()
        .map(|path| -> anyhow::Result<Source> {
            Ok(Source {
                name: path.display().to_string(),
                text: std::fs::read_to_string(path)?,
            })
        })
        .collect::<anyhow::Result<Vec<_>>>()?;

    let mut violations = gql_lint_cli::lint_sources(&sources, &lint_config, cli.parallel)?;

    if cli.diff {
        let merge_base_content = gql_lint_cli::gitdiff::merge_base_files(&cwd, &cli.base, &files)?;
        let merge_base_sources: Vec<Source> = files
            .iter()
            .filter_map(|file| {
                merge_base_content.get(file).map(|text| Source {
                    name: file.display().to_string(),
                    text: text.clone(),
                })
            })
            .collect();
        let merge_base_violations =
            gql_lint_cli::lint_sources(&merge_base_sources, &lint_config, cli.parallel)?;
        violations = gql_lint_core::diff::new(&merge_base_violations, &violations);
    }

    let has_error = gql_lint_cli::report(&violations, &lint_config);
    Ok(if has_error {
        ExitCode::FAILURE
    } else {
        ExitCode::SUCCESS
    })
}
