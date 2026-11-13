// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Publishes a Python client to AWS CodeArtifact with
// Poetry, as the stencil Python client hooks and publish:pyclient task
// do, with an existence check first.

package release

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/rlog"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"github.com/getoutreach/gobox/pkg/box"
)

// These are the errors of the Python registry.
var (
	// errPythonNoName is returned when pyproject.toml has no project
	// name.
	errPythonNoName = errors.New("pyproject.toml has no [project] name")

	// errPythonNoAccount is returned when neither the plugin nor the box
	// config has the AWS account of the CodeArtifact domain.
	errPythonNoAccount = errors.New("no AWS account for CodeArtifact: set aws.defaultAccountID in the box config")

	// errPythonNoToken is returned when the CodeArtifact token is empty.
	errPythonNoToken = errors.New("missing AWS CodeArtifact token")

	// errPythonNoPoetry is returned when mise finds no poetry.
	errPythonNoPoetry = errors.New("mise which poetry printed nothing")
)

// codeArtifactNotFoundRe matches the error of the AWS CLI (botocore
// ClientError) for a package or version that does not exist.
var codeArtifactNotFoundRe = regexp.MustCompile(`\(ResourceNotFoundException\) when calling the DescribePackageVersion operation`)

// pypiNameSeparatorsRe matches the runs of separators that PEP 503 name
// normalization replaces with "-".
var pypiNameSeparatorsRe = regexp.MustCompile(`[-_.]+`)

// These are the names of the Poetry repository and its environment, as
// the publish:pyclient task sets them.
const (
	// poetryRepository is the repository name of poetry publish.
	poetryRepository = "codeartifact"

	// poetryURLEnv is the URL of the repository.
	poetryURLEnv = "POETRY_REPOSITORIES_CODEARTIFACT_URL"

	// poetryUserEnv is the user name of the repository.
	poetryUserEnv = "POETRY_HTTP_BASIC_CODEARTIFACT_USERNAME"

	// poetryPasswordEnv is the password of the repository: the token.
	// The name has a secret word, so the logs mask the token.
	poetryPasswordEnv = "POETRY_HTTP_BASIC_CODEARTIFACT_PASSWORD" //nolint:gosec // Why: a variable name, not a credential.
)

// pythonRegistry publishes the package of a Python plugin.
type pythonRegistry struct {
	// plugin is the plugin with its defaults.
	plugin spec.PythonPlugin

	// boxAccount returns aws.defaultAccountID of the box config.
	boxAccount func() (string, error)
}

// newPythonRegistry returns the registry of a Python plugin.
func newPythonRegistry(pl *spec.PythonPlugin) registry {
	return &pythonRegistry{plugin: pl.WithDefaults(), boxAccount: boxDefaultAccount}
}

// boxDefaultAccount returns aws.defaultAccountID of the box config, as
// get_box_field in devbase shell/lib/box.sh reads it.
func boxDefaultAccount() (string, error) {
	c, err := box.LoadBox()
	if err != nil {
		return "", fmt.Errorf("read the box config: %w", err)
	}
	return c.AWS.DefaultAccountID, nil
}

// name names the registry in messages.
func (py *pythonRegistry) name() string {
	return "CodeArtifact " + py.plugin.Repository
}

// pythonSession is the package and AWS account of one step.
type pythonSession struct {
	// dir is the package folder.
	dir string

	// pkg is the normalized package name.
	pkg string

	// version is the PEP 440 version.
	version string

	// account is the AWS account that owns the domain.
	account string

	// env is the environment of the commands.
	env []string
}

