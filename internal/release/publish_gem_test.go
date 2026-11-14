// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the gem registry, with stubs of the bundle and
// gem commands and a fake GitHub Packages API.

package release

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/rlog"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"github.com/getoutreach/gobox/pkg/exec"
	"github.com/getoutreach/gobox/pkg/exec/execstub"
	"gotest.tools/v3/assert"
)

// gemVersionRb is the version.rb that stencil renders, with a
// placeholder version.
const gemVersionRb = "# Copyright 2024 Outreach Corporation. All Rights Reserved.\n" +
	"module WidgetsClient\n  VERSION = \"0.0.0-development\"\nend\n"

// gemCredentials is the ~/.gem/credentials that
// shell/ci/auth/github_packages.sh writes.
const gemCredentials = "---\n:github: Bearer ghp_s3cr3t\n"

// fakePackages is a fake of the GitHub Packages REST API endpoints "List
// package versions for a package owned by an organization", which
// returns versions in pages of two, or status when it is not zero, and
// "List packages for an organization", which returns the packages in
// pages of two, or orgStatus when it is not zero.
type fakePackages struct {
	*httptest.Server

	status, orgStatus int
	versions          []string

	// packages are the names of the packages of the organization.
	packages []string

	// paths and auths are the request paths and Authorization headers.
	paths, auths []string

	// orgQuery is the query of the last organization request.
	orgQuery string
}

// newFakePackages starts a fakePackages that the test closes.
func newFakePackages(t *testing.T, versions ...string) *fakePackages {
	t.Helper()
	f := &fakePackages{versions: versions}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.paths = append(f.paths, r.URL.Path)
		f.auths = append(f.auths, r.Header.Get("Authorization"))
		if !strings.HasSuffix(r.URL.Path, "/versions") {
			f.orgQuery = r.URL.RawQuery
			if f.orgStatus != 0 {
				w.WriteHeader(f.orgStatus)
				_, _ = w.Write([]byte(`{"message": "Not Found"}`))
				return
			}
			f.writePage(w, r, f.packages)
			return
		}
		if f.status != 0 {
			w.WriteHeader(f.status)
			_, _ = w.Write([]byte(`{"message": "Package not found."}`))
			return
		}
		f.writePage(w, r, f.versions)
	}))
	t.Cleanup(f.Close)
	return f
}

// writePage writes the page of r of the names, as objects with an id
// and a name, in pages of two with a Link header for the next page.
func (f *fakePackages) writePage(w http.ResponseWriter, r *http.Request, names []string) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	page = max(page, 1)
	end := min(2*page, len(names))
	out := []map[string]any{}
	for i := 2 * (page - 1); i < end; i++ {
		out = append(out, map[string]any{"id": i, "name": names[i]})
	}
	if end < len(names) {
		q := r.URL.Query()
		q.Set("page", strconv.Itoa(page+1))
		w.Header().Set("Link", fmt.Sprintf(`<%s%s?%s>; rel="next"`, f.URL, r.URL.Path, q.Encode()))
	}
	_ = json.NewEncoder(w).Encode(out)
}

// gemFixture is a repository with service.yaml and the stencil gem
// folder, an isolated HOME with the gem credentials, stubs of the
// commands and a fake API.
type gemFixture struct {
	t    *testing.T
	repo string
	home string
	env  []string
	cmds *cmdStubs
	api  *fakePackages
	log  bytes.Buffer
}

// newGemFixture makes a gemFixture whose API has the versions.
func newGemFixture(t *testing.T, versions ...string) *gemFixture {
	t.Helper()
	f := &gemFixture{t: t, repo: t.TempDir(), home: t.TempDir(), cmds: newCmdStubs(t)}
	f.api = newFakePackages(t, versions...)
	writeFile(t, f.repo, "service.yaml", "name: widgets\narguments: {}\n")
	writeFile(t, f.repo, "api/clients/ruby/lib/widgets_client/version.rb", gemVersionRb)
	writeFile(t, f.home, ".gem/credentials", gemCredentials)
	f.env = []string{"HOME=" + f.home, "PATH=/usr/bin:/bin", "GITHUB_API_URL=" + f.api.URL}
	return f
}

