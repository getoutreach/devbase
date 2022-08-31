#!/usr/bin/env bash
# Linters for terraform

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(tf tfvars)

tflint() {
  for tfdir in deployments monitoring; do
    if [[ ! -e $tfdir ]]; then
      continue
    fi

    if ! terraform fmt -diff -check "$tfdir"; then
      error "terraform fmt (./$tfdir) failed on some files. Run 'make fmt' to fix."
      exit 1
    fi
  done
}

linter() {
  run_linter "tflint" tflint
}