// open reads the package name from pyproject.toml, and finds the PEP 440
// version and the AWS account.
func (py *pythonRegistry) open(p *regTarget) (*pythonSession, error) {
	s := &pythonSession{dir: filepath.Join(p.dir, filepath.FromSlash(py.plugin.Dir)), env: envOrProcess(p.env)}
	name, err := readPythonName(s.dir)
	if err != nil {
		return nil, err
	}
	s.pkg = normalizePythonName(name)
	if s.version, err = spec.PEP440Version(p.version); err != nil {
		return nil, err
	}
	s.account = py.plugin.Account
	if s.account == "" {
		if s.account, err = py.boxAccount(); err != nil {
			return nil, err
		}
		if s.account == "" {
			return nil, errPythonNoAccount
		}
	}
	return s, nil
}

// readPythonName returns the [project] name of the pyproject.toml of
// dir.
func readPythonName(dir string) (string, error) {
	file := filepath.Join(dir, "pyproject.toml")
	b, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("read the Python package: %w", err)
	}
	var pyproject struct {
		Project struct {
			Name string `toml:"name"`
		} `toml:"project"`
	}
	if err := toml.Unmarshal(b, &pyproject); err != nil {
		return "", fmt.Errorf("parse %s: %w", file, err)
	}
	if pyproject.Project.Name == "" {
		return "", fmt.Errorf("%w in %s", errPythonNoName, dir)
	}
	return pyproject.Project.Name, nil
}

// normalizePythonName returns the PEP 503 normalized name of a package,
// which the CodeArtifact API needs.
func normalizePythonName(name string) string {
	return strings.ToLower(pypiNameSeparatorsRe.ReplaceAllString(name, "-"))
}

// prepare checks the package, the AWS account and the AWS credentials,
// as the task does before it publishes, and then sets the version in
// pyproject.toml with "poetry version", as the generateNotesCmd hook
// does. Poetry writes the PEP 440 form of the version. The version
// changes only in the checkout (requirement 12).
func (py *pythonRegistry) prepare(ctx context.Context, p *regTarget) error {
	s, err := py.open(p)
	if err != nil {
		return err
	}
	if _, err := p.run(ctx, p.log, &command{dir: p.dir, env: s.env, name: "aws", args: []string{"sts", "get-caller-identity"}}); err != nil {
		return fmt.Errorf("AWS credentials are not available or not valid: %w", err)
	}
	poetry, err := py.poetry(ctx, p, s)
	if err != nil {
		return err
	}
	fmt.Fprintf(p.log, "Write version %s to pyproject.toml in %s\n", s.version, s.dir)
	if _, err := p.run(ctx, p.log, &command{dir: s.dir, env: s.env, name: poetry, args: []string{"version", p.version}}); err != nil {
		return fmt.Errorf("poetry version: %w", err)
	}
	return nil
}

// publish builds the package and publishes it to the CodeArtifact
// repository with Poetry, as the publish:pyclient task does. It skips
// the publish when the repository has the version (requirement 8). It
// returns no release info, as the exec hook printed none.
func (py *pythonRegistry) publish(ctx context.Context, p *regTarget) (*forge.Published, error) {
	s, err := py.open(p)
	if err != nil {
		return nil, err
	}
	found, err := py.describe(ctx, p, s)
	if err != nil {
		return nil, err
	}
	if found {
		fmt.Fprintf(p.log, "Skip publishing to %s as %s %s exists\n", py.name(), s.pkg, s.version)
		return nil, nil //nolint:nilnil // Why: the hook gave no release info.
	}
	env, err := py.poetryEnv(ctx, p, s)
	if err != nil {
		return nil, err
	}
	// The token is in env, so the commands mask it.
	ctx = rlog.WithMasker(ctx, rlog.NewMasker(env))
	poetry, err := py.poetry(ctx, p, s)
	if err != nil {
		return nil, err
	}
	if _, err := p.run(ctx, p.log, &command{dir: s.dir, env: env, name: poetry, args: []string{"build"}}); err != nil {
		return nil, fmt.Errorf("poetry build: %w", err)
	}
	fmt.Fprintf(p.log, "Publishing %s %s to %s\n", s.pkg, s.version, py.name())
	if _, err := p.run(ctx, p.log, &command{
		dir: s.dir, env: env, name: poetry, args: []string{"publish", "--repository", poetryRepository},
	}); err != nil {
		return nil, fmt.Errorf("poetry publish: %w", err)
	}
	fmt.Fprintf(p.log, "Published %s %s to %s\n", s.pkg, s.version, py.name())
	return nil, nil //nolint:nilnil // Why: the hook gave no release info.
}