// target returns the registry target of version, with the stubs.
func (f *gemFixture) target(version string) *regTarget {
	return &regTarget{dir: f.repo, version: version, env: f.env, exec: f.cmds.spy, log: &f.log}
}

// registry returns the gem registry of the stencil plugin, with the box
// config org acme.
func (f *gemFixture) registry() *gemRegistry {
	return &gemRegistry{plugin: &spec.GemPlugin{Dir: spec.DefaultGemDir}, boxOrg: fakeBoxOrg("acme", nil)}
}

// fakeBoxOrg returns a boxOrg that returns org and err.
func fakeBoxOrg(org string, err error) func() (string, error) {
	return func() (string, error) { return org, err }
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
	expectGemBuild(f.cmds, rakeBuildWrites("widgets_client", "lib/widgets_client/version.rb"))
	assert.NilError(t, f.registry().prepare(context.Background(), f.target("1.2.0-rc.1")))

	// The version is in version.rb as semantic-release gives it, as
	// build.sh writes it, and only in the checkout.
	b, err := os.ReadFile(filepath.Join(f.gemDir(), "lib", "widgets_client", "version.rb"))
	assert.NilError(t, err)
	assert.Equal(t, string(b), strings.Replace(gemVersionRb, "0.0.0-development", "1.2.0-rc.1", 1))
	for _, c := range f.cmds.calls() {
		assert.Equal(t, c.Cmd.Dir, f.gemDir())
		assert.DeepEqual(t, c.Cmd.Env, f.env)
	}
	_, err = os.Stat(filepath.Join(f.gemDir(), "pkg", "widgets_client-1.2.0.pre.rc.1.gem"))
	assert.NilError(t, err)
	assert.Equal(t, len(f.api.paths), 0, "prepare does not call the API")
}

func TestGemPrepareErrors(t *testing.T) {
	t.Run("no token", func(t *testing.T) {
		f := newGemFixture(t)
		assert.NilError(t, os.Remove(filepath.Join(f.home, ".gem", "credentials")))
		err := f.registry().prepare(context.Background(), f.target("1.0.0"))
		assert.ErrorIs(t, err, errGemNoToken)
	})
	t.Run("no VERSION line", func(t *testing.T) {
		f := newGemFixture(t)
		writeFile(t, f.gemDir(), "lib/widgets_client/version.rb", "module WidgetsClient\nend\n")
		err := f.registry().prepare(context.Background(), f.target("1.0.0"))
		assert.ErrorIs(t, err, errGemNoVersion)
	})
	t.Run("no service.yaml", func(t *testing.T) {
		f := newGemFixture(t)
		assert.NilError(t, os.Remove(filepath.Join(f.repo, "service.yaml")))
		assert.ErrorContains(t, f.registry().prepare(context.Background(), f.target("1.0.0")), "service.yaml")
	})
	t.Run("failed build", func(t *testing.T) {
		f := newGemFixture(t)
		expectGemBuild(f.cmds, cmdFails("rake aborted!"))
		err := f.registry().prepare(context.Background(), f.target("1.0.0"))
		assertErrIs(t, err, errExit)
		assert.ErrorContains(t, err, "bundle exec rake build: exit status 1: rake aborted!")
	})
}

