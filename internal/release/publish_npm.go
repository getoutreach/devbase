// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Publishes an npm package, as @semantic-release/npm 13.2.0
// does.

package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
)

// These are the errors of the npm registry, with the codes of the npm
// plugin.
var (
	// errNPMNoPkg is ENOPKG: the package has no package.json file.
	errNPMNoPkg = errors.New("ENOPKG: missing package.json file")

	// errNPMNoPkgName is ENOPKGNAME: package.json has no name.
	errNPMNoPkgName = errors.New("ENOPKGNAME: missing name property in package.json")

	// errNPMNoToken is ENONPMTOKEN: the npm config has no authentication
	// for the registry, and NPM_TOKEN is not set.
	errNPMNoToken = errors.New("ENONPMTOKEN: no npm token specified")

	// errNPMInvalidToken is EINVALIDNPMTOKEN: npm whoami failed.
	errNPMInvalidToken = errors.New("EINVALIDNPMTOKEN: invalid npm token")
)

// npmNotFoundRe matches the E404 error of npm view for a missing package
// or version, from npm 7 and later ("npm error code E404") and npm 6
// ("npm ERR! code E404").
var npmNotFoundRe = regexp.MustCompile(`(?m)^npm (error|ERR!) code E404\b`)

// npmPackage is the part of package.json that the npm plugin reads.
type npmPackage struct {
	// Name is the package name.
	Name string `json:"name"`

	// Version is the package version.
	Version string `json:"version"`

	// Private is true only for the JSON value true.
	Private any `json:"private"`

	// PublishConfig holds the registry of the package.
	PublishConfig struct {
		// Registry is the registry URL, or empty.
		Registry string `json:"registry"`
	} `json:"publishConfig"`
}

// isPrivate reports whether package.json has "private": true.
func (pkg *npmPackage) isPrivate() bool {
	b, ok := pkg.Private.(bool)
	return ok && b
}

// npmRegistry publishes the npm package of an npm plugin.
type npmRegistry struct {
	plugin *spec.NPMPlugin
}

// newNPMRegistry returns the registry of an npm plugin.
func newNPMRegistry(pl *spec.NPMPlugin) registry {
	return &npmRegistry{plugin: pl}
}

// name names the registry in messages.
func (n *npmRegistry) name() string {
	return "npm"
}

// prepare verifies the package and the npm authentication, as the
// plugin's verifyConditions does, runs the install command of the
// plugin, and then sets the version in package.json with npm version, as
// prepare.js does. The version changes only in the checkout
// (requirement 12).
func (n *npmRegistry) prepare(ctx context.Context, p *regTarget) error {
	s, err := n.open(ctx, p, true)
	if err != nil {
		return err
	}
	defer s.close()
	if err := n.install(ctx, p, s); err != nil {
		return err
	}
	version := p.version
	fmt.Fprintf(p.log, "Write version %s to package.json in %s\n", version, s.dir)
	_, err = p.run(ctx, p.log, &command{
		dir: s.dir, env: s.env, name: "npm",
		args: []string{"version", version, "--userconfig", s.npmrc, "--no-git-tag-version", "--allow-same-version"},
	})
	if err != nil {
		return fmt.Errorf("npm version: %w", err)
	}
	return nil
}

// install runs the install command of the plugin in the package
// folder, if it has one.
func (n *npmRegistry) install(ctx context.Context, p *regTarget, s *npmSession) error {
	if n.plugin.Install == "" {
		return nil
	}
	args, err := npmInstallArgs(n.plugin.Install)
	if err != nil {
		return err
	}
	if _, err := p.run(ctx, p.log, &command{dir: s.dir, env: s.env, name: args[0], args: args[1:]}); err != nil {
		return fmt.Errorf("npm install command: %w", err)
	}
	return nil
}

