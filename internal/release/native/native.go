// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Loads the release section of scripts/devbase.yaml, the
// native release configuration, into the release model.

// Package native loads the release section of scripts/devbase.yaml into
// spec.Config, and writes a spec.Config as that section. The section
// replaces .releaserc.yaml and .goreleaser.yml. Unknown keys are errors,
// and the defaults are the values that stencil renders.
//
// This is a full section for a CLI repo with prereleases. Each value
// that is the default is marked; a section can leave those out.
//
//	release:
//	  # The release branches, in order: exactly one release branch, and
//	  # any number of prerelease branches, each with its own identifier.
//	  branches:
//	    - name: main
//	      prerelease: rc
//	    - name: release
//	  # Default false: a major version is an error.
//	  allowMajorVersions: false
//	  # The commit rules that win over the default rules. The default
//	  # is this list; with allowMajorVersions, it has no breaking rule.
//	  # A release is patch, minor, major or none.
//	  rules:
//	    - breaking: true
//	      release: minor
//	    - type: revert
//	      release: patch
//	    - type: perf
//	      release: patch
//	  # The Go binaries and archives. Without this key, nothing is built.
//	  build:
//	    before: [make dep]                # Default.
//	    builds:
//	      - binary: widgets
//	        id: widgets                   # Default: the binary.
//	        main: ./cmd/widgets           # Default: ./cmd/<binary>.
//	        ldflags:                      # Default: these three.
//	          - -w -s -X "github.com/getoutreach/gobox/pkg/app.Version=v{{ .Version }}"
//	          - -X "main.HoneycombTracingKey={{ .Env.HONEYCOMB_APIKEY }}"
//	          - -X "main.TeleforkAPIKey={{ .Env.TELEFORK_APIKEY }}"
//	        env: [CGO_ENABLED=0]          # Default.
//	      - binary: widgetctl
//	    targets: [linux/amd64, linux/arm64, darwin/arm64] # Default.
//	    archive:
//	      files:                          # Default: license*, LICENSE*, readme*, README*, changelog*, CHANGELOG*.
//	        - src: README*
//	        - src: release/yq
//	          dst: yq
//	          mode: 0755
//	    checksums: checksums.txt          # Default.
//	  # What the release makes and publishes. Each step of the release
//	  # (generateNotes, prepare, publish, success) runs the publishers in
//	  # this order. Default: [notes, github].
//	  publishers:
//	    - notes                           # The conventionalcommits notes.
//	    - github:
//	        # Default with build: the archives and the checksum file.
//	        assets: [dist/*.tar.gz, dist/checksums.txt]
//	    - npm:
//	        packageRoot: api/clients/node # Default: the repository.
//	    - gem:                            # The Ruby client gem.
//	        dir: api/clients/ruby         # Default.
//	        name: widgets_client          # Default: <service.yaml name>_client.
//	        versionFile: lib/widgets_client/version.rb # Default: lib/<name>/version.rb.
//	        owner: acme                   # Default: the owner of the origin remote.
//	    - python:                         # The Python client on AWS CodeArtifact.
//	        dir: api/clients/python       # Default.
//	        domain: acme                  # Required.
//	        account: "123456789012"       # Default: aws.defaultAccountID of the box config.
//	        region: eu-west-1             # Required.
//	        repository: acme-pypi         # Required.
//	    - hooks:                          # Shell commands, one for each step.
//	        generateNotes: ./scripts/notes.sh ${nextRelease.version}
//	        prepare: mise run orb:build
//	        publish: ./scripts/publish.sh ${nextRelease.version}
//	        success: echo done
//	    - commit:                         # The release commit. It never has
//	        files: [CHANGELOG.md]         # the client version files.
//	    - orb:
//	        name: getoutreach/shared
//	        path: orb.yml                 # Default: orb.yml.
//	  # The path rules of the root release when there are units.
//	  root:
//	    exclude: [node/**]                # Changes that never count for the root.
//	    shared: [.circleci/config.yml]    # Root changes only without a unit change.
//	  # Extra release units, each with its own tags, in release order.
//	  units:
//	    - kind: npm                       # go-module, npm or generic.
//	      dir: node/widgets-client
//	      name: widgets-client            # Default: the base name of dir.
//	      paths: [node/widgets-client]    # Default: dir.
//	      exclude: [node/widgets-client/README.md]
//	      tagFormat: nodejs-widgets-client-v${version}
//	      strategy: independent           # Default; mirror is the Go default.
//	      skipPrereleases: false          # Default.
//	      allowMajorVersions: false       # Default.
//	      forgeRelease: false             # Default.
//	      npm:
//	        packageRoot: node/widgets-client # Default: dir.
//	        install: yarn install --frozen-lockfile
//	    - kind: go-module
//	      dir: api                        # Tags: api/v${version}.
//	      go:
//	        pinRoot: auto                 # Default; or never.
//
// The build and publishers keys describe the release of the repository
// root. The loader of the repository fills in the unit defaults, with
// the module path from the go.mod of a Go module unit.
package native

