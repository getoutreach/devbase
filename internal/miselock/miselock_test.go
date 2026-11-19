// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for the mise lockfile checks.

package miselock

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

// parse parses a mise config and a lockfile, failing the test if
// either is invalid.
func parse(t *testing.T, config, lock string) (requested map[string][]string, locked map[string][]lockedTool) {
	t.Helper()
	requested, err := parseConfig([]byte(config))
	assert.NilError(t, err)
	locked, err = parseLockfile([]byte(lock))
	assert.NilError(t, err)
	return requested, locked
}

// TestMissingVersions confirms that each requested version needs a
// locked version that equals it, or that it is a dotted prefix of, and
// that "latest" accepts any locked version.
func TestMissingVersions(t *testing.T) {
	cases := []struct {
		name   string
		config string
		lock   string
		want   []string
	}{
		{
			name:   "reports a requested version missing from the lockfile",
			config: "[tools]\n\"github:getoutreach/ci\" = \"1.68.0\"\nkubectl = \"1.34.7\"\n",
			lock: "[[tools.\"github:getoutreach/ci\"]]\nversion = \"1.67.0\"\n" +
				"[[tools.kubectl]]\nversion = \"1.34.7\"\n",
			want: []string{"github:getoutreach/ci@1.68.0"},
		},
		{
			name: "checks inline table and array tool entries",
			config: "[tools]\nnode = { version = \"20.11.0\" }\ngojq = [\"0.12.17\", \"0.12.18\"]\n" +
				"kubectl = { version = \"1.34.7\", os = [\"linux\"] }\n",
			lock: "[[tools.node]]\nversion = \"20.11.0\"\n" +
				"[[tools.gojq]]\nversion = \"0.12.17\"\n",
			want: []string{"gojq@0.12.18", "kubectl@1.34.7"},
		},
		{
			name:   "accepts fuzzy and latest requests resolved in the lockfile",
			config: "[tools]\nnode = \"20\"\nshfmt = \"latest\"\n",
			lock: "[[tools.node]]\nversion = \"20.11.0\"\n" +
				"[[tools.shfmt]]\nversion = \"3.12.0\"\n",
			want: nil,
		},
		{
			name:   "a fuzzy request matches whole version components only",
			config: "[tools]\nterraform = \"1.2\"\n",
			lock:   "[[tools.terraform]]\nversion = \"1.20.0\"\n",
			want:   []string{"terraform@1.2"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requested, locked := parse(t, tc.config, tc.lock)
			assert.DeepEqual(t, missingVersions(requested, locked), tc.want)
		})
	}
}

// TestStaleTools confirms that a tool locked but not requested is
// reported, and that a tool requested but not locked is not.
func TestStaleTools(t *testing.T) {
	requested, locked := parse(t,
		"[tools]\nnode = \"20\"\ngojq = \"0.12.18\"\n",
		"[[tools.node]]\nversion = \"20.11.0\"\n"+
			"[[tools.kubectl]]\nversion = \"1.34.7\"\n"+
			"[[tools.aws-cli]]\nversion = \"2.34.31\"\n",
	)
	assert.DeepEqual(t, staleTools(requested, locked), []string{"aws-cli", "kubectl"})
}

// TestStaleVersions confirms that a locked version of a requested tool
// is reported when no request for that tool matches it.
func TestStaleVersions(t *testing.T) {
	cases := []struct {
		name   string
		config string
		lock   string
		want   []string
	}{
		{
			name:   "reports a locked version that is not requested",
			config: "[tools]\nnode = \"20.11.0\"\n",
			lock:   "[[tools.node]]\nversion = \"20.11.0\"\n[[tools.node]]\nversion = \"18.19.0\"\n",
			want:   []string{"node@18.19.0"},
		},
		{
			name:   "a fuzzy request matches only the versions it is a prefix of",
			config: "[tools]\nnode = \"20\"\n",
			lock:   "[[tools.node]]\nversion = \"20.11.0\"\n[[tools.node]]\nversion = \"200.0.0\"\n",
			want:   []string{"node@200.0.0"},
		},
		{
			name:   "latest matches any locked version",
			config: "[tools]\nshfmt = \"latest\"\n",
			lock:   "[[tools.shfmt]]\nversion = \"3.12.0\"\n",
			want:   nil,
		},
		{
			name:   "each version in an array request matches",
			config: "[tools]\ngojq = [\"0.12.17\", \"0.12.18\"]\n",
			lock: "[[tools.gojq]]\nversion = \"0.12.16\"\n[[tools.gojq]]\nversion = \"0.12.17\"\n" +
				"[[tools.gojq]]\nversion = \"0.12.18\"\n",
			want: []string{"gojq@0.12.16"},
		},
		{
			name:   "versions of a tool that is not requested are left to staleTools",
			config: "[tools]\n",
			lock:   "[[tools.kubectl]]\nversion = \"1.34.7\"\n",
			want:   nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requested, locked := parse(t, tc.config, tc.lock)
			assert.DeepEqual(t, staleVersions(requested, locked), tc.want)
		})
	}
}

