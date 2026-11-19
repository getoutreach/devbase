// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Checks that a mise lockfile matches the config that requests its tools.

// Package miselock checks a mise lockfile (mise.<env>.lock) against the
// mise config (mise.<env>.toml) whose [tools] it locks. It reads both
// files and needs no network access.
package miselock

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// lockedTool is one locked version of a tool in a mise lockfile.
type lockedTool struct {
	// Version is the exact version that is locked.
	Version string

	// Backend is the mise backend that installs the tool, for example
	// "aqua:bufbuild/buf" or "go:golang.org/x/tools/cmd/goimports".
	Backend string

	// Platforms maps a platform (for example "linux-x64") to what is
	// locked for it.
	Platforms map[string]platform
}

// platform is what a lockfile locks for one platform of a tool version.
type platform struct {
	// URL is where the tool is downloaded from.
	URL string

	// Checksum is the checksum of the download, for example
	// "sha256:...".
	Checksum string
}

// errNotTOML is returned when a mise config path does not end in .toml,
// so the path of its lockfile is unknown.
var errNotTOML = errors.New("mise config path does not end in .toml")

// lockPlatformPrefix prefixes the key of each platform table in a
// locked tool version, for example "platforms.linux-x64".
const lockPlatformPrefix = "platforms."

// parseConfig returns the versions that the [tools] table of a mise
// config requests, by tool. A tool's value is a version string, an
// inline table with a version key, or an array of version strings; any
// other value requests no version.
func parseConfig(data []byte) (map[string][]string, error) {
	var config struct {
		Tools map[string]any `toml:"tools"`
	}
	if _, err := toml.Decode(string(data), &config); err != nil {
		return nil, fmt.Errorf("parse mise config: %w", err)
	}

	requested := make(map[string][]string, len(config.Tools))
	for tool, value := range config.Tools {
		var versions []string
		switch v := value.(type) {
		case string:
			versions = append(versions, v)
		case map[string]any:
			if version, ok := v["version"].(string); ok {
				versions = append(versions, version)
			}
		case []any:
			for _, item := range v {
				if version, ok := item.(string); ok {
					versions = append(versions, version)
				}
			}
		}
		requested[tool] = versions
	}
	return requested, nil
}

// parseLockfile returns the versions that a mise lockfile locks, by
// tool.
func parseLockfile(data []byte) (map[string][]lockedTool, error) {
	var lock struct {
		Tools map[string][]map[string]any `toml:"tools"`
	}
	if _, err := toml.Decode(string(data), &lock); err != nil {
		return nil, fmt.Errorf("parse mise lockfile: %w", err)
	}

	locked := make(map[string][]lockedTool, len(lock.Tools))
	for tool, entries := range lock.Tools {
		for _, entry := range entries {
			lt := lockedTool{Platforms: map[string]platform{}}
			lt.Version, _ = entry["version"].(string)
			lt.Backend, _ = entry["backend"].(string)
			for key, value := range entry {
				name, ok := strings.CutPrefix(key, lockPlatformPrefix)
				if !ok {
					continue
				}
				table, _ := value.(map[string]any)
				var p platform
				p.URL, _ = table["url"].(string)
				p.Checksum, _ = table["checksum"].(string)
				lt.Platforms[name] = p
			}
			locked[tool] = append(locked[tool], lt)
		}
	}
	return locked, nil
}

// matches reports whether the locked version satisfies the requested
// one: it is the same version, the request is a prefix of it that ends
// at a version component ("20" matches "20.11.0"), or the request is
// "latest".
func matches(requested, lockedVersion string) bool {
	return requested == "latest" || lockedVersion == requested || strings.HasPrefix(lockedVersion, requested+".")
}

// missingVersions returns "tool@version" for each version in requested
// that no version in locked matches, sorted by tool.
func missingVersions(requested map[string][]string, locked map[string][]lockedTool) []string {
	var missing []string
	for _, tool := range slices.Sorted(maps.Keys(requested)) {
		for _, version := range requested[tool] {
			if !slices.ContainsFunc(locked[tool], func(lt lockedTool) bool { return matches(version, lt.Version) }) {
				missing = append(missing, tool+"@"+version)
			}
		}
	}
	return missing
}

// staleTools returns each tool in locked that requested does not
// request, sorted.
func staleTools(requested map[string][]string, locked map[string][]lockedTool) []string {
	var stale []string
	for _, tool := range slices.Sorted(maps.Keys(locked)) {
		if _, ok := requested[tool]; !ok {
			stale = append(stale, tool)
		}
	}
	return stale
}

