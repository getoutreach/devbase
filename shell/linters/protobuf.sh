#!/usr/bin/env bash
# Linters for protobuf
PROTOFMT=$("$DIR/gobin.sh" -p github.com/bufbuild/buf/cmd/buf@v"$(get_tool_version "buf")")

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(proto)

buf() {
  if ! "$PROTOFMT" format --exit-code >/dev/null 2>&1; then
    error "protobuf format (buf format) failed on some files. Run 'make fmt' to fix."
    exit 1
  fi
}

linter() {
  run_linter "buf" buf
}
