//go:build mage

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/getoutreach/gobox/pkg/cfg"
	"github.com/magefile/mage/sh"
)

// getAppVersion returns the current application version, or pseudo-version if
// not aligned with a tag
func getAppVersion() string {
	version, err := sh.Output("git", "describe", "--match", "v[0-9]*", "--tags", "--always", "HEAD")
	if err != nil {
		return "0.0.0-dev"
	}

	return version
}

// getAppName returns the app name
func getAppName() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return filepath.Base(cwd)
}

// readSecret reads a secret from well-defined paths on the user's machine
func readSecret(ctx context.Context, path string) (cfg.SecretData, error) {
	appName := getAppName()
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	lookupPaths := []string{
		"/run/secrets/outreach.io",
		filepath.Join(homeDir, ".outreach", appName),
	}
	for _, p := range lookupPaths {
		secretPath := filepath.Join(p, path)
		if _, err := os.Stat(secretPath); err == nil {
			return cfg.Secret{Path: secretPath}.Data(ctx)
		}
	}

	return "", fmt.Errorf("failed to find secret at any of %v", lookupPaths)
}
