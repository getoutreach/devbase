// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Reads the release options of service.yaml.

package release

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// serviceOptions are the arguments.releaseOptions of service.yaml that
// the release commands use.
type serviceOptions struct {
	// EnablePrereleases turns on the unstable and RC releases.
	EnablePrereleases bool `yaml:"enablePrereleases"`

	// PrereleasesBranch is the branch of the unstable release.
	PrereleasesBranch string `yaml:"prereleasesBranch"`
}

// loadServiceOptions reads the release options of service.yaml in dir. A
// repository without service.yaml has none.
func loadServiceOptions(dir string) (*serviceOptions, error) {
	b, err := os.ReadFile(filepath.Join(dir, "service.yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		return &serviceOptions{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read service.yaml: %w", err)
	}
	var svc struct {
		Arguments struct {
			ReleaseOptions serviceOptions `yaml:"releaseOptions"`
		} `yaml:"arguments"`
	}
	if err := yaml.Unmarshal(b, &svc); err != nil {
		return nil, fmt.Errorf("parse service.yaml: %w", err)
	}
	return &svc.Arguments.ReleaseOptions, nil
}
