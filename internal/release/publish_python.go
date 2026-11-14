// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Publishes a Python client to AWS CodeArtifact with
// Poetry, as the stencil Python client hooks and publish:pyclient task
// do, with an existence check first.

package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

	// errPythonNoBuild is returned when poetry build wrote no files of
	// the version.
	errPythonNoBuild = errors.New("poetry build wrote no files of the version")
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
	return &pythonRegistry{
		plugin:     pl.WithDefaults(),
		boxAccount: boxField(func(c *box.Config) string { return c.AWS.DefaultAccountID }),
	}
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

	// endpoint is the repository endpoint, after endpoint gets it.
	endpoint string
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
// repository with Poetry, as the publish:pyclient task does. When the
// repository has the version (requirement 8), it uploads only the files
// of the build that the version does not have, and skips the publish
// when it has all of them. It gets the token only for an upload. It
// returns no release info, as the exec hook printed none.
//
// The build comes before the skip, because the wheel file name has
// tags that only the build knows: poetry-core takes them from the
// Python versions of the package and from a build script, if any. The
// build also gives the files whose hashes publish compares with the
// files of the version.
func (py *pythonRegistry) publish(ctx context.Context, p *regTarget) (*forge.Published, error) {
	s, err := py.open(p)
	if err != nil {
		return nil, err
	}
	found, err := py.describe(ctx, p, s)
	if err != nil {
		return nil, err
	}
	var assets []pythonAsset
	if found {
		if assets, err = py.assets(ctx, p, s); err != nil {
			return nil, err
		}
		names := make([]string, len(assets))
		for i, a := range assets {
			names[i] = a.Name
		}
		fmt.Fprintf(p.log, "%s has %s %s with the files: %s\n", py.name(), s.pkg, s.version, strings.Join(names, ", "))
	}
	poetry, err := py.poetry(ctx, p, s)
	if err != nil {
		return nil, err
	}
	if _, err := p.run(ctx, p.log, &command{dir: s.dir, env: s.env, name: poetry, args: []string{"build"}}); err != nil {
		return nil, fmt.Errorf("poetry build: %w", err)
	}
	args := []string{"publish", "--repository", poetryRepository}
	if found {
		// A rerun builds the same commit again, and uploads only the
		// files that the version does not have.
		missing, err := py.missingFiles(p, s, assets)
		if err != nil {
			return nil, err
		}
		if len(missing) == 0 {
			fmt.Fprintf(p.log, "Skip publishing to %s as %s %s exists with all the files of the build\n", py.name(), s.pkg, s.version)
			return nil, nil //nolint:nilnil // Why: the hook gave no release info.
		}
		dir, err := stageFiles(filepath.Join(s.dir, "dist"), missing)
		defer os.RemoveAll(dir)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(p.log, "Upload the missing files of %s %s: %s\n", s.pkg, s.version, strings.Join(missing, ", "))
		args = append(args, "--dist-dir", dir)
	}
	env, err := py.poetryEnv(ctx, p, s)
	if err != nil {
		return nil, err
	}
	// The token is in env, so the command masks it.
	ctx = rlog.WithMasker(ctx, rlog.NewMasker(env))
	fmt.Fprintf(p.log, "Publishing %s %s to %s\n", s.pkg, s.version, py.name())
	if _, err := p.run(ctx, p.log, &command{dir: s.dir, env: env, name: poetry, args: args}); err != nil {
		return nil, fmt.Errorf("poetry publish: %w", err)
	}
	fmt.Fprintf(p.log, "Published %s %s to %s\n", s.pkg, s.version, py.name())
	return nil, nil //nolint:nilnil // Why: the hook gave no release info.
}

// missingFiles returns the files of the build that the version does not
// have. For a file that the version has, it compares the SHA-256 of the
// build with the hash of the version, and a difference is a warning, not
// an error: CodeArtifact refuses other content for an existing file name
// (HTTP 409 Conflict) until the version is deleted, so a rerun cannot
// replace the file, and users already get it. The difference comes from
// another build: poetry-core builds the same files again byte for byte,
// but another Poetry version or other source files can change them.
func (py *pythonRegistry) missingFiles(p *regTarget, s *pythonSession, assets []pythonAsset) ([]string, error) {
	files, err := s.distFiles()
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, f := range files {
		i := slices.IndexFunc(assets, func(a pythonAsset) bool { return a.Name == f })
		if i < 0 {
			missing = append(missing, f)
			continue
		}
		want := assets[i].Hashes["SHA-256"]
		if want == "" {
			fmt.Fprintf(p.log, "Warning: %s gave no SHA-256 for %s, so publish cannot compare it with the build\n", py.name(), f)
			continue
		}
		got, err := sha256File(filepath.Join(s.dir, "dist", f))
		if err != nil {
			return nil, err
		}
		if want != got {
			fmt.Fprintf(p.log, "Warning: %s has %s with SHA-256 %s, but the build has %s; "+
				"CodeArtifact refuses other content for a file name, so the file in %s stays\n",
				py.name(), f, want, got, py.name())
		}
	}
	return missing, nil
}

