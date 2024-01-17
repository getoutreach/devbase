//go:build mage

package main

import (
	"context"
	"fmt"
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

// Version prints the current application version
func Version() {
	fmt.Println(getAppVersion())
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

// GoBuild builds a Go project
func Gobuild(ctx context.Context) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// TODO(jaredallard)[DT-2796]: This is a hack to get around the fact that plugins
	// still don't implement the commands framework. Can remove when DT-2796 is done.
	_, cmdErr := os.Stat("cmd")
	_, pluginDirErr := os.Stat("plugin")
	if cmdErr != nil && pluginDirErr != nil {
		log.Warn().Msg("This repository produces no artifacts (no 'cmd' or 'plugin' directory found)")
		return nil
	}
	binDir, err := ensureBinDirExists(cwd)
	if err != nil {
		return err
	}

	honeycombKey, err := readSecret(ctx, "honeycomb/apiKey")
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get honeycomb api key (did you run .bootstrap/shell/devconfig.sh?)")
	}

	teleforkKey, err := readSecret(ctx, "telefork/api-keys/default")
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get telefork api key (did you run .bootstrap/shell/devconfig.sh?)")
	}

	ldFlags := getLDFlagsStringFromMap(map[string]string{
		"github.com/getoutreach/gobox/pkg/app.Version": getAppVersion(),
		"main.HoneycombTracingKey":                     string(honeycombKey),
		"main.TeleforkAPIKey":                          string(teleforkKey),
	})
	if os.Getenv("DLV_PORT") == "" {
		// When not running in DLV, strip out symbols
		ldFlags += "-w -s"
	}

	log.Info().Msg("Building...")

	// TODO(jaredallard)[DT-2796]: This is a hack to get around the fact that plugins
	// still don't implement the commands framework. Can remove when DT-2796 is done.
	buildPath := "./cmd"
	if pluginDirErr == nil {
		buildPath = "./plugin"
	}

	args := []string{"build", "-v", "-o", binDir, "-ldflags", ldFlags}
	if gcFlags := os.Getenv("GC_FLAGS"); gcFlags != "" {
		args = append(args, "-gcflags", gcFlags)
	}

	// SKIP_TRIMPATH is used for devspace binary sync, where you want to have same file paths for delve to work correctly
	if os.Getenv("SKIP_TRIMPATH") == "true" {
		log.Debug().Msg("Skipping trimpath argument for go build")
	} else {
		// Build with -trimpath to ensure we have consistent module filenames embedded.
		args = append(args, "-trimpath")
	}

	args = append(args, buildPath+"/...")

	return runGoCommand(log, args...)
}
