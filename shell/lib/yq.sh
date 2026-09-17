#!/usr/bin/env bash
# Helpers for shell/yq.sh. Assumes logging.sh has been sourced already.
#
# gojq's --yaml-input/--yaml-output mode does not support every flag that
# python-yq supports (no -Y/--yaml-roundtrip comment+style+anchor
# preservation, no -i/--in-place, no --width, no XML/TOML mode, etc). gojq's
# own flag parser already rejects these with an "unknown flag" error, but
# check_unsupported_yq_flags gives a clearer, specific error before we even
# invoke gojq, naming the flag. This wrapper always prefers gojq over
# python-yq when gojq is installed (see shell/yq.sh), so these errors tell
# the user to invoke python-yq directly rather than suggesting it as a
# fallback this wrapper would use on its own.
#
# This is a hardcoded denylist, not derived from python-yq itself. If
# python-yq is upgraded, re-diff this list against its current --help/
# argparse flags (see yq/yq/parser.py upstream) for anything new. Keep this
# file in sync with its sibling in getoutreach/orc at
# internal/steps/scripts/embed/yq.sh.

# Long-form python-yq flags with no gojq equivalent.
declare -Ag UNSUPPORTED_YQ_LONG_FLAGS=(
  ["--yaml-roundtrip"]=1 ["--yml-roundtrip"]=1
  ["--yaml-output-grammar-version"]=1 ["--yml-out-ver"]=1
  ["--width"]=1
  ["--indentless-lists"]=1 ["--indentless"]=1
  ["--explicit-start"]=1 ["--explicit-end"]=1
  ["--no-expand-aliases"]=1
  ["--max-expansion-factor"]=1
  ["--xml-output"]=1 ["--xml-item-depth"]=1 ["--xml-dtd"]=1 ["--xml-root"]=1 ["--xml-force-list"]=1 ["--xml-short-empty-elements"]=1
  ["--toml-output"]=1 ["--toml-roundtrip"]=1
  ["--in-place"]=1
)

# Single-letter equivalents of some of the flags above, which python-yq also
# allows bundled with other short flags (e.g. `-ni`), mapped to a
# human-readable name for the error message. Note: lowercase -y
# (--yaml-output) is deliberately excluded, since gojq supports it too.
declare -Ag UNSUPPORTED_YQ_SHORT_FLAGS=(
  [Y]="-Y/--yaml-roundtrip"
  [w]="-w/--width"
  [x]="-x/--xml-output"
  [t]="-t/--toml-output"
  [T]="-T/--toml-roundtrip"
  [i]="-i/--in-place"
)

# suggest_in_place_command prints a gojq-based equivalent for a rejected
# -i/--in-place invocation and returns 0, or returns 1 if it can't
# confidently build one (the caller then falls back to the generic error).
# gojq has no in-place mode; the suggestion writes to a temp file and moves
# it over the original, the standard safe way to edit a file "in place"
# without a dedicated flag. This assumes the common single-file usage
# (`yq -i '<filter>' <file>`): the last remaining argument, after removing
# -i/--in-place, is treated as the target file. python-yq's -i also
# supports multiple files edited independently; this suggestion does not
# reconstruct that for each file.
suggest_in_place_command() {
  local args=() arg stripped file qfile rest
  for arg in "$@"; do
    case "$arg" in
    --in-place | --in-place=*) continue ;;
    -i) continue ;;
    -[!-]*i*)
      stripped="${arg//i/}"
      [[ $stripped == "-" ]] && continue
      args+=("$stripped")
      ;;
    *) args+=("$arg") ;;
    esac
  done

  # Need at least a filter and one file left to make a useful suggestion.
  [[ ${#args[@]} -ge 2 ]] || return 1

  file="${args[-1]}"
  qfile="$(printf '%q' "$file")"
  rest="$(printf '%q ' --yaml-input --yaml-output "${args[@]}")"

  error "yq flag '-i'/'--in-place' is not supported by gojq. For the common single-file case, use a temp file instead:"
  printf '  gojq %s> %s.tmp && mv %s.tmp %s\n' "$rest" "$qfile" "$qfile" "$qfile" >&2
}

# reject_flag reports that a yq flag (given as a human-readable display,
# e.g. '--width' or '-Y (-Y/--yaml-roundtrip)') is not supported by gojq,
# and exits.
reject_flag() {
  fatal "yq flag '$1' is not supported by gojq, and this wrapper always prefers gojq over python-yq" \
    "when gojq is installed. Invoke python-yq directly if you need this feature."
}

# check_unsupported_yq_flags fails with a specific error if any argument is a
# python-yq-only flag, instead of letting gojq reject it with a generic
# "unknown flag" error. For -i/--in-place specifically, it also tries to
# suggest a working gojq-based equivalent, since this wrapper never invokes
# python-yq itself while gojq is installed.
check_unsupported_yq_flags() {
  local arg stripped char i
  for arg in "$@"; do
    if [[ $arg == "--" ]]; then
      break
    fi
    stripped="${arg%%=*}"
    if [[ -v UNSUPPORTED_YQ_LONG_FLAGS[$stripped] ]]; then
      if [[ $stripped == "--in-place" ]] && suggest_in_place_command "$@"; then
        exit 1
      fi
      reject_flag "$stripped"
    fi
    if [[ $arg == -[!-]* ]]; then
      for ((i = 1; i < ${#arg}; i++)); do
        char="${arg:i:1}"
        if [[ -v UNSUPPORTED_YQ_SHORT_FLAGS[$char] ]]; then
          if [[ $char == "i" ]] && suggest_in_place_command "$@"; then
            exit 1
          fi
          reject_flag "-$char (${UNSUPPORTED_YQ_SHORT_FLAGS[$char]})"
        fi
      done
    fi
  done
}