// TestPlatformProblems confirms that a download backend needs a url on
// every platform it locks, and a checksum on every platform once any
// platform has one, unless its upstream publishes no checksums.
func TestPlatformProblems(t *testing.T) {
	cases := []struct {
		name string
		lock string
		want []string
	}{
		{
			name: "a url and a checksum on every platform pass",
			lock: `
[[tools.buf]]
version = "1.60.0"
backend = "aqua:bufbuild/buf"
[tools.buf."platforms.linux-x64"]
checksum = "sha256:aa"
url = "https://example.com/buf-linux"
[tools.buf."platforms.macos-arm64"]
checksum = "sha256:bb"
url = "https://example.com/buf-darwin"
`,
			want: nil,
		},
		{
			name: "reports a platform with no url",
			lock: `
[[tools."github:getoutreach/ci"]]
version = "1.6.14"
backend = "github:getoutreach/ci"
[tools."github:getoutreach/ci"."platforms.linux-x64"]
checksum = "sha256:aa"
url = "https://example.com/ci-linux"
[tools."github:getoutreach/ci"."platforms.macos-arm64"]
checksum = "sha256:bb"
`,
			want: []string{"github:getoutreach/ci@1.6.14 has no url for macos-arm64"},
		},
		{
			name: "reports platforms with no checksum when another platform has one",
			lock: `
[[tools.buf]]
version = "1.60.0"
backend = "aqua:bufbuild/buf"
[tools.buf."platforms.linux-x64"]
checksum = "sha256:aa"
url = "https://example.com/buf-linux"
[tools.buf."platforms.macos-x64"]
url = "https://example.com/buf-darwin-x64"
[tools.buf."platforms.macos-arm64"]
url = "https://example.com/buf-darwin-arm64"
`,
			want: []string{"buf@1.60.0 has no checksum for macos-arm64, macos-x64"},
		},
		{
			name: "no checksum on any platform passes",
			lock: `
[[tools.aws-cli]]
version = "2.34.31"
backend = "aqua:aws/aws-cli"
[tools.aws-cli."platforms.linux-x64"]
url = "https://example.com/aws-linux"
[tools.aws-cli."platforms.macos-arm64"]
url = "https://example.com/aws-darwin"
`,
			want: nil,
		},
		{
			name: "an allow-listed backend may have a checksum on only some platforms",
			lock: `
[[tools.aws-cli]]
version = "2.34.31"
backend = "aqua:aws/aws-cli"
[tools.aws-cli."platforms.linux-x64"]
checksum = "blake3:aa"
url = "https://example.com/aws-linux"
[tools.aws-cli."platforms.macos-arm64"]
url = "https://example.com/aws-darwin"
`,
			want: nil,
		},
		{
			name: "an allow-listed backend still needs a url on every platform",
			lock: `
[[tools.aws-cli]]
version = "2.34.31"
backend = "aqua:aws/aws-cli"
[tools.aws-cli."platforms.linux-x64"]
checksum = "blake3:aa"
url = "https://example.com/aws-linux"
[tools.aws-cli."platforms.macos-arm64"]
`,
			want: []string{"aws-cli@2.34.31 has no url for macos-arm64"},
		},
		{
			name: "reports a download backend that locks no platforms",
			lock: `
[[tools.buf]]
version = "1.60.0"
backend = "aqua:bufbuild/buf"
`,
			want: []string{"buf@1.60.0 locks no platforms"},
		},
		{
			name: "a backend that does not download per platform needs no platforms",
			lock: `
[[tools."go:golang.org/x/tools/cmd/goimports"]]
version = "0.43.0"
backend = "go:golang.org/x/tools/cmd/goimports"
`,
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			locked, err := parseLockfile([]byte(tc.lock))
			assert.NilError(t, err)
			assert.DeepEqual(t, platformProblems(locked), tc.want)
		})
	}
}

// writeFile writes content to name in dir and returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	assert.NilError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// TestCheck confirms Check reads the lockfile next to the config and
// reports each kind of problem against both files.
func TestCheck(t *testing.T) {
	dir := t.TempDir()
	config := writeFile(t, dir, "mise.test.toml", "[tools]\nnode = \"20\"\ngojq = \"0.12.18\"\n")
	lock := writeFile(t, dir, "mise.test.lock", `
[[tools.node]]
version = "20.11.0"
backend = "aqua:nodejs/node"
[tools.node."platforms.linux-x64"]
checksum = "sha256:aa"
url = "https://example.com/node-linux"

[[tools.node]]
version = "18.19.0"
backend = "aqua:nodejs/node"

[[tools."go:golang.org/x/tools/cmd/goimports"]]
version = "0.43.0"
backend = "go:golang.org/x/tools/cmd/goimports"
`)

	problems, err := Check(config)
	assert.NilError(t, err)
	assert.DeepEqual(t, problems, []string{
		lock + ": gojq@0.12.18 is requested in " + config + " but not locked",
		lock + ": go:golang.org/x/tools/cmd/goimports is locked but not requested in " + config,
		lock + ": node@18.19.0 is locked but matches no version requested in " + config,
		lock + ": node@18.19.0 locks no platforms",
	})
}

// TestCheckErrors confirms Check fails, instead of reporting no
// problems, when it cannot read the config or its lockfile.
func TestCheckErrors(t *testing.T) {
	t.Run("config path without a .toml extension", func(t *testing.T) {
		_, err := Check(writeFile(t, t.TempDir(), "mise.test.yaml", ""))
		assert.ErrorIs(t, err, errNotTOML)
	})

	t.Run("missing lockfile", func(t *testing.T) {
		_, err := Check(writeFile(t, t.TempDir(), "mise.test.toml", "[tools]\n"))
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("lockfile that is not in the array of tables format", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "mise.test.lock", "[tools.node]\nversion = \"20.11.0\"\n")
		_, err := Check(writeFile(t, dir, "mise.test.toml", "[tools]\nnode = \"20\"\n"))
		assert.ErrorContains(t, err, "parse mise lockfile")
	})
}
