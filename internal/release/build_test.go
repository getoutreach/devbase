// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the GoReleaser-compatible builder.

package release

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/goreleaser"
	"gotest.tools/v3/assert"
)

// goreleaserFixture is a stencil-golang style .goreleaser.yml.
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
	f, err := os.Open(path)
	assert.NilError(t, err)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	assert.NilError(t, err)
	tr := tar.NewReader(gz)
	var out []string
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		out = append(out, os.FileMode(hdr.Mode).String()+" "+hdr.Name)
	}
	return out
}

func TestBuild(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not in PATH")
	}
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

	sums := filepath.Join(dir, "dist", ChecksumsFile)
	assert.NilError(t, WriteChecksums(sums, archives))
	b, err := os.ReadFile(sums)
	assert.NilError(t, err)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	assert.Equal(t, len(lines), 2)
	assert.Assert(t, strings.HasSuffix(lines[0], "  hello_1.2.3_darwin_arm64.tar.gz"), lines[0])
	assert.Assert(t, strings.HasSuffix(lines[1], "  hello_1.2.3_linux_amd64.tar.gz"), lines[1])
}