import (
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/release/internal/strictyaml"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"go.yaml.in/yaml/v3"
)

// File is the file of the release section, relative to the repository.
const File = "scripts/devbase.yaml"

// Key is the top-level key of the release section.
const Key = "release"

// These are the values that stencil renders, which are the defaults.
const (
	// defaultChecksums is the default checksum file name.
	defaultChecksums = "checksums.txt"

	// defaultMainDir is the folder of the default main package, which is
	// defaultMainDir + the binary name.
	defaultMainDir = "./cmd/"

	// assetDir is the folder of the release archives.
	assetDir = "dist/"
)

// These are the defaults of lists, which stencil renders.
//
//nolint:gochecknoglobals // Why: read-only lists.
var (
	// defaultBefore are the default build before hooks.
	defaultBefore = []string{"make dep"}

	// defaultLDFlags are the default ldflags of a build.
	defaultLDFlags = []string{
		`-w -s -X "github.com/getoutreach/gobox/pkg/app.Version=v{{ .Version }}"`,
		`-X "main.HoneycombTracingKey={{ .Env.HONEYCOMB_APIKEY }}"`,
		`-X "main.TeleforkAPIKey={{ .Env.TELEFORK_APIKEY }}"`,
	}

	// defaultEnv is the default environment of a build.
	defaultEnv = []string{"CGO_ENABLED=0"}

	// defaultTargets are the default build targets.
	defaultTargets = []string{"linux/amd64", "linux/arm64", "darwin/arm64"}

	// defaultArchiveFiles are the default extra files of the archives,
	// which are the GoReleaser defaults.
	defaultArchiveFiles = []string{"license*", "LICENSE*", "readme*", "README*", "changelog*", "CHANGELOG*"}

	// defaultRules are the default release rules.
	defaultRules = []spec.Rule{{Type: "revert", Release: spec.BumpPatch}, {Type: "perf", Release: spec.BumpPatch}}

	// breakingRule is the first default rule when major versions are
	// not allowed: a breaking change makes a minor release.
	breakingRule = spec.Rule{Breaking: true, Release: spec.BumpMinor}
)

// buildCommands are the build and major version check commands that the
// old configuration ran as exec hooks. The build key and
// allowMajorVersions replace them, so a hook that runs them is an error.
//
//nolint:gochecknoglobals // Why: read-only list.
var buildCommands = []string{"make release", "release:build", "major-release-checker.js"}

// submodulesScript is the script that tags the Go submodules in the old
// configuration. Go module units replace it.
const submodulesScript = "release-submodules.sh"

// clientHookRe matches a hook that runs the stencil Ruby or Python
// client scripts, which the gem and python publishers replace.
var clientHookRe = regexp.MustCompile(`(?s)ruby/(build|publish)\.sh|poetry.*\bversion\b|publish:pyclient`)

