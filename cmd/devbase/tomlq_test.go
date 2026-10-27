// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: End-to-end tests for the "tomlq" CLI subcommand.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// runTomlqCommand runs the "tomlq" subcommand like runYqCommand does
// for "yq".
func runTomlqCommand(t *testing.T, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return runSubcommand(t, newTomlqCommand(), stdin, args...)
}

// TestTomlqCommand_StdinFilter confirms `devbase tomlq <filter>` reads
// TOML from stdin and writes JSON to stdout.
func TestTomlqCommand_StdinFilter(t *testing.T) {
	stdout, _, err := runTomlqCommand(t, "[tools]\nkubectl = \"1.34.7\"\n", "-r", ".tools.kubectl")
	assert.NilError(t, err)
	assert.Equal(t, stdout, "1.34.7\n")

	stdout, _, err = runTomlqCommand(t, "n = 3\nwhen = 1979-05-27\n", "-c", ".")
	assert.NilError(t, err)
	assert.Equal(t, stdout, "{\"n\":3,\"when\":\"1979-05-27\"}\n")
}

// TestTomlqCommand_KeyOrder confirms output keeps the document's key
// order by default, and sorts it under -S.
func TestTomlqCommand_KeyOrder(t *testing.T) {
	const doc = "zebra = 1\nalpha = 2\n[tbl]\nz = 1\na = 2\n"

	stdout, _, err := runTomlqCommand(t, doc, "-c", ".")
	assert.NilError(t, err)
	assert.Equal(t, stdout, "{\"zebra\":1,\"alpha\":2,\"tbl\":{\"z\":1,\"a\":2}}\n")

	stdout, _, err = runTomlqCommand(t, doc, "-c", "-S", ".")
	assert.NilError(t, err)
	assert.Equal(t, stdout, "{\"alpha\":2,\"tbl\":{\"a\":2,\"z\":1},\"zebra\":1}\n")
}

// lockDriftFilter lists each tool version a mise TOML requests that
// its lockfile (bound to $lock, as tool -> locked versions) does not
// satisfy. A request is a plain string, an inline table's "version", or
// each string in an array. A request is satisfied by an exact match, by
// a locked version it is a dotted prefix of ("20" matches "20.11.0", but
// not "200.1.0"), or, for "latest", by any locked version.
const lockDriftFilter = `.tools | to_entries[] | .key as $tool
  | (.value | if type == "object" then .version elif type == "array" then .[] else . end)
  | select(type == "string") as $req
  | select(($lock[$tool] // []) | any(. == $req or startswith($req + ".") or $req == "latest") | not)
  | "\($tool)@\($req)"`

// TestTomlqCommand_LockfileDrift is the mise lockfile check from
// shell/lib/mise_test.bats, as tomlq runs it. It covers the TOML forms
// and the fuzzy versions that a line-based awk parse cannot handle.
func TestTomlqCommand_LockfileDrift(t *testing.T) {
	dir := t.TempDir()
	toml := filepath.Join(dir, "mise.toml")
	lock := filepath.Join(dir, "mise.lock")
	assert.NilError(t, os.WriteFile(toml, []byte(`[tools]
"github:getoutreach/devenv" = "1.68.0"
kubectl = "1.34.7"
a = { version = "1.2.3" }
b=["2", "3"]
c = "20"
d = "latest"
e = "1.0"
f = "20"
`), 0o600))
	assert.NilError(t, os.WriteFile(lock, []byte(`[[tools."github:getoutreach/devenv"]]
version = "1.67.0"

[[tools.kubectl]]
version = "1.34.7"

[tools.kubectl."platforms.linux-x64"]
url = "https://example.com"

[[tools.a]]
version = "1.2.3"

[[tools.b]]
version = "2.5.1"

[[tools.c]]
version = "20.11.0"

[[tools.d]]
version = "9.9.9"

[[tools.f]]
version = "200.1.0"
`), 0o600))

	locked, _, err := runTomlqCommand(t, "", "-c", ".tools | map_values([.[].version])", lock)
	assert.NilError(t, err)

	stdout, _, err := runTomlqCommand(t, "", "-r", "--argjson", "lock", locked, lockDriftFilter, toml)
	assert.NilError(t, err)
	assert.Equal(t, stdout, "b@3\ne@1.0\nf@20\ngithub:getoutreach/devenv@1.68.0\n")
}

// TestTomlqCommand_Errors confirms failures carry the tomlq prefix and
// a non-zero exit code, including for flags that cannot apply to TOML.
func TestTomlqCommand_Errors(t *testing.T) {
	for _, tt := range []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"invalid toml", "a = \n", []string{"."}, "tomlq: decode toml"},
		{"yaml output", "a = 1\n", []string{"-y", "."}, "tomlq: -y/--yaml-output is not supported"},
		{"in place", "", []string{"-i", ".", "x.toml"}, "tomlq: -i/--in-place is not supported"},
		{"toml output", "a = 1\n", []string{"-t", "."}, "tomlq: -t/--toml-output is not supported"},
		{"yaml only flag", "a = 1\n", []string{"--explicit-start", "."}, "tomlq: --explicit-start is not supported"},
		{"xml flag", "a = 1\n", []string{"-x", "."}, "tomlq: -x/--xml-output is not supported"},
		{"no filter", "", nil, "tomlq: missing filter argument"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, err := runTomlqCommand(t, tt.stdin, tt.args...)
			assert.Equal(t, exitCode(t, err), 1)
			assert.Assert(t, strings.Contains(stderr, tt.want), stderr)
		})
	}
}

// TestTomlqCommand_Version confirms -V identifies devbase tomlq.
func TestTomlqCommand_Version(t *testing.T) {
	stdout, _, err := runTomlqCommand(t, "", "-V")
	assert.NilError(t, err)
	assert.Assert(t, strings.HasPrefix(stdout, "devbase tomlq "), stdout)
}
