#!/bin/bash

set +e # Continue on errors
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

gh auth setup-git

# IDEA: Maybe do this in the image build?
# We actually don't want it to expand, we want it to be a literal string written to the file
# shellcheck disable=SC2086
if ! grep -q '. "$HOME/.asdf/asdf.sh"' "$HOME/.bashrc"; then
  # We actually don't want it to expand, we want it to be a literal string written to the file
  # shellcheck disable=SC2086
  echo '. "$HOME/.asdf/asdf.sh"' >>"$HOME/.bashrc"
fi

COLOR_CYAN="\033[0;36m"
COLOR_RESET="\033[0m"

APPNAME="$(get_app_name)"

echo -e "${COLOR_CYAN}
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
- Run \`${COLOR_CYAN}make dev${COLOR_RESET}\` to start the development server with reloading
- Run \`${COLOR_CYAN}./bin/${APPNAME}${COLOR_RESET}\` to start the server
- ${COLOR_CYAN}Files will be synchronized${COLOR_RESET} between your local machine and this container
- Some ports will be forwarded.
"

bash