// file is the top-level shape of scripts/devbase.yaml. Other tools own
// the other keys.
type file struct {
	Release *section       `yaml:"release"`
	Other   map[string]any `yaml:",inline"`
}

// section is the release section. Its build and publishers keys are the
// release of the repository root, and the units key adds other units.
type section struct {
	Branches           []branch     `yaml:"branches"`
	AllowMajorVersions bool         `yaml:"allowMajorVersions,omitempty"`
	Rules              *[]rule      `yaml:"rules,omitempty"`
	Build              *build       `yaml:"build,omitempty"`
	Publishers         *[]publisher `yaml:"publishers,omitempty"`
	Root               *rootPaths   `yaml:"root,omitempty"`
	Units              []unit       `yaml:"units,omitempty"`
}

// branch is a release branch, or a prerelease branch with an identifier.
type branch struct {
	Name       string  `yaml:"name"`
	Prerelease *string `yaml:"prerelease,omitempty"`
}

// rule is a release rule. Type, scope and subject are globs.
type rule struct {
	Type     *string `yaml:"type,omitempty"`
	Scope    *string `yaml:"scope,omitempty"`
	Subject  *string `yaml:"subject,omitempty"`
	Breaking bool    `yaml:"breaking,omitempty"`
	Revert   bool    `yaml:"revert,omitempty"`
	Release  string  `yaml:"release"`
}

// releaseNone is the release of a rule that stops a release.
const releaseNone = "none"

// build is the Go binaries and archives of the release.
type build struct {
	Before    *[]string `yaml:"before,omitempty"`
	Builds    []goBuild `yaml:"builds"`
	Targets   *[]string `yaml:"targets,omitempty"`
	Archive   *archive  `yaml:"archive,omitempty"`
	Checksums string    `yaml:"checksums,omitempty"`
}

// goBuild is one Go program.
type goBuild struct {
	Binary  string    `yaml:"binary"`
	ID      string    `yaml:"id,omitempty"`
	Main    string    `yaml:"main,omitempty"`
	LDFlags *[]string `yaml:"ldflags,omitempty"`
	Env     *[]string `yaml:"env,omitempty"`
}

// archive is the tar.gz archive of each target.
type archive struct {
	Files *[]archiveFile `yaml:"files,omitempty"`
}

// archiveFile is an extra file of the archives.
type archiveFile struct {
	Src  string      `yaml:"src"`
	Dst  string      `yaml:"dst,omitempty"`
	Mode fs.FileMode `yaml:"mode,omitempty"`
}

// Parse parses the release section of the scripts/devbase.yaml contents
// b into the release model, applies the defaults and validates it. It
// also returns warnings: problems that do not stop a release. It returns
// a nil config and no error when b has no release section.
func Parse(b []byte) (*spec.Config, []string, error) {
	if ok, err := hasSection(b); err != nil || !ok {
		return nil, nil, err
	}
	cfg, warnings, err := parse(b)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %s: %w", File, Key, err)
	}
	return cfg, warnings, nil
}

// hasSection reports whether the scripts/devbase.yaml contents b have a
// release key, even an empty one.
func hasSection(b []byte) (bool, error) {
	var top map[string]yaml.Node
	if err := yaml.Unmarshal(b, &top); err != nil {
		return false, fmt.Errorf("parse %s: %w", File, err)
	}
	_, ok := top[Key]
	return ok, nil
}

// parse parses the release section, which b has, without the file and
// key in errors.
func parse(b []byte) (*spec.Config, []string, error) {
	var f file
	if err := strictyaml.Decode(b, &f); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", spec.ErrUnsupported, err)
	}
	if f.Release == nil {
		return nil, nil, fmt.Errorf("%w: the section is empty", spec.ErrUnsupported)
	}
	cfg, err := f.Release.config()
	if err != nil {
		return nil, nil, err
	}
	warnings := cfg.DropClientVersionFiles()
	w, err := cfg.Validate()
	if err != nil {
		return nil, nil, err
	}
	// The loader of the repository checks the units with their defaults,
	// some of which come from the files, such as go.mod.
	f.Release.setUnits(cfg)
	return cfg, append(warnings, w...), nil
}

