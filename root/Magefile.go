//go:build mage

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// E2etestbuild builds binaries of e2e tests
func E2etestbuild(ctx context.Context) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	buildDir, err := ensureBinExists(cwd)
	if err != nil {
		return err
	}

	e2ePackages, err := e2e.GetE2eTestPaths(".", filepath.Walk, os.ReadDir, os.ReadFile)
	if err != nil {
		return errors.Wrap(err, "Unable to find e2e packages")
	}

	for _, e2ePackage := range e2ePackages {
		binaryName := "e2e_" + strings.Replace(e2ePackage, "/", "_", -1)
		binaryPath := filepath.Join(buildDir, binaryName)
		log.Info().Msgf("Building e2e test package %s to bin dir. Name %s", e2ePackage, binaryName)
		if err := runGoCommand(log, "test", "-tags", "or_test,or_e2e", "-c", "-o", binaryPath, "./"+e2ePackage, "-ldflags",
			"-X github.com/getoutreach/go-outreach/v2/pkg/app.Version=testing -X github.com/getoutreach/gobox/pkg/app.Version=testing"); err != nil {
			return errors.Wrap(err, "Unable to build e2e test package")
		}
	}

	return nil
}

func ensureBinExists(cwd string) (string, error) {
	buildDir := filepath.Join(cwd, "bin")
	if _, err := os.Stat(buildDir); os.IsNotExist(err) {
		if err := os.Mkdir(buildDir, 0o755); err != nil {
			return "", errors.Wrapf(err, "failed to mkdir %s", buildDir)
		}
	}
	return buildDir, nil
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
	buildDir, err := ensureBinExists(cwd)
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

	args := []string{"build", "-v", "-o", buildDir, "-ldflags", ldFlags}

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