// TestGemCheckBuild checks the build check of the dry runs: it builds
// as prepare does, without the token or the owner, and restores the
// version file.
func TestGemCheckBuild(t *testing.T) {
	f := newGemFixture(t)
	assert.NilError(t, os.Remove(filepath.Join(f.home, ".gem", "credentials")))
	reg := f.registry()
	reg.boxOrg = fakeBoxOrg("", nil) // The fixture has no origin remote, so no owner.
	file := filepath.Join(f.gemDir(), "lib", "widgets_client", "version.rb")
	// rake build sees the release version, and writes the .gem file.
	var built []byte
	build := rakeBuildWrites("widgets_client", "lib/widgets_client/version.rb")
	expectGemBuild(f.cmds, execstub.Result{Func: func(ctx context.Context, cmd *exec.Cmd, stdin io.Reader, stdout, stderr io.Writer) error {
		var err error
		if built, err = os.ReadFile(file); err != nil {
			return err
		}
		return build.Func(ctx, cmd, stdin, stdout, stderr)
	}})
	assert.NilError(t, reg.checkBuild(context.Background(), f.target("1.2.0-rc.1")))

	for _, c := range f.cmds.calls() {
		assert.Equal(t, c.Cmd.Dir, f.gemDir())
	}
	assert.Equal(t, string(built), strings.Replace(gemVersionRb, "0.0.0-development", "1.2.0-rc.1", 1))
	b, err := os.ReadFile(file)
	assert.NilError(t, err)
	assert.Equal(t, string(b), gemVersionRb)
	assert.Assert(t, strings.Contains(f.log.String(), "Build check: widgets_client 1.2.0.pre.rc.1\n"), f.log.String())
	assert.Equal(t, len(f.api.paths), 0, "the build check does not call the API")
}