// npmInstallArgs splits an install command into arguments.
func npmInstallArgs(cmd string) ([]string, error) {
	args, err := splitShellWords(cmd)
	if err != nil {
		return nil, fmt.Errorf("npm install command %q: %w", cmd, err)
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("npm install command %q: %w", cmd, errEmptyHook)
	}
	return args, nil
}

// publish publishes the package with npm publish on the dist-tag of the
// channel, as publish.js does, and returns the release info of
// get-release-info.js. It skips a private package and returns nil. It
// skips the publish when the registry has the version (requirement 8),
// and then returns the release info.
func (n *npmRegistry) publish(ctx context.Context, p *regTarget) (*forge.Published, error) {
	s, err := n.open(ctx, p, false)
	if err != nil {
		return nil, err
	}
	defer s.close()
	if s.pkg.isPrivate() {
		fmt.Fprintln(p.log, "Skip publishing to npm registry as package.json's private property is true")
		return nil, nil //nolint:nilnil // Why: a private package is published nowhere.
	}
	version := p.version
	distTag := npmDistTag(p.channel)
	info := npmReleaseInfo(s.pkg.Name, version, distTag, s.registry, s.env)
	found, err := n.view(ctx, p, s)
	if err != nil {
		return nil, err
	}
	if found {
		fmt.Fprintf(p.log, "Skip publishing to npm registry as %s@%s is already on %s\n", s.pkg.Name, version, s.registry)
		return &info, nil
	}
	fmt.Fprintf(p.log, "Publishing version %s to npm registry on dist-tag %s\n", version, distTag)
	_, err = p.run(ctx, p.log, &command{
		dir: p.dir, env: s.env, name: "npm",
		args: []string{"publish", s.dir, "--userconfig", s.npmrc, "--tag", distTag, "--registry", s.registry},
	})
	if err != nil {
		return nil, fmt.Errorf("npm publish: %w", err)
	}
	fmt.Fprintf(p.log, "Published %s@%s to dist-tag @%s on %s\n", s.pkg.Name, version, distTag, s.registry)
	return &info, nil
}

// exists reports whether the registry has the version of the plan. A
// private package is never published, so it is false. npm view gives
// E404 for a missing version, but also for a wrong registry URL and for
// a restricted package that the credentials cannot read. So a missing
// version counts only when the registry returns the package document,
// or, for a package that it does not return, when confirmRegistry
// accepts the registry and the credentials. This works on any registry,
// not only on the default registry where prepare checks the token.
func (n *npmRegistry) exists(ctx context.Context, p *regTarget) (bool, error) {
	s, err := n.open(ctx, p, false)
	if err != nil {
		return false, err
	}
	defer s.close()
	if s.pkg.isPrivate() {
		return false, nil
	}
	found, err := n.view(ctx, p, s)
	if err != nil || found {
		return found, err
	}
	// The registry returns the package document, so only the version
	// is missing.
	if _, seen, err := s.npmView(ctx, p, s.pkg.Name, "name"); err != nil || seen {
		return false, err
	}
	if err := s.confirmRegistry(ctx, p); err != nil {
		return false, fmt.Errorf("npm view gave E404 for %s, which can be a missing package or wrong credentials: %w", s.pkg.Name, err)
	}
	return false, nil
}

// view reports whether the registry has the version, with
// "npm view <name>@<version> version". npm prints the version when it
// exists. For a missing version, npm 7 and later fail with E404, and
// older versions print nothing.
func (n *npmRegistry) view(ctx context.Context, p *regTarget, s *npmSession) (bool, error) {
	out, ok, err := s.npmView(ctx, p, s.pkg.Name+"@"+p.version, "version")
	return ok && strings.TrimSpace(out) != "", err
}

// npmSession is the package and the temporary npm config of one npm
// step, as the plugin keeps them between its steps.
type npmSession struct {
	// pkg is the package.json of dir.
	pkg *npmPackage

	// dir is the package folder: pkgRoot in the repository.
	dir string

	// registry is the registry of the package.
	registry string

	// npmrc is the temporary npm config file, which exists only when the
	// package is not private.
	npmrc string

	// tmp is the folder of npmrc.
	tmp string

	// env is the environment of the npm commands.
	env []string

	// basicAuth is true when npm authenticates to the registry with a
	// user name and password, not a token.
	basicAuth bool
}

