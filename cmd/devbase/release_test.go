// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the release command setup.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/rlog"
	"github.com/urfave/cli/v3"
	"gotest.tools/v3/assert"
)

// errTest is the error that the test action wraps.
var errTest = errors.New("push failed")

func TestMaskErrors(t *testing.T) {
	m := rlog.NewMasker([]string{"GH_TOKEN=ghp_secret"})
	action := maskErrors(&m, func(context.Context, *cli.Command) error {
		return fmt.Errorf("git push https://ghp_secret@github.com/o/r: %w", errTest)
	})
	err := action(context.Background(), &cli.Command{})
	assert.Equal(t, err.Error(), "git push https://[secure]@github.com/o/r: push failed")
	assert.ErrorIs(t, err, errTest)

	ok := maskErrors(&m, func(context.Context, *cli.Command) error { return nil })
	assert.NilError(t, ok(context.Background(), &cli.Command{}))
}

// TestReleaseConfigConvert runs "release config convert" in a repository
// with a .releaserc.yaml, and "release config", which has no action.
func TestReleaseConfigConvert(t *testing.T) {
	dir := t.TempDir()
	rc := "preset: conventionalcommits\nbranches: [main]\nplugins:\n" +
		"  - \"@semantic-release/release-notes-generator\"\n  - \"@semantic-release/github\"\n"
	assert.NilError(t, os.WriteFile(filepath.Join(dir, ".releaserc.yaml"), []byte(rc), 0o600))
	t.Chdir(dir)

	var out bytes.Buffer
	root := &cli.Command{Name: "devbase", Commands: []*cli.Command{newReleaseCommand()}, Writer: &out, ErrWriter: io.Discard}
	assert.NilError(t, root.Run(context.Background(), []string{"devbase", "release", "config", "convert"}))
	// The rules are the default rules of semantic-release, not of the section.
	assert.Equal(t, out.String(), "release:\n  branches:\n    - name: main\n  allowMajorVersions: true\n  rules: []\n")

	out.Reset()
	assert.NilError(t, root.Run(context.Background(), []string{"devbase", "release", "config"}))
	assert.Assert(t, strings.Contains(out.String(), "convert"), out.String())
}
