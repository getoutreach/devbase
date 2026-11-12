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
	"strings"

	"github.com/getoutreach/devbase/v2/internal/release/goreleaser"
	"github.com/getoutreach/devbase/v2/internal/release/native"
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

	// Digest is the sha256 of the names and contents of the configuration
	// files as parsed: scripts/devbase.yaml, or .releaserc.yaml and
	// .goreleaser.yml.
	Digest string
}

// ErrConfigConflict is returned when scripts/devbase.yaml has a release
// section and .releaserc.yaml or .goreleaser.yml also exists.
var ErrConfigConflict = errors.New("two release configurations")

// LoadRepo reads the release configuration of the repository in dir: the
// release section of scripts/devbase.yaml when it has one, and else
// .releaserc.yaml, and .goreleaser.yml when .releaserc.yaml has the build
// hook. Each file is read once, and the digest uses the same bytes.
func LoadRepo(dir string) (*Repo, error) {
	r := &Repo{Git: &Git{Dir: dir}}
	h := sha256.New()
	var err error
	if r.Config, r.Warnings, err = loadNative(dir, h); err != nil {
		return nil, err
	}
	if r.Config == nil {
		if r.Config, r.Warnings, err = loadLegacy(dir, h); err != nil {
			return nil, err
		}
	}
	r.Digest = "sha256:" + hex.EncodeToString(h.Sum(nil))
	// Only the release section has units. SetUnits adds their defaults
	// and checks them with the files of the repository.
	if units, root := r.Config.Units, r.Config.RootPaths; len(units) > 0 || !root.IsZero() {
		r.Config.Units, r.Config.RootPaths = nil, spec.RootPaths{}
		if err := r.SetUnits(units, root); err != nil {
			return nil, fmt.Errorf("%s: %w", native.File, err)
		}
	}
	return r, nil
}

// loadNative reads the release section of scripts/devbase.yaml and adds
// the file to h. It returns a nil config, and leaves h as it is, when the
// file or the section does not exist. The section replaces the old files,
// so it is an error when one of them also exists.
func loadNative(dir string, h hash.Hash) (*spec.Config, []string, error) {
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(native.File)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", native.File, err)
	}
	cfg, warnings, err := native.Parse(b)
	if err != nil || cfg == nil {
		return nil, nil, err
	}
	var old []string
	for _, name := range []string{releaserc.File, goreleaser.File} {
		ok, err := exists(dir, name)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			old = append(old, name)
		}
	}
	if len(old) > 0 {
		return nil, nil, fmt.Errorf("%w: %s has a %s section, so delete %s",
			ErrConfigConflict, native.File, native.Key, strings.Join(old, " and "))
	}
	h.Write([]byte(native.File))
	h.Write(b)
	return cfg, warnings, nil
}

// loadLegacy reads .releaserc.yaml, and .goreleaser.yml when it has the
// build hook, and adds the files that it reads to h.
func loadLegacy(dir string, h hash.Hash) (*spec.Config, []string, error) {
	b, err := readConfig(dir, releaserc.File, h)
	if err != nil {
		return nil, nil, err
	}
	cfg, warnings, err := releaserc.Parse(b)
	if err != nil {
		return nil, nil, err
	}
	// The CodeArtifact repository of the Python client hooks comes from
	// the environment, so a plan for another repository does not publish.
	for _, pl := range cfg.Plugins {
		if pl.Python != nil {
			fmt.Fprintf(h, "\x00python\x00%s\x00%s\x00%s", pl.Python.Domain, pl.Python.Repository, pl.Python.Region)
		}
	}
	// Only a config that builds reads .goreleaser.yml, as semantic-release
	// runs GoReleaser only from the build hook.
	if cfg.Build {
		if b, err = readConfig(dir, goreleaser.File, h); err != nil {
			return nil, nil, err
		}
		if cfg.Artifacts, err = goreleaser.Parse(b); err != nil {
			return nil, nil, err
		}
	}
	return cfg, warnings, nil
}

// ConvertConfig reads .releaserc.yaml, and .goreleaser.yml when it has
// the build hook, in dir, and returns the scripts/devbase.yaml document
// with the equivalent release section. It also returns notes for the
// user: the warnings of the configuration, and a .goreleaser.yml that the
// section leaves out.
func ConvertConfig(dir string) (section []byte, notes []string, err error) {
	cfg, notes, err := loadLegacy(dir, sha256.New())
	if err != nil {
		return nil, nil, err
	}
	if !cfg.Build {
		ok, err := exists(dir, goreleaser.File)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			notes = append(notes, fmt.Sprintf("%s has no build hook, so the section has no build key and "+
				"the unstable release builds nothing; add a build key to build both releases", releaserc.File))
		}
	}
	if section, err = native.Marshal(cfg); err != nil {
		return nil, nil, err
	}
	return section, notes, nil
}

// exists reports whether the file name exists in dir.
func exists(dir, name string) (bool, error) {
	_, err := os.Lstat(filepath.Join(dir, name))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	}
	return false, fmt.Errorf("check %s: %w", name, err)
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
