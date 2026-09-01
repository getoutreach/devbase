//! Ports `internal/graphql/gitdiff/gitdiff.go`'s `MergeBaseFiles`: reads
//! each of a set of files, as they existed at the merge-base commit
//! between `HEAD` and a base ref, for `--diff` mode. Uses `gix` rather
//! than `go-git`'s Go equivalent — no git binary or `libgit2` needed at
//! runtime, same reason the Go tool picked `go-git`.

use anyhow::Context;
use std::collections::HashMap;
use std::path::{Path, PathBuf};

/// Opens the git repository containing `dir`, resolves the merge-base
/// commit between `HEAD` and `base`, and reads each of `files` (paths
/// relative to `dir`) from that commit's tree. The returned map is keyed
/// by the same [`PathBuf`]s given in `files`; a file that did not yet
/// exist at the merge-base is omitted rather than treated as an error,
/// since it can't have had any violations at that point.
///
/// # Errors
/// Fails fast if the repository is a shallow clone — its object store can
/// be missing the merge-base commit, so `--diff` refuses to risk an
/// incomplete or wrong comparison rather than silently computing one.
/// Also fails if `dir` isn't inside a git repository, `base` doesn't
/// resolve, or `HEAD`/`base` share no common ancestor.
pub fn merge_base_files(
    dir: &Path,
    base: &str,
    files: &[PathBuf],
) -> anyhow::Result<HashMap<PathBuf, String>> {
    let repo = gix::discover(dir)
        .with_context(|| format!("open git repository containing {}", dir.display()))?;

    if repo.is_shallow() {
        anyhow::bail!(
            "shallow clone: deepen it (e.g. `git fetch --unshallow`) before using --diff"
        );
    }

    let head = repo.head_id().context("resolve HEAD")?;
    let base_id = repo
        .rev_parse_single(base)
        .with_context(|| format!("resolve --base {base:?}"))?;
    let merge_base = repo
        .merge_base(head, base_id)
        .with_context(|| format!("compute merge-base of HEAD and {base:?}"))?;

    let root = repo
        .workdir()
        .with_context(|| format!("{} has no working tree", dir.display()))?;
    // `root` (gix's resolved worktree root) and `dir`/`files` (given by the
    // caller) can reach the same directory through different symlinks --
    // e.g. on macOS, where a temp directory's real path is under /private
    // but the caller's own cwd reports the /var symlink to it. Resolving
    // both sides before computing a relative path avoids silently computing
    // the wrong one, mirroring gitdiff.go's own comment on this exact bug.
    let real_root = std::fs::canonicalize(root)
        .with_context(|| format!("resolve symlinks for repository root {}", root.display()))?;

    let commit = merge_base
        .object()
        .context("load merge-base commit object")?
        .try_into_commit()
        .context("merge-base is not a commit")?;
    let tree = commit.tree().context("load merge-base commit's tree")?;

    let mut contents = HashMap::with_capacity(files.len());
    for file in files {
        let abs_path = if file.is_absolute() {
            file.clone()
        } else {
            dir.join(file)
        };
        let real_path = std::fs::canonicalize(&abs_path)
            .with_context(|| format!("resolve symlinks for {}", file.display()))?;
        let rel_path = real_path.strip_prefix(&real_root).with_context(|| {
            format!(
                "resolve {} relative to repository root {}",
                file.display(),
                root.display()
            )
        })?;

        let Some(entry) = tree
            .lookup_entry_by_path(rel_path)
            .with_context(|| format!("read {} at merge-base", file.display()))?
        else {
            continue;
        };
        let blob = entry
            .object()
            .with_context(|| format!("read {} at merge-base", file.display()))?
            .try_into_blob()
            .with_context(|| format!("{} at merge-base is not a file", file.display()))?;
        let text = String::from_utf8(blob.data.clone())
            .with_context(|| format!("{} at merge-base is not valid UTF-8", file.display()))?;
        contents.insert(file.clone(), text);
    }

    Ok(contents)
}