// close removes the temporary npm config.
func (s *npmSession) close() {
	_ = os.RemoveAll(s.tmp)
}

// npmView runs "npm view <arg> <field>" and returns its stdout, and
// false when npm fails with E404. npm fails with E404 when it does not
// get the package document, and when no version matches arg. With the
// package name as arg, it exits with 0 when it gets the document, also
// when no version has the latest dist-tag.
func (s *npmSession) npmView(ctx context.Context, p *regTarget, arg, field string) (stdout string, ok bool, err error) {
	res, err := p.run(ctx, p.log, &command{
		dir: p.dir, env: s.env, name: "npm",
		args: []string{"view", arg, field, "--userconfig", s.npmrc, "--registry", s.registry},
	})
	if err != nil {
		if npmNotFoundRe.MatchString(res.stderr) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("npm view %s: %w", arg, err)
	}
	return res.stdout, true, nil
}

// confirmRegistry checks that the registry URL is an npm registry that
// accepts the credentials. With a token, npm whoami asks the registry
// for the user of the token (GET /-/whoami), so it fails for a wrong
// URL or a bad token. With a user name and password, npm whoami prints
// the user name without a request, so confirmRegistry runs npm ping
// (GET /-/ping) with the credentials instead, which fails for a wrong
// URL, and for bad credentials only when the registry checks them on
// ping.
func (s *npmSession) confirmRegistry(ctx context.Context, p *regTarget) error {
	if !s.basicAuth {
		return s.whoami(ctx, p)
	}
	if err := s.registryCommand(ctx, p, "ping"); err != nil {
		return fmt.Errorf("npm ping %s: %w", s.registry, err)
	}
	return nil
}

// whoami checks the token with npm whoami, as verify-auth.js does.
func (s *npmSession) whoami(ctx context.Context, p *regTarget) error {
	if err := s.registryCommand(ctx, p, "whoami"); err != nil {
		return fmt.Errorf("%w for %s: %w", errNPMInvalidToken, s.registry, err)
	}
	return nil
}

// registryCommand runs "npm <op>" on the registry of s, without
// DEFAULT_NPM_REGISTRY in the environment, as verify-auth.js runs npm
// whoami.
func (s *npmSession) registryCommand(ctx context.Context, p *regTarget, op string) error {
	_, err := p.run(ctx, p.log, &command{
		dir: p.dir, env: withoutEnv(s.env, "DEFAULT_NPM_REGISTRY"), name: "npm",
		args: []string{op, "--userconfig", s.npmrc, "--registry", s.registry},
	})
	return err
}

// open reads package.json and, for a package that is not private, writes
// the temporary npm config with the authentication, as verify-auth.js
// does. With whoami, it then checks the token with npm whoami when the
// registry is the default registry.
func (n *npmRegistry) open(ctx context.Context, p *regTarget, whoami bool) (*npmSession, error) {
	cwd := p.dir
	s := &npmSession{dir: cwd, env: envOrProcess(p.env)}
	if root := n.plugin.PkgRoot; root != "" {
		s.dir = root
		if !filepath.IsAbs(root) {
			s.dir = filepath.Join(cwd, root)
		}
	}
	pkg, err := readNPMPackage(s.dir)
	if err != nil {
		return nil, err
	}
	s.pkg = pkg
	cfg, files := npmrcConfig(cwd, s.env)
	s.registry = npmRegistryURL(pkg, cfg, s.env)
	if s.tmp, err = os.MkdirTemp("", "devbase-npm-"); err != nil {
		return nil, fmt.Errorf("make the npm config folder: %w", err)
	}
	s.npmrc = filepath.Join(s.tmp, ".npmrc")
	if pkg.isPrivate() {
		return s, nil
	}
	if err := setNpmrcAuth(p, s, cfg, files); err != nil {
		s.close()
		return nil, err
	}
	s.basicAuth = npmBasicAuth(s.registry, cfg)
	if whoami && normalizeRegistryURL(s.registry) == normalizeRegistryURL(defaultNPMRegistry(s.env)) {
		if err := s.whoami(ctx, p); err != nil {
			s.close()
			return nil, err
		}
	}
	return s, nil
}

