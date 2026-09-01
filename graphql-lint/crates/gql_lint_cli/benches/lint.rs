//! In-process benchmarks for `gql-lint`'s full pipeline (Tier 1 through
//! Tier 2/3), run against a real corpus rather than synthetic data —
//! `getoutreach/giraffe` (branch `claude/devbase-graphql-lint-config`),
//! attached to this project's session specifically so this suite doesn't
//! need a stand-in generator. **Requires a local clone at
//! `/home/user/giraffe`** — `cargo bench` panics with a clear message if
//! it's missing, rather than silently falling back to something smaller.
//!
//! This measures in-process Rust code only (`gql_lint_cli::lint_sources`
//! called directly on source text already in memory) — it is not the same
//! thing as a process-level timing comparison against the Go binary,
//! which lives in `benches/compare_with_go.sh` instead, since `criterion`
//! has no way to measure a separate process/language fairly.

use criterion::{BenchmarkId, Criterion, criterion_group, criterion_main};
use gql_lint_cli::config::Lint;
use gql_lint_core::Source;
use std::path::Path;

const GIRAFFE_ROOT: &str = "/home/user/giraffe";

/// Files with a real, pre-existing bug in giraffe's own schema, unrelated
/// to this port: `@markeplaceAuth(actors: [user, admin])` passes bare,
/// unquoted enum-shaped literals where the directive itself declares
/// `actors: [String!]!`. Confirmed (by running the Go binary against this
/// same corpus, which accepts it silently) to be a genuine
/// apollo-compiler-vs-gqlparser strictness difference, not a bug in this
/// port — but left in, it fails Tier 1 for the combined schema, so no
/// Tier 2/3 rule would ever run and this benchmark would only measure a
/// failing validation path. Excluded here, for benchmarking purposes
/// only — never patched, and never excluded from the real correctness
/// comparison in `compare_with_go.sh`'s own sanity check.
const EXCLUDED_FOR_BENCHMARK: &[&str] = &[
    "src/modules/extensibility/schema.graphql",
    "src/modules/support-admin/schema.graphql",
];

/// Turns a path relative to `GIRAFFE_ROOT` into a glob pattern anchored at
/// its real absolute location, so it matches the absolute paths
/// `gql_lint_cli::find_graphql_files` walks when given an absolute root
/// (unlike the CLI's own normal invocation from inside the repo with a
/// relative `.` root).
fn absolute_exclude(pattern: &str) -> String {
    format!("{GIRAFFE_ROOT}/{pattern}")
}

/// Loads giraffe's own `scripts/devbase.yaml` and every real `.graphql`
/// source under it (minus its own configured excludes and
/// [`EXCLUDED_FOR_BENCHMARK`]), sorted the same way the CLI's own file
/// discovery does.
///
/// # Panics
/// If `/home/user/giraffe` isn't a real clone of `getoutreach/giraffe`
/// (branch `claude/devbase-graphql-lint-config`) with its own
/// `scripts/devbase.yaml` in place.
fn load_giraffe() -> (Lint, Vec<Source>) {
    let root = Path::new(GIRAFFE_ROOT);
    let (config, _) = gql_lint_cli::config::load(root).unwrap_or_else(|e| {
        panic!(
            "load {GIRAFFE_ROOT}'s scripts/devbase.yaml: {e} — clone getoutreach/giraffe \
             (branch claude/devbase-graphql-lint-config) to {GIRAFFE_ROOT} to run this benchmark"
        )
    });

    let excludes: Vec<String> = config
        .exclude
        .iter()
        .map(String::as_str)
        .chain(EXCLUDED_FOR_BENCHMARK.iter().copied())
        .map(absolute_exclude)
        .collect();

    let files = gql_lint_cli::find_graphql_files(&[root.to_path_buf()], &excludes)
        .expect("walk /home/user/giraffe for *.graphql files");
    assert!(
        !files.is_empty(),
        "found no .graphql files under {GIRAFFE_ROOT}"
    );

    let sources = files
        .iter()
        .map(|path| Source {
            name: path.display().to_string(),
            text: std::fs::read_to_string(path)
                .unwrap_or_else(|e| panic!("read {}: {e}", path.display())),
        })
        .collect();
    (config, sources)
}

/// Benchmarks the full pipeline against one real, large file
/// (`src/modules/commit-bridge/schema.graphql`, 4,689 lines — the largest
/// in the corpus) — stresses one big parse/validate/lint pass, as opposed
/// to [`bench_corpus`]'s many-small-files shape. Needs the corpus's real
/// federation/scalars config even for a single file, since this file uses
/// federation directives and custom scalars declared through it.
fn bench_single_large_file(c: &mut Criterion) {
    let (config, sources) = load_giraffe();
    let large_file = sources
        .iter()
        .find(|s| s.name.ends_with("commit-bridge/schema.graphql"))
        .expect("commit-bridge/schema.graphql present in the corpus")
        .clone_for_bench();
    let single = std::slice::from_ref(&large_file);

    let mut group = c.benchmark_group("single_large_file");
    group.bench_function("sequential", |b| {
        b.iter(|| {
            gql_lint_cli::lint_sources(std::hint::black_box(single), &config, false).unwrap()
        });
    });
    group.bench_function("parallel", |b| {
        b.iter(|| gql_lint_cli::lint_sources(std::hint::black_box(single), &config, true).unwrap());
    });
    group.finish();
}

/// Benchmarks the full pipeline against the real corpus at a few sizes
/// (a stable prefix of the sorted file list, plus the full 230), both
/// sequentially and with `rayon`-parallel Tier 2/3 rule dispatch — the
/// comparison the plan's `rayon` question actually turns on: whether
/// parallelizing across rules pays for its own thread overhead at a
/// realistic corpus size.
fn bench_corpus(c: &mut Criterion) {
    let (config, sources) = load_giraffe();

    let mut group = c.benchmark_group("corpus");
    for size in [10, 50, 150, sources.len()] {
        let subset: Vec<Source> = sources
            .iter()
            .take(size)
            .map(Source::clone_for_bench)
            .collect();

        group.bench_with_input(
            BenchmarkId::new("sequential", size),
            &subset,
            |b, subset| {
                b.iter(|| {
                    gql_lint_cli::lint_sources(std::hint::black_box(subset), &config, false)
                        .unwrap()
                });
            },
        );
        group.bench_with_input(BenchmarkId::new("parallel", size), &subset, |b, subset| {
            b.iter(|| {
                gql_lint_cli::lint_sources(std::hint::black_box(subset), &config, true).unwrap()
            });
        });
    }
    group.finish();
}

/// `gql_lint_core::Source` derives no `Clone` (its normal callers only
/// ever build it once from a file read) — benchmarks need to re-run the
/// same input every iteration, so this crate-local extension clones it
/// for that purpose only, rather than adding a `Clone` impl the rest of
/// the port has never needed.
trait CloneForBench {
    fn clone_for_bench(&self) -> Self;
}

impl CloneForBench for Source {
    fn clone_for_bench(&self) -> Self {
        Source {
            name: self.name.clone(),
            text: self.text.clone(),
        }
    }
}

criterion_group!(benches, bench_single_large_file, bench_corpus);
criterion_main!(benches);
