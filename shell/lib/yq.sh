#!/usr/bin/env bash
# Helpers for shell/yq.sh. Assumes logging.sh has been sourced already.
#
# gojq's --yaml-input/--yaml-output mode does not support every flag that
# python-yq supports (no -Y/--yaml-roundtrip comment+style+anchor
# preservation, no -i/--in-place, no --width, no XML/TOML mode, etc). This
# wrapper always prefers gojq over python-yq when gojq is installed (see
# shell/yq.sh), so the errors below tell the user to invoke python-yq
# directly rather than suggesting it as a fallback this wrapper would use
# on its own.
#
# This is a hardcoded denylist, not derived from python-yq itself. If
# python-yq is upgraded, re-diff this list against its current --help/
# argparse flags (see yq/yq/parser.py upstream) for anything new. Keep this
# file in sync with its sibling in getoutreach/orc at
# internal/steps/scripts/embed/yq.sh.

# These are long-form python-yq flags with no gojq equivalent. python-yq
# forwards any flag it doesn't recognize itself straight to jq (it uses
# argparse's parse_known_args), so --sort-keys and --ascii-output reach jq
# today even though yq's own parser never mentions them; gojq has neither.
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
  ["--sort-keys"]=1
  ["--ascii-output"]=1
)

# Single-letter equivalents of some of the flags above, which python-yq also
# allows bundled with other short flags (e.g. `-ni`), mapped to a
# human-readable name for the error message. Lowercase -y (--yaml-output) is
# excluded: gojq supports that flag too, just not under its short spelling,
# so normalize_yq_args rewrites it instead of rejecting it.
declare -Ag UNSUPPORTED_YQ_SHORT_FLAGS=(
  [Y]="-Y/--yaml-roundtrip"
  [w]="-w/--width"
  [x]="-x/--xml-output"
  [t]="-t/--toml-output"
  [T]="-T/--toml-roundtrip"
  [i]="-i/--in-place"
  [S]="-S/--sort-keys"
  [a]="-a/--ascii-output"
)

# Short flags gojq only recognizes by their long name. Bare or bundled
# (e.g. `-ry`), normalize_yq_args rewrites these out of a short-flag cluster
# into their long form.
declare -Ag YQ_SHORT_TO_LONG_FLAGS=(
  [y]="--yaml-output"
)

# normalize_yq_args rewrites any flag in YQ_SHORT_TO_LONG_FLAGS found in a
# short-flag cluster into its long form. Sets the global array
# NORMALIZED_YQ_ARGS. Arguments after a `--` separator pass through
# untouched.
normalize_yq_args() {
  NORMALIZED_YQ_ARGS=()
  local arg stripped char i replacement stop=0
  for arg in "$@"; do
    if ((stop)); then
      NORMALIZED_YQ_ARGS+=("$arg")
      continue
    fi
    if [[ $arg == "--" ]]; then
      stop=1
      NORMALIZED_YQ_ARGS+=("$arg")
      continue
    fi
    if [[ $arg == -[!-]* ]]; then
      replacement=""
      stripped=""
      for ((i = 1; i < ${#arg}; i++)); do
        char="${arg:i:1}"
        if [[ -v YQ_SHORT_TO_LONG_FLAGS[$char] ]]; then
          replacement="${YQ_SHORT_TO_LONG_FLAGS[$char]}"
        else
          stripped+="$char"
        fi
      done
      if [[ -n $replacement ]]; then
        [[ -n $stripped ]] && NORMALIZED_YQ_ARGS+=("-$stripped")
        NORMALIZED_YQ_ARGS+=("$replacement")
        continue
      fi
    fi
    NORMALIZED_YQ_ARGS+=("$arg")
  done
}

# suggest_in_place_command prints a gojq-based workaround for a rejected
# -i/--in-place invocation, since gojq has no in-place mode, using a temp
# file so a failed edit doesn't corrupt the original. It returns 0 on
# success, or 1 if it can't confidently build one, in which case the caller
# falls back to the generic error. It refuses to guess if another
# unsupported flag rode along with -i (the suggested command would fail
# too) or if fewer than two non-flag arguments are left (there's no filter
# and file to build a suggestion from, as in `yq -ni <filter>` with no
# file). This assumes the common single-file usage (`yq -i '<filter>'
# <file>`): the last remaining argument is treated as the target file.
# python-yq's -i also supports multiple files edited independently; this
# does not reconstruct that.
suggest_in_place_command() {
  local args=() arg stripped file qfile rest positional=0 char j
  for arg in "$@"; do
    case "$arg" in
    --in-place | --in-place=*) continue ;;
    -i) continue ;;
    # Already implied by the --yaml-output this function always prepends
    # below; arguments reach here post-normalize_yq_args, so a bundled -y
    # (e.g. -yi) has already become this exact standalone flag.
    --yaml-output) continue ;;
    -[!-]*i*)
      stripped="${arg//i/}"
      [[ $stripped == "-" ]] && continue
      args+=("$stripped")
      ;;
    *) args+=("$arg") ;;
    esac
  done

  for arg in "${args[@]}"; do
    if [[ $arg == -* ]]; then
      stripped="${arg%%=*}"
      [[ -v UNSUPPORTED_YQ_LONG_FLAGS[$stripped] ]] && return 1
      if [[ $arg == -[!-]* ]]; then
        for ((j = 1; j < ${#arg}; j++)); do
          char="${arg:j:1}"
          [[ -v UNSUPPORTED_YQ_SHORT_FLAGS[$char] ]] && return 1
        done
      fi
    else
      ((++positional))
    fi
  done

  # Need at least a filter and one file left to make a useful suggestion.
  [[ $positional -ge 2 ]] || return 1

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
#
# It normalizes its arguments first (see normalize_yq_args) and leaves the
# result in NORMALIZED_YQ_ARGS for the caller to actually invoke gojq with.
check_unsupported_yq_flags() {
  normalize_yq_args "$@"
  local arg stripped char i
  for arg in "${NORMALIZED_YQ_ARGS[@]}"; do
    if [[ $arg == "--" ]]; then
      break
    fi
    stripped="${arg%%=*}"
    if [[ -v UNSUPPORTED_YQ_LONG_FLAGS[$stripped] ]]; then
      if [[ $stripped == "--in-place" ]] && suggest_in_place_command "${NORMALIZED_YQ_ARGS[@]}"; then
        exit 1
      fi
      reject_flag "$stripped"
    fi
    if [[ $arg == -[!-]* ]]; then
      for ((i = 1; i < ${#arg}; i++)); do
        char="${arg:i:1}"
        if [[ -v UNSUPPORTED_YQ_SHORT_FLAGS[$char] ]]; then
          if [[ $char == "i" ]] && suggest_in_place_command "${NORMALIZED_YQ_ARGS[@]}"; then
            exit 1
          fi
          reject_flag "-$char (${UNSUPPORTED_YQ_SHORT_FLAGS[$char]})"
        fi
      done
    fi
  done
}