// npmBasicAuth reports whether npm authenticates to registry with a user
// name and password, as getCredentialsByURI of npm 10 finds them for
// the nerf-dart of the registry: a token first, then a username and
// _password, then _auth.
func npmBasicAuth(registry string, cfg map[string]string) bool {
	nerf := nerfDart(registry)
	if cfg[nerf+":_authToken"] != "" {
		return false
	}
	return cfg[nerf+":username"] != "" && cfg[nerf+":_password"] != "" || cfg[nerf+":_auth"] != ""
}

// readNPMPackage reads the package.json of dir, as get-pkg.js does.
func readNPMPackage(dir string) (*npmPackage, error) {
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w in %s", errNPMNoPkg, dir)
	}
	if err != nil {
		return nil, fmt.Errorf("read package.json: %w", err)
	}
	var pkg npmPackage
	if err := json.Unmarshal(b, &pkg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Join(dir, "package.json"), err)
	}
	if pkg.Name == "" {
		return nil, fmt.Errorf("%w in %s", errNPMNoPkgName, dir)
	}
	return &pkg, nil
}

// setNpmrcAuth writes the temporary npm config, as set-npmrc-auth.js
// does: the npm config files that it read, and, when they have no
// authentication for the registry, a token line that names the
// NPM_TOKEN variable. The file never holds the value of NPM_TOKEN.
func setNpmrcAuth(p *regTarget, s *npmSession, cfg map[string]string, files []string) error {
	fmt.Fprintf(p.log, "Verify authentication for registry %s\n", s.registry)
	if len(files) > 0 {
		fmt.Fprintf(p.log, "Reading npm config from %s\n", strings.Join(files, ", "))
	}
	contents := make([]string, 0, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read npm config: %w", err)
		}
		contents = append(contents, string(b))
	}
	current := strings.Join(contents, "\n")
	if !npmAuthConfigured(s.registry, cfg) {
		if envValue(s.env, "NPM_TOKEN") == "" {
			return fmt.Errorf("%w: an npm token must be set in NPM_TOKEN for the registry %s", errNPMNoToken, s.registry)
		}
		if current != "" {
			current += "\n"
		}
		current += nerfDart(s.registry) + ":_authToken = ${NPM_TOKEN}"
		defer fmt.Fprintf(p.log, "Wrote NPM_TOKEN to %s\n", s.npmrc)
	}
	if err := os.WriteFile(s.npmrc, []byte(current), 0o600); err != nil {
		return fmt.Errorf("write the npm config: %w", err)
	}
	return nil
}

// defaultNPMRegistry returns DEFAULT_NPM_REGISTRY, or the official
// registry.
func defaultNPMRegistry(env []string) string {
	if r := envValue(env, "DEFAULT_NPM_REGISTRY"); r != "" {
		return r
	}
	return officialNPMRegistry
}

// npmReleaseInfo returns the release info of get-release-info.js: the
// dist-tag in the name, and the npmjs.com URL only for the default
// registry.
func npmReleaseInfo(name, version, distTag, registry string, env []string) forge.Published {
	info := forge.Published{Name: "npm package (@" + distTag + " dist-tag)"}
	if normalizeRegistryURL(registry) == normalizeRegistryURL(defaultNPMRegistry(env)) {
		info.URL = "https://www.npmjs.com/package/" + name + "/v/" + version
	}
	return info
}
