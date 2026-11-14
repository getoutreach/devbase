// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests of the keyless signing of checksums.txt in Publish.

package release

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/gobox/pkg/exec"
	"github.com/getoutreach/gobox/pkg/exec/execstub"
	"gotest.tools/v3/assert"
)

// testOIDCToken is the CircleCI OIDC token of the tests.
const testOIDCToken = "eyJhbGciOiJSUzI1NiJ9.eyJhdWQiOiJzaWdzdG9yZSJ9.c2lnbmF0dXJl"

// testBundle is the content of the Sigstore bundle that the fake cosign
// writes.
const testBundle = `{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json"}`

// errBadToken is returned by the fake cosign for a wrong token file.
var errBadToken = errors.New("wrong OIDC token file")

// keylessFixture is a planned and built release whose service.yaml sets
// releaseOptions.signing, with stubs of the commands.
type keylessFixture struct {
	*publishFixture

	repo *Repo
	plan *Plan

	// log is the publish log.
	log strings.Builder

	// signs records the state of each cosign run.
	signs []signState
}

// signState is the state of the release when cosign ran.
type signState struct {
	// remoteTag is the commit of the release tag on the remote, or "".
	remoteTag string

	// tokenFile is the token file that cosign got.
	tokenFile string
}

// newKeylessFixture makes a keylessFixture with signing in service.yaml,
// or without service.yaml when signing is empty.
func newKeylessFixture(t *testing.T, signing string) *keylessFixture {
	t.Helper()
	p := newPublishFixture(t, "")
	p.cmds = newCmdStubs(t)
	repo, plan := p.planAndBuild()
	if signing != "" {
		writeFile(t, p.git.Dir, "service.yaml", "name: widgets\narguments:\n  releaseOptions:\n    signing: "+signing+"\n")
	}
	return &keylessFixture{publishFixture: p, repo: repo, plan: plan}
}

// publish runs Publish with the log of the fixture.
func (k *keylessFixture) publish(dryRun bool) error {
	k.t.Helper()
	return Publish(context.Background(), k.repo, k.plan, &PublishOptions{
		Dist: "dist", Remote: k.remote, Forge: k.forge, Env: os.Environ(), Exec: k.exec(), Log: &k.log,
		DryRun: dryRun,
	})
}

// dist returns the path of name in the dist folder.
func (k *keylessFixture) dist(name string) string {
	return filepath.Join(k.git.Dir, "dist", name)
}

// expectSign adds the commands of a keyless signature: the CircleCI OIDC
// token, then cosign, which records the state of the release, checks
// the token file, prints stdout and stderr, and writes the bundle when
// it exits with 0.
func (k *keylessFixture) expectSign(stdout, stderr string, exitCode int) {
	k.cmds.expect(cmdOK(testOIDCToken+"\n"), "circleci", "run", "oidc", "get", "--aud", "sigstore", "--root-issuer")
	sums, bundle := k.dist("checksums.txt"), k.dist("checksums.txt.sigstore.json")
	k.cmds.expect(execstub.Result{
		Stdout: stdout, Stderr: stderr, ExitCode: exitCode,
		Func: func(ctx context.Context, cmd *exec.Cmd, _ io.Reader, _, _ io.Writer) error {
			out, _ := k.git.Run(ctx, "ls-remote", "origin", "refs/tags/"+k.plan.NextRelease.GitTag)
			tag, _, _ := strings.Cut(out, "\t")
			tokenFile := cmd.Args[7]
			k.signs = append(k.signs, signState{remoteTag: tag, tokenFile: tokenFile})
			if b, err := os.ReadFile(tokenFile); err != nil || string(b) != testOIDCToken {
				return errors.Join(fmt.Errorf("%w: %q", errBadToken, b), err)
			}
			if exitCode != 0 {
				return nil
			}
			return os.WriteFile(bundle, []byte(testBundle), 0o600)
		},
	}, "cosign", "sign-blob", "--yes", "--new-bundle-format", "--bundle", bundle, "--identity-token", execstub.AnyArg, sums)
}