// config maps the section into the release model, with the defaults.
func (s *section) config() (*spec.Config, error) {
	if len(s.Branches) == 0 {
		return nil, fmt.Errorf("%w: no branches", spec.ErrUnsupported)
	}
	cfg := &spec.Config{MajorCheck: !s.AllowMajorVersions}
	for i := range s.Branches {
		b, err := s.Branches[i].branch()
		if err != nil {
			return nil, fmt.Errorf("branches[%d]: %w", i, err)
		}
		cfg.Branches = append(cfg.Branches, b)
	}
	if err := s.addRules(cfg); err != nil {
		return nil, err
	}
	if s.Build != nil {
		a, err := s.Build.artifacts()
		if err != nil {
			return nil, fmt.Errorf("build: %w", err)
		}
		cfg.Build, cfg.Artifacts = true, a
	}
	publishers := defaultPublisherList()
	if s.Publishers != nil {
		publishers = *s.Publishers
	}
	var err error
	if cfg.Plugins, err = plugins(publishers, cfg.Artifacts); err != nil {
		return nil, err
	}
	return cfg, nil
}

// plugins maps the publishers into the release model. a is the artifacts
// of the release, which set the default assets of the GitHub release.
func plugins(publishers []publisher, a *spec.Artifacts) ([]spec.Plugin, error) {
	var out []spec.Plugin
	for i := range publishers {
		pl, err := publishers[i].plugin(a)
		if err != nil {
			return nil, fmt.Errorf("publishers[%d]: %w", i, err)
		}
		out = append(out, pl)
	}
	return out, nil
}

// branch maps a branch. A prerelease branch is its own channel, as in
// semantic-release.
func (b *branch) branch() (spec.Branch, error) {
	switch {
	case b.Prerelease == nil:
		return spec.Branch{Name: b.Name}, nil
	case *b.Prerelease == "":
		return spec.Branch{}, fmt.Errorf("%w: branch %q with an empty prerelease", spec.ErrUnsupported, b.Name)
	}
	return spec.Branch{Name: b.Name, Prerelease: *b.Prerelease, Channel: b.Name}, nil
}

// addRules adds the rules, or the default rules, to cfg.
func (s *section) addRules(cfg *spec.Config) error {
	if s.Rules == nil {
		cfg.Rules = defaultRuleList(s.AllowMajorVersions)
		return nil
	}
	for i := range *s.Rules {
		r, err := (*s.Rules)[i].rule()
		if err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
		cfg.Rules = append(cfg.Rules, r)
	}
	return nil
}

// defaultRuleList returns the default rules.
func defaultRuleList(allowMajor bool) []spec.Rule {
	if allowMajor {
		return slices.Clone(defaultRules)
	}
	return append([]spec.Rule{breakingRule}, defaultRules...)
}

// rule maps a rule. An empty glob is an error, as micromatch throws on it.
func (r *rule) rule() (spec.Rule, error) {
	out := spec.Rule{Breaking: r.Breaking, Revert: r.Revert}
	for _, f := range []struct {
		name string
		in   *string
		out  *string
	}{{"type", r.Type, &out.Type}, {"scope", r.Scope, &out.Scope}, {"subject", r.Subject, &out.Subject}} {
		if f.in == nil {
			continue
		}
		if *f.in == "" {
			return spec.Rule{}, fmt.Errorf("%w: rule with an empty %s", spec.ErrUnsupported, f.name)
		}
		*f.out = *f.in
	}
	switch r.Release {
	case string(spec.BumpPatch), string(spec.BumpMinor), string(spec.BumpMajor):
		out.Release = spec.Bump(r.Release)
	case releaseNone:
		out.Release = spec.BumpNoRelease
	default:
		return spec.Rule{}, fmt.Errorf("%w: rule release %q (patch, minor, major or none)", spec.ErrUnsupported, r.Release)
	}
	return out, nil
}

