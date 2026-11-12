// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the release command setup.

package main

import (
	"context"
	"errors"
	"fmt"
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