// TestPublishKeyless checks that publish signs checksums.txt before the
// tag push, uploads the bundle with the release, removes the token file,
// and masks the token in the log.
func TestPublishKeyless(t *testing.T) {
	k := newKeylessFixture(t, "keyless")
	k.expectSign("Wrote bundle with token "+testOIDCToken+"\n", "", 0)
	assert.NilError(t, k.publish(false))

	assert.Equal(t, len(k.signs), 1)
	assert.Equal(t, k.signs[0].remoteTag, "", "cosign runs before the tag push")
	_, err := os.Stat(k.signs[0].tokenFile)
	assert.Assert(t, errors.Is(err, os.ErrNotExist), "the token file is removed: %v", err)
	assert.Equal(t, k.remoteRef("refs/tags/v1.0.0"), k.plan.Commit)

	rel := k.forge.Release("v1.0.0")
	assert.Assert(t, !rel.Draft)
	assert.DeepEqual(t, assetNames(rel),
		[]string{"widgets_1.0.0_linux_amd64.tar.gz", "checksums.txt", "checksums.txt.sigstore.json"})
	assert.Equal(t, string(k.forge.Uploaded["v1.0.0"]["checksums.txt.sigstore.json"]), testBundle)
	assert.Equal(t, k.forge.Calls[0], "Visibility")

	log := k.log.String()
	assert.Assert(t, !strings.Contains(log, testOIDCToken), log)
	assert.Assert(t, strings.Contains(log, "Wrote bundle with token [secure]\n"), log)
}

// TestPublishKeylessNotPublic checks that keyless signing in a repository
// that is not public stops before any change: the archives stay where
// the build put them, and no command, tag or release runs.
func TestPublishKeylessNotPublic(t *testing.T) {
	k := newKeylessFixture(t, "keyless")
	k.forge.RepoVisibility = "internal"
	err := k.publish(false)
	assert.ErrorIs(t, err, ErrNotPublic)
	assert.ErrorContains(t, err, "the repository is internal")

	_, err = os.Stat(k.dist("checksums.txt"))
	assert.Assert(t, errors.Is(err, os.ErrNotExist), "no checksums: %v", err)
	_, err = os.Stat(k.dist(filepath.Join("linux_amd64", "widgets_1.0.0_linux_amd64.tar.gz")))
	assert.NilError(t, err)
	assert.Equal(t, k.remoteRef("refs/tags/v1.0.0"), "")
	assert.DeepEqual(t, k.forge.Calls, []string{"Visibility", "Fail"})

	// A dry run stops too.
	assert.ErrorIs(t, k.publish(true), ErrNotPublic)
}

// TestPublishKeylessSignFails checks that a cosign failure stops the
// release before the tag push, with the token masked in the error.
func TestPublishKeylessSignFails(t *testing.T) {
	k := newKeylessFixture(t, "keyless")
	k.expectSign("", "error: getting signer: token "+testOIDCToken+" rejected", 1)
	err := k.publish(false)
	assertErrIs(t, err, errExit)
	assert.ErrorContains(t, err, "sign checksums.txt: ")
	assert.ErrorContains(t, err, "token [secure] rejected")
	assert.Assert(t, !strings.Contains(err.Error(), testOIDCToken))

	assert.Equal(t, k.remoteRef("refs/tags/v1.0.0"), "")
	assert.Assert(t, k.forge.Release("v1.0.0") == nil)
	_, err = os.Stat(filepath.Join(k.git.Dir, "hooks.log"))
	assert.Assert(t, errors.Is(err, os.ErrNotExist), "no hook ran: %v", err)
}

