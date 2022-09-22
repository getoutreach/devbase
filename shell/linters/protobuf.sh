#!/usr/bin/env bash
# Linters for protobuf
PROTOFMT=$("$DIR/gobin.sh" -p github.com/bufbuild/buf/cmd/buf@v"$(get_tool_version "buf")")

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(proto)

buf_linter() {
  git ls-files '*.proto' | xargs -n40 "$PROTOFMT" format --exit-code
}

buf_formatter() {
  git ls-files '*.proto' | xargs -n40 "$PROTOFMT" format -w
}

linter() {
  run_command "buf" buf_linter
}

formatter() {
  run_command "buf" buf
}
