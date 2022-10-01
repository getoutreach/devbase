#!/usr/bin/env bash
# Linters for terraform
# Note: This file does not support directories outside
# of deployments+monitoring currently.

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

terraform_fmt() {
  for tfdir in deployments monitoring; do
    if [[ ! -e $tfdir ]]; then
      continue
    fi

    terraform fmt "$tfdir"
  done
}

linter() {
  run_command "tflint" tflint
}

formatter() {
  run_command "terraform fmt" terraform_fmt
}