// TestPublishKeylessRerun checks that a rerun signs again only when the
// draft of the earlier run has no complete bundle.
func TestPublishKeylessRerun(t *testing.T) {
	t.Run("draft with the bundle", func(t *testing.T) {
		k := newKeylessFixture(t, "keyless")
		k.expectSign("", "", 0)
		k.forge.FailNext("PublishRelease", forge.ErrForge)
		assert.ErrorIs(t, k.publish(false), forge.ErrForge)
		assert.Assert(t, k.forge.Release("v1.0.0").Draft)

		// The rerun runs no command, and keeps the uploaded bundle.
		assert.NilError(t, k.publish(false))
		assert.Equal(t, len(k.signs), 1)
		rel := k.forge.Release("v1.0.0")
		assert.Assert(t, !rel.Draft)
		assert.DeepEqual(t, assetNames(rel),
			[]string{"widgets_1.0.0_linux_amd64.tar.gz", "checksums.txt", "checksums.txt.sigstore.json"})
		assert.Equal(t, countCalls(k.forge.Calls, "UploadAsset"), 3)
		assert.Assert(t, strings.Contains(k.log.String(), "Skip signing checksums.txt, as the release v1.0.0 has checksums.txt.sigstore.json\n"))
	})

	t.Run("draft without the bundle", func(t *testing.T) {
		k := newKeylessFixture(t, "keyless")
		k.expectSign("", "", 0)
		k.forge.FailNext("UploadAsset", forge.ErrForge)
		assert.ErrorIs(t, k.publish(false), forge.ErrForge)
		assert.Equal(t, len(k.forge.Release("v1.0.0").Assets), 0)

		// The rerun signs again, after the tag of the earlier run.
		k.expectSign("", "", 0)
		assert.NilError(t, k.publish(false))
		assert.Equal(t, len(k.signs), 2)
		assert.Equal(t, k.signs[1].remoteTag, k.plan.Commit)
		assert.DeepEqual(t, assetNames(k.forge.Release("v1.0.0")),
			[]string{"widgets_1.0.0_linux_amd64.tar.gz", "checksums.txt", "checksums.txt.sigstore.json"})
	})
}

// countCalls returns how many times calls has method.
func countCalls(calls []string, method string) int {
	n := 0
	for _, c := range calls {
		if c == method {
			n++
		}
	}
	return n
}

// TestPublishSigningNone checks that publish without keyless signing
// runs no command and does not read the repository visibility.
func TestPublishSigningNone(t *testing.T) {
	for _, signing := range []string{"", "none"} {
		t.Run("signing="+signing, func(t *testing.T) {
			k := newKeylessFixture(t, signing)
			assert.NilError(t, k.publish(false))
			assert.Assert(t, !slices.Contains(k.forge.Calls, "Visibility"), "calls: %v", k.forge.Calls)
			assert.DeepEqual(t, assetNames(k.forge.Release("v1.0.0")),
				[]string{"widgets_1.0.0_linux_amd64.tar.gz", "checksums.txt"})
			assert.Equal(t, len(k.cmds.calls()), 0)
		})
	}
}

// TestPublishSigningUnknown checks that an unknown signing setting stops
// publish before any change.
func TestPublishSigningUnknown(t *testing.T) {
	k := newKeylessFixture(t, "cosign")
	err := k.publish(false)
	assert.ErrorIs(t, err, ErrSigning)
	assert.ErrorContains(t, err, `"cosign", use "none" or "keyless"`)
	assert.DeepEqual(t, k.forge.Calls, []string{"Fail"})
	assert.Equal(t, k.remoteRef("refs/tags/v1.0.0"), "")
}

// TestPublishKeylessDryRun checks that a dry run checks the visibility
// but does not sign.
func TestPublishKeylessDryRun(t *testing.T) {
	k := newKeylessFixture(t, "keyless")
	assert.NilError(t, k.publish(true))
	assert.DeepEqual(t, k.forge.Calls, []string{"Visibility"})
	assert.Equal(t, len(k.cmds.calls()), 0)
	_, err := os.Stat(k.dist("checksums.txt.sigstore.json"))
	assert.Assert(t, errors.Is(err, os.ErrNotExist), "no bundle: %v", err)
}
