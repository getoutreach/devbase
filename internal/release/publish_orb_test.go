// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the orb registry, with the cases of
// @getoutreach/semantic-release-circleci-orb 3.0.0 (test/verify.test.js)
// and stubs of the circleci commands.

package release

import (
	"context"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"github.com/getoutreach/gobox/pkg/exec/execstub"
	"gotest.tools/v3/assert"
)

// orbNotFound is the stderr of circleci-cli 0.1.34950 for a missing orb
// version.
const orbNotFound = "Error: Failed to get info for 'outreach/devbase@1.0.0': " +
	"no Orb 'outreach/devbase@1.0.0' was found; please check that the Orb reference is correct\n"

// orbFixture returns an orb registry, a publisher of version with the
// stubs of the fixture, and the fixture.
func orbFixture(t *testing.T, plugin *spec.OrbPlugin, version string, env ...string) (*orbRegistry, *regTarget, *npmFixture) {
	t.Helper()
	f := newNPMFixture(t, "", "", env...)
	p := f.publisher(version, "")
	return &orbRegistry{plugin: plugin}, p, f
}

// circleciVersion is the argv of the CLI check.
var circleciVersion = []string{"circleci", "version"}

// TestOrbPrepare ports test/verify.test.js: prepare runs the plugin's
// verifyConditions.
func TestOrbPrepare(t *testing.T) {
	named := &spec.OrbPlugin{Name: "org/orb"}
	t.Run(`Verify "CIRCLECI_API_TOKEN" is set`, func(t *testing.T) {
		o, p, f := orbFixture(t, named, "1.0.0", "CIRCLECI_API_TOKEN=my_token")
		f.cmds.expect(cmdOK("0.1.34950\n"), circleciVersion...)
		assert.NilError(t, o.prepare(context.Background(), p))
		assert.Equal(t, f.cmds.calls()[0].Cmd.Dir, f.repo)
	})
	t.Run(`Return SemanticReleaseError if "CIRCLECI_API_TOKEN" is not set`, func(t *testing.T) {
		o, p, f := orbFixture(t, named, "1.0.0")
		f.cmds.expect(cmdOK(""), circleciVersion...)
		assert.ErrorIs(t, o.prepare(context.Background(), p), errOrbNoToken)
	})
	t.Run(`Return SemanticReleaseError if "circleci" is not installed`, func(t *testing.T) {
		o, p, f := orbFixture(t, named, "1.0.0", "CIRCLECI_API_TOKEN=my_token")
		f.cmds.expect(cmdFails(""), circleciVersion...)
		assert.ErrorIs(t, o.prepare(context.Background(), p), errOrbNoCLI)
	})
	t.Run(`Return SemanticReleaseError if "orbName" option is not set`, func(t *testing.T) {
		o, p, f := orbFixture(t, &spec.OrbPlugin{}, "1.0.0", "CIRCLECI_API_TOKEN=my_token")
		f.cmds.expect(cmdOK(""), circleciVersion...)
		assert.ErrorIs(t, o.prepare(context.Background(), p), errOrbNoName)
	})
	t.Run("all errors", func(t *testing.T) {
		o, p, f := orbFixture(t, &spec.OrbPlugin{}, "1.0.0")
		f.cmds.expect(cmdFails(""), circleciVersion...)
		err := o.prepare(context.Background(), p)
		assert.ErrorIs(t, err, errOrbNoToken)
		assert.ErrorIs(t, err, errOrbNoCLI)
		assert.ErrorIs(t, err, errOrbNoName)
	})
}