// exists reports whether the CodeArtifact repository has the version of
// the plan.
func (py *pythonRegistry) exists(ctx context.Context, p *regTarget) (bool, error) {
	s, err := py.open(p)
	if err != nil {
		return false, err
	}
	return py.describe(ctx, p, s)
}

// describe reports whether the repository has the package version, with
// "aws codeartifact describe-package-version", which fails with
// ResourceNotFoundException for a missing package or version. A version
// in any status counts, because CodeArtifact keeps its name.
func (py *pythonRegistry) describe(ctx context.Context, p *regTarget, s *pythonSession) (bool, error) {
	res, err := p.run(ctx, p.log, py.awsCommand(p, s, "describe-package-version",
		"--repository", py.plugin.Repository, "--format", "pypi",
		"--package", s.pkg, "--package-version", s.version,
		"--query", "packageVersion.status", "--output", "text"))
	if err != nil {
		if codeArtifactNotFoundRe.MatchString(res.stderr) {
			return false, nil
		}
		return false, fmt.Errorf("describe %s %s in %s: %w", s.pkg, s.version, py.name(), err)
	}
	return true, nil
}

// poetryEnv returns the environment of poetry publish: the repository
// endpoint and an authorization token, as the task gets them. The token
// command logs nothing, because its stdout is the token.
func (py *pythonRegistry) poetryEnv(ctx context.Context, p *regTarget, s *pythonSession) ([]string, error) {
	res, err := p.run(ctx, p.log, py.awsCommand(p, s, "get-repository-endpoint",
		"--repository", py.plugin.Repository, "--format", "pypi",
		"--query", "repositoryEndpoint", "--output", "text"))
	if err != nil {
		return nil, fmt.Errorf("get the CodeArtifact endpoint: %w", err)
	}
	endpoint := strings.TrimSpace(res.stdout)
	fmt.Fprintf(p.log, "Get a CodeArtifact authorization token for %s\n", py.plugin.Domain)
	res, err = p.run(ctx, io.Discard, py.awsCommand(p, s, "get-authorization-token",
		"--query", "authorizationToken", "--output", "text"))
	if err != nil {
		return nil, fmt.Errorf("get the CodeArtifact token: %w", err)
	}
	token := strings.TrimSpace(res.stdout)
	if token == "" {
		return nil, errPythonNoToken
	}
	return append(s.env[:len(s.env):len(s.env)],
		poetryURLEnv+"="+endpoint, poetryUserEnv+"=aws", poetryPasswordEnv+"="+token), nil
}

// awsCommand returns an "aws codeartifact" command for the domain of the
// plugin.
func (py *pythonRegistry) awsCommand(p *regTarget, s *pythonSession, op string, args ...string) *command {
	return &command{
		dir: p.dir, env: s.env, name: "aws",
		args: append([]string{
			"codeartifact", op, "--domain", py.plugin.Domain, "--domain-owner", s.account,
			"--region", py.plugin.Region,
		}, args...),
	}
}

// poetry returns the Poetry program of the repository, with
// "mise which poetry", as the hook and the task find it.
func (py *pythonRegistry) poetry(ctx context.Context, p *regTarget, s *pythonSession) (string, error) {
	res, err := p.run(ctx, p.log, &command{dir: p.dir, env: s.env, name: "mise", args: []string{"which", "poetry"}})
	if err != nil {
		return "", fmt.Errorf("find poetry: %w", err)
	}
	poetry := strings.TrimSpace(res.stdout)
	if poetry == "" {
		return "", fmt.Errorf("find poetry: %w in %s", errPythonNoPoetry, p.dir)
	}
	return poetry, nil
}