// distFiles returns the names of the files of the version in the dist
// folder, which poetry build writes and poetry publish uploads: the
// wheels and the sdist.
func (s *pythonSession) distFiles() ([]string, error) {
	dist := filepath.Join(s.dir, "dist")
	prefix := strings.ReplaceAll(s.pkg, "-", "_") + "-" + s.version
	var files []string
	for _, pattern := range []string{prefix + "-*.whl", prefix + ".tar.gz"} {
		matches, err := filepath.Glob(filepath.Join(dist, pattern))
		if err != nil {
			return nil, fmt.Errorf("find the Python build output: %w", err)
		}
		for _, m := range matches {
			files = append(files, filepath.Base(m))
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%w: no %s files in %s", errPythonNoBuild, prefix, dist)
	}
	return files, nil
}

// pythonAsset is a file of a package version in CodeArtifact: an
// AssetSummary of the API.
type pythonAsset struct {
	// Name is the file name.
	Name string `json:"name"`

	// Hashes are the hex hashes of the file by algorithm, such as
	// "SHA-256".
	Hashes map[string]string `json:"hashes"`
}

// assets returns the files of the package version, with
// "aws codeartifact list-package-version-assets", which gets all the
// pages.
func (py *pythonRegistry) assets(ctx context.Context, p *regTarget, s *pythonSession) ([]pythonAsset, error) {
	res, err := p.run(ctx, p.log, py.awsCommand(p, s, "list-package-version-assets",
		"--repository", py.plugin.Repository, "--format", "pypi",
		"--package", s.pkg, "--package-version", s.version,
		"--query", "assets", "--output", "json"))
	if err != nil {
		return nil, fmt.Errorf("list the files of %s %s in %s: %w", s.pkg, s.version, py.name(), err)
	}
	var assets []pythonAsset
	if err := json.Unmarshal([]byte(res.stdout), &assets); err != nil {
		return nil, fmt.Errorf("parse the files of %s %s in %s: %w", s.pkg, s.version, py.name(), err)
	}
	return assets, nil
}

// stageFiles copies the files of dir to a new temporary folder, and
// returns the folder, which the caller removes also on an error.
func stageFiles(dir string, files []string) (string, error) {
	tmp, err := os.MkdirTemp("", "devbase-pypi-")
	if err != nil {
		return "", fmt.Errorf("make the folder of the missing files: %w", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return tmp, fmt.Errorf("read %s: %w", f, err)
		}
		if err := os.WriteFile(filepath.Join(tmp, f), b, 0o600); err != nil {
			return tmp, fmt.Errorf("write %s: %w", f, err)
		}
	}
	return tmp, nil
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
// "aws codeartifact describe-package-version". A version in any status
// counts, because CodeArtifact keeps its name. ResourceNotFoundException
// means a missing package or version, but also a missing domain or
// repository, so describe then gets the endpoint, which fails for those.
func (py *pythonRegistry) describe(ctx context.Context, p *regTarget, s *pythonSession) (bool, error) {
	res, err := p.run(ctx, p.log, py.awsCommand(p, s, "describe-package-version",
		"--repository", py.plugin.Repository, "--format", "pypi",
		"--package", s.pkg, "--package-version", s.version,
		"--query", "packageVersion.status", "--output", "text"))
	switch {
	case err == nil:
		return true, nil
	case !codeArtifactNotFoundRe.MatchString(res.stderr):
		return false, fmt.Errorf("describe %s %s in %s: %w", s.pkg, s.version, py.name(), err)
	}
	_, err = py.endpoint(ctx, p, s)
	return false, err
}

// endpoint returns the repository endpoint, which it gets once for s. It
// fails for a missing domain or repository.
func (py *pythonRegistry) endpoint(ctx context.Context, p *regTarget, s *pythonSession) (string, error) {
	if s.endpoint != "" {
		return s.endpoint, nil
	}
	res, err := p.run(ctx, p.log, py.awsCommand(p, s, "get-repository-endpoint",
		"--repository", py.plugin.Repository, "--format", "pypi",
		"--query", "repositoryEndpoint", "--output", "text"))
	if err != nil {
		return "", fmt.Errorf("get the endpoint of %s: %w", py.name(), err)
	}
	s.endpoint = strings.TrimSpace(res.stdout)
	return s.endpoint, nil
}

// poetryEnv returns the environment of poetry publish: the repository
// endpoint and an authorization token, as the task gets them. The token
// command logs nothing, because its stdout is the token.
func (py *pythonRegistry) poetryEnv(ctx context.Context, p *regTarget, s *pythonSession) ([]string, error) {
	endpoint, err := py.endpoint(ctx, p, s)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(p.log, "Get a CodeArtifact authorization token for %s\n", py.plugin.Domain)
	res, err := p.run(ctx, io.Discard, py.awsCommand(p, s, "get-authorization-token",
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
