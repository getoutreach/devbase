// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Builds a Ruby gem and publishes it to the GitHub Packages
// RubyGems registry, as shell/ruby/build.sh and shell/ruby/publish.sh
// do.

package release

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	gh "github.com/google/go-github/v84/github"
	"go.yaml.in/yaml/v3"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
)

// These are the errors of the gem registry.
var (
	// errGemNoVersion is returned when the version file has no VERSION
	// line to replace.
	errGemNoVersion = errors.New("the gem version file has no VERSION line")

	// errGemNoFile is returned when the built .gem file is missing after
	// a build check, or before a publish, as publish.sh refuses to
	// publish nothing.
	errGemNoFile = errors.New(".gem file not found")

	// errGemNoToken is returned when no GitHub Packages token is found
	// for the gem push and the version lookup.
	errGemNoToken = errors.New("no GitHub Packages token for the gem registry")

	// errGemOwner is returned when the registry owner cannot be found.
	errGemOwner = errors.New("cannot find the GitHub owner of the gem")

	// errGemLookup is returned when the version lookup in GitHub
	// Packages fails.
	errGemLookup = errors.New("cannot look up the gem version in GitHub Packages")

	// errGemNoName is returned when service.yaml has no name for the gem
	// name.
	errGemNoName = errors.New("service.yaml has no name for the gem name")
)

// These are the GitHub Packages RubyGems registry settings of publish.sh.
const (
	// gemHost is the registry host; the owner is its path.
	gemHost = "https://rubygems.pkg.github.com/"

	// gemKey is the key of the token in the gem credentials file, which
	// shell/ci/auth/github_packages.sh writes as ":github: Bearer TOKEN".
	gemKey = "github"

	// gemAPIURL is the GitHub API URL when GITHUB_API_URL is not set, as
	// for @semantic-release/github.
	gemAPIURL = "https://api.github.com/"
)

// gemRegistry builds and publishes the gem of a gem plugin.
type gemRegistry struct {
	plugin *spec.GemPlugin
}

// newGemRegistry returns the registry of a gem plugin.
func newGemRegistry(pl *spec.GemPlugin) registry {
	return &gemRegistry{plugin: pl}
}

// name names the registry in messages.
func (g *gemRegistry) name() string {
	return "GitHub Packages gem"
}

// gemSession is the gem of one registry step.
type gemSession struct {
	// dir is the gem folder.
	dir string

	// name is the gem name.
	name string

	// version is the gem version: the release version as
	// Gem::Version writes it.
	version string

	// versionFile is the gem version file.
	versionFile string

	// env is the environment of the commands.
	env []string
}

// open finds the gem folder and name.
func (g *gemRegistry) open(p *regTarget) (*gemSession, error) {
	s := &gemSession{
		dir: filepath.Join(p.dir, filepath.FromSlash(g.plugin.GemDir())), name: g.plugin.Name,
		version: gemVersion(p.version), env: envOrProcess(p.env),
	}
	if s.name == "" {
		app, err := serviceName(p.dir)
		if err != nil {
			return nil, err
		}
		s.name = app + "_client"
	}
	s.versionFile = filepath.Join(p.dir, filepath.FromSlash(g.plugin.VersionPath(s.name)))
	return s, nil
}

// prepare writes the release version into the gem version file and
// builds the gem, as build.sh does. The version changes only in the
// checkout (requirement 12). It also checks that publish will find the
// token and the owner, so that a missing one fails before the tag push.
func (g *gemRegistry) prepare(ctx context.Context, p *regTarget) error {
	s, err := g.open(p)
	if err != nil {
		return err
	}
	if _, err := gemToken(s.env); err != nil {
		return err
	}
	if _, err := g.owner(ctx, p); err != nil {
		return err
	}
	return g.build(ctx, p, s)
}

// checkBuild builds the gem as prepare does, and checks that the .gem
// file exists. It needs no token or owner and pushes nothing, so that
// branch builds and forks can run it. It restores the gem version file
// afterwards, also on a failure.
func (g *gemRegistry) checkBuild(ctx context.Context, p *regTarget) (err error) {
	s, err := g.open(p)
	if err != nil {
		return err
	}
	fmt.Fprintf(p.log, "Build check: %s %s\n", s.name, s.version)
	orig, err := os.ReadFile(s.versionFile)
	if err != nil {
		return fmt.Errorf("read the gem version file: %w", err)
	}
	defer func() {
		if werr := os.WriteFile(s.versionFile, orig, 0o600); werr != nil {
			err = errors.Join(err, fmt.Errorf("restore the gem version file: %w", werr))
		}
	}()
	if err := g.build(ctx, p, s); err != nil {
		return err
	}
	_, err = s.gemFile()
	return err
}

