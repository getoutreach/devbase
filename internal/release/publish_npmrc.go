// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Reads the npm configuration and finds the npm registry and
// its authentication, as @semantic-release/npm 13.2.0 does with rc 1.2.8,
// ini 1.3.8, registry-auth-token 5.1.1, nerf-dart 1.0.0 and
// normalize-url 9.

package release

import (
	"encoding/json"
	"maps"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// officialNPMRegistry is the npm registry of the npm plugin when nothing
// configures another one.
const officialNPMRegistry = "https://registry.npmjs.org/"

// These are the patterns of the ini parser of rc.
var (
	// iniJSONRe matches a config file in JSON format.
	iniJSONRe = regexp.MustCompile(`^\s*{`)

	// iniLineSepRe separates the lines of a config file.
	iniLineSepRe = regexp.MustCompile(`[\r\n]+`)

	// iniCommentRe matches a comment line.
	iniCommentRe = regexp.MustCompile(`^\s*[;#]`)

	// iniLineRe matches a section line or a key with an optional value.
	iniLineRe = regexp.MustCompile(`^\[([^\]]*)\]$|^([^=]+)(=(.*))?$`)
)

// npmrcConfig loads the npm config as rc("npm", {registry:
// officialNPMRegistry}, {config: userconfig}) does, where userconfig is
// NPM_CONFIG_USERCONFIG or the .npmrc file of cwd. It returns the string
// values of the merged config and the files that it read, in order. The
// environment is env, and cwd stands for the process folder of
// semantic-release, which is the repository.
func npmrcConfig(cwd string, env []string) (cfg map[string]string, files []string) {
	cfg = map[string]string{"registry": officialNPMRegistry}
	add := func(file string) {
		if file == "" {
			return
		}
		if !filepath.IsAbs(file) {
			file = filepath.Join(cwd, file)
		}
		if slices.Contains(files, file) {
			return
		}
		b, err := os.ReadFile(file)
		if err != nil || len(b) == 0 {
			return
		}
		maps.Copy(cfg, parseNpmrc(string(b)))
		files = append(files, file)
	}
	add("/etc/npm/config")
	add("/etc/npmrc")
	if home := envValue(env, "HOME"); home != "" {
		add(filepath.Join(home, ".config", "npm", "config"))
		add(filepath.Join(home, ".config", "npm"))
		add(filepath.Join(home, ".npm", "config"))
		add(filepath.Join(home, ".npmrc"))
	}
	add(findUp(cwd, ".npmrc"))
	vars := npmrcEnv(env)
	add(vars["config"])
	userconfig := envValue(env, "NPM_CONFIG_USERCONFIG")
	if userconfig == "" {
		userconfig = filepath.Join(cwd, ".npmrc")
	}
	add(userconfig)
	maps.Copy(cfg, vars)
	return cfg, files
}

// findUp returns the first dir/name that exists, from dir up to the root,
// or "".
func findUp(dir, name string) string {
	for {
		file := filepath.Join(dir, name)
		if _, err := os.Stat(file); err == nil {
			return file
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// npmrcEnv returns the config values of the environment variables that
// start with "npm_" in any case, without the prefix, as rc reads them. rc
// makes nested values from "__" in a name; they are not strings and are
// left out.
func npmrcEnv(env []string) map[string]string {
	out := map[string]string{}
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || len(k) < 4 || !strings.EqualFold(k[:4], "npm_") {
			continue
		}
		parts := slices.DeleteFunc(strings.Split(k[4:], "__"), func(s string) bool { return s == "" })
		if len(parts) == 1 {
			out[parts[0]] = v
		}
	}
	return out
}

// parseNpmrc returns the top-level string values of a config file in ini
// or JSON format, as rc and ini 1.3.8 parse it. Values in sections,
// arrays, and the values true, false and null are left out, because the
// registry and auth lookups use only top-level strings.
func parseNpmrc(content string) map[string]string {
	out := map[string]string{}
	if iniJSONRe.MatchString(content) {
		var values map[string]any
		if err := json.Unmarshal([]byte(content), &values); err == nil {
			for k, v := range values {
				if s, ok := v.(string); ok {
					out[k] = s
				}
			}
		}
		return out
	}
	inSection := false
	for _, line := range iniLineSepRe.Split(content, -1) {
		if line == "" || iniCommentRe.MatchString(line) {
			continue
		}
		m := iniLineRe.FindStringSubmatchIndex(line)
		switch {
		case m == nil:
			continue
		case m[2] >= 0:
			inSection = true
			continue
		case inSection || m[6] < 0:
			continue
		}
		key, value := iniUnsafe(line[m[4]:m[5]]), iniUnsafe(line[m[8]:m[9]])
		if key == "__proto__" || strings.HasSuffix(key, "[]") || value == "true" || value == "false" || value == "null" {
			continue
		}
		out[key] = value
	}
	return out
}

// iniUnsafe unquotes an ini key or value, or cuts it at the first
// comment character that is not escaped, as ini 1.3.8 does.
func iniUnsafe(val string) string {
	val = strings.TrimSpace(val)
	if len(val) >= 2 && (val[0] == '"' && val[len(val)-1] == '"' || val[0] == '\'' && val[len(val)-1] == '\'') {
		if val[0] == '\'' {
			val = val[1 : len(val)-1]
		}
		var s string
		if err := json.Unmarshal([]byte(val), &s); err == nil {
			return s
		}
		return val
	}
	var b strings.Builder
	esc := false
	for _, c := range val {
		switch {
		case esc:
			if !strings.ContainsRune(`\;#`, c) {
				b.WriteByte('\\')
			}
			b.WriteRune(c)
			esc = false
		case c == ';' || c == '#':
			return strings.TrimSpace(b.String())
		case c == '\\':
			esc = true
		default:
			b.WriteRune(c)
		}
	}
	if esc {
		b.WriteByte('\\')
	}
	return strings.TrimSpace(b.String())
}

// npmRegistryURL returns the registry of a package, as get-registry.js
// does: publishConfig.registry, NPM_CONFIG_REGISTRY, the registry of the
// package scope in the npm config, or the registry in the npm config,
// with a trailing slash.
func npmRegistryURL(pkg *npmPackage, cfg map[string]string, env []string) string {
	if pkg.PublishConfig.Registry != "" {
		return pkg.PublishConfig.Registry
	}
	if r := envValue(env, "NPM_CONFIG_REGISTRY"); r != "" {
		return r
	}
	scope, _, _ := strings.Cut(pkg.Name, "/")
	r := cfg[scope+":registry"]
	if r == "" {
		r = cfg["registry"]
	}
	if !strings.HasSuffix(r, "/") {
		r += "/"
	}
	return r
}

// npmAuthConfigured reports whether the npm config has authentication
// for registry, as getAuthToken(registry, {npmrc}) of
// registry-auth-token is truthy: a token, a username and password, or a
// legacy _auth for the registry URL or, when it has no trailing slash,
// its parent path; or a top-level _auth. A token that names an
// environment variable counts even when the variable is not set.
func npmAuthConfigured(registry string, cfg map[string]string) bool {
	raw := registry
	if strings.HasPrefix(raw, "//") {
		raw = "http:" + raw
	}
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		p := u.Path
		if p == "" {
			p = "/"
		}
		host := urlHost(u)
		if npmAuthForURL(cfg, "//"+host+strings.TrimSuffix(p, "/")) {
			return true
		}
		if !strings.HasSuffix(registry, "/") {
			dir := p[:strings.LastIndex(p, "/")+1]
			if npmAuthForURL(cfg, "//"+host+strings.TrimSuffix(dir, "/")) {
				return true
			}
		}
	}
	return cfg["_auth"] != ""
}

// npmAuthForURL reports whether cfg has authentication for a registry
// URL without scheme, such as "//registry.npmjs.org".
func npmAuthForURL(cfg map[string]string, regURL string) bool {
	get := func(key string) string {
		if v := cfg[regURL+key]; v != "" {
			return v
		}
		return cfg[regURL+"/"+key]
	}
	return get(":_authToken") != "" || get(":username") != "" && get(":_password") != "" || get(":_auth") != ""
}

// urlHost returns the host of u as a WHATWG URL has it: lower case and
// without the default port of the scheme.
func urlHost(u *url.URL) string {
	host, port := strings.ToLower(u.Hostname()), u.Port()
	if port == "" || u.Scheme == "http" && port == "80" || u.Scheme == "https" && port == "443" {
		return host
	}
	return net.JoinHostPort(host, port)
}

// nerfDart returns the npm config key prefix of a registry URL, such as
// "//registry.npmjs.org/", as nerf-dart does: the URL without scheme,
// credentials, query and hash, and without the part after the last slash
// of the path.
func nerfDart(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	dir := "/"
	if i := strings.LastIndex(u.Path, "/"); i >= 0 {
		dir = u.Path[:i+1]
	}
	return "//" + strings.ToLower(u.Host) + dir
}

// normalizeRegistryURL normalizes a URL for comparison, as normalize-url
// does with its default options: lower-case scheme and host, no "www.",
// no default port, no trailing slash, no hash, and sorted query
// parameters.
func normalizeRegistryURL(s string) string {
	raw := strings.TrimSpace(s)
	if strings.HasPrefix(raw, "//") {
		raw = "http:" + raw
	} else if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return s
	}
	scheme := strings.ToLower(u.Scheme)
	u.Scheme = scheme
	host := strings.TrimPrefix(urlHost(u), "www.")
	out := scheme + "://" + host + strings.TrimRight(u.EscapedPath(), "/")
	if q := u.Query(); len(q) > 0 {
		keys := slices.Collect(maps.Keys(q))
		sort.Strings(keys)
		v := url.Values{}
		for _, k := range keys {
			v[k] = q[k]
		}
		out += "?" + v.Encode()
	}
	return out
}

// npmDistTag returns the dist-tag of a channel, as get-channel.js does:
// "latest" for the default channel, and "release-<channel>" when the
// channel is a valid semver range, such as "1.x".
func npmDistTag(channel string) string {
	if channel == "" {
		return "latest"
	}
	if _, err := semver.NewConstraint(channel); err == nil {
		return "release-" + channel
	}
	return channel
}
