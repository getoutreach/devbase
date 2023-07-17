package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func nofilesDirReader(name string) ([]os.DirEntry, error) {
	return []os.DirEntry{}, nil
}

func emptyFileReader(name string) ([]byte, error) {
	return []byte{}, nil
}

func TestWalkerError(t *testing.T) {
	walker := func(path string, walk filepath.WalkFunc) error {
		return errors.New("Error opening dir")
	}

	_, err := GetE2eTestPaths(".", walker, nofilesDirReader, emptyFileReader)

	assert.Equal("Error opening dir", err.Error())
}

// func TestDummy(t *testing.T) {

// 	walker := func(path string, walk filepath.WalkFunc) error {
// 		assert.Equal(".", path)
// 		walk(".git")
// 		walk("./git/subdir")
// 		walk("subdir/.hidden")
// 		return nil
// 	}

// 	GetE2eTestPaths(".", walker)
// }
