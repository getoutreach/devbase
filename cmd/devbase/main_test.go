// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Setup for the tests of the devbase command.

package main

import (
	"os"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/gittest"
)

// TestMain isolates the git commands of the tests from the git environment of the caller.
func TestMain(m *testing.M) {
	os.Exit(gittest.Run(m))
}
