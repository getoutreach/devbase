// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Loads a repository's release configuration files into the
// release model. This is the only place that knows the file formats.

package release

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/getoutreach/devbase/v2/internal/release/goreleaser"
	"github.com/getoutreach/devbase/v2/internal/release/releaserc"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
)

// Repo is a repository with its release configuration.
type Repo struct {
	Git *Git

	// Config is the release model of the repository.
	Config *spec.Config

	// Warnings are config problems that do not stop a release.
	Warnings []string

	// Digest is the sha256 of the configuration files as parsed.
	Digest string
}

// LoadRepo reads the release configuration of the repository in dir:
// .releaserc.yaml, and .goreleaser.yml when .releaserc.yaml has the build
// hook. Each file is read once, and the digest uses the same bytes.
func LoadRepo(dir string) (*Repo, error) {
	r := &Repo{Git: &Git{Dir: dir}}
	h := sha256.New()
	b, err := readConfig(dir, releaserc.File, h)
	if err != nil {
		return nil, err
	}
	if r.Config, r.Warnings, err = releaserc.Parse(b); err != nil {
		return nil, err
	}
	// The CodeArtifact repository of the Python client hooks comes from
	// the environment, so a plan for another repository does not publish.
	for _, pl := range r.Config.Plugins {
		if pl.Python != nil {
			fmt.Fprintf(h, "\x00python\x00%s\x00%s\x00%s", pl.Python.Domain, pl.Python.Repository, pl.Python.Region)
		}
	}
	// Only a config that builds reads .goreleaser.yml, as semantic-release
	// runs GoReleaser only from the build hook.
	if r.Config.Build {
		if b, err = readConfig(dir, goreleaser.File, h); err != nil {
			return nil, err
		}
		if r.Config.Artifacts, err = goreleaser.Parse(b); err != nil {
			return nil, err
		}
	}
	r.Digest = "sha256:" + hex.EncodeToString(h.Sum(nil))
	return r, nil
}

// readConfig reads a config file and adds its name and contents to h.
func readConfig(dir, name string, h hash.Hash) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	h.Write([]byte(name))
	h.Write(b)
	return b, nil
}

// unstableArtifacts returns the artifacts of the unstable release: the
// release artifacts, or those of .goreleaser.yml when the release does
// not build, as pre-release.sh reads it. It returns nil when the
// repository has no .goreleaser.yml.
func (r *Repo) unstableArtifacts() (*spec.Artifacts, error) {
	if r.Config.Artifacts != nil {
		return r.Config.Artifacts, nil
	}
	b, err := os.ReadFile(filepath.Join(r.Git.Dir, goreleaser.File))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil //nolint:nilnil // Why: no .goreleaser.yml means no artifacts.
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", goreleaser.File, err)
	}
	return goreleaser.Parse(b)
}
