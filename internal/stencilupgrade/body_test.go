// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for the stencil-upgrade pull request body.

package stencilupgrade

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

const testModule = "github.com/org/mod"

var (
	errBoom        = errors.New("boom")
	errRateLimited = errors.New("rate limited")
)

// fakeLister is a VersionLister that returns fixed data.
type fakeLister struct {
	releases      []Release
	releasesErr   error
	defaultBranch string
	branchErr     error
}

func (f fakeLister) Releases(_ context.Context, _ string) ([]Release, error) {
	return f.releases, f.releasesErr
}

func (f fakeLister) DefaultBranch(_ context.Context, _ string) (string, error) {
	return f.defaultBranch, f.branchErr
}

// releasesOf builds stable releases from tags.
func releasesOf(tags ...string) []Release {
	rs := make([]Release, 0, len(tags))
	for _, t := range tags {
		rs = append(rs, Release{Tag: t})
	}
	return rs
}

func releaseLink(tag string) string {
	return "https://github.com/org/mod/releases/tag/" + tag
}

func render(t *testing.T, in BodyInput, vl VersionLister) string {
	t.Helper()
	body, err := RenderBody(context.Background(), in, vl)
	assert.NilError(t, err)
	return body
}

func TestRenderBodyVersionLinks(t *testing.T) {
	vl := fakeLister{releases: []Release{
		{Tag: "v1.0.0"}, {Tag: "v1.1.0"}, {Tag: "v1.2.0-rc.1", Prerelease: true}, {Tag: "v1.2.0"}, {Tag: "v1.3.0"},
	}}
	body := render(t, BodyInput{Changes: []ModuleChange{{Module: testModule, Old: "v1.0.0", New: "v1.3.0"}}}, vl)

	for _, tag := range []string{"v1.1.0", "v1.2.0", "v1.3.0"} {
		assert.Assert(t, strings.Contains(body, releaseLink(tag)), body)
	}
	assert.Assert(t, !strings.Contains(body, releaseLink("v1.0.0")), body)
	assert.Assert(t, !strings.Contains(body, "v1.2.0-rc.1"), body)
}

func TestRenderBodyVersionOrder(t *testing.T) {
	vl := fakeLister{releases: releasesOf("v1.9.0", "v1.10.0", "v1.11.0", "latest", "nightly")}
	body := render(t, BodyInput{Changes: []ModuleChange{{Module: testModule, Old: "v1.8.0", New: "v1.11.0"}}}, vl)

	i9 := strings.Index(body, releaseLink("v1.9.0"))
	i10 := strings.Index(body, releaseLink("v1.10.0"))
	i11 := strings.Index(body, releaseLink("v1.11.0"))
	assert.Assert(t, i9 >= 0 && i9 < i10 && i10 < i11, body)
	assert.Assert(t, !strings.Contains(body, "latest"), body)
	assert.Assert(t, !strings.Contains(body, "nightly"), body)
}

func TestRenderBodyRCChannel(t *testing.T) {
	vl := fakeLister{releases: []Release{{Tag: "v1.0.0"}, {Tag: "v1.1.0-rc.1", Prerelease: true}, {Tag: "v1.1.0"}}}
	in := BodyInput{
		Changes:  []ModuleChange{{Module: testModule, Old: "v1.0.0", New: "v1.1.0"}},
		Channels: map[string]string{testModule: "rc"},
	}
	assert.Assert(t, strings.Contains(render(t, in, vl), releaseLink("v1.1.0-rc.1")))
}

func TestRenderBodyRCChannelFromVersion(t *testing.T) {
	vl := fakeLister{releases: []Release{{Tag: "v1.0.0"}, {Tag: "v1.1.0-rc.1", Prerelease: true}, {Tag: "v1.1.0-rc.2", Prerelease: true}}}
	in := BodyInput{Changes: []ModuleChange{{Module: testModule, Old: "v1.0.0", New: "v1.1.0-rc.2"}}}
	body := render(t, in, vl)
	assert.Assert(t, strings.Contains(body, releaseLink("v1.1.0-rc.1")), body)
	assert.Assert(t, strings.Contains(body, releaseLink("v1.1.0-rc.2")), body)
}

func TestRenderBodyNewModule(t *testing.T) {
	vl := fakeLister{releases: releasesOf("v0.1.0", "v0.2.0", "v0.3.0")}
	body := render(t, BodyInput{Changes: []ModuleChange{{Module: testModule, New: "v0.3.0"}}}, vl)

	assert.Assert(t, strings.Contains(body, releaseLink("v0.3.0")), body)
	assert.Assert(t, !strings.Contains(body, releaseLink("v0.1.0")), body)
	assert.Assert(t, !strings.Contains(body, releaseLink("v0.2.0")), body)
}

func TestRenderBodyUnstable(t *testing.T) {
	vl := fakeLister{defaultBranch: "main"}
	in := BodyInput{Changes: []ModuleChange{{Module: testModule, Old: UnstableVersion, New: UnstableVersion, Unstable: true}}}
	body := render(t, in, vl)
	assert.Assert(t, strings.Contains(body, "https://github.com/org/mod/commits/main"), body)
}

