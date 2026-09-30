//go:build mage

package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/getoutreach/devbase/v2/root/e2e"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	logger "github.com/rs/zerolog/log"
)

// log is the logger used by this magefile
var log = logger.Output(zerolog.ConsoleWriter{Out: os.Stderr})

// Dep installs all the dependencies needed to run the project.
func Dep() error {
	if err := runGoCommand(log, "mod", "download", "-x"); err != nil {
		return err
	}

	return runGoCommand(log, "mod", "tidy")
}

// E2ETestBuild builds binaries of e2e tests
func E2ETestBuild(ctx context.Context) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	binDir, err := ensureBinDirExists(cwd)
	if err != nil {
		return err
	}

	e2ePackages, err := e2e.GetE2eTestPaths(".", filepath.Walk, os.ReadDir, os.ReadFile)
	if err != nil {
		return errors.Wrap(err, "Error when searching e2e test packages")
	}

	if err := e2e.BuildE2ETestPackages(log, e2ePackages, binDir, runGoCommand); err != nil {
		return errors.Wrap(err, "Unable to build e2e test package")
	}

	return nil
}

func ensureBinDirExists(cwd string) (string, error) {
	binDir := filepath.Join(cwd, "bin")
	if _, err := os.Stat(binDir); os.IsNotExist(err) {
		if err := os.Mkdir(binDir, 0o755); err != nil {
			return "", errors.Wrapf(err, "failed to mkdir %s", binDir)
		}
	}
	return binDir, nil
}
