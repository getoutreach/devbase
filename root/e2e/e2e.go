// Copyright 2023 Outreach Corporation. All Rights Reserved.

// Description: This file implements helpers for e2e test discovery.

// Package e2e contains e2e test related build logic
package e2e

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
)

// DirectoryWalker abstracts filepath.Walk
type DirectoryWalker = func(string, filepath.WalkFunc) error

// DirectoryReader abstracts os.ReadDir
type DirectoryReader = func(name string) ([]os.DirEntry, error)

// FileReader abstracts os.ReadFile
type FileReader = func(name string) ([]byte, error)

// GetE2eTestPaths returns list of paths of packages that contain at least one go file with or_e2e in it
func GetE2eTestPaths(rootDir string, walk DirectoryWalker, readDir DirectoryReader, readFile FileReader) ([]string, error) {
	e2ePackages := make([]string, 0)
	err := walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories/files that are not relevant
		if !info.IsDir() {
			return nil
		}

		// ignore hidden (sub)directories
		if strings.HasPrefix(path, ".") || strings.Contains(path, "/.") {
			return nil
		}

		files, err := readDir(path)
		if err != nil {
			return err
		}

		for _, file := range files {
			if file.IsDir() {
				continue // Skip subdirectories
			}

			if filepath.Ext(file.Name()) == ".go" {
				filePath := filepath.Join(path, file.Name())
				contentBytes, err := readFile(filePath)
				if err != nil {
					return err
				}
				content := string(contentBytes)

				// We care for packages that has at least one test entrypoint in file with or_e2e tag
				if strings.Contains(content, "or_e2e") && strings.Contains(content, "func Test") {
					e2ePackages = append(e2ePackages, path)
					break // Exit the loop since we've found a matching file
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return e2ePackages, nil
}

// createFileNameFromPackagePath creates binary name for e2e test package
func createFileNameFromPackagePath(path string) string {
	prefix := "e2e"
	separator := "_"
	pathWithoutSlashes := strings.ReplaceAll(path, "/", separator)

	return prefix + separator + pathWithoutSlashes
}

// RunGoCommand abstracts function that invokes go cmd
type RunGoCommand = func(log zerolog.Logger, args ...string) error

// BuildE2ETestPackages buils e2e packages for given package paths
// nolint:gocritic // Why: hugeParam: 89 bytes is not "huge"
func BuildE2ETestPackages(log zerolog.Logger, packagePaths []string, binDir string, runGoCommand RunGoCommand) error {
	for _, e2ePackage := range packagePaths {
		binaryName := createFileNameFromPackagePath(e2ePackage)
		binaryPath := filepath.Join(binDir, binaryName)
		log.Info().Msgf("Building e2e test package %s to bin dir. Name %s", e2ePackage, binaryName)
		if err := runGoCommand(log, "test", "-tags", "or_test,or_e2e", "-c", "-o", binaryPath, "./"+e2ePackage, "-ldflags",
			"-X github.com/getoutreach/go-outreach/v2/pkg/app.Version=testing -X github.com/getoutreach/gobox/pkg/app.Version=testing"); err != nil {
			return err
		}
	}
	return nil
}
