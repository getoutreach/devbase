// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the .goreleaser.yml loader.

package goreleaser

import (
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

// fixture is a stencil-golang style .goreleaser.yml.
const fixture = `before:
  hooks: [make dep]
builds:
  - main: ./cmd/widgets
    id: &name widgets
    binary: *name
    goos: [linux, darwin]
    goarch: [amd64, arm64]
    ignore:
      - goos: darwin
        goarch: amd64
    ldflags:
      - '-w -s -X "main.version=v{{ .Version }}"'
    env:
      - CGO_ENABLED=0
archives: []
checksum:
  name_template: "checksums.txt"
release:
  disable: true
`

func TestParse(t *testing.T) {
	cfg, err := Parse([]byte(fixture))
	assert.NilError(t, err)
	assert.DeepEqual(t, cfg.Targets, []spec.Target{
		spec.NewTarget("linux", "amd64"), spec.NewTarget("linux", "arm64"), spec.NewTarget("darwin", "arm64"),
	})
	assert.Equal(t, len(cfg.Files), len(defaultArchiveFiles))
	for i, f := range defaultArchiveFiles {
		assert.Equal(t, cfg.Files[i].Src, f.Src)
	}
}

func TestParseRejectsUnsupported(t *testing.T) {
	for name, cfg := range map[string]string{
		"unknown key":       fixture + "nfpms: []\n",
		"checksum template": strings.Replace(fixture, `"checksums.txt"`, `"{{ .ProjectName }}_checksums.txt"`, 1),
	} {
		_, err := Parse([]byte(cfg))
		assert.ErrorIs(t, err, spec.ErrUnsupported, name)
	}
}

// templateFields puts a template in each templated field of fixture.
var templateFields = map[string]func(string) string{
	"before hook": func(s string) string {
		return strings.Replace(fixture, "hooks: [make dep]", "hooks: ['make "+s+"']", 1)
	},
	"ldflags": func(s string) string {
		return strings.Replace(fixture, `'-w -s -X "main.version=v{{ .Version }}"'`, "'-X main.x="+s+"'", 1)
	},
	"env": func(s string) string { return strings.Replace(fixture, "- CGO_ENABLED=0", "- 'X="+s+"'", 1) },
	"archive src": func(s string) string {
		return strings.Replace(fixture, "archives: []", "archives: [{files: ['"+s+"']}]", 1)
	},
}

func TestParseRejectsTemplates(t *testing.T) {
	for _, tc := range []struct{ tmpl, wantErr string }{
		{"{{ .Commit }}", "field .Commit"},
		{"{{ .Date }}", "field .Date"},
		{"{{ .ProjectName }}", "field .ProjectName"},
		{"{{ .Os }}", "field .Os"},
		{"{{ .Env.A.B }}", "field .Env.A.B"},
		{"{{ $.Commit }}", "field .Commit"},
		{"{{ $e := .Env }}{{ $e.A }}", "field $e.A"},
		{"{{ (.Env).A }}", `"(.Env).A"`},
		{"{{ range .Env }}{{ .Commit }}{{ end }}", "field .Commit"},
		{"{{ with .Env.A }}{{ else }}{{ .Date }}{{ end }}", "field .Date"},
		{`{{ printf "%s" .Version }}`, `function "printf"`},
		{`{{ if eq .Version "1" }}x{{ end }}`, `function "eq"`},
		{"{{ .Version | len }}", `function "len"`},
		// GoReleaser has this function; text/template does not.
		{"{{ tolower .Version }}", `function "tolower" not defined`},
		{`{{ template "x" }}`, `"{{template \"x\"}}"`},
		{`{{ define "x" }}y{{ end }}`, "define"},
		{"{{ .Version }", `unexpected "}" in operand`},
	} {
		for field, config := range templateFields {
			t.Run(field+" "+tc.tmpl, func(t *testing.T) {
				_, err := Parse([]byte(config(tc.tmpl)))
				assert.ErrorIs(t, err, spec.ErrUnsupported)
				assert.ErrorContains(t, err, tc.wantErr)
			})
		}
	}
}

func TestTemplateAllowed(t *testing.T) {
	tmpl := &spec.Template{Version: "1.2.3", Tag: "v1.2.3"}
	env := []string{"A=a", "B=b"}
	for _, tc := range []struct{ tmpl, want string }{
		{"{{ .Version }}", "1.2.3"},
		{"{{ .Tag }}", "v1.2.3"},
		{"{{ .Env.A }}", "a"},
		{"{{ $.Version }}-{{ $.Env.B }}", "1.2.3-b"},
		{"{{ $v := .Tag }}{{ $v }}", "v1.2.3"},
		{"{{ range $k, $v := .Env }}{{ $k }}={{ $v }};{{ end }}", "A=a;B=b;"},
		{"{{ range .Env }}{{ . }}{{ break }}{{ end }}", "a"},
		{"{{ if .Env.A }}yes{{ else }}no{{ end }}", "yes"},
		{"{{ with .Env.B }}{{ . }}{{ end }}", "b"},
		{`{{/* comment */}}{{ "lit" }}{{ 1 }}{{ true }}`, "lit1true"},
	} {
		t.Run(tc.tmpl, func(t *testing.T) {
			for field, config := range templateFields {
				_, err := Parse([]byte(config(tc.tmpl)))
				assert.NilError(t, err, field)
			}
			got, err := tmpl.Apply(tc.tmpl, env)
			assert.NilError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}
}

func TestArchiveFileGlob(t *testing.T) {
	tmpl := &spec.Template{Version: "1.2.3"}
	for _, src := range []string{"a", "./a/b", "a/b/", "a/*.txt", "testdata/a.txt"} {
		t.Run("accepted "+src, func(t *testing.T) {
			_, err := Parse([]byte(templateFields["archive src"](src)))
			assert.NilError(t, err)
			f := spec.ArchiveFile{Src: "{{ .Env.X }}"}
			got, err := f.Glob(tmpl, []string{"X=" + src})
			assert.NilError(t, err)
			assert.Equal(t, got, src)
		})
	}

	for src, wantErr := range map[string]string{
		"../x":   "outside the repository",
		"/x":     "outside the repository",
		"./../x": "outside the repository",
		"":       ": empty",
		// goreleaser v1.20.0 adds every file, with .git and dist.
		".":         `archive file ".": the repository`,
		"./":        ": empty",
		"a/**/*.go": "** globs",
		// goreleaser v1.20.0 fails: stat a/../README.md: invalid argument.
		"a/../b": `".", ".." or empty element`,
		// goreleaser v1.20.0 fails: stat testdata/./a/a.txt: invalid argument.
		"a/./b": `".", ".." or empty element`,
		// goreleaser v1.20.0 matches no file for testdata//a/a.txt and testdata/a//.
		"a//b": `".", ".." or empty element`,
		"a//":  `".", ".." or empty element`,
		// goreleaser v1.20.0 matches README.md for these two globs.
		"README.{md,txt}": "in a glob",
		"[!X]EADME.md":    "in a glob",
		// goreleaser v1.20.0 matches no file for this glob.
		"[^X]EADME.md": "in a glob",
		`a\*`:          "in a glob",
	} {
		t.Run("rejected "+src, func(t *testing.T) {
			_, err := Parse([]byte(templateFields["archive src"](src)))
			assert.ErrorIs(t, err, spec.ErrUnsupported)
			assert.ErrorContains(t, err, wantErr)
			f := spec.ArchiveFile{Src: "{{ .Env.X }}"}
			got, err := f.Glob(tmpl, []string{"X=" + src})
			assert.ErrorIs(t, err, spec.ErrUnsupported)
			assert.ErrorContains(t, err, wantErr)
			assert.Equal(t, got, "")
		})
	}

	t.Run("template error", func(t *testing.T) {
		f := spec.ArchiveFile{Src: "{{ .Env.X }}"}
		_, err := f.Glob(tmpl, nil)
		assert.ErrorContains(t, err, `map has no entry for key "X"`)
	})
}
