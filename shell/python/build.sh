#!/usr/bin/env bash
# Builds a Python package
set -euo pipefail

# Determine directories relative to this script.
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)/.."
SCRIPTS_DIR="$DIR/../shell"
LIB_DIR="$SCRIPTS_DIR/lib"

# Source helper scripts (bootstrap and logging)
# shellcheck source=../lib/bootstrap.sh
source "$LIB_DIR/bootstrap.sh"
# shellcheck source=../lib/logging.sh
source "$LIB_DIR/logging.sh"

# Get the application name; adjust get_app_name as needed.
appName="$(get_app_name)"
# Define the directory for the Python client package.
clientDir="$(get_repo_directory)/api/clients/python"
# Define the version file; this assumes your version is stored in a file like:
#   mypackage/__version__.py with a line such as: __version__ = "..."
versionFile="$clientDir/${appName}/__version__.py"

newVersion="$1"
if [[ -z $newVersion ]]; then
  echo "Error: Must pass in version" >&2
  exit 1
fi

echo "Setting package version to $newVersion" >&2
# Update the version file.
# This command expects a line like: __version__ = "old_version"
sed -i.bak "s/__version__ = \".*\"/__version__ = \"$newVersion\"/" "$versionFile" && rm "$versionFile.bak"

echo "Building Python package"
pushd "$clientDir" >/dev/null || exit 1

# (Optional) Install required dependencies if needed.
# For example: pip install -r requirements.txt

# Build the package: this creates source and wheel distributions in a 'dist' directory.
python setup.py sdist bdist_wheel

popd >/dev/null || exit 1

