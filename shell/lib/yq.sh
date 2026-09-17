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
# argparse flags (see yq/yq/parser.py upstream) for anything new.

# Long-form python-yq flags with no gojq equivalent.
UNSUPPORTED_YQ_LONG_FLAGS=(
  --yaml-roundtrip --yml-roundtrip
  --yaml-output-grammar-version --yml-out-ver
  --width
  --indentless-lists --indentless
  --explicit-start --explicit-end
  --no-expand-aliases
  --max-expansion-factor
  --xml-output --xml-item-depth --xml-dtd --xml-root --xml-force-list --xml-short-empty-elements
  --toml-output --toml-roundtrip
  --in-place
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
  local args=() arg stripped file rest
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
  rest="$(printf '%q ' --yaml-input --yaml-output "${args[@]}")"

  error "yq flag '-i'/'--in-place' is not supported by gojq, and this wrapper always prefers gojq over python-yq" \
    "when gojq is installed. Invoke python-yq directly for real in-place editing."
  echo "Or, for the common single-file case, use gojq directly instead:" >&2
  printf '  gojq %s> %q.tmp && mv %q.tmp %q\n' "$rest" "$file" "$file" "$file" >&2
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
    case " ${UNSUPPORTED_YQ_LONG_FLAGS[*]} " in
    *" $stripped "*)
      if [[ $stripped == "--in-place" ]] && suggest_in_place_command "$@"; then
        exit 1
      fi
      fatal "yq flag '$stripped' is not supported by gojq, and this wrapper always prefers gojq over python-yq" \
        "when gojq is installed. Invoke python-yq directly if you need this feature."
      ;;
    esac
    if [[ $arg == -[!-]* ]]; then
      for ((i = 1; i < ${#arg}; i++)); do
        char="${arg:i:1}"
        if [[ -v UNSUPPORTED_YQ_SHORT_FLAGS[$char] ]]; then
          if [[ $char == "i" ]] && suggest_in_place_command "$@"; then
            exit 1
          fi
          fatal "yq flag '-$char' (${UNSUPPORTED_YQ_SHORT_FLAGS[$char]}) is not supported by gojq, and this" \
            "wrapper always prefers gojq over python-yq when gojq is installed. Invoke python-yq directly" \
            "if you need this feature."
        fi
      done
    fi
  done
}
