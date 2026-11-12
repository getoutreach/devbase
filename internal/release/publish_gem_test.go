// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the gem registry, with a fake runner and a fake
// GitHub Packages API.

package release

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

// gemVersionRb is the version.rb that stencil renders, with a
// placeholder version.
const gemVersionRb = "# Copyright 2024 Outreach Corporation. All Rights Reserved.\n" +
	"module WidgetsClient\n  VERSION = \"0.0.0-development\"\nend\n"

// gemCredentials is the ~/.gem/credentials that
// shell/ci/auth/github_packages.sh writes.
const gemCredentials = "---\n:github: Bearer ghp_s3cr3t\n"

// fakePackages is a fake of the GitHub Packages REST API endpoint "List
// package versions for a package owned by an organization". It returns
// versions in pages of two, or status when it is not zero.
type fakePackages struct {
	*httptest.Server

	status   int
	versions []string

	// paths and auths are the request paths and Authorization headers.
	paths, auths []string
}

// newFakePackages starts a fakePackages that the test closes.
func newFakePackages(t *testing.T, versions ...string) *fakePackages {
	t.Helper()
	f := &fakePackages{versions: versions}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.paths = append(f.paths, r.URL.Path)
		f.auths = append(f.auths, r.Header.Get("Authorization"))
		if f.status != 0 {
			w.WriteHeader(f.status)
			_, _ = w.Write([]byte(`{"message": "Package not found."}`))
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		page = max(page, 1)
		end := min(2*page, len(f.versions))
		var out []map[string]any
		for i := 2 * (page - 1); i < end; i++ {
			out = append(out, map[string]any{"id": i, "name": f.versions[i]})
		}
		if end < len(f.versions) {
			w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=%d>; rel="next"`, f.URL, r.URL.Path, page+1))
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(f.Close)
	return f
}

// gemFixture is a repository with service.yaml and the stencil gem
// folder, an isolated HOME with the gem credentials, and a fake API.
type gemFixture struct {
	t    *testing.T
	repo string
	home string
	env  []string
	run  *fakeRunner
	api  *fakePackages
	log  bytes.Buffer
}

// newGemFixture makes a gemFixture whose API has the versions.
func newGemFixture(t *testing.T, versions ...string) *gemFixture {
	t.Helper()
	f := &gemFixture{t: t, repo: t.TempDir(), home: t.TempDir(), run: &fakeRunner{replies: map[string]fakeReply{}}}
	f.api = newFakePackages(t, versions...)
	writeFile(t, f.repo, "service.yaml", "name: widgets\narguments: {}\n")
	writeFile(t, f.repo, "api/clients/ruby/lib/widgets_client/version.rb", gemVersionRb)
	writeFile(t, f.home, ".gem/credentials", gemCredentials)
	f.env = []string{"HOME=" + f.home, "PATH=/usr/bin:/bin", "GITHUB_API_URL=" + f.api.URL}
	return f
}

// target returns the registry target of version.
func (f *gemFixture) target(version string) *regTarget {
	return &regTarget{dir: f.repo, version: version, env: f.env, log: &f.log}
}

// registry returns the gem registry of the stencil plugin, with the
// owner acme, and the fake runner.
func (f *gemFixture) registry() *gemRegistry {
	return &gemRegistry{plugin: &spec.GemPlugin{Dir: spec.DefaultGemDir, Owner: "acme"}, run: f.run.run}
}

// gemDir is the gem folder of the fixture.
func (f *gemFixture) gemDir() string {
	return filepath.Join(f.repo, "api", "clients", "ruby")
}

// buildGem writes the .gem file of version, as rake build does.
func (f *gemFixture) buildGem(version string) string {
	f.t.Helper()
	name := "widgets_client-" + version + ".gem"
	writeFile(f.t, f.gemDir(), filepath.Join("pkg", name), "gem")
	return filepath.Join(f.gemDir(), "pkg", name)
}

func TestGemPrepare(t *testing.T) {
	f := newGemFixture(t)
	assert.NilError(t, f.registry().prepare(context.Background(), f.target("1.2.0-rc.1")))

	// The version is in version.rb as semantic-release gives it, as
	// build.sh writes it, and only in the checkout.
	b, err := os.ReadFile(filepath.Join(f.gemDir(), "lib", "widgets_client", "version.rb"))
	assert.NilError(t, err)
	assert.Equal(t, string(b), strings.Replace(gemVersionRb, "0.0.0-development", "1.2.0-rc.1", 1))
	assert.DeepEqual(t, f.run.argvs(), [][]string{
		{"bundle", "install"},
		{"bundle", "exec", "rake", "build"},
	})
	for _, c := range f.run.calls {
		assert.Equal(t, c.dir, f.gemDir())
		assert.DeepEqual(t, c.env, f.env)
	}
	assert.Equal(t, len(f.api.paths), 0, "prepare does not call the API")
}

func TestGemPrepareErrors(t *testing.T) {
	t.Run("no token", func(t *testing.T) {
		f := newGemFixture(t)
		assert.NilError(t, os.Remove(filepath.Join(f.home, ".gem", "credentials")))
		err := f.registry().prepare(context.Background(), f.target("1.0.0"))
		assert.ErrorIs(t, err, errGemNoToken)
		assert.Equal(t, len(f.run.calls), 0)
	})
	t.Run("no VERSION line", func(t *testing.T) {
		f := newGemFixture(t)
		writeFile(t, f.gemDir(), "lib/widgets_client/version.rb", "module WidgetsClient\nend\n")
		err := f.registry().prepare(context.Background(), f.target("1.0.0"))
		assert.ErrorIs(t, err, errGemNoVersion)
		assert.Equal(t, len(f.run.calls), 0)
	})
	t.Run("no service.yaml", func(t *testing.T) {
		f := newGemFixture(t)
		assert.NilError(t, os.Remove(filepath.Join(f.repo, "service.yaml")))
		assert.ErrorContains(t, f.registry().prepare(context.Background(), f.target("1.0.0")), "service.yaml")
	})
	t.Run("failed build", func(t *testing.T) {
		f := newGemFixture(t)
		f.run.replies["bundle exec"] = fakeReply{stderr: "rake aborted!", err: errExit}
		assert.ErrorIs(t, f.registry().prepare(context.Background(), f.target("1.0.0")), errExit)
	})
}

// TestGemPublishScenarios checks the gem push and the version lookup.
func TestGemPublishScenarios(t *testing.T) {
	const host = "https://rubygems.pkg.github.com/acme"
	tests := []struct {
		name      string
		version   string
		built     string
		published []string
		status    int
		wantPush  bool
		wantPages int
		wantErr   error
	}{
		{name: "first publish", version: "1.0.0", built: "1.0.0", published: []string{"0.9.0"}, wantPush: true, wantPages: 1},
		{
			// publish.sh maps -rc to .pre.rc, as Gem::Version does.
			name: "rc version", version: "1.2.0-rc.1", built: "1.2.0.pre.rc.1",
			published: []string{"1.2.0.pre.rc.0"}, wantPush: true, wantPages: 1,
		},
		{name: "new package", version: "1.0.0", built: "1.0.0", status: http.StatusNotFound, wantPush: true, wantPages: 1},
		{
			// Requirement 8: a rerun skips a version that exists, also on
			// a later page.
			name: "existing version skipped", version: "1.2.0-rc.1", built: "1.2.0.pre.rc.1",
			published: []string{"1.0.0", "1.1.0", "1.2.0.pre.rc.1"}, wantPages: 2,
		},
		{name: "missing gem", version: "1.2.0-rc.1", built: "1.2.0-rc.1", wantErr: errGemNoFile},
		{name: "API failure", version: "1.0.0", built: "1.0.0", status: http.StatusForbidden, wantErr: errGemLookup, wantPages: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newGemFixture(t, tc.published...)
			f.api.status = tc.status
			gemFile := f.buildGem(tc.built)
			info, err := f.registry().publish(context.Background(), f.target(tc.version))
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				assert.NilError(t, err)
			}
			assert.Assert(t, info == nil, "publish.sh gives no release info")
			var want [][]string
			if tc.wantPush {
				want = [][]string{{"gem", "push", "--key", "github", "--host", host, gemFile}}
			}
			assert.DeepEqual(t, f.run.argvs(), want)
			for _, c := range f.run.calls {
				assert.Equal(t, c.dir, f.gemDir())
			}
			assert.Equal(t, len(f.api.paths), tc.wantPages)
			for i := range f.api.paths {
				assert.Equal(t, f.api.paths[i], "/orgs/acme/packages/rubygems/widgets_client/versions")
				assert.Equal(t, f.api.auths[i], "Bearer ghp_s3cr3t")
			}
			assert.Assert(t, !strings.Contains(f.log.String(), "ghp_s3cr3t"))
		})
	}
}

func TestGemExists(t *testing.T) {
	f := newGemFixture(t, "1.0.0", "1.1.0.pre.rc.1")
	for version, want := range map[string]bool{"1.0.0": true, "1.1.0-rc.1": true, "1.1.0": false} {
		got, err := f.registry().exists(context.Background(), f.target(version))
		assert.NilError(t, err)
		assert.Equal(t, got, want, version)
	}
}

func TestGemToken(t *testing.T) {
	home, xdg := t.TempDir(), t.TempDir()
	env := []string{"HOME=" + home, "XDG_DATA_HOME=" + xdg}
	_, err := gemToken(env)
	assert.ErrorIs(t, err, errGemNoToken)

	// RubyGems reads $XDG_DATA_HOME/gem/credentials when
	// ~/.gem/credentials does not exist.
	writeFile(t, xdg, "gem/credentials", ":github: Bearer from-xdg\n")
	token, err := gemToken(env)
	assert.NilError(t, err)
	assert.Equal(t, token, "from-xdg")

	writeFile(t, home, ".gem/credentials", ":rubygems_api_key: other\n")
	_, err = gemToken(env)
	assert.ErrorIs(t, err, errGemNoToken)

	writeFile(t, home, ".gem/credentials", gemCredentials)
	token, err = gemToken(env)
	assert.NilError(t, err)
	assert.Equal(t, token, "ghp_s3cr3t")

	// gem push uses GEM_HOST_API_KEY first.
	token, err = gemToken(append(env, "GEM_HOST_API_KEY=from-env"))
	assert.NilError(t, err)
	assert.Equal(t, token, "from-env")
}

func TestGemOwner(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for remote, want := range map[string]string{
		"https://github.com/acme/widgets.git": "acme",
		"git@github.com:acme/widgets.git":     "acme",
		"https://gitlab.com/acme/widgets.git": "",
	} {
		g := &Git{Dir: t.TempDir()}
		_, err := g.Run(context.Background(), "init", "-q")
		assert.NilError(t, err)
		_, err = g.Run(context.Background(), "remote", "add", "origin", remote)
		assert.NilError(t, err)
		reg := &gemRegistry{plugin: &spec.GemPlugin{}}
		got, err := reg.owner(context.Background(), &regTarget{dir: g.Dir})
		if want == "" {
			assert.ErrorIs(t, err, errGemOwner, remote)
			continue
		}
		assert.NilError(t, err)
		assert.Equal(t, got, want, remote)
	}
}

// TestGemRealBuild builds a gem with the production runner, bundle and
// rake, as build.sh does, and checks the .gem file name that publish
// expects. It is skipped without ruby, gem and bundle.
func TestGemRealBuild(t *testing.T) {
	for _, tool := range []string{"ruby", "gem", "bundle"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " is not installed")
		}
	}
	// bundle exec finds rake in the Ruby bin folder, which a shim such as
	// rbenv's does not put on PATH.
	bindir, err := exec.CommandContext(context.Background(), "ruby", "-e", "print RbConfig::CONFIG['bindir']").Output()
	assert.NilError(t, err)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	f := newGemFixture(t)
	f.env = append(os.Environ(), "HOME="+f.home, "PATH="+string(bindir)+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GITHUB_API_URL="+f.api.URL)
	// A gem with no runtime dependencies and no gem source, so that bundle
	// install needs no network.
	writeFile(t, f.gemDir(), "client.gemspec", `require_relative 'lib/widgets_client/version'
Gem::Specification.new do |spec|
  spec.name    = "widgets_client"
  spec.version = WidgetsClient::VERSION
  spec.summary = "gRPC client for widgets service"
  spec.authors = ["test"]
  spec.files = Dir.chdir(File.expand_path(__dir__)) do
    `+"`git ls-files -z lib`"+`.split("\x0")
  end
  spec.add_development_dependency 'rake'
end
`)
	writeFile(t, f.gemDir(), "Gemfile", "gemspec\n")
	writeFile(t, f.gemDir(), "Rakefile", "require \"bundler/gem_tasks\"\n")
	g := &Git{Dir: f.repo}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}} {
		_, err := g.Run(context.Background(), args...)
		assert.NilError(t, err)
	}
	reg := &gemRegistry{plugin: &spec.GemPlugin{Owner: "acme"}, run: execCommand}
	assert.NilError(t, reg.prepare(context.Background(), f.target("1.2.0-rc.1")))
	assert.Assert(t, strings.Contains(f.log.String(), "widgets_client 1.2.0.pre.rc.1 built to pkg/widgets_client-1.2.0.pre.rc.1.gem"),
		f.log.String())

	// publish finds the built gem and pushes it.
	reg.run = f.run.run
	_, err = reg.publish(context.Background(), f.target("1.2.0-rc.1"))
	assert.NilError(t, err)
	assert.DeepEqual(t, f.run.argvs(), [][]string{{
		"gem", "push", "--key", "github", "--host", "https://rubygems.pkg.github.com/acme",
		filepath.Join(f.gemDir(), "pkg", "widgets_client-1.2.0.pre.rc.1.gem"),
	}})
}
