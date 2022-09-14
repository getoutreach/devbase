//go:build mage

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	logger "github.com/rs/zerolog/log"
)

// Dep installs all the dependencies needed to run the project.
func Dep() error {
	if err := runGoCommand("mod", "download", "-x"); err != nil {
		return err
	}

	return runGoCommand("mod", "tidy")
}

// Version prints the current application version
func Version() {
	fmt.Println(getAppVersion())
}

// GoBuild builds a Go project
func Gobuild(ctx context.Context) error {
	log := logger.Output(zerolog.ConsoleWriter{Out: os.Stderr})
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

	buildDir := filepath.Join(cwd, "bin")
	if _, err := os.Stat(buildDir); os.IsNotExist(err) {
		if err := os.Mkdir(buildDir, 0o755); err != nil {
			return errors.Wrapf(err, "failed to mkdir %s", buildDir)
		}
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

	return runGoCommand("build", "-v", "-o", buildDir, "-ldflags", ldFlags, buildPath+"/...")
}
