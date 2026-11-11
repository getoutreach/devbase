// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the GoReleaser-compatible builder.

package release

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/goreleaser"
	"gotest.tools/v3/assert"
)

// goreleaserFixture is a stencil-golang style .goreleaser.yml. Its
// ldflags need HELLO_KEY in the environment.
const goreleaserFixture = `# yaml-language-server: $schema=https://goreleaser.com/static/schema.json
before:
  hooks:
    - touch hook-ran
builds:
  - main: ./cmd/hello
    id: &name hello
    binary: *name
    goos: [linux, darwin]
    goarch: [amd64, arm64]
    ignore:
      - goos: darwin
        goarch: amd64
    ldflags:
      - '-w -s -X "main.version=v{{ .Version }}"'
      - '-X "main.key={{ .Env.HELLO_KEY }}"'
    env:
      - CGO_ENABLED=0
archives:
  - files:
      - src: README*
      - src: bin/tool
        dst: tool
        info:
          mode: 0755
checksum:
  name_template: "checksums.txt"
release:
  disable: true
`

// requireGo skips the test when the go command is not in PATH.
func requireGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not in PATH")
	}
}

// writeFile writes a file under dir, creating parent directories.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	assert.NilError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	assert.NilError(t, os.WriteFile(p, []byte(content), 0o600))
}

// tarEntries returns "mode name" for each entry of a tar.gz.
func tarEntries(t *testing.T, path string) []string {
	t.Helper()
	hdrs := grTarHeaders(t, path)
	out := make([]string, 0, len(hdrs))
	for _, h := range hdrs {
		out = append(out, os.FileMode(h.Mode).String()+" "+h.Name)
	}
	return out
}

func TestBuild(t *testing.T) {
	requireGo(t)
	dir := t.TempDir()
	writeFile(t, dir, ".goreleaser.yml", goreleaserFixture)
	writeFile(t, dir, "go.mod", "module example.com/hello\n\ngo 1.25\n")
	writeFile(t, dir, "cmd/hello/main.go", "package main\n\nvar version, key string\n\nfunc main() { println(version, key) }\n")
	writeFile(t, dir, "README.md", "hello\n")
	writeFile(t, dir, "bin/tool", "#!/bin/sh\n")

	cfg, err := goreleaser.Parse([]byte(goreleaserFixture))
	assert.NilError(t, err)

	plan := &Plan{
		ProjectName: "hello",
		CommitTime:  1_700_000_000,
		NextRelease: &PlanRelease{Version: "1.2.3"},
		Targets:     []string{"linux/amd64", "linux/arm64", "darwin/arm64"},
	}
	_, err = Build(context.Background(), cfg, plan, &BuildOptions{Dir: dir, Targets: []string{"windows/amd64"}})
	assert.ErrorIs(t, err, ErrUnknownTarget)

	env := append(os.Environ(), "HELLO_KEY=k")
	archives, err := Build(context.Background(), cfg, plan, &BuildOptions{
		Dir: dir, Dist: "dist", Targets: []string{"linux/amd64", "darwin/arm64"}, Env: env,
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, archives, []string{
		filepath.Join(dir, "dist", "linux_amd64", "hello_1.2.3_linux_amd64.tar.gz"),
		filepath.Join(dir, "dist", "darwin_arm64", "hello_1.2.3_darwin_arm64.tar.gz"),
	})
	_, err = os.Stat(filepath.Join(dir, "hook-ran"))
	assert.NilError(t, err, "before hook did not run")

	// Extra files first, sorted by destination; then the binary as 0755.
	assert.DeepEqual(t, tarEntries(t, archives[0]), []string{
		"-rw------- README.md",
		"-rwxr-xr-x tool",
		"-rwxr-xr-x hello",
	})

	out, err := exec.CommandContext(context.Background(), "go", "version", "-m",
		filepath.Join(dir, "dist", "linux_amd64", "hello", "hello")).Output()
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(out), `-X \"main.version=v1.2.3\" -X \"main.key=k\"`), string(out))

	// TestGoReleaserUpstreamChecksums checks the line format and order.
	sums := filepath.Join(dir, "dist", "checksums.txt")
	assert.NilError(t, WriteChecksums(sums, archives))
	b, err := os.ReadFile(sums)
	assert.NilError(t, err)
	names := make([]string, 0, len(archives))
	for line := range strings.Lines(string(b)) {
		_, name, _ := strings.Cut(strings.TrimSuffix(line, "\n"), "  ")
		names = append(names, name)
	}
	slices.Sort(names)
	assert.DeepEqual(t, names, []string{"hello_1.2.3_darwin_arm64.tar.gz", "hello_1.2.3_linux_amd64.tar.gz"})
}
