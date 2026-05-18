# Bug: mise Error in E2E Tests After Stenciling CDC Producers

## Summary

When two `nodejs` entries exist in `.tool-versions` (added by the CDC producers
stencil template), the e2e CI bootstrap fails with a mise error because
`version_from_toolversions` returns a multi-line string that is passed as a
single invalid version argument to mise.

## Root Cause

**File:** [`shell/lib/mise.sh`](shell/lib/mise.sh) — lines 492–501

```bash
version_from_toolversions() {
  local repoDir="$1"
  local tool="$2"
  local version
  version="$(awk -v tool="$tool" '$1 == tool {print $2}' "$repoDir/.tool-versions")"
  if [[ -z $version ]]; then
    return 1
  fi
  echo "$version"
}
```

The `awk` expression has no `exit` guard — it prints **every** matching line.
With two `nodejs` entries in `.tool-versions`, `$version` becomes:

```
22.22.0
20.16.0
```

This multi-line string flows into `install_tool_with_mise` (line 184):

```bash
tool="$name@$version"   # → "node@22.22.0\n20.16.0"
run_mise use --global "$tool"
```

mise receives `"node@22.22.0\n20.16.0"` as a single argument — an invalid
version specifier — and errors out.

## Why Only E2E Tests Are Affected

The e2e job uses a slim bootstrap path (triggered by `INSTALL_E2E_TOOLS=true`)
in the following files:

- [`shell/ci/env/mise.sh`](shell/ci/env/mise.sh) — lines 54–57
- [`shell/circleci/machine.sh`](shell/circleci/machine.sh) — lines 58–61

Both manually call `version_from_toolversions` and pass the result to
`install_tool_with_mise node "$nodeVersion"`.

The normal (non-e2e) job calls `run_mise install --cd "$repoDir"` directly,
which lets mise resolve multiple nodejs entries from `.tool-versions` natively —
so it is unaffected.

## Applied Fix

Three files were changed:

### 1. [`shell/lib/mise.sh`](shell/lib/mise.sh)

- `version_from_toolversions` now exits after the first `awk` match (`{print $2; exit}`), preventing multi-line output when a tool has multiple entries.
- A new `version_all_from_toolversions` function was added that returns **all** declared versions (one per line), for callers that need to install every declared version.

### 2. [`shell/ci/env/mise.sh`](shell/ci/env/mise.sh)

The e2e bootstrap now loops over all nodejs versions via `version_all_from_toolversions`:

```bash
while IFS= read -r nodeVersion; do
  install_tool_with_mise node "$nodeVersion"
done < <(version_all_from_toolversions "$repoDir" nodejs ||
  fatal "nodejs version not found in $repoDir/.tool-versions")
```

### 3. [`shell/circleci/machine.sh`](shell/circleci/machine.sh)

Same loop applied to the machine executor bootstrap:

```bash
while IFS= read -r nodeVersion; do
  install_tool_with_mise node "$nodeVersion"
done < <(version_all_from_toolversions "$ROOT_DIR" nodejs ||
  fatal "nodejs version not found in $ROOT_DIR/.tool-versions")
```

Both callers now correctly install **all** declared nodejs versions in the e2e
environment, so both `22.22.0` (primary) and `20.16.0` (Knock CLI) are available.
