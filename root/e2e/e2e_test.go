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

func TestGetE2eTestPathsOnePackageDir(t *testing.T) {
	walker := func(path string, walk filepath.WalkFunc) error {
		walk(path, StubFileInfo{FileName: path, IsDirectory: true}, nil)
		return nil
	}

	dirReader := func(name string) ([]os.DirEntry, error) {
		assert.Equal(t, name, "dir")
		return []os.DirEntry{StubFileInfo{FileName: "e2e_test.go", IsDirectory: false}}, nil
	}

	fileReader := func(name string) ([]byte, error) {
		assert.Equal(t, name, "dir/e2e_test.go")
		fileContents := `//go:build or_e2e

func TestPingPong(t *testing.T) {
}`
		return []byte(fileContents), nil
	}

	dir, _ := GetE2eTestPaths("dir", walker, dirReader, fileReader)
	assert.Equal(t, len(dir), 1)
	assert.Equal(t, dir[0], "dir")
}

func TestGetE2eTestPathsErrorOpening(t *testing.T) {
	walker := func(path string, walk filepath.WalkFunc) error {
		return errors.New("Error opening dir")
	}

	_, err := GetE2eTestPaths(".", walker, nofilesDirReader, emptyFileReader)

	assert.Equal(t, "Error opening dir", err.Error())
}

func TestGetE2eTestPathsSkippedFilesAndDirs(t *testing.T) {
	walker := func(path string, walk filepath.WalkFunc) error {
		walk("file.txt", StubFileInfo{FileName: "file.txt", IsDirectory: false}, nil)
		walk(".hidden", StubFileInfo{FileName: ".hidden", IsDirectory: true}, nil)
		walk(".hidden/e2e_test.go", StubFileInfo{FileName: "e2e_test.go", IsDirectory: true}, nil)
		return nil
	}

	dirReader := func(name string) ([]os.DirEntry, error) {
		t.Error("Dir reader should not be called")
		t.FailNow()
		return []os.DirEntry{}, nil
	}

	fileReader := func(name string) ([]byte, error) {
		t.Error("File reader should not be called")
		t.FailNow()
		return []byte{}, nil
	}

	dir, err := GetE2eTestPaths("dir", walker, dirReader, fileReader)
	assert.Equal(t, 0, len(dir))
	assert.Equal(t, nil, err)
}

func TestGetE2eTestPathsOneFileInDir(t *testing.T) {
	walker := func(path string, walk filepath.WalkFunc) error {
		file := "file.txt"
		walk(file, StubFileInfo{FileName: file, IsDirectory: false}, nil)
		return nil
	}

	dir, err := GetE2eTestPaths("dir", walker, nofilesDirReader, emptyFileReader)
	assert.Equal(t, 0, len(dir))
	assert.Equal(t, nil, err)
}
