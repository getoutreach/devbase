#!/bin/bash
# This file is ran when a devspace instance is started
set +e # Continue on errors
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/box.sh
source "$DIR/lib/box.sh"

GH_NO_UPDATE_NOTIFIER=true gh auth setup-git

# SSH -> HTTPS, since we're not using SSH keys
git config --global url.https://github.com/.insteadOf git@github.com:
# Trust /home/dev/app
git config --global --add safe.directory /home/dev/app

download_box

if [[ -n $NPM_TOKEN ]]; then
  # We actually don't want it to expand, we want it to be a literal string written to the file
  # shellcheck disable=SC2016
  # shellcheck disable=SC2086
  echo '//registry.npmjs.org/:_authToken=${NPM_TOKEN}' >"$HOME/.npmrc"
fi

if [[ -n $GH_TOKEN ]]; then
  # We actually don't want it to expand, we want it to be a literal string written to the file
  # shellcheck disable=SC2016
  # shellcheck disable=SC2086
  echo '//npm.pkg.github.com/:_authToken=${GH_TOKEN}' >>"$HOME/.npmrc"

  # We need bundler to be a thing so source ASDF
  # shellcheck disable=SC1090
  # shellcheck disable=SC1091
  . "$HOME/.asdf/asdf.sh"

  mkdir -p "$HOME/.bundle"
  {
    echo "---"
    echo "BUNDLE_HTTPS://RUBYGEMS__PKG__GITHUB__COM/GETOUTREACH/: \"x-access-token:$GH_TOKEN\""
  } >"$HOME/.bundle/config"
fi

# IDEA: Maybe do this in the image build?
# We actually don't want it to expand, we want it to be a literal string written to the file
# shellcheck disable=SC2016
# shellcheck disable=SC2086
if ! grep -q '. "$HOME/.asdf/asdf.sh"' "$HOME/.bashrc"; then
  # We actually don't want it to expand, we want it to be a literal string written to the file
  # shellcheck disable=SC2016
  # shellcheck disable=SC2086
  echo '. "$HOME/.asdf/asdf.sh"' >>"$HOME/.bashrc"
fi

echo "$(tput bold)Ensuring asdf plugins are installed$(tput sgr0)"
pushd /home/dev/app >/dev/null || exit 1
./.bootstrap/root/ensure_asdf.sh
popd >/dev/null || exit 1

COLOR_CYAN="\033[0;36m"
COLOR_RESET="\033[0m"

BANNER="${COLOR_CYAN}
   ____              ____
  |  _ \  _____   __/ ___| _ __   __ _  ___ ___
  | | | |/ _ \ \ / /\___ \| '_ \ / _\` |/ __/ _ \\
  | |_| |  __/\ V /  ___) | |_) | (_| | (_|  __/
  |____/ \___| \_/  |____/| .__/ \__,_|\___\___|
                          |_|
${COLOR_RESET}
Welcome to your development container!

This is how you can work with it:
- Run \`${COLOR_CYAN}make${COLOR_RESET}\` to build the application
- Run \`${COLOR_CYAN}make run${COLOR_RESET}\` to build and start the server
- Run \`${COLOR_CYAN}make dev${COLOR_RESET}\` to start the development server with reloading
- Run \`${COLOR_CYAN}make debug${COLOR_RESET}\` to start the development server with delve attached
- Try \`${COLOR_CYAN}make [run|dev|debug] LOGFMT=1${COLOR_RESET}\` for more dev friendly terminal logs
- ${COLOR_CYAN}Files will be synchronized${COLOR_RESET} between your local machine and this container
"

if [[ -z $DEV_CONTAINER_LOGFILE ]] || [[ $DEVENV_DEV_TERMINAL == "true" ]]; then
  echo -e "$BANNER"
  exec bash
elif [[ -n $E2E ]]; then
  # We need to have a git repo test files added so that make test can see them
  git init >/dev/null 2>&1
  git add -A >/dev/null 2>&1
  make test-e2e | tee -ai "${DEV_CONTAINER_LOGFILE:-/tmp/app.log}"
  exit "${PIPESTATUS[0]}"
else
  exec make dev
fi