// TestOrbPublishScenarios checks the argv, dir and env of the circleci
// commands of publish (lib/publish.js), with the existence check of
// requirement 8.
func TestOrbPublishScenarios(t *testing.T) {
	const token = "my_token"
	info := func(ref string) []string { return []string{"circleci", "orb", "info", ref, "--token", token} }
	publish := func(path, ref string) []string {
		return []string{"circleci", "orb", "publish", path, ref, "--token", token}
	}
	tests := []struct {
		name     string
		plugin   *spec.OrbPlugin
		version  string
		cmds     []stubCmd
		wantInfo *forge.Published
		wantErr  error
	}{
		{
			name: "first publish", plugin: &spec.OrbPlugin{Name: "outreach/devbase", Path: "orb.yml"}, version: "1.0.0",
			cmds: []stubCmd{
				{info("outreach/devbase@1.0.0"), cmdFails(orbNotFound)},
				{publish("orb.yml", "outreach/devbase@1.0.0"), cmdOK("")},
			},
			wantInfo: &forge.Published{Name: "CircleCI Orb", URL: "https://circleci.com/orbs/registry/orb/outreach/devbase@1.0.0"},
		},
		{
			name: "default orb path", plugin: &spec.OrbPlugin{Name: "outreach/devbase"}, version: "1.0.0",
			cmds: []stubCmd{
				{info("outreach/devbase@1.0.0"), cmdFails(orbNotFound)},
				{publish("orb.yml", "outreach/devbase@1.0.0"), cmdOK("")},
			},
			wantInfo: &forge.Published{Name: "CircleCI Orb", URL: "https://circleci.com/orbs/registry/orb/outreach/devbase@1.0.0"},
		},
		{
			// Scenario 2 of the RFC: an rc version becomes a dev label.
			name: "prerelease dev label", plugin: &spec.OrbPlugin{Name: "outreach/devbase", Path: "dist/orb.yml"}, version: "1.4.0-rc.3",
			cmds: []stubCmd{
				{info("outreach/devbase@dev:1.4.0-rc.3"), cmdFails(orbNotFound)},
				{publish("dist/orb.yml", "outreach/devbase@dev:1.4.0-rc.3"), cmdOK("")},
			},
			wantInfo: &forge.Published{Name: "CircleCI Orb", URL: "https://circleci.com/orbs/registry/orb/outreach/devbase@dev:1.4.0-rc.3"},
		},
		{
			name: "unstable dev:first is always published", plugin: &spec.OrbPlugin{Name: "outreach/devbase"},
			version:  "0.0.0-unstable+abc123",
			cmds:     []stubCmd{{publish("orb.yml", "outreach/devbase@dev:first"), cmdOK("")}},
			wantInfo: &forge.Published{Name: "CircleCI Orb", URL: "https://circleci.com/orbs/registry/orb/outreach/devbase@dev:first"},
		},
		{
			name: "already published version skipped", plugin: &spec.OrbPlugin{Name: "outreach/devbase"}, version: "1.0.0",
			cmds:     []stubCmd{{info("outreach/devbase@1.0.0"), cmdOK("Latest: outreach/devbase@1.0.0\n")}},
			wantInfo: &forge.Published{Name: "CircleCI Orb", URL: "https://circleci.com/orbs/registry/orb/outreach/devbase@1.0.0"},
		},
		{
			name: "orb info failure", plugin: &spec.OrbPlugin{Name: "outreach/devbase"}, version: "1.0.0",
			cmds:    []stubCmd{{info("outreach/devbase@1.0.0"), cmdFails("Error: unauthorized\n")}},
			wantErr: errExit,
		},
		{
			name: "publish failure", plugin: &spec.OrbPlugin{Name: "outreach/devbase"}, version: "1.0.0",
			cmds: []stubCmd{
				{info("outreach/devbase@1.0.0"), cmdFails(orbNotFound)},
				{publish("orb.yml", "outreach/devbase@1.0.0"), cmdFails("Error: invalid orb\n")},
			},
			wantErr: errExit,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o, p, f := orbFixture(t, tc.plugin, tc.version, "CIRCLECI_API_TOKEN="+token)
			f.cmds.expectAll(tc.cmds)
			info, err := o.publish(context.Background(), p)
			if tc.wantErr != nil {
				assertErrIs(t, err, tc.wantErr)
				// The error has the command line without the token.
				assert.ErrorContains(t, err, "--token [secure]: exit status 1: Error: ")
			} else {
				assert.NilError(t, err)
				assert.DeepEqual(t, info, tc.wantInfo)
			}
			for _, c := range f.cmds.calls() {
				assert.Equal(t, c.Cmd.Dir, f.repo)
				assert.DeepEqual(t, c.Cmd.Env, f.env)
			}
			assert.Assert(t, strings.Contains(f.log.String(), "Run circleci orb "), f.log.String())
			assert.Assert(t, !strings.Contains(f.log.String(), token), f.log.String())
		})
	}
}

func TestOrbExists(t *testing.T) {
	plugin := &spec.OrbPlugin{Name: "outreach/devbase"}
	for _, tc := range []struct {
		name string
		res  execstub.Result
		want bool
	}{
		{name: "published", res: cmdOK("Latest: outreach/devbase@1.0.0\n"), want: true},
		{name: "missing", res: cmdFails(orbNotFound)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, p, f := orbFixture(t, plugin, "1.0.0", "CIRCLECI_API_TOKEN=t")
			f.cmds.expect(tc.res, "circleci", "orb", "info", "outreach/devbase@1.0.0", "--token", "t")
			got, err := o.exists(context.Background(), p)
			assert.NilError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}
}