func TestRenderBodyUnstableBranchError(t *testing.T) {
	vl := fakeLister{branchErr: errBoom}
	in := BodyInput{Changes: []ModuleChange{{Module: testModule, Old: UnstableVersion, New: UnstableVersion, Unstable: true}}}
	body := render(t, in, vl)
	assert.Assert(t, strings.Contains(body, "(https://github.com/org/mod)"), body)
	assert.Assert(t, !strings.Contains(body, "/commits/"), body)
}

func TestRenderBodyLocal(t *testing.T) {
	vl := fakeLister{releases: releasesOf("v1.0.0", "v1.1.0", "v1.2.0")}
	body := render(t, BodyInput{Changes: []ModuleChange{{Module: testModule, Old: "local", New: "v1.2.0"}}}, vl)
	assert.Assert(t, !strings.Contains(body, "/releases/tag/"), body)
}

func TestRenderBodyListerError(t *testing.T) {
	vl := fakeLister{releasesErr: errRateLimited}
	body := render(t, BodyInput{Changes: []ModuleChange{{Module: testModule, Old: "v1.0.0", New: "v1.3.0"}}}, vl)
	assert.Assert(t, strings.Contains(body, "https://github.com/org/mod/compare/v1.0.0...v1.3.0"), body)
}

func TestRenderBodyCap(t *testing.T) {
	tags := make([]string, 0, 25)
	for i := 1; i <= 25; i++ {
		tags = append(tags, fmt.Sprintf("v1.%d.0", i))
	}
	vl := fakeLister{releases: releasesOf(tags...)}
	body := render(t, BodyInput{Changes: []ModuleChange{{Module: testModule, Old: "v1.0.0", New: "v1.25.0"}}}, vl)

	assert.Equal(t, strings.Count(body, "/releases/tag/"), MaxLinksPerModule)
	assert.Assert(t, strings.Contains(body, "and 5 more"), body)
}

func TestRenderBodyEmptyChanges(t *testing.T) {
	body := render(t, BodyInput{StencilVersion: "v1.47.0"}, fakeLister{})
	assert.Assert(t, strings.Contains(body, "No module version changed"), body)
	assert.Assert(t, !strings.Contains(body, "| Module |"), body)
	assert.Assert(t, strings.Contains(body, "<code>v1.47.0</code>"), body)
}

func TestRenderBodyStencilVersion(t *testing.T) {
	body := render(t, BodyInput{
		Changes:        []ModuleChange{{Module: testModule, Old: "v1.0.0", New: "v1.1.0"}},
		StencilVersion: "v1.47.0",
	}, fakeLister{releases: releasesOf("v1.1.0")})
	assert.Assert(t, strings.Contains(body, "<code>v1.47.0</code>"), body)
}

func TestRenderBodyNoInternalLinks(t *testing.T) {
	body := strings.ToLower(render(t, BodyInput{
		Changes:        []ModuleChange{{Module: testModule, Old: "v1.0.0", New: "v1.1.0"}},
		StencilVersion: "v1.47.0",
	}, fakeLister{releases: releasesOf("v1.1.0")}))
	assert.Assert(t, !strings.Contains(body, "dtss-tools"), body)
	assert.Assert(t, !strings.Contains(body, "slack"), body)
}

func TestRenderBodyLayout(t *testing.T) {
	body := render(t, BodyInput{
		Changes:        []ModuleChange{{Module: testModule, Old: "v1.0.0", New: "v1.1.0"}},
		StencilVersion: "v1.47.0",
	}, fakeLister{releases: releasesOf("v1.1.0")})
	want := "This PR contains the following updates:\n\n" +
		"| Module | Change |\n| --- | --- |\n" +
		"| [github.com/org/mod](https://github.com/org/mod) | `v1.0.0` -> `v1.1.0` |\n\n" +
		"---\n\n### Release Notes\n\n" +
		"<details><summary>github.com/org/mod</summary>\n\n" +
		"- [v1.1.0](https://github.com/org/mod/releases/tag/v1.1.0)\n\n</details>\n\n" +
		"---\n\n<details><summary>Metadata</summary><b>Stencil Version</b>: <code>v1.47.0</code></details>\n"
	assert.Equal(t, body, want)
}

func TestChannelsFromServiceYaml(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.yaml")
	yml := "name: svc\nmodules:\n  - name: github.com/org/a\n    channel: rc\n  - name: github.com/org/b\n"
	assert.NilError(t, os.WriteFile(path, []byte(yml), 0o600))

	got, err := ChannelsFromServiceYaml(path)
	assert.NilError(t, err)
	assert.DeepEqual(t, got, map[string]string{"github.com/org/a": "rc"})

	_, err = ChannelsFromServiceYaml(filepath.Join(t.TempDir(), "missing.yaml"))
	assert.Assert(t, err != nil)
}
