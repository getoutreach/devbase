// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Publishes a CircleCI orb, as
// @getoutreach/semantic-release-circleci-orb 3.0.0 does.

package release

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
)

// These are the errors of the orb registry, with the codes of the orb
// plugin.
var (
	// errOrbNoToken is ENOCIRCLECITOKEN: CIRCLECI_API_TOKEN is not set.
	errOrbNoToken = errors.New("ENOCIRCLECITOKEN: no CircleCI token specified in CIRCLECI_API_TOKEN")

	// errOrbNoCLI is ENOCIRCLECICLI: "circleci version" failed.
	errOrbNoCLI = errors.New("ENOCIRCLECICLI: the circleci CLI must be installed")

	// errOrbNoName is ENOORBNAME: the orbName option is not set.
	errOrbNoName = errors.New("ENOORBNAME: missing orbName option")
)

// orbNotFoundRe matches the error of "circleci orb info" for an orb
// version that does not exist (ErrOrbVersionNotExists in circleci-cli).
var orbNotFoundRe = regexp.MustCompile(`no Orb '[^']*' was found`)

// These are the defaults of the orb plugin (resolve-config.js).
const (
	// orbCommand is the circleci CLI.
	orbCommand = "circleci"

	// orbDefaultPath is the orb file when orbPath is not set.
	orbDefaultPath = "orb.yml"

	// orbDevFirst is the dev label of an unstable version.
	orbDevFirst = "dev:first"
)

// orbRegistry publishes the orb of an orb plugin.
type orbRegistry struct {
	plugin *spec.OrbPlugin

	// run runs the circleci commands.
	run cmdRunner
}

// newOrbRegistry returns the registry of an orb plugin.
func newOrbRegistry(pl *spec.OrbPlugin) registry {
	return &orbRegistry{plugin: pl, run: execCommand}
}

// name names the registry in messages.
func (o *orbRegistry) name() string {
	return "orb " + o.plugin.Name
}

// prepare runs the plugin's verifyConditions (verify.js), because the
// plugin has no prepare step: CIRCLECI_API_TOKEN, the circleci CLI and
// the orbName option. It returns all the errors.
func (o *orbRegistry) prepare(ctx context.Context, p *regTarget) error {
	env := envOrProcess(p.env)
	var errs []error
	if envValue(env, "CIRCLECI_API_TOKEN") == "" {
		errs = append(errs, errOrbNoToken)
	}
	if _, err := o.run(ctx, p.log, &command{dir: p.dir, env: env, name: orbCommand, args: []string{"version"}}); err != nil {
		errs = append(errs, fmt.Errorf("%w: %w", errOrbNoCLI, err))
	}
	if o.plugin.Name == "" {
		errs = append(errs, errOrbNoName)
	}
	return errors.Join(errs...)
}

// publish publishes the orb with "circleci orb publish", as publish.js
// does, and returns the plugin's release info. It skips the publish when
// the registry has the version (requirement 8), and then returns the
// release info. The dev:first label of an unstable version is always
// published again, because it does not name the version.
func (o *orbRegistry) publish(ctx context.Context, p *regTarget) (*forge.Published, error) {
	ref := o.ref(p)
	info := &forge.Published{Name: "CircleCI Orb", URL: "https://circleci.com/orbs/registry/orb/" + ref}
	if !strings.HasSuffix(ref, "@"+orbDevFirst) {
		found, err := o.exists(ctx, p)
		if err != nil {
			return nil, err
		}
		if found {
			fmt.Fprintf(p.log, "Skip publishing to CircleCI Orb registry as %s exists\n", ref)
			return info, nil
		}
	}
	fmt.Fprintf(p.log, "Publishing version %s to CircleCI Orb registry\n", p.version)
	path := o.plugin.Path
	if path == "" {
		path = orbDefaultPath
	}
	if _, err := o.run(ctx, p.log, o.cliCommand(p, "orb", "publish", path, ref)); err != nil {
		return nil, fmt.Errorf("circleci orb publish: %w", err)
	}
	fmt.Fprintf(p.log, "Published %s\n", ref)
	return info, nil
}

// exists reports whether the registry has the orb version of the plan,
// with "circleci orb info". circleci-cli fails with "no Orb ... was
// found" for a missing orb or version.
func (o *orbRegistry) exists(ctx context.Context, p *regTarget) (bool, error) {
	ref := o.ref(p)
	res, err := o.run(ctx, p.log, o.cliCommand(p, "orb", "info", ref))
	if err != nil {
		if orbNotFoundRe.MatchString(res.stderr) {
			return false, nil
		}
		return false, fmt.Errorf("circleci orb info %s: %w", ref, err)
	}
	return true, nil
}

// ref returns the orb reference of the plan, as publish.js makes it: an
// rc version becomes "dev:<version>", and an unstable version becomes
// "dev:first".
func (o *orbRegistry) ref(p *regTarget) string {
	version := p.version
	switch {
	case strings.Contains(version, "-rc."):
		version = "dev:" + version
	case strings.Contains(version, "unstable"):
		version = orbDevFirst
	}
	return o.plugin.Name + "@" + version
}

// cliCommand returns a circleci command in the repository with the
// CircleCI token, as the plugin passes it. The token's variable name is
// a secret name, so the logs and errors mask it.
func (o *orbRegistry) cliCommand(p *regTarget, args ...string) *command {
	env := envOrProcess(p.env)
	token := envValue(env, "CIRCLECI_API_TOKEN")
	return &command{
		dir: p.dir, env: env, name: orbCommand,
		args: append(args, "--token", token),
	}
}
