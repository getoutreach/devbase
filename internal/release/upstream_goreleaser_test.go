// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Ports the GoReleaser v1.20.0 tests for builds, before
// hooks, archives, checksums and templates that apply to the builder,
// and the go-shellwords v1.0.12 tests that apply to before hooks.

package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/getoutreach/devbase/v2/internal/release/goreleaser"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

// grBase is the smallest valid .goreleaser.yml. Tests add top-level
// keys after it.
const grBase = `checksum:
  name_template: checksums.txt
builds:
  - id: hello
    binary: hello
    ldflags: [-s -w]
`

// grFakeGoScript is a fake go command. It writes its arguments and its
// environment to the -o file, or fails like an unknown flag when
// GR_FAKE_FAIL is set.
const grFakeGoScript = `#!/bin/sh
if [ -n "$GR_FAKE_FAIL" ]; then
  echo "flag provided but not defined: -flag-that-dont-exists-to-force-failure" >&2
  exit 2
fi
prev=
for a in "$@"; do
  if [ "$prev" = "-o" ]; then out=$a; fi
  prev=$a
done
mkdir -p "$(dirname "$out")"
{ for a in "$@"; do printf 'arg=%s\n' "$a"; done; env | sort; } > "$out"
`

// grFakeGo puts grFakeGoScript first in PATH as "go".
func grFakeGo(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake go command is a shell script")
	}
	bin := t.TempDir()
	writeFile(t, bin, "go", grFakeGoScript)
	assert.NilError(t, os.Chmod(filepath.Join(bin, "go"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// grWriteTree writes each file under dir with its name as the content.
func grWriteTree(t *testing.T, dir string, files ...string) {
	t.Helper()
	for _, f := range files {
		writeFile(t, dir, f, f+"\n")
	}
}

// grTarHeaders returns the headers of a tar.gz, in order.
func grTarHeaders(t *testing.T, path string) []*tar.Header {
	t.Helper()
	f, err := os.Open(path)
	assert.NilError(t, err)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	assert.NilError(t, err)
	tr := tar.NewReader(gz)
	var out []*tar.Header
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		assert.NilError(t, err)
		out = append(out, hdr)
	}
}

// grNames returns the names of tar headers.
func grNames(hdrs []*tar.Header) []string {
	if len(hdrs) == 0 {
		return nil
	}
	out := make([]string, 0, len(hdrs))
	for _, h := range hdrs {
		out = append(out, h.Name)
	}
	return out
}

// grArchiveFiles parses grBase with an archives[0].files YAML list,
// resolves the files in dir with version 1.2.3 and env, and writes them
// to a tar.gz. It returns the headers of that archive, or the first
// error, which can come from Parse.
func grArchiveFiles(t *testing.T, dir, files string, env ...string) ([]*tar.Header, error) {
	t.Helper()
	cfg, err := goreleaser.Parse([]byte(grBase + "archives:\n  - files:\n" + files))
	if err != nil {
		return nil, err
	}
	extras, err := archiveExtras(cfg, &spec.Template{Version: "1.2.3", Tag: "v1.2.3"}, env, dir)
	if err != nil {
		return nil, err
	}
	out := filepath.Join(t.TempDir(), "a.tar.gz")
	if err := writeArchive(out, extras, time.Unix(1_700_000_000, 0)); err != nil {
		return nil, err
	}
	return grTarHeaders(t, out), nil
}

// grBuild parses config and builds all of its targets with project
// "foobar" and version 0.0.1. env and HELLO_KEY=k are added to the
// process environment. It returns the first error, which can come from
// Parse.
func grBuild(t *testing.T, dir, config string, env ...string) ([]string, error) {
	t.Helper()
	cfg, err := goreleaser.Parse([]byte(config))
	if err != nil {
		return nil, err
	}
	plan := &Plan{ProjectName: "foobar", CommitTime: 1_700_000_000, NextRelease: &PlanRelease{Version: "0.0.1"}}
	for _, target := range cfg.Targets {
		plan.Targets = append(plan.Targets, target.String())
	}
	return Build(context.Background(), cfg, plan, &BuildOptions{
		Dir: dir, Dist: "dist", Env: slices.Concat(os.Environ(), []string{"HELLO_KEY=k"}, env),
	})
}

// grFakeBinary returns the arguments and the environment that the fake
// go command got for one binary.
func grFakeBinary(t *testing.T, path string) (args []string, env map[string]string) {
	t.Helper()
	b, err := os.ReadFile(path)
	assert.NilError(t, err)
	env = map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
		if a, ok := strings.CutPrefix(line, "arg="); ok {
			args = append(args, a)
		} else if k, v, ok := strings.Cut(line, "="); ok {
			env[k] = v
		}
	}
	return args, env
}

// grHooks runs only the before hooks of a config through Build.
func grHooks(t *testing.T, dir string, hooks []string, skip bool, env ...string) error {
	t.Helper()
	cfg := &spec.Artifacts{Hooks: hooks}
	plan := &Plan{ProjectName: "foobar", NextRelease: &PlanRelease{Version: "0.0.1"}}
	_, err := Build(context.Background(), cfg, plan, &BuildOptions{
		Dir: dir, Env: append(os.Environ(), env...), SkipHooks: skip,
	})
	return err
}

// grLDFlags renders the ldflags of one build for linux/amd64 with
// version 1.2.3, as buildTarget does: the templates see the process
// environment, the builds[].env items and the target variables.
func grLDFlags(ldflags, items, env []string) (string, error) {
	b := &spec.Build{ID: "foo", LDFlags: ldflags, Env: items}
	tmpl := &spec.Template{Version: "1.2.3", Tag: "v1.2.3"}
	t := spec.NewTarget("linux", "amd64")
	fullEnv, err := buildEnv(b, &t, tmpl, env)
	if err != nil {
		return "", err
	}
	return renderLDFlags(b, tmpl, fullEnv)
}

// TestGoReleaserUpstreamArchiveFiles ports the archives[].files cases of
// internal/archivefiles/archivefiles_test.go and
// internal/pipe/archive/archive_test.go. The cases that use "**" or a
// src outside the repository are rejected at Parse; the comment in each
// one gives the GoReleaser result.
func TestGoReleaserUpstreamArchiveFiles(t *testing.T) {
	newTree := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		grWriteTree(t, dir, "testdata/a/a.txt", "testdata/a/b/a.txt", "testdata/a/b/c/d.txt", "README.md")
		return dir
	}
	tree := []string{"testdata/a/a.txt", "testdata/a/b/a.txt", "testdata/a/b/c/d.txt"}
	cases := []struct {
		name, files string
		env         []string
		want        []string
		wantMode    os.FileMode
		wantErr     error
	}{
		{
			// GoReleaser fails while it globs.
			name:  "archivefiles_test.go TestEval/invalid glob",
			files: "      - src: ../testdata/**/nope.txt\n        dst: var/foobar/d.txt\n", wantErr: spec.ErrUnsupported,
		},
		{
			name:  "archivefiles_test.go TestEval/templated src (glob)",
			files: "      - src: './testdata/{{ .Env.FOLDER }}/*.txt'\n        dst: var/foobar/\n",
			env:   []string{"FOLDER=a"}, want: []string{"var/foobar/a.txt"},
		},
		{
			// goreleaser v1.20.0 gives var/foobar: an exact src takes dst as its name.
			name:  "archivefiles_test.go TestEval/templated src (exact file)",
			files: "      - src: './testdata/{{ .Env.FOLDER }}/a.txt'\n        dst: var/foobar/\n",
			env:   []string{"FOLDER=a"}, want: []string{"var/foobar"},
		},
		{
			// GoReleaser gives var/foobar/d.txt.
			name:  "archivefiles_test.go TestEval/single file",
			files: "      - src: ./testdata/**/d.txt\n        dst: var/foobar\n", wantErr: spec.ErrUnsupported,
		},
		{
			// GoReleaser gives foo/bar/a.txt and foo/bar/c/d.txt.
			name:  "archivefiles_test.go TestEval/rlcp",
			files: "      - src: ./testdata/a/**/*\n        dst: foo/bar\n", wantErr: spec.ErrUnsupported,
		},
		{
			// goreleaser v1.20.0 gives the same result for this glob without "**".
			name:  "archivefiles_test.go TestEval/rlcp (one level)",
			files: "      - src: ./testdata/a/*\n        dst: foo/bar\n",
			want:  []string{"foo/bar/a.txt", "foo/bar/b/a.txt", "foo/bar/b/c/d.txt"},
		},
		{
			name:  "archivefiles_test.go TestEval/rlcp empty destination (one level)",
			files: "      - src: ./testdata/a/*\n", want: tree,
		},
		{
			name:  "archivefiles_test.go TestEval/rlcp no results",
			files: "      - src: ./testdata/abc/*\n        dst: foo/bar\n",
		},
		{
			name:  "archivefiles_test.go TestEval/match multiple files within tree without destination",
			files: "      - src: ./testdata/a\n", want: tree,
		},
		{
			name:     "archivefiles_test.go TestEval/match multiple files within tree specific destination",
			files:    "      - src: ./testdata/a\n        dst: usr/local/test\n        info:\n          mode: 0755\n",
			want:     []string{"usr/local/test/a.txt", "usr/local/test/b/a.txt", "usr/local/test/b/c/d.txt"},
			wantMode: 0o755,
		},
		{
			name:  "archive_test.go TestArchive_globbing/exact src file",
			files: "      - src: testdata/a/a.txt\n", want: []string{"testdata/a/a.txt"},
		},
		{
			name:  "archive_test.go TestArchive_globbing/exact src file with dst",
			files: "      - src: testdata/a/a.txt\n        dst: foo/\n", want: []string{"foo"},
		},
		{
			// GoReleaser gives all three files.
			name:  "archive_test.go TestArchive_globbing/glob src",
			files: "      - src: testdata/**/*.txt\n", wantErr: spec.ErrUnsupported,
		},
		{
			// GoReleaser gives var/yada/a.txt, var/yada/b/a.txt and var/yada/b/c/d.txt.
			name:  "archive_test.go TestArchive_globbing/glob src with dst",
			files: "      - src: testdata/**/*.txt\n        dst: var/yada\n", wantErr: spec.ErrUnsupported,
		},
		{
			name:  "archive_test.go TestRunPipeInvalidGlob",
			files: "      - src: '[x-]'\n", wantErr: filepath.ErrBadPattern,
		},
		{
			name:  "archive_test.go TestDuplicateFilesInsideArchive (archivefiles.go unique)",
			files: "      - src: testdata/a/a.txt\n      - src: testdata/a/a.txt\n", want: []string{"testdata/a/a.txt"},
		},
		{
			name:  "archive_test.go TestDuplicateFilesInsideArchive (first file wins)",
			files: "      - src: testdata/a/a.txt\n        dst: x\n      - src: README.md\n        dst: x\n",
			want:  []string{"x"},
		},
		{
			name:  "config_archive_files_test.go TestArchiveFiles_stringFiles",
			files: "      - README.md\n      - src: README*\n", want: []string{"README.md"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hdrs, err := grArchiveFiles(t, newTree(t), tc.files, tc.env...)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.DeepEqual(t, grNames(hdrs), tc.want)
			if tc.wantMode != 0 {
				for _, h := range hdrs {
					assert.Equal(t, os.FileMode(h.Mode), tc.wantMode, h.Name)
				}
			}
		})
	}

	t.Run("archive_test.go TestDuplicateFilesInsideArchive (first file wins, content)", func(t *testing.T) {
		dir := newTree(t)
		cfg, err := goreleaser.Parse([]byte(grBase +
			"archives:\n  - files:\n      - {src: testdata/a/a.txt, dst: x}\n      - {src: README.md, dst: x}\n"))
		assert.NilError(t, err)
		extras, err := archiveExtras(cfg, &spec.Template{}, nil, dir)
		assert.NilError(t, err)
		assert.Equal(t, len(extras), 1)
		assert.Equal(t, extras[0].src, filepath.Join(dir, "testdata", "a", "a.txt"))
	})

	t.Run("archivefiles_test.go TestEval/templated src error", func(t *testing.T) {
		_, err := grArchiveFiles(t, newTree(t), "      - src: './testdata/{{ .Env.NOPE }}/*.txt'\n        dst: var/foobar/\n")
		assert.ErrorContains(t, err, `map has no entry for key "NOPE"`)
	})

	// Parse checks the src as written; the builder checks the rendered
	// src too, so a template cannot reach outside the repository.
	for name, value := range map[string]string{"outside the repository": "../secret.txt", "globstar": "testdata/**"} {
		t.Run("templated src "+name, func(t *testing.T) {
			_, err := grArchiveFiles(t, newTree(t), "      - src: '{{ .Env.X }}'\n", "X="+value)
			assert.ErrorIs(t, err, spec.ErrUnsupported)
		})
	}

	t.Run("archive_test.go TestRunPipeInvalidFilesNameTemplate", func(t *testing.T) {
		// GoReleaser fails while it archives; the port fails at Parse.
		_, err := grArchiveFiles(t, newTree(t), "      - src: '{{.asdsd}'\n")
		assert.ErrorIs(t, err, spec.ErrUnsupported)
		assert.ErrorContains(t, err, "template: ")
	})

	t.Run("archivefiles.go Eval (exact src that does not exist)", func(t *testing.T) {
		// goreleaser v1.20.0 fails: matching "./nope.txt": file does not exist.
		_, err := grArchiveFiles(t, newTree(t), "      - src: nope.txt\n")
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("archivefiles.go Eval (empty folder)", func(t *testing.T) {
		dir := newTree(t)
		assert.NilError(t, os.Mkdir(filepath.Join(dir, "empty"), 0o755))
		hdrs, err := grArchiveFiles(t, dir, "      - src: empty\n")
		assert.NilError(t, err)
		assert.Equal(t, len(hdrs), 0)
	})

	t.Run("archive_test.go TestDefault (default files)", func(t *testing.T) {
		dir := t.TempDir()
		grWriteTree(t, dir, "LICENSE", "README.md", "CHANGELOG.md", "main.go", "docs/README.md")
		hdrs, err := grArchiveFiles(t, dir, "      []\n")
		assert.NilError(t, err)
		assert.DeepEqual(t, grNames(hdrs), []string{"CHANGELOG.md", "LICENSE", "README.md"})
	})

	t.Run("archive_test.go TestDefaultSet (files replace the defaults)", func(t *testing.T) {
		dir := t.TempDir()
		grWriteTree(t, dir, "LICENSE", "README.md", "foo")
		hdrs, err := grArchiveFiles(t, dir, "      - src: foo\n")
		assert.NilError(t, err)
		assert.DeepEqual(t, grNames(hdrs), []string{"foo"})
	})
}

// TestGoReleaserUpstreamArchiveName ports the archive names of
// internal/pipe/archive/archive_test.go TestRunPipe and the default
// goarm, gomips and goamd64 of internal/builders/golang/build_test.go
// TestWithDefaults. The default name template is
// {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}, then "v" and
// goarm, "_" and gomips, and goamd64 unless it is v1.
func TestGoReleaserUpstreamArchiveName(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target spec.Target
		want   string
	}{
		{"archive_test.go TestRunPipe/darwin amd64 v1", spec.NewTarget("darwin", "amd64"), "darwin_amd64"},
		{"archive_test.go TestRunPipe/linux 386", spec.NewTarget("linux", "386"), "linux_386"},
		{"archive_test.go TestBinaryOverride/darwin arm64", spec.NewTarget("darwin", "arm64"), "darwin_arm64"},
		{"archive_test.go TestRunPipe/linux arm (goarm 6)", spec.NewTarget("linux", "arm"), "linux_armv6"},
		{
			"build_test.go TestWithDefaults/custom targets no mips (gomips hardfloat)",
			spec.NewTarget("linux", "mips"),
			"linux_mips_hardfloat",
		},
		{
			"build_test.go TestWithDefaults/custom targets no mips64le (gomips hardfloat)",
			spec.NewTarget("linux", "mips64le"),
			"linux_mips64le_hardfloat",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, ArchiveName("foobar", "0.0.1", &tc.target), "foobar_0.0.1_"+tc.want+".tar.gz")
		})
	}
}

