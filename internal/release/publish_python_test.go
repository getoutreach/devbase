// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the Python registry with a fake runner, and
// with the real poetry when it is installed.

package release

import (
	"context"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

// codeArtifactNotFound is the stderr of the AWS CLI for a missing
// package version.
const codeArtifactNotFound = "\nAn error occurred (ResourceNotFoundException) when calling the " +
	"DescribePackageVersion operation: Package version not found\n"

// pythonPyproject is a pyproject.toml whose name PEP 503 normalizes to
// "widgets-client".
const pythonPyproject = "[project]\nname = \"Widgets_Client\"\nversion = \"0.0.1\"\n"

// pythonEndpoint is the CodeArtifact endpoint of the fake runner.
const pythonEndpoint = "https://acme-123456789012.d.codeartifact.eu-west-1.amazonaws.com/pypi/acme-pypi/"

// pythonToken is the CodeArtifact token of the fake runner.
const pythonToken = "s3cr3t-codeartifact-token"

// pythonFixture returns a Python registry with a fake runner, the box
// account 123456789012, and a package in the default folder.
func pythonFixture(t *testing.T, replies map[string]fakeReply) (*pythonRegistry, *npmFixture) {
	t.Helper()
	f := newNPMFixture(t, "", "")
	writeFile(t, f.repo, filepath.Join(spec.DefaultPythonDir, "pyproject.toml"), pythonPyproject)
	f.run.replies = map[string]fakeReply{
		"mise which": {stdout: "poetry\n"},
		"aws codeartifact get-repository-endpoint": {stdout: pythonEndpoint + "\n"},
		"aws codeartifact get-authorization-token": {stdout: pythonToken + "\n"},
	}
	maps.Copy(f.run.replies, replies)
	py := &pythonRegistry{
		plugin:     spec.PythonPlugin{Dir: spec.DefaultPythonDir, Domain: "acme", Region: "eu-west-1", Repository: "acme-pypi"},
		run:        f.run.run,
		boxAccount: func() (string, error) { return "123456789012", nil },
	}
	return py, f
}

// codeArtifactArgs returns the argv of an aws codeartifact command for
// the domain of pythonFixture.
func codeArtifactArgs(op string, args ...string) []string {
	return append([]string{
		"aws", "codeartifact", op, "--domain", "acme", "--domain-owner", "123456789012", "--region", "eu-west-1",
	}, args...)
}

// describeArgs is the argv of the existence check of version.
func describeArgs(version string) []string {
	return codeArtifactArgs("describe-package-version", "--repository", "acme-pypi", "--format", "pypi",
		"--package", "widgets-client", "--package-version", version, "--query", "packageVersion.status", "--output", "text")
}

func TestPythonPrepare(t *testing.T) {
	py, f := pythonFixture(t, nil)
	assert.NilError(t, py.prepare(context.Background(), f.publisher("1.2.0-rc.1", "main")))
	assert.DeepEqual(t, f.run.argvs(), [][]string{
		{"aws", "sts", "get-caller-identity"},
		{"mise", "which", "poetry"},
		{"poetry", "version", "1.2.0-rc.1"},
	})
	pkgDir := filepath.Join(f.repo, filepath.FromSlash(spec.DefaultPythonDir))
	assert.Equal(t, f.run.calls[2].dir, pkgDir)
	assert.Assert(t, strings.Contains(f.log.String(), "Write version 1.2.0rc1 to pyproject.toml in "+pkgDir))

	// Without AWS credentials, prepare fails before poetry version.
	py, f = pythonFixture(t, map[string]fakeReply{"aws sts": {stderr: "Unable to locate credentials", err: errExit}})
	assert.ErrorContains(t, py.prepare(context.Background(), f.publisher("1.2.0", "")), "AWS credentials")
	assert.Equal(t, len(f.run.calls), 1)
}

func TestPythonPublish(t *testing.T) {
	py, f := pythonFixture(t, map[string]fakeReply{
		"aws codeartifact describe-package-version": {stderr: codeArtifactNotFound, err: errExit},
	})
	info, err := py.publish(context.Background(), f.publisher("1.2.0-rc.1", "main"))
	assert.NilError(t, err)
	assert.Assert(t, info == nil)
	assert.DeepEqual(t, f.run.argvs(), [][]string{
		describeArgs("1.2.0rc1"),
		codeArtifactArgs("get-repository-endpoint", "--repository", "acme-pypi", "--format", "pypi",
			"--query", "repositoryEndpoint", "--output", "text"),
		codeArtifactArgs("get-authorization-token", "--query", "authorizationToken", "--output", "text"),
		{"mise", "which", "poetry"},
		{"poetry", "build"},
		{"poetry", "publish", "--repository", "codeartifact"},
	})

	// Poetry gets the repository and the token in its environment, and
	// the log has no token.
	publish := f.run.calls[len(f.run.calls)-1]
	assert.Equal(t, publish.dir, filepath.Join(f.repo, filepath.FromSlash(spec.DefaultPythonDir)))
	assert.Equal(t, envValue(publish.env, "POETRY_REPOSITORIES_CODEARTIFACT_URL"), pythonEndpoint)
	assert.Equal(t, envValue(publish.env, "POETRY_HTTP_BASIC_CODEARTIFACT_USERNAME"), "aws")
	assert.Equal(t, envValue(publish.env, "POETRY_HTTP_BASIC_CODEARTIFACT_PASSWORD"), pythonToken)
	assert.Assert(t, !strings.Contains(f.log.String(), pythonToken), f.log.String())
	assert.Assert(t, !slices.ContainsFunc(f.env, func(kv string) bool { return strings.HasPrefix(kv, "POETRY_") }))
}

func TestPythonPublishSkipsExistingVersion(t *testing.T) {
	py, f := pythonFixture(t, map[string]fakeReply{
		"aws codeartifact describe-package-version": {stdout: "Published\n"},
	})
	info, err := py.publish(context.Background(), f.publisher("1.2.0", ""))
	assert.NilError(t, err)
	assert.Assert(t, info == nil)
	assert.DeepEqual(t, f.run.argvs(), [][]string{describeArgs("1.2.0")})
	assert.Assert(t, strings.Contains(f.log.String(), "Skip publishing to CodeArtifact acme-pypi as widgets-client 1.2.0 exists"))
}

func TestPythonExists(t *testing.T) {
	tests := []struct {
		name    string
		reply   fakeReply
		want    bool
		wantErr string
	}{
		{name: "published", reply: fakeReply{stdout: "Published\n"}, want: true},
		{name: "archived", reply: fakeReply{stdout: "Archived\n"}, want: true},
		{name: "missing", reply: fakeReply{stderr: codeArtifactNotFound, err: errExit}},
		{name: "access denied", reply: fakeReply{
			stderr: "An error occurred (AccessDeniedException) when calling the DescribePackageVersion operation", err: errExit,
		}, wantErr: "describe widgets-client 1.2.0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			py, f := pythonFixture(t, map[string]fakeReply{"aws codeartifact describe-package-version": tc.reply})
			got, err := py.exists(context.Background(), f.publisher("1.2.0", ""))
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}
}

func TestPythonAccount(t *testing.T) {
	// The plugin's account replaces the box config.
	py, f := pythonFixture(t, map[string]fakeReply{"aws codeartifact describe-package-version": {stdout: "Published\n"}})
	py.plugin.Account = "210987654321"
	py.boxAccount = func() (string, error) { return "", errExit }
	_, err := py.exists(context.Background(), f.publisher("1.2.0", ""))
	assert.NilError(t, err)
	assert.Assert(t, slices.Contains(f.run.calls[0].argv, "210987654321"))

	py.plugin.Account = ""
	py.boxAccount = func() (string, error) { return "", nil }
	_, err = py.exists(context.Background(), f.publisher("1.2.0", ""))
	assert.ErrorIs(t, err, errPythonNoAccount)
}

func TestPythonPackageErrors(t *testing.T) {
	py, f := pythonFixture(t, nil)
	writeFile(t, f.repo, filepath.Join(spec.DefaultPythonDir, "pyproject.toml"), "[tool.poetry]\nname = \"old\"\n")
	_, err := py.exists(context.Background(), f.publisher("1.2.0", ""))
	assert.ErrorIs(t, err, errPythonNoName)

	writeFile(t, f.repo, filepath.Join(spec.DefaultPythonDir, "pyproject.toml"), pythonPyproject)
	_, err = py.exists(context.Background(), f.publisher("1.2.0-foo.1", ""))
	assert.ErrorIs(t, err, spec.ErrUnsupported)
	assert.Equal(t, len(f.run.calls), 0)
}

func TestNormalizePythonName(t *testing.T) {
	for name, want := range map[string]string{
		"acme-client":    "acme-client",
		"Acme_Client":    "acme-client",
		"widgets.cli":    "widgets-cli",
		"a__b-._c":       "a-b-c",
		"Widgets_Client": "widgets-client",
	} {
		assert.Equal(t, normalizePythonName(name), want, name)
	}
}

// TestPythonRealPoetry runs poetry version and poetry build with the
// production runner, and fakes mise, aws and poetry publish. It is
// skipped without poetry.
func TestPythonRealPoetry(t *testing.T) {
	poetry, err := exec.LookPath("poetry")
	if err != nil {
		t.Skip("poetry is not installed")
	}
	py, f := pythonFixture(t, map[string]fakeReply{
		"aws codeartifact describe-package-version": {stderr: codeArtifactNotFound, err: errExit},
	})
	f.env = os.Environ()
	writeFile(t, f.repo, filepath.Join(spec.DefaultPythonDir, "src", "widgets_client", "__init__.py"), "")
	writeFile(t, f.repo, filepath.Join(spec.DefaultPythonDir, "pyproject.toml"), pythonPyproject+`requires-python = ">=3.9"

[tool.poetry]
packages = [{ include = "widgets_client", from = "src" }]

[build-system]
requires = ["poetry-core>=2.0.0,<3.0.0"]
build-backend = "poetry.core.masonry.api"
`)
	py.run = func(ctx context.Context, log io.Writer, c *command) (cmdResult, error) {
		if c.name == "poetry" && (c.args[0] == "version" || c.args[0] == "build") {
			realCmd := *c
			realCmd.name = poetry
			return execCommand(ctx, log, &realCmd)
		}
		return f.run.run(ctx, log, c)
	}
	p := f.publisher("1.2.0-rc.1", "main")
	assert.NilError(t, py.prepare(context.Background(), p))
	_, err = py.publish(context.Background(), p)
	assert.NilError(t, err)

	pkgDir := filepath.Join(f.repo, filepath.FromSlash(spec.DefaultPythonDir))
	b, err := os.ReadFile(filepath.Join(pkgDir, "pyproject.toml"))
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(b), `version = "1.2.0rc1"`), string(b))
	dist, err := filepath.Glob(filepath.Join(pkgDir, "dist", "*"))
	assert.NilError(t, err)
	for i := range dist {
		dist[i] = filepath.Base(dist[i])
	}
	assert.DeepEqual(t, dist, []string{"widgets_client-1.2.0rc1-py3-none-any.whl", "widgets_client-1.2.0rc1.tar.gz"})
}
