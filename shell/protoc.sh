#!/usr/bin/env bash
# Generates code from proto files for Go, gRPC, and other languages if
# configured (currently limited to Ruby and JavaScript/TypeScript).
set -euo pipefail

# Generates proto types and clients from proto filess
SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
GOBIN="$SCRIPTS_DIR/gobin.sh"

# shellcheck source=./lib/logging.sh
source "$SCRIPTS_DIR/lib/logging.sh"
# shellcheck source=./lib/bootstrap.sh
source "$SCRIPTS_DIR/lib/bootstrap.sh"
# shellcheck source=./lib/yaml.sh
source "$SCRIPTS_DIR/lib/yaml.sh"

# SUBDIR is the directory to run protoc in relative to the current root
# (normally /api). If not set, defaults to "" (no sub directory).
SUBDIR=${1:-}

# The script should be ran in the directory where the protobuf generation occurs.
# We get the working directory here and then trim off of the repository directory
# to use the relative path in various places in this script.
#
# Even when the script is ran using the go:generate directive, this still works.
workDir="$(pwd)"
workDir=${workDir#"$(get_repo_directory)"}
info "Running protoc generation in $workDir (protoc $(protoc --version))"

PROTO_DOCS_DIR="$(get_repo_directory)/apidocs/proto"
PROTOC_IMPORTS_BASE_PATH="$HOME/.outreach/.protoc-imports"
mkdir -p "$PROTOC_IMPORTS_BASE_PATH"

# get_package_prefix returns the directory that a specific Node.js package
# lives in based on the name and version.
get_package_prefix() {
  local package_name="$1"
  local package_version="$2"
  local prefix_dir="$HOME/.outreach/.node-cache/$package_name/$package_version"
  echo "$prefix_dir"
}

# install_npm_package installs a specific version of an npm package
# globally
install_npm_package() {
  local package_name="$1"
  local package_version="$2"

  # Use a specific npm prefix for a "gobin" like experience that ensures
  # we use the correct versions
  # shellcheck disable=SC2155
  local prefix_dir="$(get_package_prefix "$package_name" "$package_version")"

  # Pre-create the prefix and bin,lib directories to avoid npm install errors.
  mkdir -p "$prefix_dir" "$prefix_dir/"{bin,lib}

  # Check and see if the package is already installed, if it's not
  # then install it.
  if ! npm list -g --prefix "$prefix_dir" | grep -q "$package_name@$package_version" 2>/dev/null; then
    local npm_args=("--prefix" "$prefix_dir" "$package_name@$package_version")

    # grpc-tools does not ship with an arm64 version at the moment.
    # So, we use the x64 version instead.
    if [[ "$(uname)" == "Darwin" && "$(uname -m)" == "arm64" && $package_name == "grpc-tools" ]]; then
      npm_args=("--target_arch=x64" "${npm_args[@]}")
    fi

    npm install -g "${npm_args[@]}"
  fi
}

info "Ensuring protoc imports are avaliable locally"

imports=$(
  get_list "go-protoc-imports"
)

default_args=()
import_paths=()
for import in $imports; do
  module_version_str=$(jq -r .module <<<"$import")
  info_sub "$module_version_str"

  # shellcheck disable=SC2206
  module_version_arr=(${module_version_str//@/ })
  module="${module_version_arr[0]}"

  if [[ ${#module_version_arr[@]} -eq 1 ]]; then
    echo "$module needs a specified version (append @<tag> to the module path)" >&2
    exit 1
  fi

  version="${module_version_arr[1]}"

  if [[ $version == "latest" ]]; then
    echo " -> $module is using @latest, please consider pinning a version for a more reproducible build" >&2
    version="$(gh release -R "$module" list -L 1 | awk '{print $1}')"
    echo " -> discovered $version for $module using @latest"
  fi

  import_path="$PROTOC_IMPORTS_BASE_PATH/$module/$version"
  path=$(jq -r .path <<<"$import")
  path=${path#"/"} # Trim / prefix if it exists.

  # Add import to list
  default_args+=("--proto_path=$import_path/$path")
  import_paths+=("$import_path/$path")

  # Check to see if we already have this version locally and remove
  # to allow a fresh clone if we already do.
  if [[ -d $import_path && -d "$import_path/.git" ]]; then
    rm -r "$import_path"
  fi

  # Clone import into the import path since we don't have it already
  mkdir -p "$import_path"
  git clone --depth 1 --branch "$version" https://"$module" "$import_path"
done

info "Generating Go gRPC client"
info_sub "Ensuring Go protoc plugins are installed"

latestGoProtobufModules=$(yaml_get_field ".arguments.grpcOptions.latestGoProtobufModules" "$(get_service_yaml)")
if [[ $latestGoProtobufModules == "true" ]]; then
  warn "!!! Using latest Go protobuf modules (this is ALPHA) !!!"
  protoc_gen_go=$("$GOBIN" -p google.golang.org/protobuf/cmd/protoc-gen-go@v"$(get_application_version "protoc-gen-go-latest")")
  protoc_gen_go_grpc=$(BUILD_DIR=cmd/protoc-gen-go-grpc BUILD_PATH=. "$GOBIN" -p google.golang.org/grpc/cmd/protoc-gen-go-grpc@"$(get_application_version "protoc-gen-go-grpc")")
else
  protoc_gen_go=$("$GOBIN" -p github.com/golang/protobuf/protoc-gen-go@v"$(get_application_version "protoc-gen-go")")
  protoc_gen_doc=$("$GOBIN" -p github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@v"$(get_application_version "protoc-gen-doc")")
fi

info_sub "Running Go protobuf generation"

go_args=("${default_args[@]}")
go_args+=(--plugin=protoc-gen-go="$protoc_gen_go")

# Default behaviour.
if [[ $latestGoProtobufModules != "true" ]]; then
  go_args+=(
    --go_out=plugins=grpc:".$SUBDIR"
    --go_opt=paths=source_relative
  )
else
  go_args+=(
    --plugin=protoc-gen-go-grpc="$protoc_gen_go_grpc"
    --go_out=.
    --go_opt=paths=source_relative
    --go-grpc_out=.
    --go-grpc_opt=paths=source_relative
  )
fi

disableDocGeneration=$(yaml_get_field ".arguments.grpcOptions.disableDocGeneration" "$(get_service_yaml)")
if [[ $disableDocGeneration != "true" ]]; then
  go_args+=(
    --plugin=protoc-gen-doc="$protoc_gen_doc"
    --doc_out="$(get_repo_directory)$workDir/doc"
    --doc_opt="html,index.html"
  )
fi

go_args+=(--proto_path="$(get_repo_directory)$workDir$SUBDIR")

if has_feature "validation"; then
  protoc_gen_validate=$("$GOBIN" -p github.com/envoyproxy/protoc-gen-validate@v"$(get_application_version "protoc-gen-validate")")

  go_args+=(
    --plugin=protoc-gen-validate="$protoc_gen_validate"
    "--validate_out=lang=go,paths=source_relative:$(get_repo_directory)$workDir$SUBDIR"
  )
fi

delete_validate() {
  sed -i '' '/validate_pb/d' "$1" 2>/dev/null || sed -i '/validate_pb/d' "$1"
}

# Make docs output directory if it doesn't exist.
if [[ $disableDocGeneration != "true" ]]; then
  mkdir -p "$(get_repo_directory)$workDir/doc"
fi

# Run protoc for Go.
(
  protoDir=$(get_repo_directory)$workDir$SUBDIR
  set -x
  protoc "${go_args[@]}" "$protoDir/"*.proto
)

# Move docs into the actual docs directory.
if [[ $disableDocGeneration != "true" ]]; then
  mkdir -p "$PROTO_DOCS_DIR"
  mv "$(get_repo_directory)$workDir/doc/index.html" "$PROTO_DOCS_DIR"
fi

if has_grpc_client "node"; then
  info "Generating Node gRPC client"
  info_sub "Ensuring Node protoc plugins are installed"

  node_tools_version="$(get_application_version "node-grpc-tools")"

  install_npm_package "grpc-tools" "$node_tools_version"
  grpc_tools_node_bin="$(get_package_prefix "grpc-tools" "$node_tools_version")/bin/grpc_tools_node_protoc"
  grpc_tools_node_plugin="$(get_package_prefix "grpc-tools" "$node_tools_version")/bin/grpc_tools_node_protoc_plugin"

  # Make node/TS output directory if it doesn't exist.
  mkdir -p "$(get_repo_directory)$workDir/clients/node/src/grpc$SUBDIR"

  info_sub "Running Node protobuf generation"
  grpc_path="$(get_repo_directory)$workDir/clients/node/src/grpc$SUBDIR"

  # Copy imported pb files.
  for import_path in "${import_paths[@]}"; do
    import_grpc_path="$import_path/clients/node/src/grpc"
    if [[ -d $import_grpc_path ]]; then
      cp -r "$import_grpc_path/." "$grpc_path"
    fi
  done

  node_args=("${default_args[@]}")
  node_args+=(
    --plugin=protoc-gen-grpc="$grpc_tools_node_plugin"
    --js_out=import_style="commonjs,binary:$grpc_path"
    --grpc_out=grpc_js:"$grpc_path"
    --proto_path "$(get_repo_directory)$workDir$SUBDIR"
  )

  "$grpc_tools_node_bin" "${node_args[@]}" "$(get_repo_directory)$workDir$SUBDIR/"*.proto

  info_sub "Running TS protobuf generation"

  ts_protoc_version="$(get_application_version "node-ts-grpc-tools")"
  install_npm_package "grpc_tools_node_protoc_ts" "$ts_protoc_version"
  ts_protoc_bin="$(get_package_prefix "grpc_tools_node_protoc_ts" "$ts_protoc_version")/bin/protoc-gen-ts"

  ts_args=("${default_args[@]}")
  ts_args+=(
    --plugin=protoc-gen-ts="$ts_protoc_bin"
    --ts_out=grpc_js:"$grpc_path"
    --proto_path "$(get_repo_directory)$workDir$SUBDIR"
  )

  "$grpc_tools_node_bin" "${ts_args[@]}" "$(get_repo_directory)$workDir$SUBDIR/"*.proto

  # remove unsupported validate proto plugin
  if pushd "$grpc_path"; then
    find . -name '*.js' -or -name '*.ts' | while read -r file; do
      delete_validate "$file"
    done
    popd || exit 1
  fi
fi

if has_grpc_client "ruby"; then
  info "Generating Ruby gRPC client"
  info_sub "Ensuring Ruby protoc plugins are installed"

  ruby_grpc_tools_version="$(get_application_version "ruby-grpc-tools")"

  if ! gem list | grep "grpc (" | grep "$ruby_grpc_tools_version" >/dev/null 2>&1; then
    gem install grpc -v "$ruby_grpc_tools_version"
  fi

  if ! gem list | grep "grpc-tools (" | grep "$ruby_grpc_tools_version" >/dev/null 2>&1; then
    gem install grpc-tools -v "$ruby_grpc_tools_version"
  fi

  grpc_tools_ruby_bin="$(gem env | grep "\- INSTALLATION DIRECTORY" | awk '{print $4}')/gems/grpc-tools-$ruby_grpc_tools_version/bin/grpc_tools_ruby_protoc"
  grpc_tools_ruby_plugin="$(gem env | grep "\- INSTALLATION DIRECTORY" | awk '{print $4}')/gems/grpc-tools-$ruby_grpc_tools_version/bin/grpc_tools_ruby_protoc_plugin"

  # Make ruby output directory if it doesn't exist.
  mkdir -p "$(get_repo_directory)$workDir/clients/ruby/lib/$(get_app_name)_client$SUBDIR"

  info_sub "Running Ruby protobuf generation"

  ruby_args=("${default_args[@]}")
  ruby_args+=(
    --plugin=grpc_tools_ruby_protoc_plugin="$grpc_tools_ruby_plugin"
    --ruby_out="$(get_repo_directory)$workDir/clients/ruby/lib/$(get_app_name)_client$SUBDIR"
    --grpc_out="$(get_repo_directory)$workDir/clients/ruby/lib/$(get_app_name)_client$SUBDIR"
    --proto_path "$(get_repo_directory)$workDir$SUBDIR"
  )

  "$grpc_tools_ruby_bin" "${ruby_args[@]}" "$(get_repo_directory)$workDir$SUBDIR/"*.proto

  # remove unsupported validate proto plugin
  if pushd "$(get_repo_directory)/api/clients/ruby/lib/$(get_app_name)_client$SUBDIR"; then
    find . -name '*.rb' | while read -r file; do
      delete_validate "$file"
    done
    popd || exit 1
  fi
fi
