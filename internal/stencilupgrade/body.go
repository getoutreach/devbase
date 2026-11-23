// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Renders the body of the stencil-upgrade pull request.

package stencilupgrade

import (
	"bytes"
	"context"
	_ "embed" // Used to embed body.md.tmpl.
	"fmt"
	"os"
	"slices"
	"strings"
	"text/template"

	"github.com/Masterminds/semver/v3"
	"go.yaml.in/yaml/v3"
)

// MaxLinksPerModule is the largest number of version links shown for
// one module. The newest versions are shown, and the older ones are
// counted in an "and N more" line.
const MaxLinksPerModule = 20

const (
	// localVersion is the version stencil records for a module that is
	// used from a local directory.
	localVersion = "local"

	// channelRC is the service.yaml channel that includes pre-releases.
	channelRC = "rc"

	// channelStable is the channel of a module that sets none.
	channelStable = "stable"
)

//go:embed body.md.tmpl
var bodyTemplateText string

// BodyInput is the data for RenderBody.
type BodyInput struct {
	// Changes are the modules that changed, from LockDiff.
	Changes []ModuleChange

	// StencilVersion is the version of stencil that made the changes.
	StencilVersion string

	// Channels maps a module name to its channel in service.yaml. An
	// empty or missing channel means "stable".
	Channels map[string]string

	// Note is Markdown that is shown at the top of the body, before the
	// table of changes. Empty means no note.
	Note string
}

// bodyData is the data given to the body template.
type bodyData struct {
	Rows           []bodyRow
	Notes          []bodyNote
	StencilVersion string
	Note           string
}

// bodyRow is one row of the table of changes.
type bodyRow struct {
	Module  string
	RepoURL string
	Change  string
}

// bodyNote is the release notes of one module. Each link is Markdown.
type bodyNote struct {
	Module string
	Links  []string
}

// RenderBody returns the Markdown body of the stencil-upgrade pull
// request. A failure to list versions does not fail the render: the
// module gets a compare or repository link instead.
func RenderBody(ctx context.Context, in BodyInput, vl VersionLister) (string, error) {
	data := bodyData{StencilVersion: in.StencilVersion, Note: strings.TrimSpace(in.Note)}
	for _, c := range in.Changes {
		data.Rows = append(data.Rows, bodyRow{Module: c.Module, RepoURL: repositoryURL(c.Module), Change: changeText(c)})
		if links := moduleLinks(ctx, c, in.Channels, vl); len(links) > 0 {
			data.Notes = append(data.Notes, bodyNote{Module: c.Module, Links: links})
		}
	}

	tmpl, err := template.New("body").Parse(bodyTemplateText)
	if err != nil {
		return "", fmt.Errorf("parse pull request body template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render pull request body: %w", err)
	}
	return buf.String(), nil
}

// changeText describes the version change of c for the table.
func changeText(c ModuleChange) string {
	if c.Old == "" {
		return fmt.Sprintf("%#q (new)", c.New)
	}
	return fmt.Sprintf("%#q -> %#q", c.Old, c.New)
}

// repositoryURL returns the URL of the GitHub repository of module, so
// that a major version suffix such as "/v2" or a module in a
// subdirectory links to the repository root. A module that is not on
// GitHub links to its full path.
func repositoryURL(module string) string {
	owner, repo, err := splitModule(module)
	if err != nil {
		return "https://" + module
	}
	return "https://github.com/" + owner + "/" + repo
}

// moduleLinks returns the Markdown links for the release notes of c.
func moduleLinks(ctx context.Context, c ModuleChange, channels map[string]string, vl VersionLister) []string {
	repoURL := repositoryURL(c.Module)
	switch {
	case c.Old == localVersion || c.New == localVersion:
		return nil
	case c.Unstable || c.New == UnstableVersion:
		branch, err := vl.DefaultBranch(ctx, c.Module)
		if err != nil || branch == "" {
			return []string{fmt.Sprintf("[Repository](%s)", repoURL)}
		}
		return []string{fmt.Sprintf("[Commit history](%s/commits/%s)", repoURL, branch)}
	}

	newVer := parseVersion(c.New)
	if newVer == nil {
		return nil
	}
	// An old version that is empty or not a version, such as "unstable",
	// has no range to list: only the new version is linked.
	oldVer := parseVersion(c.Old)

	releases, err := vl.Releases(ctx, c.Module)
	if err != nil {
		if oldVer == nil {
			return []string{tagLink(repoURL, c.New)}
		}
		return []string{fmt.Sprintf("[Compare %s...%s](%s/compare/%s...%s)", c.Old, c.New, repoURL, c.Old, c.New)}
	}

	tags := []string{c.New}
	if oldVer != nil {
		tags = versionsBetween(releases, oldVer, newVer, channelOf(c, channels) == channelRC)
	}

	// The newest versions matter most, so the oldest ones are left out.
	links := make([]string, 0, min(len(tags), MaxLinksPerModule)+1)
	if more := len(tags) - MaxLinksPerModule; more > 0 {
		links = append(links, fmt.Sprintf("and %d more", more))
		tags = tags[more:]
	}
	for _, tag := range tags {
		links = append(links, tagLink(repoURL, tag))
	}
	return links
}

// parseVersion returns the semantic version in v, or nil if v is not
// one.
func parseVersion(v string) *semver.Version {
	ver, err := semver.NewVersion(v)
	if err != nil {
		return nil
	}
	return ver
}

// tagLink returns a Markdown link to the release of tag.
func tagLink(repoURL, tag string) string {
	return fmt.Sprintf("[%s](%s/releases/tag/%s)", tag, repoURL, tag)
}

// channelOf returns the channel of the module of c: the one in
// service.yaml, else "rc" if the new version is a release candidate,
// else "stable".
func channelOf(c ModuleChange, channels map[string]string) string {
	if ch := channels[c.Module]; ch != "" {
		return ch
	}
	if strings.Contains(c.New, "-rc") {
		return channelRC
	}
	return channelStable
}

// versionsBetween returns the tags of releases that are newer than
// oldVer and not newer than newVer, in
// ascending version order. Tags that are not versions are skipped, and
// so are pre-releases unless includePre is true.
func versionsBetween(releases []Release, oldVer, newVer *semver.Version, includePre bool) []string {
	type tagged struct {
		tag string
		ver *semver.Version
	}

	var found []tagged
	for _, r := range releases {
		v, err := semver.NewVersion(r.Tag)
		if err != nil {
			continue
		}
		if !includePre && (r.Prerelease || v.Prerelease() != "") {
			continue
		}
		if !v.GreaterThan(oldVer) || v.GreaterThan(newVer) {
			continue
		}
		found = append(found, tagged{r.Tag, v})
	}
	slices.SortStableFunc(found, func(a, b tagged) int { return a.ver.Compare(b.ver) })

	tags := make([]string, len(found))
	for i, f := range found {
		tags[i] = f.tag
	}
	return tags
}

// ChannelsFromServiceYaml returns the channel of each module in the
// service.yaml at path that sets one.
func ChannelsFromServiceYaml(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read service manifest: %w", err)
	}

	var manifest struct {
		Modules []struct {
			Name    string `yaml:"name"`
			Channel string `yaml:"channel"`
		} `yaml:"modules"`
	}
	if err := yaml.Unmarshal(b, &manifest); err != nil {
		return nil, fmt.Errorf("decode service manifest %s: %w", path, err)
	}

	channels := make(map[string]string)
	for _, m := range manifest.Modules {
		if m.Channel != "" {
			channels[m.Name] = m.Channel
		}
	}
	return channels, nil
}