// artifacts maps the build key, with the defaults.
func (b *build) artifacts() (*spec.Artifacts, error) {
	a := &spec.Artifacts{
		Hooks:     orDefault(b.Before, defaultBefore),
		Checksums: b.Checksums,
	}
	if a.Checksums == "" {
		a.Checksums = defaultChecksums
	}
	for _, s := range orDefault(b.Targets, defaultTargets) {
		goos, goarch, ok := strings.Cut(s, "/")
		if !ok || goos == "" || goarch == "" {
			return nil, fmt.Errorf("%w: target %q is not os/arch", spec.ErrUnsupported, s)
		}
		a.Targets = append(a.Targets, spec.NewTarget(goos, goarch))
	}
	for i := range b.Builds {
		g := &b.Builds[i]
		if g.Binary == "" {
			return nil, fmt.Errorf("%w: builds[%d] without a binary", spec.ErrUnsupported, i)
		}
		out := spec.Build{
			ID: g.ID, Binary: g.Binary, Main: g.Main,
			LDFlags: orDefault(g.LDFlags, defaultLDFlags),
			Env:     orDefault(g.Env, defaultEnv),
		}
		if out.ID == "" {
			out.ID = g.Binary
		}
		if out.Main == "" {
			out.Main = defaultMainDir + g.Binary
		}
		a.Builds = append(a.Builds, out)
	}
	if b.Archive == nil || b.Archive.Files == nil {
		a.Files = defaultFiles()
	} else {
		for _, f := range *b.Archive.Files {
			a.Files = append(a.Files, spec.ArchiveFile{Src: f.Src, Dst: f.Dst, Mode: f.Mode})
		}
	}
	return a, nil
}

// defaultFiles returns the default extra files of the archives.
func defaultFiles() []spec.ArchiveFile {
	out := make([]spec.ArchiveFile, 0, len(defaultArchiveFiles))
	for _, src := range defaultArchiveFiles {
		out = append(out, spec.ArchiveFile{Src: src})
	}
	return out
}

// defaultAssets returns the default assets of the GitHub release: the
// archives and the checksum file of a, or none.
func defaultAssets(a *spec.Artifacts) []string {
	if a == nil {
		return nil
	}
	return []string{assetDir + "*.tar.gz", assetDir + a.Checksums}
}

// orDefault returns a copy of *list, or of def when list is nil. An empty
// list is nil, which the release model treats as empty.
func orDefault(list *[]string, def []string) []string {
	if list == nil {
		return slices.Clone(def)
	}
	if len(*list) == 0 {
		return nil
	}
	return *list
}

// checkHookCommand rejects a hook that runs the build or the major
// version check of the old configuration, which the build key and
// allowMajorVersions replace, a client publish, which the gem and python
// publishers replace, or the Go submodule tags, which units replace. A
// client hook would publish without the existence check.
func checkHookCommand(cmd string) error {
	for _, c := range buildCommands {
		if strings.Contains(cmd, c) {
			return fmt.Errorf("%w: hook %q runs %s; use the build key or allowMajorVersions", spec.ErrUnsupported, cmd, c)
		}
	}
	if clientHookRe.MatchString(cmd) {
		return fmt.Errorf("%w: hook %q publishes a client; use the gem or python publisher", spec.ErrUnsupported, cmd)
	}
	if strings.Contains(cmd, submodulesScript) {
		return fmt.Errorf("%w: hook %q runs %s; release the Go submodules as units", spec.ErrUnsupported, cmd, submodulesScript)
	}
	return nil
}
