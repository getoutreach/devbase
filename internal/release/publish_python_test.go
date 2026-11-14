// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the Python registry with stubs of the
// commands, and with the real poetry when it is installed.

package release

import (
	"context"
	"fmt"
	"io"
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

// codeArtifactNoRepository is a stderr of the AWS CLI for a missing
// repository, in the form of codeArtifactNotFound.
const codeArtifactNoRepository = "\nAn error occurred (ResourceNotFoundException) when calling the " +
	"GetRepositoryEndpoint operation: Repository not found\n"

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

// endpointArgs is the argv that gets the repository endpoint.
var endpointArgs = codeArtifactArgs("get-repository-endpoint", "--repository", "acme-pypi", "--format", "pypi",
	"--query", "repositoryEndpoint", "--output", "text")

// expectMissingVersion adds the commands of the existence check of a
// missing version: the package version, and then the repository
// endpoint.
func expectMissingVersion(s *cmdStubs, version string) {
	s.expect(cmdFails(codeArtifactNotFound), describeArgs(version)...)
	s.expect(cmdOK(pythonEndpoint+"\n"), endpointArgs...)
}

// These are the commands that find poetry, and that get the token of
// poetry publish.
var (
	misePoetry        = stubCmd{[]string{"mise", "which", "poetry"}, cmdOK("poetry\n")}
	codeArtifactToken = stubCmd{
		codeArtifactArgs("get-authorization-token", "--query", "authorizationToken", "--output", "text"), cmdOK(pythonToken + "\n"),
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
	expectMissingVersion(f.cmds, "1.2.0rc1")
	f.cmds.expectAll([]stubCmd{
		codeArtifactToken,
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

// pythonDist is the build output of version 1.2.0 of pythonFixture.
var pythonDist = []string{"widgets_client-1.2.0-py3-none-any.whl", "widgets_client-1.2.0.tar.gz"}

// assetsArgs is the argv that lists the files of version.
func assetsArgs(version string) []string {
	return codeArtifactArgs("list-package-version-assets", "--repository", "acme-pypi", "--format", "pypi",
		"--package", "widgets-client", "--package-version", version, "--query", "assets[].name", "--output", "json")
}

// poetryBuildWrites is the result of poetry build that writes files to
// the dist folder, and a file of another version that publish ignores.
func poetryBuildWrites(files ...string) execstub.Result {
	return execstub.Result{Func: func(_ context.Context, cmd *exec.Cmd, _ io.Reader, _, _ io.Writer) error {
		dist := filepath.Join(cmd.Dir, "dist")
		if err := os.MkdirAll(dist, 0o755); err != nil {
			return fmt.Errorf("make %s: %w", dist, err)
		}
		for _, f := range append(files, "widgets_client-1.1.0.tar.gz") {
			if err := os.WriteFile(filepath.Join(dist, f), []byte(f), 0o600); err != nil {
				return fmt.Errorf("write %s: %w", f, err)
			}
		}
		return nil
	}}
}

// TestPythonPublishExistingVersion checks a rerun for a version that
// the repository has: it uploads only the files that the version does
// not have (requirement 8).
func TestPythonPublishExistingVersion(t *testing.T) {
	tests := []struct {
		name    string
		assets  execstub.Result
		want    []string
		wantLog string
		wantErr string
	}{
		{
			name: "all files", assets: cmdOK(`["widgets_client-1.2.0.tar.gz", "widgets_client-1.2.0-py3-none-any.whl"]`),
			wantLog: "Skip publishing to CodeArtifact acme-pypi as widgets-client 1.2.0 exists with all the files of the build",
		},
		{
			name: "only the sdist", assets: cmdOK("[\n    \"widgets_client-1.2.0.tar.gz\"\n]\n"),
			want:    []string{"widgets_client-1.2.0-py3-none-any.whl"},
			wantLog: "Upload the missing files of widgets-client 1.2.0: widgets_client-1.2.0-py3-none-any.whl",
		},
		{
			name: "list failure", assets: cmdFails("An error occurred (AccessDeniedException) when calling the ListPackageVersionAssets operation"),
			wantErr: "list the files of widgets-client 1.2.0 in CodeArtifact acme-pypi",
		},
		{name: "bad list output", assets: cmdOK("widgets_client-1.2.0.tar.gz\n"), wantErr: "parse the files of widgets-client 1.2.0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			py, f := pythonFixture(t)
			f.cmds.expect(cmdOK("Published\n"), describeArgs("1.2.0")...)
			f.cmds.expect(tc.assets, assetsArgs("1.2.0")...)
			var distDir string
			var uploaded []string
			if tc.wantErr == "" {
				f.cmds.expectAll([]stubCmd{
					{endpointArgs, cmdOK(pythonEndpoint + "\n")},
					codeArtifactToken,
					misePoetry,
					{[]string{"poetry", "build"}, poetryBuildWrites(pythonDist...)},
				})
			}
			if tc.want != nil {
				// poetry publish reads the files of --dist-dir, which
				// publish removes afterwards.
				f.cmds.expect(execstub.Result{Func: func(_ context.Context, cmd *exec.Cmd, _ io.Reader, _, _ io.Writer) error {
					distDir = cmd.Args[len(cmd.Args)-1]
					entries, err := os.ReadDir(distDir)
					for _, e := range entries {
						uploaded = append(uploaded, e.Name())
					}
					return err
				}}, "poetry", "publish", "--repository", "codeartifact", "--dist-dir", execstub.AnyArg)
			}
			info, err := py.publish(context.Background(), f.publisher("1.2.0", ""))
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.Assert(t, info == nil)
			assert.DeepEqual(t, uploaded, tc.want)
			assert.Assert(t, strings.Contains(f.log.String(), tc.wantLog), f.log.String())
			assert.Assert(t, strings.Contains(f.log.String(),
				"CodeArtifact acme-pypi has widgets-client 1.2.0 with the files: widgets_client-1.2.0"), f.log.String())
			if tc.want != nil {
				_, err := os.Stat(distDir)
				assert.Assert(t, os.IsNotExist(err), "publish removes the folder of the missing files")
			}
		})
	}
}

// TestPythonPublishNoBuildOutput checks that a rerun fails when poetry
// build writes no files of the version.
func TestPythonPublishNoBuildOutput(t *testing.T) {
	py, f := pythonFixture(t)
	f.cmds.expect(cmdOK("Published\n"), describeArgs("1.2.0")...)
	f.cmds.expect(cmdOK("[]"), assetsArgs("1.2.0")...)
	f.cmds.expectAll([]stubCmd{
		{endpointArgs, cmdOK(pythonEndpoint + "\n")},
		codeArtifactToken,
		misePoetry,
		{[]string{"poetry", "build"}, poetryBuildWrites()},
	})
	_, err := py.publish(context.Background(), f.publisher("1.2.0", ""))
	assert.ErrorIs(t, err, errPythonNoBuild)
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
			if tc.name == "missing" {
				// A missing version also checks the repository.
				f.cmds.expect(cmdOK(pythonEndpoint+"\n"), endpointArgs...)
			}
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

// TestPythonExistsMissingRepository checks that a missing CodeArtifact
// repository is an error, not a missing version, so that rollback does
// not delete a release whose package is in another repository.
func TestPythonExistsMissingRepository(t *testing.T) {
	py, f := pythonFixture(t)
	f.cmds.expect(cmdFails(codeArtifactNotFound), describeArgs("1.2.0")...)
	f.cmds.expect(cmdFails(codeArtifactNoRepository), endpointArgs...)
	_, err := py.exists(context.Background(), f.publisher("1.2.0", ""))
	assert.ErrorContains(t, err, "get the endpoint of CodeArtifact acme-pypi")
	assert.ErrorContains(t, err, "ResourceNotFoundException")
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
// processes, and stubs mise, aws and poetry publish, which runs for real
// only as a dry run. It is skipped without poetry.
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
	expectMissingVersion(f.cmds, "1.2.0rc1")
	f.cmds.expectAll([]stubCmd{
		codeArtifactToken,
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

	// A rerun for a version with only the sdist builds again, and the
	// real poetry publish, as a dry run, takes only the wheel from
	// --dist-dir.
	var out strings.Builder
	f.cmds.expect(cmdOK("Published\n"), describeArgs("1.2.0rc1")...)
	f.cmds.expect(cmdOK(`["widgets_client-1.2.0rc1.tar.gz"]`), assetsArgs("1.2.0rc1")...)
	f.cmds.expectAll([]stubCmd{
		{endpointArgs, cmdOK(pythonEndpoint + "\n")},
		codeArtifactToken,
		misePoetry,
		{[]string{"poetry", "build"}, realPoetry},
		{[]string{"poetry", "publish", "--repository", "codeartifact", "--dist-dir", execstub.AnyArg}, execstub.Result{
			Func: func(ctx context.Context, cmd *exec.Cmd, _ io.Reader, _, _ io.Writer) error {
				dry := exec.CommandContext(ctx, cmd.Args[0], append(cmd.Args[1:], "--dry-run")...)
				dry.Dir, dry.Env = cmd.Dir, cmd.Env
				b, err := dry.CombinedOutput()
				out.Write(b)
				return err
			},
		}},
	})
	_, err = py.publish(context.Background(), p)
	assert.NilError(t, err, out.String())
	assert.Assert(t, strings.Contains(out.String(), "Uploading widgets_client-1.2.0rc1-py3-none-any.whl"), out.String())
	assert.Assert(t, !strings.Contains(out.String(), ".tar.gz"), out.String())
}
