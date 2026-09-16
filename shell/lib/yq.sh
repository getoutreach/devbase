#!/usr/bin/env bash
# Helpers for shell/yq.sh. Assumes logging.sh has been sourced already.
#
# gojq's --yaml-input/--yaml-output mode does not support every flag that
# python-yq supports (no -Y/--yaml-roundtrip comment+style+anchor
# preservation, no -i/--in-place, no --width, no XML/TOML mode, etc). gojq's
# own flag parser already rejects these with an "unknown flag" error, but
# check_unsupported_yq_flags gives a clearer, specific error before we even
# invoke gojq, naming the flag and pointing at python-yq as the fallback.
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

# check_unsupported_yq_flags fails with a specific error if any argument is a
# python-yq-only flag, instead of letting gojq reject it with a generic
# "unknown flag" error.
check_unsupported_yq_flags() {
  local arg stripped char i
  for arg in "$@"; do
    if [[ $arg == "--" ]]; then
      break
    fi
    stripped="${arg%%=*}"
    case " ${UNSUPPORTED_YQ_LONG_FLAGS[*]} " in
    *" $stripped "*)
      fatal "yq flag '$stripped' is not supported by gojq. Install python-yq if you need this feature."
      ;;
    esac
    if [[ $arg == -[!-]* ]]; then
      for ((i = 1; i < ${#arg}; i++)); do
        char="${arg:i:1}"
        if [[ -v UNSUPPORTED_YQ_SHORT_FLAGS[$char] ]]; then
          fatal "yq flag '-$char' (${UNSUPPORTED_YQ_SHORT_FLAGS[$char]}) is not supported by gojq." \
            "Install python-yq if you need this feature."
        fi
      done
    fi
  done
}