// TestGoReleaserUpstreamChecksums ports
// internal/pipe/checksums/checksums_test.go for sha256 and checksums.txt.
func TestGoReleaserUpstreamChecksums(t *testing.T) {
	const sum = "61d034473102d7dac305902770471fd50f4c5b26f6831a56dd90b5184b3c30fc  "

	t.Run("checksums_test.go TestPipe/default", func(t *testing.T) {
		dir := t.TempDir()
		files := make([]string, 0, 3)
		for _, name := range []string{"binary.tar.gz", "binary", "binary.rpm"} {
			files = append(files, filepath.Join(dir, name))
			writeFile(t, dir, name, "some string")
		}
		out := filepath.Join(dir, "checksums.txt")
		assert.NilError(t, WriteChecksums(out, files))
		b, err := os.ReadFile(out)
		assert.NilError(t, err)
		assert.Equal(t, string(b), sum+"binary\n"+sum+"binary.rpm\n"+sum+"binary.tar.gz\n")
	})

	t.Run("checksums.go refresh (sort.Strings of the lines)", func(t *testing.T) {
		// The lines sort by checksum, not by file name.
		dir := t.TempDir()
		writeFile(t, dir, "a.tar.gz", "a")
		writeFile(t, dir, "b.tar.gz", "b")
		out := filepath.Join(dir, "checksums.txt")
		assert.NilError(t, WriteChecksums(out, []string{filepath.Join(dir, "a.tar.gz"), filepath.Join(dir, "b.tar.gz")}))
		b, err := os.ReadFile(out)
		assert.NilError(t, err)
		assert.Equal(t, string(b),
			"3e23e8160039594a33894f6564e1b1348bbd7a0088d42c4acb73eeaed59c009d  b.tar.gz\n"+
				"ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb  a.tar.gz\n")
	})

	t.Run("checksums.go refresh (file mode 0644)", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "a.tar.gz", "a")
		out := filepath.Join(dir, "checksums.txt")
		assert.NilError(t, WriteChecksums(out, []string{filepath.Join(dir, "a.tar.gz")}))
		info, err := os.Stat(out)
		assert.NilError(t, err)
		assert.Equal(t, info.Mode().Perm(), os.FileMode(0o644)&^grUmask(t))
	})

	t.Run("checksums_test.go TestRefreshModifying", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "binary")
		writeFile(t, dir, "binary", "some string")
		out := filepath.Join(dir, "checksums.txt")
		assert.NilError(t, WriteChecksums(out, []string{file}))
		previous, err := os.ReadFile(out)
		assert.NilError(t, err)
		writeFile(t, dir, "binary", "some other string")
		assert.NilError(t, WriteChecksums(out, []string{file}))
		current, err := os.ReadFile(out)
		assert.NilError(t, err)
		assert.Assert(t, !bytes.Equal(previous, current))
	})

	t.Run("checksums_test.go TestPipeFileNotExist", func(t *testing.T) {
		err := WriteChecksums(filepath.Join(t.TempDir(), "checksums.txt"), []string{"/nope"})
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("checksums_test.go TestPipeCouldNotOpenChecksumsTxt", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root can write a file with mode 0000")
		}
		dir := t.TempDir()
		writeFile(t, dir, "bin", "fake artifact")
		out := filepath.Join(dir, "checksums.txt")
		assert.NilError(t, os.WriteFile(out, []byte("some string"), 0o000))
		err := WriteChecksums(out, []string{filepath.Join(dir, "bin")})
		assert.ErrorIs(t, err, os.ErrPermission)
	})

	t.Run("checksums_test.go TestPipeWhenNoArtifacts", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "checksums.txt")
		assert.NilError(t, WriteChecksums(out, nil))
		_, err := os.Stat(out)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
}

