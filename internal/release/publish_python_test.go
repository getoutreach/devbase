// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the Python registry with stubs of the
// commands, and with the real poetry when it is installed.

package release

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"github.com/getoutreach/gobox/pkg/exec"
	"github.com/getoutreach/gobox/pkg/exec/execstub"
	"gotest.tools/v3/assert"
)

// codeArtifactNotFound is the stderr of the AWS CLI for a missing
// package version.
const codeArtifactNotFound = "\nAn error occurred (ResourceNotFoundException) when calling the " +
	"DescribePackageVersion operation: Package version not found\n"

// pythonPyproject is a pyproject.toml whose name PEP 503 normalizes to
// "widgets-client".
const pythonPyproject = "[project]\nname = \"Widgets_Client\"\nversion = \"0.0.1\"\n"

// pythonEndpoint is the CodeArtifact endpoint of the stubs.
const pythonEndpoint = "https://acme-123456789012.d.codeartifact.eu-west-1.amazonaws.com/pypi/acme-pypi/"

// pythonToken is the CodeArtifact token of the stubs.
const pythonToken = "s3cr3t-codeartifact-token"

// pythonFixture returns a Python registry with the box account
// 123456789012, and a fixture with a package in the default folder.
func pythonFixture(t *testing.T) (*pythonRegistry, *npmFixture) {
	t.Helper()
	f := newNPMFixture(t, "", "")
	writeFile(t, f.repo, filepath.Join(spec.DefaultPythonDir, "pyproject.toml"), pythonPyproject)
	py := &pythonRegistry{
		plugin:     spec.PythonPlugin{Dir: spec.DefaultPythonDir, Domain: "acme", Region: "eu-west-1", Repository: "acme-pypi"},
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

// These are the commands that find poetry, and that get the repository
// and the token of poetry publish.
var (
	misePoetry       = stubCmd{[]string{"mise", "which", "poetry"}, cmdOK("poetry\n")}
	pythonPublishEnv = []stubCmd{
		{codeArtifactArgs("get-repository-endpoint", "--repository", "acme-pypi", "--format", "pypi",
			"--query", "repositoryEndpoint", "--output", "text"), cmdOK(pythonEndpoint + "\n")},
		{codeArtifactArgs("get-authorization-token", "--query", "authorizationToken", "--output", "text"), cmdOK(pythonToken + "\n")},
	}
	awsIdentity = []string{"aws", "sts", "get-caller-identity"}
)

func TestPythonPrepare(t *testing.T) {
	py, f := pythonFixture(t)
	f.cmds.expect(cmdOK("{}\n"), awsIdentity...)
	f.cmds.expectAll([]stubCmd{misePoetry, {[]string{"poetry", "version", "1.2.0-rc.1"}, cmdOK("")}})
	assert.NilError(t, py.prepare(context.Background(), f.publisher("1.2.0-rc.1", "main")))
	pkgDir := filepath.Join(f.repo, filepath.FromSlash(spec.DefaultPythonDir))
	assert.Equal(t, f.cmds.calls()[2].Cmd.Dir, pkgDir)
	assert.Assert(t, strings.Contains(f.log.String(), "Write version 1.2.0rc1 to pyproject.toml in "+pkgDir))

	// Without AWS credentials, prepare fails before poetry version.
	py, f = pythonFixture(t)
	f.cmds.expect(cmdFails("Unable to locate credentials"), awsIdentity...)
	err := py.prepare(context.Background(), f.publisher("1.2.0", ""))
	assert.ErrorContains(t, err, "AWS credentials are not available or not valid: aws sts get-caller-identity: "+
		"exit status 1: Unable to locate credentials")
}

func TestPythonPublish(t *testing.T) {
	py, f := pythonFixture(t)
	f.cmds.expect(cmdFails(codeArtifactNotFound), describeArgs("1.2.0rc1")...)
	f.cmds.expectAll(pythonPublishEnv)
	f.cmds.expectAll([]stubCmd{
		misePoetry,
		{[]string{"poetry", "build"}, cmdOK("Building widgets_client\n")},
		{[]string{"poetry", "publish", "--repository", "codeartifact"}, cmdOK("Publishing with " + pythonToken + "\n")},
	})
	info, err := py.publish(context.Background(), f.publisher("1.2.0-rc.1", "main"))
	assert.NilError(t, err)
	assert.Assert(t, info == nil)

	// Poetry gets the repository and the token in its environment, and
	// the log has no token.
	calls := f.cmds.calls()
	publish := calls[len(calls)-1].Cmd
	assert.Equal(t, publish.Dir, filepath.Join(f.repo, filepath.FromSlash(spec.DefaultPythonDir)))
	assert.Equal(t, envValue(publish.Env, "POETRY_REPOSITORIES_CODEARTIFACT_URL"), pythonEndpoint)
	assert.Equal(t, envValue(publish.Env, "POETRY_HTTP_BASIC_CODEARTIFACT_USERNAME"), "aws")
	assert.Equal(t, envValue(publish.Env, "POETRY_HTTP_BASIC_CODEARTIFACT_PASSWORD"), pythonToken)
	assert.Assert(t, strings.Contains(f.log.String(), "Publishing with [secure]\n"), f.log.String())
	assert.Assert(t, !strings.Contains(f.log.String(), pythonToken), f.log.String())
	assert.Assert(t, !slices.ContainsFunc(f.env, func(kv string) bool { return strings.HasPrefix(kv, "POETRY_") }))
}

func TestPythonPublishSkipsExistingVersion(t *testing.T) {
	py, f := pythonFixture(t)
	f.cmds.expect(cmdOK("Published\n"), describeArgs("1.2.0")...)
	info, err := py.publish(context.Background(), f.publisher("1.2.0", ""))
	assert.NilError(t, err)
	assert.Assert(t, info == nil)
	assert.Assert(t, strings.Contains(f.log.String(), "Skip publishing to CodeArtifact acme-pypi as widgets-client 1.2.0 exists"))
}

func TestPythonExists(t *testing.T) {
	tests := []struct {
		name    string
		res     execstub.Result
		want    bool
		wantErr string
	}{
		{name: "published", res: cmdOK("Published\n"), want: true},
		{name: "archived", res: cmdOK("Archived\n"), want: true},
		{name: "missing", res: cmdFails(codeArtifactNotFound)},
		{name: "access denied", res: cmdFails(
			"An error occurred (AccessDeniedException) when calling the DescribePackageVersion operation",
		), wantErr: "describe widgets-client 1.2.0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			py, f := pythonFixture(t)
			f.cmds.expect(tc.res, describeArgs("1.2.0")...)
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
	py, f := pythonFixture(t)
	py.plugin.Account = "210987654321"
	py.boxAccount = func() (string, error) { return "", errExit }
	argv := describeArgs("1.2.0")
	argv[slices.Index(argv, "123456789012")] = "210987654321"
	f.cmds.expect(cmdOK("Published\n"), argv...)
	_, err := py.exists(context.Background(), f.publisher("1.2.0", ""))
	assert.NilError(t, err)

	py.plugin.Account = ""
	py.boxAccount = func() (string, error) { return "", nil }
	_, err = py.exists(context.Background(), f.publisher("1.2.0", ""))
	assert.ErrorIs(t, err, errPythonNoAccount)
}

func TestPythonPackageErrors(t *testing.T) {
	py, f := pythonFixture(t)
	writeFile(t, f.repo, filepath.Join(spec.DefaultPythonDir, "pyproject.toml"), "[tool.poetry]\nname = \"old\"\n")
	_, err := py.exists(context.Background(), f.publisher("1.2.0", ""))
	assert.ErrorIs(t, err, errPythonNoName)

	writeFile(t, f.repo, filepath.Join(spec.DefaultPythonDir, "pyproject.toml"), pythonPyproject)
	_, err = py.exists(context.Background(), f.publisher("1.2.0-foo.1", ""))
	assert.ErrorIs(t, err, spec.ErrUnsupported)
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

// TestPythonRealPoetry runs poetry version and poetry build as real
// processes, and stubs mise, aws and poetry publish. It is skipped
// without poetry.
func TestPythonRealPoetry(t *testing.T) {
	if _, err := exec.LookPath("poetry"); err != nil {
		t.Skip("poetry is not installed")
	}
	py, f := pythonFixture(t)
	f.env = os.Environ()
	writeFile(t, f.repo, filepath.Join(spec.DefaultPythonDir, "src", "widgets_client", "__init__.py"), "")
	writeFile(t, f.repo, filepath.Join(spec.DefaultPythonDir, "pyproject.toml"), pythonPyproject+`requires-python = ">=3.9"

[tool.poetry]
packages = [{ include = "widgets_client", from = "src" }]

[build-system]
requires = ["poetry-core>=2.0.0,<3.0.0"]
build-backend = "poetry.core.masonry.api"
`)
	// realPoetry runs the command with the real poetry.
	realPoetry := execstub.Result{Real: true}
	f.cmds.expect(cmdOK("{}\n"), awsIdentity...)
	f.cmds.expectAll([]stubCmd{misePoetry, {[]string{"poetry", "version", "1.2.0-rc.1"}, realPoetry}})
	f.cmds.expect(cmdFails(codeArtifactNotFound), describeArgs("1.2.0rc1")...)
	f.cmds.expectAll(pythonPublishEnv)
	f.cmds.expectAll([]stubCmd{
		misePoetry,
		{[]string{"poetry", "build"}, realPoetry},
		{[]string{"poetry", "publish", "--repository", "codeartifact"}, cmdOK("")},
	})
	p := f.publisher("1.2.0-rc.1", "main")
	assert.NilError(t, py.prepare(context.Background(), p))
	_, err := py.publish(context.Background(), p)
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
