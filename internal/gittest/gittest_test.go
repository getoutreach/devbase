// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the isolation of test git commands from the git environment of the caller.

package gittest_test

import (
	"os"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/gittest"
	"gotest.tools/v3/assert"
)

func TestIsolateEnv(t *testing.T) {
	removed := []string{"GIT_DIR", "GIT_INDEX_FILE", "GIT_WORK_TREE", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT"}
	for _, name := range removed {
		t.Setenv(name, "from-the-caller")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "from-the-caller")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "")
	t.Setenv("GIT_CEILING_DIRECTORIES", "from-the-caller")

	assert.NilError(t, gittest.IsolateEnv())

	for _, name := range removed {
		_, ok := os.LookupEnv(name)
		assert.Assert(t, !ok, "%s is set", name)
	}
	assert.Equal(t, os.Getenv("GIT_CONFIG_GLOBAL"), os.DevNull)
	assert.Equal(t, os.Getenv("GIT_CONFIG_NOSYSTEM"), "1")
	assert.Equal(t, os.Getenv("GIT_CEILING_DIRECTORIES"), os.TempDir())
}
