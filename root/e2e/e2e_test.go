package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

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

func TestGetE2eTestPathsErrorOpening(t *testing.T) {
	walker := func(path string, walk filepath.WalkFunc) error {
		return errors.New("Error opening dir")
	}

	_, err := GetE2eTestPaths(".", walker, nofilesDirReader, emptyFileReader)

	assert.Equal(t, "Error opening dir", err.Error())
}

func nofilesDirReader(name string) ([]os.DirEntry, error) {
	return []os.DirEntry{}, nil
}

func emptyFileReader(name string) ([]byte, error) {
	return []byte{}, nil
}

func TestBuildE2ETestPackages(t *testing.T) {
	called := false
	runGoCommand := func(log zerolog.Logger, args ...string) error {
		called = true
		expectedBinary := "bin/e2e_internal_e2e_prospects"
		expectedPackage := "./internal/e2e/prospects"
		assert.ElementsMatch(t, args, []string{"test", "-tags", "or_test,or_e2e", "-c", "-o", expectedBinary, expectedPackage, "-ldflags",
			"-X github.com/getoutreach/go-outreach/v2/pkg/app.Version=testing -X github.com/getoutreach/gobox/pkg/app.Version=testing"})
		return nil
	}

	err := BuildE2ETestPackages(zerolog.Logger{}, []string{"internal/e2e/prospects"}, "./bin", runGoCommand)
	assert.Equal(t, err, nil)
	assert.Equal(t, called, true)
}
