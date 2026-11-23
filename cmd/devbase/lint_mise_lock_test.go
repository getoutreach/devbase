// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for the lint mise-lock subcommand.

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

// writeMiseEnv writes mise.<env>.toml and mise.<env>.lock to dir, and
// returns their paths.
func writeMiseEnv(t *testing.T, dir, env, config, lock string) (configPath, lockPath string) {
	t.Helper()
	configPath = filepath.Join(dir, "mise."+env+".toml")
	lockPath = filepath.Join(dir, "mise."+env+".lock")
	assert.NilError(t, os.WriteFile(configPath, []byte(config), 0o600))
	assert.NilError(t, os.WriteFile(lockPath, []byte(lock), 0o600))
	return configPath, lockPath
}

// TestLintMiseLock confirms that "lint mise-lock" checks every config
// it is given, prints each problem, and exits 1 only if there are any.
func TestLintMiseLock(t *testing.T) {
	t.Run("prints the problems in every config and exits 1", func(t *testing.T) {
		dir := t.TempDir()
		devbaseConfig, devbaseLock := writeMiseEnv(t, dir, "devbase",
			"[tools]\ngojq = \"0.12.18\"\n",
			"[[tools.gojq]]\nversion = \"0.12.17\"\nbackend = \"go:github.com/itchyny/gojq/cmd/gojq\"\n")
		e2eConfig, e2eLock := writeMiseEnv(t, dir, "e2e",
			"[tools]\n",
			"[[tools.kubectl]]\nversion = \"1.34.7\"\nbackend = \"go:k8s.io/kubectl\"\n")

		stdout, _, err := runSubcommand(t, newLintMiseLockCommand(), "", devbaseConfig, e2eConfig)
		assert.Equal(t, exitCode(t, err), 1)
		assert.Equal(t, stdout,
			devbaseLock+": gojq@0.12.18 is requested in "+devbaseConfig+" but not locked\n"+
				devbaseLock+": gojq@0.12.17 is locked but matches no version requested in "+devbaseConfig+"\n"+
				e2eLock+": kubectl is locked but not requested in "+e2eConfig+"\n")
	})

	t.Run("prints nothing and succeeds when every lockfile matches", func(t *testing.T) {
		config, _ := writeMiseEnv(t, t.TempDir(), "devbase",
			"[tools]\ngojq = \"0.12.18\"\n",
			"[[tools.gojq]]\nversion = \"0.12.18\"\nbackend = \"go:github.com/itchyny/gojq/cmd/gojq\"\n")

		stdout, _, err := runSubcommand(t, newLintMiseLockCommand(), "", config)
		assert.NilError(t, err)
		assert.Equal(t, stdout, "")
	})

	t.Run("fails without a config", func(t *testing.T) {
		_, _, err := runSubcommand(t, newLintMiseLockCommand(), "")
		assert.ErrorIs(t, err, errMiseLockNoConfigs)
	})

	t.Run("fails when a lockfile is missing", func(t *testing.T) {
		config := filepath.Join(t.TempDir(), "mise.devbase.toml")
		assert.NilError(t, os.WriteFile(config, []byte("[tools]\n"), 0o600))

		_, _, err := runSubcommand(t, newLintMiseLockCommand(), "", config)
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})
}
