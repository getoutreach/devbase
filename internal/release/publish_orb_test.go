// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the orb registry, with the cases of
// @getoutreach/semantic-release-circleci-orb 3.0.0 (test/verify.test.js)
// and a fake runner.

package release

import (
	"context"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

// orbNotFound is the stderr of circleci-cli 0.1.34950 for a missing orb
// version.
const orbNotFound = "Error: Failed to get info for 'outreach/devbase@1.0.0': " +
	"no Orb 'outreach/devbase@1.0.0' was found; please check that the Orb reference is correct\n"

// orbFixture returns an orb registry with a fake runner, and a publisher
// of version.
func orbFixture(
	t *testing.T, plugin *spec.OrbPlugin, version string, replies map[string]fakeReply, env ...string,
) (*orbRegistry, *regTarget, *npmFixture) {
	t.Helper()
	f := newNPMFixture(t, "", "", env...)
	f.run.replies = replies
	p := f.publisher(version, "")
	return &orbRegistry{plugin: plugin, run: f.run.run}, p, f
}

// TestOrbPrepare ports test/verify.test.js: prepare runs the plugin's
// verifyConditions.
func TestOrbPrepare(t *testing.T) {
	named := &spec.OrbPlugin{Name: "org/orb"}
	t.Run(`Verify "CIRCLECI_API_TOKEN" is set`, func(t *testing.T) {
		o, p, f := orbFixture(t, named, "1.0.0", nil, "CIRCLECI_API_TOKEN=my_token")
		assert.NilError(t, o.prepare(context.Background(), p))
		assert.DeepEqual(t, f.run.argvs(), [][]string{{"circleci", "version"}})
		assert.Equal(t, f.run.calls[0].dir, f.repo)
	})
	t.Run(`Return SemanticReleaseError if "CIRCLECI_API_TOKEN" is not set`, func(t *testing.T) {
		o, p, _ := orbFixture(t, named, "1.0.0", nil)
		assert.ErrorIs(t, o.prepare(context.Background(), p), errOrbNoToken)
	})
	t.Run(`Return SemanticReleaseError if "circleci" is not installed`, func(t *testing.T) {
		o, p, _ := orbFixture(t, named, "1.0.0", map[string]fakeReply{"circleci version": {err: errExit}}, "CIRCLECI_API_TOKEN=my_token")
		assert.ErrorIs(t, o.prepare(context.Background(), p), errOrbNoCLI)
	})
	t.Run(`Return SemanticReleaseError if "orbName" option is not set`, func(t *testing.T) {
		o, p, _ := orbFixture(t, &spec.OrbPlugin{}, "1.0.0", nil, "CIRCLECI_API_TOKEN=my_token")
		assert.ErrorIs(t, o.prepare(context.Background(), p), errOrbNoName)
	})
	t.Run("all errors", func(t *testing.T) {
		o, p, _ := orbFixture(t, &spec.OrbPlugin{}, "1.0.0", map[string]fakeReply{"circleci version": {err: errExit}})
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
	tests := []struct {
		name     string
		plugin   *spec.OrbPlugin
		version  string
		replies  map[string]fakeReply
		wantArgv [][]string
		wantInfo *forge.Published
		wantErr  error
	}{
		{
			name: "first publish", plugin: &spec.OrbPlugin{Name: "outreach/devbase", Path: "orb.yml"}, version: "1.0.0",
			replies: map[string]fakeReply{"circleci orb info": {stderr: orbNotFound, err: errExit}},
			wantArgv: [][]string{
				{"circleci", "orb", "info", "outreach/devbase@1.0.0", "--token", token},
				{"circleci", "orb", "publish", "orb.yml", "outreach/devbase@1.0.0", "--token", token},
			},
			wantInfo: &forge.Published{Name: "CircleCI Orb", URL: "https://circleci.com/orbs/registry/orb/outreach/devbase@1.0.0"},
		},
		{
			name: "default orb path", plugin: &spec.OrbPlugin{Name: "outreach/devbase"}, version: "1.0.0",
			replies: map[string]fakeReply{"circleci orb info": {stderr: orbNotFound, err: errExit}},
			wantArgv: [][]string{
				{"circleci", "orb", "info", "outreach/devbase@1.0.0", "--token", token},
				{"circleci", "orb", "publish", "orb.yml", "outreach/devbase@1.0.0", "--token", token},
			},
			wantInfo: &forge.Published{Name: "CircleCI Orb", URL: "https://circleci.com/orbs/registry/orb/outreach/devbase@1.0.0"},
		},
		{
			// Scenario 2 of the RFC: an rc version becomes a dev label.
			name: "prerelease dev label", plugin: &spec.OrbPlugin{Name: "outreach/devbase", Path: "dist/orb.yml"}, version: "1.4.0-rc.3",
			replies: map[string]fakeReply{"circleci orb info": {stderr: orbNotFound, err: errExit}},
			wantArgv: [][]string{
				{"circleci", "orb", "info", "outreach/devbase@dev:1.4.0-rc.3", "--token", token},
				{"circleci", "orb", "publish", "dist/orb.yml", "outreach/devbase@dev:1.4.0-rc.3", "--token", token},
			},
			wantInfo: &forge.Published{Name: "CircleCI Orb", URL: "https://circleci.com/orbs/registry/orb/outreach/devbase@dev:1.4.0-rc.3"},
		},
		{
			name: "unstable dev:first is always published", plugin: &spec.OrbPlugin{Name: "outreach/devbase"},
			version: "0.0.0-unstable+abc123",
			wantArgv: [][]string{
				{"circleci", "orb", "publish", "orb.yml", "outreach/devbase@dev:first", "--token", token},
			},
			wantInfo: &forge.Published{Name: "CircleCI Orb", URL: "https://circleci.com/orbs/registry/orb/outreach/devbase@dev:first"},
		},
		{
			name: "already published version skipped", plugin: &spec.OrbPlugin{Name: "outreach/devbase"}, version: "1.0.0",
			replies: map[string]fakeReply{"circleci orb info": {stdout: "Latest: outreach/devbase@1.0.0\n"}},
			wantArgv: [][]string{
				{"circleci", "orb", "info", "outreach/devbase@1.0.0", "--token", token},
			},
			wantInfo: &forge.Published{Name: "CircleCI Orb", URL: "https://circleci.com/orbs/registry/orb/outreach/devbase@1.0.0"},
		},
		{
			name: "orb info failure", plugin: &spec.OrbPlugin{Name: "outreach/devbase"}, version: "1.0.0",
			replies: map[string]fakeReply{"circleci orb info": {stderr: "Error: unauthorized\n", err: errExit}},
			wantArgv: [][]string{
				{"circleci", "orb", "info", "outreach/devbase@1.0.0", "--token", token},
			},
			wantErr: errExit,
		},
		{
			name: "publish failure", plugin: &spec.OrbPlugin{Name: "outreach/devbase"}, version: "1.0.0",
			replies: map[string]fakeReply{
				"circleci orb info":    {stderr: orbNotFound, err: errExit},
				"circleci orb publish": {stderr: "Error: invalid orb\n", err: errExit},
			},
			wantArgv: [][]string{
				{"circleci", "orb", "info", "outreach/devbase@1.0.0", "--token", token},
				{"circleci", "orb", "publish", "orb.yml", "outreach/devbase@1.0.0", "--token", token},
			},
			wantErr: errExit,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o, p, f := orbFixture(t, tc.plugin, tc.version, tc.replies, "CIRCLECI_API_TOKEN="+token)
			info, err := o.publish(context.Background(), p)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				assert.NilError(t, err)
				assert.DeepEqual(t, info, tc.wantInfo)
			}
			assert.DeepEqual(t, f.run.argvs(), tc.wantArgv)
			for _, c := range f.run.calls {
				assert.Equal(t, c.dir, f.repo)
				assert.DeepEqual(t, c.env, f.env)
			}
			assert.Assert(t, !strings.Contains(f.log.String(), token), f.log.String())
		})
	}
}

func TestOrbExists(t *testing.T) {
	plugin := &spec.OrbPlugin{Name: "outreach/devbase"}
	for _, tc := range []struct {
		name  string
		reply fakeReply
		want  bool
	}{
		{name: "published", reply: fakeReply{stdout: "Latest: outreach/devbase@1.0.0\n"}, want: true},
		{name: "missing", reply: fakeReply{stderr: orbNotFound, err: errExit}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, p, _ := orbFixture(t, plugin, "1.0.0", map[string]fakeReply{"circleci orb info": tc.reply}, "CIRCLECI_API_TOKEN=t")
			got, err := o.exists(context.Background(), p)
			assert.NilError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}
}
