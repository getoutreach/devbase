// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Isolation of the git commands of tests from the git environment of the caller.

// Package gittest isolates the git commands that tests run from the git environment of the
// caller, such as the temporary index of a pre-commit hook, from the git configuration of the
// user and the system, and from a git repository that contains the temporary directory.
package gittest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Run calls IsolateEnv, then runs the tests of m, and returns the exit code for os.Exit. Call it
// in TestMain, so that the isolation also applies to the git commands of the code under test.
func Run(m *testing.M) int {
	if err := IsolateEnv(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	return m.Run()
}

// IsolateEnv changes the environment of the process, so that each git command uses the
// repository of its directory, no configuration file of the user or the system, and no
// configuration from the git command line of the caller. A git command below os.TempDir does
// not look for a repository in os.TempDir or above it, so a test directory is never part of a
// repository that contains the temporary directory. It must run before any test starts.
func IsolateEnv() error {
	ceiling, err := ceilingDirectories()
	if err != nil {
		return err
	}

	for _, name := range []string{"GIT_DIR", "GIT_INDEX_FILE", "GIT_WORK_TREE", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT"} {
		if err := os.Unsetenv(name); err != nil {
			return fmt.Errorf("unset %s: %w", name, err)
		}
	}

	for name, value := range map[string]string{
		"GIT_CONFIG_GLOBAL":       os.DevNull,
		"GIT_CONFIG_NOSYSTEM":     "1",
		"GIT_CEILING_DIRECTORIES": ceiling,
	} {
		if err := os.Setenv(name, value); err != nil {
			return fmt.Errorf("set %s: %w", name, err)
		}
	}

	return nil
}

// ceilingDirectories returns the value of GIT_CEILING_DIRECTORIES: os.TempDir, and its real path
// when it differs. git does not resolve the symbolic links of the ceiling directories, so a
// temporary directory that is a symbolic link, as on macOS, needs both paths.
func ceilingDirectories() (string, error) {
	dir := os.TempDir()
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("resolve the temporary directory %s: %w", dir, err)
	}

	if realDir == filepath.Clean(dir) {
		return dir, nil
	}

	return dir + string(os.PathListSeparator) + realDir, nil
}
