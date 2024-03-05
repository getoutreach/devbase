#!/usr/bin/env bash
# This script ensures we have a copy of devbase
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
libDir="$DIR/../.bootstrap"
lockfile="$DIR/../stencil.lock"
serviceYaml="$DIR/../service.yaml"
{{- $gojqVersion := (stencil.ReadFile "versions.yaml" | fromYaml).gojq }}
gojqVersion="{{ $gojqVersion }}"

# get_absolute_path returns the absolute path of a file
get_absolute_path() {
  python="$(command -v python3 || command -v python)"
  "$python" -c "import os,sys; print(os.path.realpath(sys.argv[1]))" "$1"
}

# gojq returns the path to a JIT-downloaded gojq binary.
gojq() {
  set -uo pipefail

  local gjDir
  gjDir="${XDG_CACHE_HOME:-$HOME/.cache}/devbase/gojq"
  local gojq="$gjDir/gojq-${gojqVersion}"
  if [[ ! -x $gojq ]]; then
    local platform arch
    mkdir -p "$gjDir"
    platform="$(uname -s | awk '{print tolower($0)}')"
    arch="$(uname -m)"
    if [[ $arch == x86_64 ]]; then
      arch=amd64
    fi
    local basename="gojq_${gojqVersion}_${platform}_${arch}"
    local tgz="$basename.tar.gz"

    if [[ ! -e "$gjDir/$tgz" ]]; then
      curl --fail --location --silent --output "$gjDir/$tgz" \
        "https://github.com/itchyny/gojq/releases/download/$gojqVersion/$tgz"
    fi
    tar --strip-components=1 --directory="$gjDir" --extract --file="$gjDir/$tgz" "$basename/gojq"
    mv "$gjDir"/gojq "$gojq"
  fi

  echo "$gojq"

  set +uo pipefail
}

# get_field_from_yaml reads a field from a yaml file via a JIT-downloaded gojq.
get_field_from_yaml() {
  local field="$1"
  local file="$2"

  "$(gojq)" --yaml-input -r "$field" <"$file"
}

# Use the version of devbase from stencil
version=$(get_field_from_yaml '.modules[] | select(.name == "github.com/getoutreach/devbase") | .version' "$lockfile")
existingVersion=$(cat "$libDir/.version" 2>/dev/null || true)

if [[ ! -e $libDir ]] || [[ $existingVersion != "$version" ]] || [[ ! -e "$libDir/.version" ]]; then
  rm -rf "$libDir"

  if [[ $version == "local" ]]; then
    # If we're using a local version, we should use ln
    # to map the local devbase into the bootstrap dir

    path=$(get_field_from_yaml '.replacements["github.com/getoutreach/devbase"]' "$serviceYaml")
    absolute_path=$(get_absolute_path "$path")

    ln -sf "$absolute_path" "$libDir"
  else
    git clone -q --single-branch --branch "$version" git@github.com:getoutreach/devbase \
      "$libDir" >/dev/null
  fi

  echo -n "$version" >"$libDir/.version"
fi

# If we're not using a local version, ensure that service.yaml doesn't exist
# in the library directory. This is because of the repo detection logic
# which looks for the base directory through the existence of that file.
if [[ $version != "local" ]]; then
  if [[ -e "$libDir/service.yaml" ]]; then
    rm "$libDir/service.yaml"
  fi
fi