// grUmask returns the process umask, which os.WriteFile applies.
func grUmask(t *testing.T) os.FileMode {
	t.Helper()
	f := filepath.Join(t.TempDir(), "umask")
	assert.NilError(t, os.WriteFile(f, nil, 0o777))
	info, err := os.Stat(f)
	assert.NilError(t, err)
	return 0o777 &^ info.Mode().Perm()
}

// TestGoReleaserUpstreamBeforeHooks ports
// internal/pipe/before/before_test.go.
func TestGoReleaserUpstreamBeforeHooks(t *testing.T) {
	t.Run("before_test.go TestRunPipe", func(t *testing.T) {
		requireGo(t)
		dir := t.TempDir()
		writeFile(t, dir, "go.mod", "module example.com/hello\n\ngo 1.25\n")
		writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
		for _, hooks := range [][]string{nil, {}, {"go version"}, {"go version", "go list"}} {
			assert.NilError(t, grHooks(t, dir, hooks, false), "%q", hooks)
		}
	})

	t.Run("before_test.go TestRunPipe (quoted arguments)", func(t *testing.T) {
		dir := t.TempDir()
		assert.NilError(t, grHooks(t, dir, []string{`sh -c "echo \"lala spaces and such\" > out.txt"`}, false))
		b, err := os.ReadFile(filepath.Join(dir, "out.txt"))
		assert.NilError(t, err)
		assert.Equal(t, string(b), "lala spaces and such\n")
	})

	t.Run("before_test.go TestRunPipeInvalidCommand", func(t *testing.T) {
		err := grHooks(t, t.TempDir(), []string{`bash -c "echo \"unterminated command\"`}, false)
		assert.ErrorIs(t, err, errInvalidCommandLine)
		assert.ErrorContains(t, err, "invalid command line string")
	})

	t.Run("before_test.go TestRunPipeFail/go tool foobar", func(t *testing.T) {
		requireGo(t)
		err := grHooks(t, t.TempDir(), []string{"go tool foobar"}, false)
		assert.ErrorContains(t, err, "go tool foobar")
		assert.ErrorContains(t, err, "exit status 2")
		assert.ErrorContains(t, err, `no such tool "foobar"`)
	})

	t.Run("before_test.go TestRunPipeFail/sh ./testdata/foo.sh", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "testdata/foo.sh", "echo lalala\nexit 1\n")
		err := grHooks(t, dir, []string{"sh ./testdata/foo.sh"}, false)
		assert.ErrorContains(t, err, "sh ./testdata/foo.sh")
		assert.ErrorContains(t, err, "exit status 1")
		assert.ErrorContains(t, err, "lalala")
	})

	t.Run("before_test.go TestRunWithEnv", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(t.TempDir(), "testfile")
		assert.NilError(t, grHooks(t, dir, []string{"touch {{ .Env.TEST_FILE }}"}, false, "TEST_FILE="+f))
		_, err := os.Stat(f)
		assert.NilError(t, err)
	})

	t.Run("before_test.go TestInvalidTemplate", func(t *testing.T) {
		// GoReleaser fails while it runs the hooks; the port fails at Parse.
		_, err := goreleaser.Parse([]byte(grBase + "before:\n  hooks: ['touch {{ .fasdsd }']\n"))
		assert.ErrorIs(t, err, spec.ErrUnsupported)
		assert.ErrorContains(t, err, `unexpected "}" in operand`)
		err = grHooks(t, t.TempDir(), []string{"touch {{ .fasdsd }"}, false)
		assert.ErrorIs(t, err, spec.ErrUnsupported)
		assert.ErrorContains(t, err, `unexpected "}" in operand`)
	})

	t.Run("before_test.go TestSkip/skip before", func(t *testing.T) {
		dir := t.TempDir()
		assert.NilError(t, grHooks(t, dir, []string{"touch ran"}, true))
		_, err := os.Stat(filepath.Join(dir, "ran"))
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("before_test.go TestSkip/dont skip (empty hook)", func(t *testing.T) {
		// goreleaser v1.20.0 panics on an empty hook (slice bounds out of
		// range); the port returns an error.
		err := grHooks(t, t.TempDir(), []string{""}, false)
		assert.ErrorIs(t, err, errEmptyHook)
	})
}

// TestGoReleaserUpstreamShellWords ports the cases of
// github.com/caarlos0/go-shellwords v1.0.12 shellwords_test.go and
// shellwords_posix_test.go that apply with ParseEnv and ParseBacktick
// off, which is how GoReleaser parses before hooks.
func TestGoReleaserUpstreamShellWords(t *testing.T) {
	for _, tc := range []struct {
		name, line string
		want       []string
	}{
		{"shellwords_test.go TestSimple/0", ``, []string{}},
		{"shellwords_test.go TestSimple/1", `""`, []string{``}},
		{"shellwords_test.go TestSimple/2", `''`, []string{``}},
		{"shellwords_test.go TestSimple/3", `var --bar=baz`, []string{`var`, `--bar=baz`}},
		{"shellwords_test.go TestSimple/4", `var --bar="baz"`, []string{`var`, `--bar=baz`}},
		{"shellwords_test.go TestSimple/5", `var "--bar=baz"`, []string{`var`, `--bar=baz`}},
		{"shellwords_test.go TestSimple/6", `var "--bar='baz'"`, []string{`var`, `--bar='baz'`}},
		{"shellwords_test.go TestSimple/7", "var --bar=`baz`", []string{`var`, "--bar=`baz`"}},
		{"shellwords_test.go TestSimple/8", `var --bar='\'`, []string{`var`, `--bar=\`}},
		{"shellwords_test.go TestSimple/9", `var "--bar baz"`, []string{`var`, `--bar baz`}},
		{"shellwords_test.go TestSimple/10", `var --"bar baz"`, []string{`var`, `--bar baz`}},
		{"shellwords_test.go TestSimple/11", `var  --"bar baz"`, []string{`var`, `--bar baz`}},
		{"shellwords_test.go TestSimple/12", `a "b"`, []string{`a`, `b`}},
		{"shellwords_test.go TestSimple/13", `a " b "`, []string{`a`, ` b `}},
		{"shellwords_test.go TestSimple/14", `a "   "`, []string{`a`, `   `}},
		{"shellwords_test.go TestSimple/15", `a 'b'`, []string{`a`, `b`}},
		{"shellwords_test.go TestSimple/16", `a ' b '`, []string{`a`, ` b `}},
		{"shellwords_test.go TestSimple/17", `a '   '`, []string{`a`, `   `}},
		{"shellwords_test.go TestSimple/18", `foo "" bar ''`, []string{`foo`, ``, `bar`, ``}},
		{"shellwords_test.go TestSimple/19", `foo \\`, []string{`foo`, `\`}},
		{"shellwords_test.go TestSimple/20", `foo \& bar`, []string{`foo`, `&`, `bar`}},
		{"shellwords_test.go TestBacktick (ParseBacktick off)/0", `echo $(echo "foo")`, []string{"echo", `$(echo "foo")`}},
		{"shellwords_test.go TestBacktick (ParseBacktick off)/1", "echo $(`echo1)", []string{"echo", "$(`echo1)"}},
		{"shellwords_test.go TestBacktickError (ParseBacktick off)/0", `echo $(echo1)`, []string{"echo", "$(echo1)"}},
		{"shellwords_test.go TestBacktickError (ParseBacktick off)/1", `echo FOO=$(echo1)`, []string{"echo", "FOO=$(echo1)"}},
		{"shellwords_test.go TestHaveMore", "echo 🍺; seq 1 10", []string{"echo", "🍺"}},
		{"shellwords_test.go TestHaveRedirect", "ls -la 2>foo", []string{"ls", "-la"}},
		{
			"shellwords_test.go TestEnvInQuoted (ParseEnv off)/0", `ssh 127.0.0.1 "echo $FOO"`,
			[]string{"ssh", "127.0.0.1", "echo $FOO"},
		},
		{
			"shellwords_test.go TestEnvInQuoted (ParseEnv off)/1", `ssh 127.0.0.1 "echo \\$FOO"`,
			[]string{"ssh", "127.0.0.1", `echo \$FOO`},
		},
		{
			"shellwords_test.go TestParseWithEnvs (Parse only)/0", "FOO=foo cmd --args=A=B",
			[]string{"FOO=foo", "cmd", "--args=A=B"},
		},
		{
			"shellwords_test.go TestParseWithEnvs (Parse only)/2", `sh -c "FOO=foo BAR=bar cmd --args=A=B -A=B"`,
			[]string{"sh", "-c", "FOO=foo BAR=bar cmd --args=A=B -A=B"},
		},
		{"shellwords_posix_test.go TestEscaping/0", "foo bar\\  ", []string{`foo`, `bar `}},
		{"shellwords_posix_test.go TestEscaping/1", "foo 'bar\\ '", []string{`foo`, `bar\ `}},
		{"shellwords_posix_test.go TestEscaping/2", `var "--bar=\"baz'"`, []string{`var`, `--bar="baz'`}},
		{"shellwords_posix_test.go TestEscaping/3", `var "--bar=\'baz\'"`, []string{`var`, `--bar='baz'`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := splitShellWords(tc.line)
			assert.NilError(t, err)
			assert.DeepEqual(t, got, tc.want)
		})
	}

	for _, tc := range []struct{ name, line string }{
		{"shellwords_test.go TestError/0", "foo '"},
		{"shellwords_test.go TestError/1", `foo "`},
		{"shellwords_test.go TestError/2", "foo `"},
		{"shellwords_test.go TestBacktickError (ParseBacktick off)/2", `echo $(echo1`},
		{"shellwords_test.go TestBacktickError (ParseBacktick off)/3", `echo $ (echo1`},
		{"shellwords_test.go TestBacktickError (ParseBacktick off)/4", `echo (echo1`},
		{"shellwords_test.go TestBacktickError (ParseBacktick off)/5", `echo )echo1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := splitShellWords(tc.line)
			assert.ErrorIs(t, err, errInvalidCommandLine)
		})
	}
}

// TestGoReleaserUpstreamTemplates ports the .Version, .Tag and .Env
// cases of internal/tmpl/tmpl_test.go and the ldflags cases of
// internal/builders/golang/build_test.go.
func TestGoReleaserUpstreamTemplates(t *testing.T) {
	env := []string{"FOO=bar"}
	for _, tc := range []struct{ name, tmpl, want string }{
		{"tmpl_test.go TestWithArtifact/.Env.FOO", "{{.Env.FOO}}", "bar"},
		{"tmpl_test.go TestWithArtifact/.Version", "{{.Version}}", "1.2.3"},
		{"tmpl_test.go TestWithArtifact/.Tag", "{{.Tag}}", "v1.2.3"},
		{"tmpl_test.go TestEnv/with env", "{{ .Env.FOO }}", "bar"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := grLDFlags([]string{tc.tmpl}, nil, env)
			assert.NilError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}

	for _, tc := range []struct {
		name, tmpl, wantErr string
		// atParse is true when the port fails at Parse with ErrUnsupported.
		// GoReleaser fails while it builds.
		atParse bool
	}{
		{"tmpl_test.go TestEnv/missing env", "{{ .Env.BAR }}", `map has no entry for key "BAR"`, false},
		{"tmpl_test.go TestEnvNotFound", "{{.Env.FOO}}", `map has no entry for key "FOO"`, false},
		{"tmpl_test.go TestInvalidTemplate", "{{{.Foo}", `unexpected "{" in command`, true},
		{"build_test.go TestInvalidTemplate/{{ .Nope }", "{{ .Nope }", `unexpected "}" in operand`, true},
		{"build_test.go TestInvalidTemplate/{{.Env.NOPE}}", "{{.Env.NOPE}}", `map has no entry for key "NOPE"`, false},
		{"build_test.go TestRunInvalidLdflags", "-s -w -X main.version={{.Version}", "template: ", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := grLDFlags([]string{tc.tmpl}, nil, nil)
			assert.ErrorContains(t, err, tc.wantErr)
			assert.Equal(t, got, "")
			_, err = goreleaser.Parse([]byte(strings.Replace(grBase, "[-s -w]", "['"+tc.tmpl+"']", 1)))
			if !tc.atParse {
				assert.NilError(t, err)
				return
			}
			assert.ErrorIs(t, err, spec.ErrUnsupported)
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}

	t.Run("tmpl_test.go TestWithEnv", func(t *testing.T) {
		env := []string{"FOO=BAR", "FOO=foo", "BAR=bar", "NOVAL=", "=NOKEY", "=", "NOTHING"}
		got, err := grLDFlags([]string{"{{ .Env.FOO }}-{{ .Env.BAR }}"}, nil, env)
		assert.NilError(t, err)
		assert.Equal(t, got, "foo-bar")
	})

	t.Run("tmpl_test.go TestWithEnv (range over .Env)", func(t *testing.T) {
		env := []string{"FOO=foo", "BAR=bar", "NOVAL=", "=NOKEY", "=", "NOTHING"}
		got, err := grApply(`{{ range $idx, $key := .Env }}{{ $idx }},{{ end }}`, env)
		assert.NilError(t, err)
		assert.Equal(t, got, "BAR,FOO,NOVAL,")
	})

	t.Run("build_test.go TestLdFlagsFullTemplate (.Version, .Tag, .Env)", func(t *testing.T) {
		got, err := grLDFlags([]string{
			`-s -w -X main.version={{.Version}} -X main.tag={{.Tag}} -X "main.foo={{.Env.FOO}}"`,
		}, nil, []string{"FOO=123"})
		assert.NilError(t, err)
		assert.Equal(t, got, `-s -w -X main.version=1.2.3 -X main.tag=v1.2.3 -X "main.foo=123"`)
	})

	t.Run("build_test.go TestBuildGoBuildLine/ldflags2", func(t *testing.T) {
		got, err := grLDFlags([]string{"-s -w", "-X main.version={{.Version}}"}, nil, nil)
		assert.NilError(t, err)
		assert.Equal(t, got, "-s -w -X main.version=1.2.3")
	})

	t.Run("build.go processFlags (builds[].env in ldflags templates)", func(t *testing.T) {
		// goreleaser v1.20.0 gives main.key=0 for this build.
		got, err := grLDFlags([]string{`-X "main.key={{ .Env.CGO_ENABLED }}"`}, []string{"CGO_ENABLED=0"}, nil)
		assert.NilError(t, err)
		assert.Equal(t, got, `-X "main.key=0"`)
	})

	t.Run("build.go processFlags (target variables in ldflags templates)", func(t *testing.T) {
		got, err := grLDFlags([]string{"{{ .Env.GOOS }}_{{ .Env.GOARCH }}_{{ .Env.GOAMD64 }}"}, nil, nil)
		assert.NilError(t, err)
		assert.Equal(t, got, "linux_amd64_v1")
	})

	t.Run("build.go Build (builds[].env items see the items before them)", func(t *testing.T) {
		got, err := grLDFlags([]string{"{{ .Env.B }}"}, []string{"A=1", "B={{ .Env.A }}2"}, nil)
		assert.NilError(t, err)
		assert.Equal(t, got, "12")
	})
}

// grApply applies one template with version 1.2.3 and env.
func grApply(s string, env []string) (string, error) {
	return (&spec.Template{Version: "1.2.3", Tag: "v1.2.3"}).Apply(s, env)
}

// TestGoReleaserUpstreamBuild ports the go build cases of
// internal/builders/golang/build_test.go and internal/pipe/build/build_test.go,
// and the archive layout of internal/pipe/archive/archive_test.go, with a
// fake go command.
func TestGoReleaserUpstreamBuild(t *testing.T) {
	grFakeGo(t)
	newDir := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		grWriteTree(t, dir, "README.md", "bin/tool")
		return dir
	}
	binPath := func(dir, target, name string) string {
		return filepath.Join(dir, "dist", target, "hello", name)
	}

	t.Run("build_test.go TestBuildGoBuildLine/ldflags2 and archive_test.go TestRunPipe", func(t *testing.T) {
		dir := newDir(t)
		archives, err := grBuild(t, dir, goreleaserFixture)
		assert.NilError(t, err)
		names := make([]string, 0, len(archives))
		for _, a := range archives {
			names = append(names, filepath.Base(a))
			// Extra files sorted by destination, then the binaries.
			assert.DeepEqual(t, grNames(grTarHeaders(t, a)), []string{"README.md", "tool", "hello"})
		}
		assert.DeepEqual(t, names, []string{
			"foobar_0.0.1_linux_amd64.tar.gz", "foobar_0.0.1_linux_arm64.tar.gz", "foobar_0.0.1_darwin_arm64.tar.gz",
		})
		bin := binPath(dir, "linux_amd64", "hello")
		args, env := grFakeBinary(t, bin)
		assert.DeepEqual(t, args, []string{
			"build", `-ldflags=-w -s -X "main.version=v0.0.1" -X "main.key=k"`, "-o", bin, "./cmd/hello",
		})
		assert.Equal(t, env["GOOS"], "linux")
		assert.Equal(t, env["GOARCH"], "amd64")
		assert.Equal(t, env["CGO_ENABLED"], "0")
	})

	t.Run("build_test.go TestBuildFailed", func(t *testing.T) {
		dir := newDir(t)
		archives, err := grBuild(t, dir, goreleaserFixture, "GR_FAKE_FAIL=1")
		assert.ErrorContains(t, err, "flag provided but not defined: -flag-that-dont-exists-to-force-failure")
		assert.Assert(t, archives == nil)
	})

	t.Run("build.go Build (target variables)", func(t *testing.T) {
		// GoReleaser sets every variant variable, so the process
		// environment cannot change the build.
		t.Setenv("GOAMD64", "v3")
		t.Setenv("GOARM", "7")
		dir := newDir(t)
		config := strings.Replace(goreleaserFixture, "goarch: [amd64, arm64]", "goarch: [amd64, arm, mips]", 1)
		_, err := grBuild(t, dir, config)
		assert.NilError(t, err)
		for target, want := range map[string][3]string{
			"linux_amd64": {"v1", "", ""},
			"linux_arm":   {"", "6", ""},
			"linux_mips":  {"", "", "hardfloat"},
		} {
			_, env := grFakeBinary(t, binPath(dir, target, "hello"))
			got := [3]string{env["GOAMD64"], env["GOARM"], env["GOMIPS"]}
			assert.Equal(t, got, want, target)
			assert.Equal(t, env["GOMIPS64"], want[2], target)
		}
	})

	t.Run("build_test.go TestBuild (binary extensions)", func(t *testing.T) {
		dir := newDir(t)
		config := strings.Replace(goreleaserFixture, "goos: [linux, darwin]", "goos: [linux, windows, js]", 1)
		config = strings.Replace(config, "goarch: [amd64, arm64]", "goarch: [amd64, wasm]", 1)
		archives, err := grBuild(t, dir, config)
		assert.NilError(t, err)
		got := map[string][]string{}
		for _, a := range archives {
			got[filepath.Base(a)] = grNames(grTarHeaders(t, a))
		}
		assert.DeepEqual(t, got, map[string][]string{
			"foobar_0.0.1_linux_amd64.tar.gz":   {"README.md", "tool", "hello"},
			"foobar_0.0.1_windows_amd64.tar.gz": {"README.md", "tool", "hello.exe"},
			"foobar_0.0.1_js_wasm.tar.gz":       {"README.md", "tool", "hello.wasm"},
		})
	})

	t.Run("build_test.go TestDefaultExpandEnv", func(t *testing.T) {
		t.Setenv("XBAR", "FOOBAR")
		dir := newDir(t)
		_, err := grBuild(t, dir, strings.Replace(goreleaserFixture, "CGO_ENABLED=0", "XFOO=bar_$XBAR", 1))
		assert.NilError(t, err)
		_, env := grFakeBinary(t, binPath(dir, "linux_amd64", "hello"))
		assert.Equal(t, env["XFOO"], "bar_FOOBAR")
	})

	t.Run("build_test.go TestBuild (env templates)", func(t *testing.T) {
		dir := newDir(t)
		config := strings.Replace(goreleaserFixture, "      - CGO_ENABLED=0\n",
			"      - CGO_ENABLED=0\n      - 'TEST_T={{ .Env.CGO_ENABLED }}x'\n", 1)
		_, err := grBuild(t, dir, config)
		assert.NilError(t, err)
		_, env := grFakeBinary(t, binPath(dir, "linux_amd64", "hello"))
		assert.Equal(t, env["TEST_T"], "0x")
	})

	t.Run("build_test.go TestBuildInvalidEnv", func(t *testing.T) {
		// GoReleaser fails while it builds: map has no entry for key "Nope".
		// The port fails at Parse, as .Nope is not a field it supports.
		_, err := grBuild(t, newDir(t), strings.Replace(goreleaserFixture, "CGO_ENABLED=0", "GO111MODULE={{ .Nope }}", 1))
		assert.ErrorIs(t, err, spec.ErrUnsupported)
		assert.ErrorContains(t, err, "field .Nope")
	})

	t.Run("build.go Build (missing .Env key in builds[].env)", func(t *testing.T) {
		dir := newDir(t)
		_, err := grBuild(t, dir, strings.Replace(goreleaserFixture, "CGO_ENABLED=0", "GO111MODULE={{ .Env.NOPE }}", 1))
		assert.ErrorContains(t, err, `map has no entry for key "NOPE"`)
	})

	t.Run("build_test.go TestDefaultEmptyBuild (main defaults to .)", func(t *testing.T) {
		dir := newDir(t)
		_, err := grBuild(t, dir, strings.Replace(goreleaserFixture, "  - main: ./cmd/hello\n    id", "  - id", 1))
		assert.NilError(t, err)
		args, _ := grFakeBinary(t, binPath(dir, "linux_amd64", "hello"))
		assert.Equal(t, args[len(args)-1], ".")
	})

	t.Run("archive_test.go TestDuplicateFilesInsideArchive (extra file named like the binary)", func(t *testing.T) {
		// goreleaser v1.20.0 builds, then fails while it archives. The port
		// fails before it runs go build.
		dir := newDir(t)
		_, err := grBuild(t, dir, strings.Replace(goreleaserFixture, "dst: tool", "dst: hello", 1))
		assert.ErrorIs(t, err, fs.ErrExist)
		_, err = os.Stat(filepath.Join(dir, "dist"))
		assert.ErrorIs(t, err, os.ErrNotExist, "go build ran")
	})

	t.Run("tar.go Add (header times and owners)", func(t *testing.T) {
		// Intentional difference: GoReleaser keeps the file mtime, uid/gid
		// and user names. The port uses the commit time, uid/gid 0 and no
		// user names, so that an archive is the same on each build.
		dir := newDir(t)
		mtime := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
		assert.NilError(t, os.Chtimes(filepath.Join(dir, "README.md"), mtime, mtime))
		archives, err := grBuild(t, dir, goreleaserFixture)
		assert.NilError(t, err)
		hdrs := grTarHeaders(t, archives[0])
		i := slices.IndexFunc(hdrs, func(h *tar.Header) bool { return h.Name == "README.md" })
		assert.Assert(t, i >= 0)
		for _, h := range hdrs {
			assert.Assert(t, h.ModTime.Equal(time.Unix(1_700_000_000, 0)), "%s: %s", h.Name, h.ModTime)
			assert.Equal(t, h.Uid, 0, h.Name)
			assert.Equal(t, h.Gid, 0, h.Name)
			assert.Equal(t, h.Uname, "", h.Name)
			assert.Equal(t, h.Gname, "", h.Name)
		}
	})
}