// staleVersions returns "tool@version" for each locked version of a
// requested tool that no request for it matches, sorted by tool.
func staleVersions(requested map[string][]string, locked map[string][]lockedTool) []string {
	var stale []string
	for _, tool := range slices.Sorted(maps.Keys(locked)) {
		versions, ok := requested[tool]
		if !ok {
			continue
		}
		for _, lt := range locked[tool] {
			if !slices.ContainsFunc(versions, func(version string) bool { return matches(version, lt.Version) }) {
				stale = append(stale, tool+"@"+lt.Version)
			}
		}
	}
	return stale
}

// downloadsPerPlatform reports whether backend downloads a release
// asset for each platform, so its lockfile entries need a url per
// platform. Other backends, such as go and npm, lock no platforms.
func downloadsPerPlatform(backend string) bool {
	kind, _, _ := strings.Cut(backend, ":")
	switch kind {
	case "aqua", "forgejo", "github", "gitlab", "http":
		return true
	}
	return false
}

// checksumOptional reports whether backend's upstream publishes no
// checksums. mise lock then records a checksum only for a platform
// where mise installed the tool and hashed the download itself, so the
// other platforms of the same version have none.
func checksumOptional(backend string) bool {
	switch backend {
	case
		// The aqua registry downloads the AWS CLI from awscli.amazonaws.com,
		// which publishes PGP signatures but no checksums.
		"aqua:aws/aws-cli",
		// This fork's releases have neither a checksums file nor GitHub
		// release asset digests.
		"github:getoutreach/kubecfg":
		return true
	}
	return false
}

// platformProblems returns a problem for each locked version of a
// download backend that locks no platforms, lacks a url for a platform,
// or has a checksum for some platforms but not others (unless
// checksumOptional), sorted by tool.
func platformProblems(locked map[string][]lockedTool) []string {
	var problems []string
	for _, tool := range slices.Sorted(maps.Keys(locked)) {
		for _, lt := range locked[tool] {
			if !downloadsPerPlatform(lt.Backend) {
				continue
			}
			name := tool + "@" + lt.Version
			if len(lt.Platforms) == 0 {
				problems = append(problems, name+" locks no platforms")
				continue
			}

			var noURL, noChecksum []string
			for _, p := range slices.Sorted(maps.Keys(lt.Platforms)) {
				if lt.Platforms[p].URL == "" {
					noURL = append(noURL, p)
				}
				if lt.Platforms[p].Checksum == "" {
					noChecksum = append(noChecksum, p)
				}
			}
			if len(noURL) > 0 {
				problems = append(problems, name+" has no url for "+strings.Join(noURL, ", "))
			}
			if len(noChecksum) > 0 && len(noChecksum) < len(lt.Platforms) && !checksumOptional(lt.Backend) {
				problems = append(problems, name+" has no checksum for "+strings.Join(noChecksum, ", "))
			}
		}
	}
	return problems
}

// Check checks the lockfile of the mise config at configPath, which is
// the same path with a .lock extension instead of .toml. It returns one
// message per problem, each prefixed with the lockfile's path, and an
// error only if it cannot read or parse either file.
func Check(configPath string) ([]string, error) {
	base, ok := strings.CutSuffix(configPath, ".toml")
	if !ok {
		return nil, fmt.Errorf("%w: %s", errNotTOML, configPath)
	}
	lockPath := base + ".lock"

	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read mise config: %w", err)
	}
	requested, err := parseConfig(configData)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", configPath, err)
	}
	lockData, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, fmt.Errorf("read mise lockfile: %w", err)
	}
	locked, err := parseLockfile(lockData)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", lockPath, err)
	}

	var problems []string
	for _, v := range missingVersions(requested, locked) {
		problems = append(problems, fmt.Sprintf("%s: %s is requested in %s but not locked", lockPath, v, configPath))
	}
	for _, tool := range staleTools(requested, locked) {
		problems = append(problems, fmt.Sprintf("%s: %s is locked but not requested in %s", lockPath, tool, configPath))
	}
	for _, v := range staleVersions(requested, locked) {
		problems = append(problems, fmt.Sprintf("%s: %s is locked but matches no version requested in %s", lockPath, v, configPath))
	}
	for _, p := range platformProblems(locked) {
		problems = append(problems, lockPath+": "+p)
	}
	return problems, nil
}