// build writes the release version into the gem version file and builds
// the gem with "bundle install" and "bundle exec rake build", as
// build.sh does.
func (g *gemRegistry) build(ctx context.Context, p *regTarget, s *gemSession) error {
	fmt.Fprintf(p.log, "Write version %s to %s\n", p.version, s.versionFile)
	if err := writeGemVersion(s.versionFile, p.version); err != nil {
		return err
	}
	for _, args := range [][]string{{"install"}, {"exec", "rake", "build"}} {
		if _, err := p.run(ctx, p.log, &command{dir: s.dir, env: s.env, name: "bundle", args: args}); err != nil {
			return fmt.Errorf("bundle %s: %w", strings.Join(args, " "), err)
		}
	}
	return nil
}

// gemFile returns the built .gem file, which rake build writes to
// pkg/<name>-<version>.gem, or an error when it does not exist.
func (s *gemSession) gemFile() (string, error) {
	file := filepath.Join(s.dir, "pkg", s.name+"-"+s.version+".gem")
	if _, err := os.Stat(file); err != nil {
		return "", fmt.Errorf("%w at %s: %w", errGemNoFile, file, err)
	}
	return file, nil
}

// publish pushes the built gem with "gem push", as publish.sh does. It
// skips the push when the registry has the version (requirement 8). It
// returns no release info, as publish.sh prints none.
func (g *gemRegistry) publish(ctx context.Context, p *regTarget) (*forge.Published, error) {
	s, err := g.open(p)
	if err != nil {
		return nil, err
	}
	gemFile, err := s.gemFile()
	if err != nil {
		return nil, fmt.Errorf("refusing to publish nothing: %w", err)
	}
	owner, err := g.owner(ctx, p)
	if err != nil {
		return nil, err
	}
	found, err := g.hasVersion(ctx, s, owner)
	if err != nil {
		return nil, err
	}
	if found {
		fmt.Fprintf(p.log, "Skip publishing to GitHub Packages as %s %s exists\n", s.name, s.version)
		return nil, nil //nolint:nilnil // Why: publish.sh returns no release info.
	}
	fmt.Fprintf(p.log, "Pushing %s %s to GitHub Packages\n", s.name, s.version)
	_, err = p.run(ctx, p.log, &command{
		dir: s.dir, env: s.env, name: "gem",
		args: []string{"push", "--key", gemKey, "--host", gemHost + owner, gemFile},
	})
	if err != nil {
		return nil, fmt.Errorf("gem push: %w", err)
	}
	return nil, nil //nolint:nilnil // Why: publish.sh returns no release info.
}

// exists reports whether GitHub Packages has the gem version.
func (g *gemRegistry) exists(ctx context.Context, p *regTarget) (bool, error) {
	s, err := g.open(p)
	if err != nil {
		return false, err
	}
	owner, err := g.owner(ctx, p)
	if err != nil {
		return false, err
	}
	return g.hasVersion(ctx, s, owner)
}

// hasVersion reports whether the GitHub Packages of owner have the gem
// version, with the REST API "List package versions for a package owned
// by an organization", and the token of gem push. A missing package is a
// 404.
func (g *gemRegistry) hasVersion(ctx context.Context, s *gemSession, owner string) (bool, error) {
	token, err := gemToken(s.env)
	if err != nil {
		return false, err
	}
	client := gh.NewClient(&http.Client{}).WithAuthToken(token)
	base := envValue(s.env, "GITHUB_API_URL")
	if base == "" {
		base = gemAPIURL
	}
	if client.BaseURL, err = url.Parse(strings.TrimSuffix(base, "/") + "/"); err != nil {
		return false, fmt.Errorf("parse GITHUB_API_URL: %w", err)
	}
	opts := &gh.PackageListOptions{ListOptions: gh.ListOptions{PerPage: 100}}
	for {
		versions, resp, err := client.Organizations.PackageGetAllVersions(ctx, owner, "rubygems", s.name, opts)
		var ghErr *gh.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response != nil && ghErr.Response.StatusCode == http.StatusNotFound {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("%w: gem %s in %s: %w", errGemLookup, s.name, owner, err)
		}
		for _, v := range versions {
			if v.GetName() == s.version {
				return true, nil
			}
		}
		if resp.NextPage == 0 {
			return false, nil
		}
		opts.Page = resp.NextPage
	}
}