func TestGemCheckBuildErrors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(f *gemFixture)
		wantErr error
	}{
		{
			name: "failed build",
			setup: func(f *gemFixture) {
				f.buildGem("1.0.0")
				expectGemBuild(f.cmds, cmdFails("rake aborted!"))
			},
			wantErr: errExit,
		},
		{name: "missing gem", setup: func(f *gemFixture) { expectGemBuild(f.cmds, cmdOK("")) }, wantErr: errGemNoFile},
		{
			name: "no VERSION line",
			setup: func(f *gemFixture) {
				writeFile(t, f.gemDir(), "lib/widgets_client/version.rb", "module WidgetsClient\nend\n")
			},
			wantErr: errGemNoVersion,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newGemFixture(t)
			tc.setup(f)
			file := filepath.Join(f.gemDir(), "lib", "widgets_client", "version.rb")
			before, err := os.ReadFile(file)
			assert.NilError(t, err)
			err = f.registry().checkBuild(context.Background(), f.target("1.0.0"))
			assertErrIs(t, err, tc.wantErr)
			after, err := os.ReadFile(file)
			assert.NilError(t, err)
			assert.Equal(t, string(after), string(before), "the version file is restored")
		})
	}
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
		orgStatus int
		// listed are the packages of the organization.
		listed    []string
		wantPush  bool
		wantPages int
		// wantOrgPages is the number of pages of the packages of the
		// organization that the lookup lists after the versions.
		wantOrgPages int
		wantErr      error
	}{
		{name: "first publish", version: "1.0.0", built: "1.0.0", published: []string{"0.9.0"}, wantPush: true, wantPages: 1},
		{
			// publish.sh maps -rc to .pre.rc, as Gem::Version does.
			name: "rc version", version: "1.2.0-rc.1", built: "1.2.0.pre.rc.1",
			published: []string{"1.2.0.pre.rc.0"}, wantPush: true, wantPages: 1,
		},
		{
			// A 404 for the versions is a new package only when the
			// packages of the organization can be listed, on all pages,
			// and do not have it.
			name: "new package", version: "1.0.0", built: "1.0.0", status: http.StatusNotFound,
			listed: []string{"acme_client", "gadgets_client", "tools_client"}, wantPush: true, wantPages: 1, wantOrgPages: 2,
		},
		{
			// GitHub lists the gem, so the 404 for its versions does not
			// mean a missing gem.
			name: "listed package", version: "1.0.0", built: "1.0.0", status: http.StatusNotFound,
			listed: []string{"acme_client", "gadgets_client", "Widgets_Client"}, wantErr: errGemLookup, wantPages: 1, wantOrgPages: 2,
		},
		{
			name: "missing owner", version: "1.0.0", built: "1.0.0", status: http.StatusNotFound,
			orgStatus: http.StatusNotFound, wantErr: errGemLookup, wantPages: 1, wantOrgPages: 1,
		},
		{
			name: "token without read:packages", version: "1.0.0", built: "1.0.0", status: http.StatusNotFound,
			orgStatus: http.StatusForbidden, wantErr: errGemLookup, wantPages: 1, wantOrgPages: 1,
		},
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
			f.api.status, f.api.orgStatus, f.api.packages = tc.status, tc.orgStatus, tc.listed
			gemFile := f.buildGem(tc.built)
			if tc.wantPush {
				f.cmds.expect(cmdOK("Pushing gem to "+host+"...\n"), "gem", "push", "--key", "github", "--host", host, gemFile)
			}
			info, err := f.registry().publish(context.Background(), f.target(tc.version))
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				assert.NilError(t, err)
			}
			assert.Assert(t, info == nil, "publish.sh gives no release info")
			for _, c := range f.cmds.calls() {
				assert.Equal(t, c.Cmd.Dir, f.gemDir())
			}
			paths := f.api.paths
			assert.Equal(t, len(paths), tc.wantPages+tc.wantOrgPages)
			for i, path := range paths {
				want := "/orgs/acme/packages/rubygems/widgets_client/versions"
				if i >= tc.wantPages {
					want = "/orgs/acme/packages"
				}
				assert.Equal(t, path, want)
			}
			if tc.wantOrgPages > 0 {
				wantQuery := "package_type=rubygems&per_page=100"
				if tc.wantOrgPages > 1 {
					wantQuery = "package_type=rubygems&page=2&per_page=100"
				}
				assert.Equal(t, f.api.orgQuery, wantQuery)
			}
			if tc.wantPush && tc.wantOrgPages > 0 {
				assert.Assert(t, strings.Contains(f.log.String(), "GitHub Packages has no gem widgets_client in acme "+
					"that the token can read; a private gem that the token cannot read looks the same"), f.log.String())
			}
			for _, auth := range f.api.auths {
				assert.Equal(t, auth, "Bearer ghp_s3cr3t")
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

	// A 404 for an owner that is not an organization is an error, so
	// that rollback does not trust it.
	f.api.status, f.api.orgStatus = http.StatusNotFound, http.StatusNotFound
	_, err := f.registry().exists(context.Background(), f.target("1.1.0"))
	assert.ErrorIs(t, err, errGemLookup)
	assert.ErrorContains(t, err, "cannot list the packages of acme; acme may not exist or may not be an organization")

	// A gem that GitHub lists, but whose versions give 404, is an error.
	f.api.orgStatus, f.api.packages = 0, []string{"widgets_client"}
	_, err = f.registry().exists(context.Background(), f.target("1.1.0"))
	assert.ErrorIs(t, err, errGemLookup)
	assert.ErrorContains(t, err, "gem widgets_client: GitHub lists it in acme, but returned 404 for its versions")

	// A gem that GitHub does not list is missing for the token.
	f.api.packages = []string{"gadgets_client"}
	got, err := f.registry().exists(context.Background(), f.target("1.1.0"))
	assert.NilError(t, err)
	assert.Assert(t, !got)
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

// gemOwnerRepo returns a repository whose origin remote is remote, or
// one without an origin remote when remote is empty.
func gemOwnerRepo(t *testing.T, remote string) string {
	t.Helper()
	g := &Git{Dir: t.TempDir()}
	_, err := g.Run(context.Background(), "init", "-q")
	assert.NilError(t, err)
	if remote != "" {
		_, err = g.Run(context.Background(), "remote", "add", "origin", remote)
		assert.NilError(t, err)
	}
	return g.Dir
}

// TestGemOwner checks the owner order: the org of the box config, and
// else the owner of the origin remote, also when the box config cannot
// be loaded.
func TestGemOwner(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	const httpsRemote, sshRemote, gitlabRemote = "https://github.com/acme/widgets.git",
		"git@github.com:acme/widgets.git", "https://gitlab.com/acme/widgets.git"
	errBox := fmt.Errorf("open box.yaml: %w", fs.ErrNotExist)
	tests := []struct {
		name, remote, org string
		boxErr            error
		want              string // Empty means errGemOwner.
	}{
		{name: "box org wins over the remote", remote: httpsRemote, org: "acme-gems", want: "acme-gems"},
		{name: "box org needs no github.com remote", remote: gitlabRemote, org: "acme-gems", want: "acme-gems"},
		{name: "empty org: https remote", remote: httpsRemote, want: "acme"},
		{name: "empty org: ssh remote", remote: sshRemote, want: "acme"},
		{name: "box load error: remote", remote: httpsRemote, boxErr: errBox, want: "acme"},
		{name: "empty org: not github.com", remote: gitlabRemote},
		{name: "box load error: not github.com", remote: gitlabRemote, boxErr: errBox},
		{name: "empty org: no origin remote"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var debug bytes.Buffer
			ctx := rlog.WithDebug(context.Background(), &debug)
			reg := &gemRegistry{plugin: &spec.GemPlugin{}, boxOrg: fakeBoxOrg(tc.org, tc.boxErr)}
			got, err := reg.owner(ctx, &regTarget{dir: gemOwnerRepo(t, tc.remote)})
			// Only a box load error is in the debug log.
			if tc.boxErr != nil {
				assert.Assert(t, strings.Contains(debug.String(), "gem owner: use the origin remote: "+tc.boxErr.Error()),
					debug.String())
			} else {
				assert.Assert(t, !strings.Contains(debug.String(), "gem owner:"), debug.String())
			}
			if tc.want == "" {
				assert.ErrorIs(t, err, errGemOwner)
				assert.ErrorContains(t, err, "no org in the box config")
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}
}

// TestGemOwnerBoxConfig checks the owner with the box config of HOME, as
// gobox box.LoadBox reads it: a missing box config falls back to the
// origin remote, and a box config with an org gives the org.
func TestGemOwnerBoxConfig(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	p := &regTarget{dir: gemOwnerRepo(t, "https://github.com/acme/widgets.git")}
	reg, ok := newGemRegistry(&spec.GemPlugin{}).(*gemRegistry)
	assert.Assert(t, ok)

	var debug bytes.Buffer
	got, err := reg.owner(rlog.WithDebug(context.Background(), &debug), p)
	assert.NilError(t, err)
	assert.Equal(t, got, "acme")
	assert.Assert(t, strings.Contains(debug.String(), "gem owner: use the origin remote: read the box config: "), debug.String())

	writeFile(t, home, filepath.Join(".outreach", ".config", "box", "box.yaml"), "config:\n  org: acme-gems\n")
	got, err = reg.owner(context.Background(), p)
	assert.NilError(t, err)
	assert.Equal(t, got, "acme-gems")
}

// TestGemRealBuild builds a gem with the real bundle and rake, as
// build.sh does, and checks the .gem file name that publish
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
	reg := &gemRegistry{plugin: &spec.GemPlugin{}, boxOrg: fakeBoxOrg("acme", nil)}
	p := f.target("1.2.0-rc.1")
	p.exec = nil
	assert.NilError(t, reg.prepare(context.Background(), p))
	assert.Assert(t, strings.Contains(f.log.String(), "widgets_client 1.2.0.pre.rc.1 built to pkg/widgets_client-1.2.0.pre.rc.1.gem"),
		f.log.String())

	// publish finds the built gem and pushes it.
	f.cmds.expect(cmdOK(""), "gem", "push", "--key", "github", "--host", "https://rubygems.pkg.github.com/acme",
		filepath.Join(f.gemDir(), "pkg", "widgets_client-1.2.0.pre.rc.1.gem"))
	_, err = reg.publish(context.Background(), f.target("1.2.0-rc.1"))
	assert.NilError(t, err)
}
