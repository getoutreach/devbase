// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Ports the GoReleaser v1.20.0 tests for config loading,
// build targets and defaults that apply to the .goreleaser.yml loader.

package goreleaser

import (
	"slices"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

// grBase is the smallest valid .goreleaser.yml. The build is last, so
// tests can add keys to the build, or top-level keys, after it.
const grBase = `checksum:
  name_template: checksums.txt
builds:
  - id: foo
    binary: foo
    ldflags: [-s -w]
`

// grTargets parses grBase with goos/goarch/ignore YAML for its build and
// returns the targets as "os_arch".
func grTargets(t *testing.T, matrix string) ([]string, error) {
	t.Helper()
	cfg, err := Parse([]byte(grBase + matrix))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, target := range cfg.Targets {
		out = append(out, target.OS+"_"+target.Arch)
	}
	return out, nil
}

// TestGoReleaserUpstreamConfig ports pkg/config/config_test.go,
// config_archive_files_test.go and config_array_test.go.
func TestGoReleaserUpstreamConfig(t *testing.T) {
	t.Run("config_archive_files_test.go TestArchiveFiles_justString", func(t *testing.T) {
		cfg, err := Parse([]byte(grBase + "archives:\n  - files:\n      - ./script.sh\n"))
		assert.NilError(t, err)
		assert.DeepEqual(t, cfg.Files, []spec.ArchiveFile{{Src: "./script.sh"}})
	})

	t.Run("config_archive_files_test.go TestArchiveFiles_stringFiles", func(t *testing.T) {
		cfg, err := Parse([]byte(grBase +
			"archives:\n  - files:\n      - ./script.sh\n      - src: ./foo\n        dst: ./bar\n"))
		assert.NilError(t, err)
		assert.DeepEqual(t, cfg.Files, []spec.ArchiveFile{{Src: "./script.sh"}, {Src: "./foo", Dst: "./bar"}})
	})

	t.Run("config_archive_files_test.go TestArchiveFiles_complex (mode)", func(t *testing.T) {
		cfg, err := Parse([]byte(grBase + "archives:\n  - files:\n      - src: ./foobar\n        dst: ./barzaz\n" +
			"        info:\n          mode: 0644\n"))
		assert.NilError(t, err)
		want := spec.ArchiveFile{Src: "./foobar", Dst: "./barzaz", Mode: 0o644}
		assert.DeepEqual(t, cfg.Files, []spec.ArchiveFile{want})
	})

	t.Run("config_archive_files_test.go TestArchiveFiles_complex (owner)", func(t *testing.T) {
		// GoReleaser accepts info.owner; the port supports only info.mode.
		_, err := Parse([]byte(grBase +
			"archives:\n  - files:\n      - src: ./foo\n        info:\n          owner: carlos\n"))
		assert.ErrorIs(t, err, spec.ErrUnsupported)
		assert.ErrorContains(t, err, "field owner not found")
	})

	t.Run("config_test.go TestFile (empty file)", func(t *testing.T) {
		// GoReleaser loads an empty file and adds a default build named
		// after the project, which the port does not know.
		_, err := Parse(nil)
		assert.ErrorIs(t, err, spec.ErrUnsupported)
	})

	t.Run("config_test.go TestInvalidFields", func(t *testing.T) {
		_, err := Parse([]byte(grBase + "    invalid_yaml: 1\n"))
		assert.ErrorIs(t, err, spec.ErrUnsupported)
		assert.ErrorContains(t, err, "field invalid_yaml not found")
	})

	t.Run("config_test.go TestInvalidYaml", func(t *testing.T) {
		_, err := Parse([]byte("this_is_not_valid: [\n"))
		assert.ErrorContains(t, err, "did not find expected node content")
	})

	t.Run("config_test.go TestConfigWithAnchors", func(t *testing.T) {
		const name = "This string will appear as the value of two keys."
		config := strings.Replace(grBase, "id: foo\n    binary: foo", "id: &name "+name+"\n    binary: *name", 1)
		cfg, err := Parse([]byte(config))
		assert.NilError(t, err)
		assert.Equal(t, cfg.Builds[0].Binary, name)
	})

	for _, tc := range []struct {
		name, yaml string
		want       []string
		wantErr    string
	}{
		// GoReleaser uses its default ldflags, which need .Commit and .Date.
		{name: "empty list", yaml: "[]", wantErr: "no ldflags"},
		{name: "list", yaml: "[one two, three]", want: []string{"one two", "three"}},
		{name: "string", yaml: "one two", want: []string{"one two"}},
		{name: "map", yaml: "{key: val}", wantErr: "cannot unmarshal"},
	} {
		t.Run("config_array_test.go TestStringArray (ldflags)/"+tc.name, func(t *testing.T) {
			cfg, err := Parse([]byte(strings.Replace(grBase, "[-s -w]", tc.yaml, 1)))
			if tc.wantErr != "" {
				assert.ErrorIs(t, err, spec.ErrUnsupported)
				assert.ErrorContains(t, err, tc.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.DeepEqual(t, cfg.Builds[0].LDFlags, tc.want)
		})
	}
}

// TestGoReleaserUpstreamTargets ports
// internal/builders/buildtarget/targets_test.go and the target cases of
// internal/builders/golang/build_test.go. GoReleaser builds each target
// with the default variant (goarm 6, gomips hardfloat, goamd64 v1), so a
// target is only goos and goarch.
func TestGoReleaserUpstreamTargets(t *testing.T) {
	t.Run("targets_test.go TestAllBuildTargets (exact ignore)", func(t *testing.T) {
		got, err := grTargets(t, `    goos: [linux, darwin, freebsd, windows]
    goarch: ["386", amd64, arm64]
    ignore:
      - goos: linux
        goarch: "386"
`)
		assert.NilError(t, err)
		assert.DeepEqual(t, got, []string{
			"linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64",
			"freebsd_386", "freebsd_amd64", "freebsd_arm64", "windows_386", "windows_amd64", "windows_arm64",
		})
	})

	t.Run("targets_test.go TestAllBuildTargets (ignore with goarch only)", func(t *testing.T) {
		got, err := grTargets(t, "    goos: [linux, darwin]\n    goarch: [amd64, arm64]\n"+
			"    ignore:\n      - goarch: amd64\n")
		assert.NilError(t, err)
		assert.DeepEqual(t, got, []string{"linux_arm64", "darwin_arm64"})
	})

	t.Run("targets_test.go TestAllBuildTargets (ignore with goos only)", func(t *testing.T) {
		// goreleaser v1.20.0 drops all windows targets for this config.
		got, err := grTargets(t, "    goos: [linux, windows]\n    goarch: [amd64, arm64]\n"+
			"    ignore:\n      - goos: windows\n")
		assert.NilError(t, err)
		assert.DeepEqual(t, got, []string{"linux_amd64", "linux_arm64"})
	})

	for _, tc := range []struct{ name, goos, goarch, wantErr string }{
		{"targets_test.go TestAllBuildTargets/invalid goos", "invalid", "amd64", "invalid goos: invalid"},
		{"targets_test.go TestAllBuildTargets/invalid goarch", "linux", "invalid", "invalid goarch: invalid"},
		{"build_test.go TestInvalidTargets/goos", "darwin, darwim", "amd64", "invalid goos: darwim"},
		{"build_test.go TestInvalidTargets/goarch", "linux", `amd64, i386, "386"`, "invalid goarch: i386"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := grTargets(t, "    goos: ["+tc.goos+"]\n    goarch: ["+tc.goarch+"]\n")
			assert.ErrorIs(t, err, spec.ErrUnsupported)
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}

	valid := []string{
		"aix/ppc64", "android/386", "android/amd64", "android/arm", "android/arm64", "darwin/amd64", "darwin/arm64",
		"dragonfly/amd64", "freebsd/386", "freebsd/amd64", "freebsd/arm", "illumos/amd64", "ios/arm64", "linux/386",
		"linux/amd64", "linux/arm", "linux/arm64", "linux/mips", "linux/mipsle", "linux/mips64", "linux/mips64le",
		"linux/ppc64", "linux/ppc64le", "linux/s390x", "linux/riscv64", "linux/loong64", "netbsd/386", "netbsd/amd64",
		"netbsd/arm", "openbsd/386", "openbsd/amd64", "openbsd/arm", "plan9/386", "plan9/amd64", "plan9/arm",
		"solaris/amd64", "windows/386", "windows/amd64", "windows/arm", "windows/arm64", "js/wasm",
	}
	invalid := []string{"darwin/386", "darwin/arm", "windows/riscv64"}
	for _, target := range slices.Concat(valid, invalid) {
		t.Run("targets_test.go TestGoosGoarchCombos/"+target, func(t *testing.T) {
			goos, goarch, _ := strings.Cut(target, "/")
			got, err := grTargets(t, "    goos: ["+goos+"]\n    goarch: [\""+goarch+"\"]\n")
			assert.NilError(t, err)
			if slices.Contains(valid, target) {
				assert.DeepEqual(t, got, []string{goos + "_" + goarch})
			} else {
				assert.Equal(t, len(got), 0)
			}
		})
	}

	t.Run("build_test.go TestWithDefaults/empty and build_test.go TestDefaultEmptyBuild", func(t *testing.T) {
		got, err := grTargets(t, "")
		assert.NilError(t, err)
		assert.DeepEqual(t, got, []string{
			"linux_amd64", "linux_arm64", "linux_386", "darwin_amd64", "darwin_arm64",
			"windows_amd64", "windows_arm64", "windows_386",
		})
	})

	t.Run("build_test.go TestDefaultPartialBuilds/build0", func(t *testing.T) {
		got, err := grTargets(t, "    goos: [linux]\n")
		assert.NilError(t, err)
		assert.DeepEqual(t, got, []string{"linux_amd64", "linux_arm64", "linux_386"})
	})

	t.Run("build_test.go TestDefaultPartialBuilds/build1", func(t *testing.T) {
		got, err := grTargets(t, "    goarch: [\"386\"]\n")
		assert.NilError(t, err)
		assert.DeepEqual(t, got, []string{"linux_386", "windows_386"})
	})
}

// TestGoReleaserUpstreamDefaults ports the Default() tests of
// internal/pipe/build, internal/pipe/archive and internal/pipe/checksums.
// Defaults that need data the port does not have (the project name, the
// commit and the build date) fail closed; the comment in each case gives
// the GoReleaser behavior.
func TestGoReleaserUpstreamDefaults(t *testing.T) {
	for _, tc := range []struct{ name, config, wantErr string }{
		{
			// GoReleaser adds one build with id and binary set to the project name.
			"build/build_test.go TestDefaultNoBuilds",
			"checksum:\n  name_template: checksums.txt\nbefore:\n  hooks: [make dep]\n", "no builds",
		},
		{
			// GoReleaser fails: found 2 builds with the ID 'a'.
			"build/build_test.go TestSeveralBuildsWithTheSameID",
			grBase + "  - id: foo\n    binary: bar\n    ldflags: [-s -w]\n", `two builds with id "foo"`,
		},
		{
			// goreleaser v1.20.0 builds both, then fails while it archives:
			// add foo: file already exists.
			"archive/archive_test.go TestDuplicateFilesInsideArchive (same binary name from two builds)",
			grBase + "  - id: bar\n    binary: foo\n    ldflags: [-s -w]\n", `two builds with binary "foo"`,
		},
		{
			// GoReleaser uses the project name as the id.
			"build/build_test.go TestDefaultBuildID",
			strings.Replace(grBase, "  - id: foo\n    binary", "  - binary", 1), "no id",
		},
		{
			// GoReleaser uses the project name as the binary.
			"build/build_test.go TestDefaultEmptyBuild (binary)",
			strings.Replace(grBase, "    binary: foo\n", "", 1), "no binary",
		},
		{
			// GoReleaser renders the binary name as a template.
			"build/build_test.go TestBuild (templated binary)",
			strings.Replace(grBase, "binary: foo", "binary: testing.v{{.Version}}", 1), "binary is a template",
		},
		{
			// GoReleaser uses -s -w -X main.version={{.Version}} -X main.commit={{.Commit}}
			// -X main.date={{.Date}} -X main.builtBy=goreleaser.
			"build/build_test.go TestDefaultEmptyBuild (ldflags)",
			strings.Replace(grBase, "    ldflags: [-s -w]\n", "", 1), "no ldflags",
		},
		{
			// GoReleaser fails: archive has different count of binaries for each platform.
			"archive/archive_test.go TestRunPipeDifferentBinaryCount",
			grBase + "    goos: [linux, darwin]\n    goarch: [amd64]\n" +
				"  - id: bar\n    binary: bar\n    ldflags: [-s -w]\n    goos: [linux]\n    goarch: [amd64]\n",
			"other targets",
		},
		{
			// GoReleaser uses {{ .ProjectName }}_{{ .Version }}_checksums.txt.
			"checksums/checksums_test.go TestDefault",
			strings.Replace(grBase, "checksum:\n  name_template: checksums.txt\n", "", 1), "checksum name_template",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.config))
			assert.ErrorIs(t, err, spec.ErrUnsupported)
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}

	t.Run("build/build_test.go TestDefaultEmptyBuild (main)", func(t *testing.T) {
		cfg, err := Parse([]byte(grBase))
		assert.NilError(t, err)
		assert.Equal(t, cfg.Builds[0].Main, ".")
	})

	t.Run("archive/archive_test.go TestDefault (format tar.gz)", func(t *testing.T) {
		_, err := Parse([]byte(grBase + "archives:\n  - format: tar.gz\n"))
		assert.NilError(t, err)
	})
}

// TestGoReleaserUpstreamUnsupported checks that upstream features out
// of the stencil-golang subset fail closed instead of being ignored.
// Each config is grBase with one change, and wantErr names that change.
func TestGoReleaserUpstreamUnsupported(t *testing.T) {
	_, err := Parse([]byte(grBase))
	assert.NilError(t, err, "grBase must be valid")

	withChecksum := func(s string) string {
		return strings.Replace(grBase, "name_template: checksums.txt", s, 1)
	}
	for _, tc := range []struct{ name, config, wantErr string }{
		{
			"archive_test.go TestFormatFor (format zip)",
			grBase + "archives:\n  - format: zip\n", `archive format "zip"`,
		},
		{
			"archive_test.go TestBinaryOverride (format_overrides)",
			grBase + "archives:\n  - format_overrides: [{goos: windows, format: zip}]\n", "field format_overrides not found",
		},
		{
			"archive_test.go TestDefaultSet (name_template)",
			grBase + "archives:\n  - name_template: foo\n", "field name_template not found",
		},
		{
			"archive_test.go TestRunPipeWrap (wrap_in_directory)",
			grBase + "archives:\n  - wrap_in_directory: true\n", "field wrap_in_directory not found",
		},
		{
			"archive_test.go TestSeveralArchivesWithTheSameID",
			grBase + "archives:\n  - files: [{src: a}]\n  - files: [{src: b}]\n", "more than one archive",
		},
		{
			// GoReleaser matches "**" across folders; filepath.Glob does not.
			"archive_test.go TestArchive_globbing/glob src",
			grBase + "archives:\n  - files: [testdata/**/*.txt]\n", "** globs",
		},
		{
			// GoReleaser fails while it globs.
			"archivefiles_test.go TestEval/invalid glob",
			grBase + "archives:\n  - files: [../testdata/nope.txt]\n", "outside the repository",
		},
		{
			"archivefiles_test.go TestEval (absolute src)",
			grBase + "archives:\n  - files: [/etc/passwd]\n", "outside the repository",
		},
		{
			"archivefiles_test.go TestEval/strip parent",
			grBase + "archives:\n  - files: [{src: a, strip_parent: true}]\n", "field strip_parent not found",
		},
		{
			"archivefiles_test.go TestEval/templated info (mtime)",
			grBase + "archives:\n  - files: [{src: a, info: {mtime: x}}]\n", "field mtime not found",
		},
		{"build_test.go TestInvalidTargets/goarm", grBase + "    goarm: [\"6\"]\n", "field goarm not found"},
		{"build_test.go TestInvalidTargets/goamd64", grBase + "    goamd64: [v1]\n", "field goamd64 not found"},
		{"build_test.go TestWithDefaults/custom targets", grBase + "    targets: [linux_386]\n", "field targets not found"},
		{"build_test.go TestRunFullPipe (build hooks)", grBase + "    hooks: {pre: touch pre}\n", "field hooks not found"},
		{"build_test.go TestBuildGoBuildLine/full (flags)", grBase + "    flags: [-v]\n", "field flags not found"},
		{
			"checksums_test.go TestPipeInvalidNameTemplate",
			withChecksum("name_template: '{{ .Pro }_checksums.txt'"), "checksum name_template",
		},
		{
			"checksums_test.go TestPipe (algorithm)",
			withChecksum("name_template: checksums.txt\n  algorithm: sha512"), "field algorithm not found",
		},
		{"release disable template", grBase + "release:\n  disable: '{{ .Env.SKIP }}'\n", "cannot unmarshal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.config))
			assert.ErrorIs(t, err, spec.ErrUnsupported)
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}