// owner returns the GitHub owner of the registry: the plugin's owner, or
// the owner of the origin remote, which must be on github.com.
func (g *gemRegistry) owner(ctx context.Context, p *regTarget) (string, error) {
	if g.plugin.Owner != "" {
		return g.plugin.Owner, nil
	}
	repoURL, err := (&Git{Dir: p.dir}).RepoURL(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errGemOwner, err)
	}
	rest, ok := strings.CutPrefix(repoURL, "https://github.com/")
	owner, _, _ := strings.Cut(rest, "/")
	if !ok || owner == "" {
		return "", fmt.Errorf("%w: the origin remote %s is not a github.com repository", errGemOwner, repoURL)
	}
	return owner, nil
}

// gemVersion returns version as Gem::Version writes it, which is the
// version in the .gem file name: each "-" becomes ".pre.", so that
// 1.2.0-rc.1 becomes 1.2.0.pre.rc.1.
func gemVersion(version string) string {
	return strings.ReplaceAll(version, "-", ".pre.")
}

// writeGemVersion replaces the value of each line of file that contains
// "VERSION ", from its first "=", with the quoted version, as the sed
// command of build.sh does. A file without such a line is an error, so
// that the gem is never built with the placeholder version.
func writeGemVersion(file, version string) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read the gem version file: %w", err)
	}
	lines := strings.SplitAfter(string(b), "\n")
	found := false
	for i, line := range lines {
		body := strings.TrimSuffix(line, "\n")
		name, _, ok := strings.Cut(body, "=")
		if !ok || !strings.Contains(body, "VERSION ") {
			continue
		}
		lines[i] = name + `= "` + version + `"` + line[len(body):]
		found = true
	}
	if !found {
		return fmt.Errorf("%w: %s", errGemNoVersion, file)
	}
	if err := os.WriteFile(file, []byte(strings.Join(lines, "")), 0o600); err != nil {
		return fmt.Errorf("write the gem version file: %w", err)
	}
	return nil
}

// serviceName returns the name in the service.yaml file of dir, as
// get_app_name in shell/lib/bootstrap.sh reads it.
func serviceName(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "service.yaml"))
	if err != nil {
		return "", fmt.Errorf("read service.yaml for the gem name: %w", err)
	}
	var svc struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(b, &svc); err != nil {
		return "", fmt.Errorf("parse service.yaml: %w", err)
	}
	if svc.Name == "" {
		return "", errGemNoName
	}
	return svc.Name, nil
}

// gemToken returns the token that "gem push --key github" uses:
// GEM_HOST_API_KEY, or else the github key of the gem credentials file,
// without its "Bearer " prefix. The credentials file is
// ~/.gem/credentials when it exists, or else
// $XDG_DATA_HOME/gem/credentials, as in RubyGems.
func gemToken(env []string) (string, error) {
	if key := envValue(env, "GEM_HOST_API_KEY"); key != "" {
		return strings.TrimPrefix(key, "Bearer "), nil
	}
	home := envValue(env, "HOME")
	file := filepath.Join(home, ".gem", "credentials")
	if _, err := os.Stat(file); err != nil {
		data := envValue(env, "XDG_DATA_HOME")
		if data == "" {
			data = filepath.Join(home, ".local", "share")
		}
		file = filepath.Join(data, "gem", "credentials")
	}
	b, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: %s does not exist", errGemNoToken, file)
	}
	if err != nil {
		return "", fmt.Errorf("read the gem credentials: %w", err)
	}
	var keys map[string]string
	if err := yaml.Unmarshal(b, &keys); err != nil {
		return "", fmt.Errorf("parse the gem credentials %s: %w", file, err)
	}
	token := keys[":"+gemKey]
	if token == "" {
		token = keys[gemKey]
	}
	if token = strings.TrimSpace(strings.TrimPrefix(token, "Bearer ")); token == "" {
		return "", fmt.Errorf("%w: no %s key in %s", errGemNoToken, gemKey, file)
	}
	return token, nil
}
