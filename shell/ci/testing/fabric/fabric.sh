#!/bin/bash

# Check if authenticated to GitHub
if ! gh auth status >/dev/null 2>&1; then
  echo "Not authenticated to GitHub. Please run 'gh auth login' first."
  exit 1
else
  access_token=$(gh auth token)
  echo "Authenticated to GitHub."
  echo "Access token: $access_token"
fi

# Function to download a repository
download_repo() {
  local repo_url=$1
  local repo_name
  repo_name=$(basename "$repo_url" .git)
  git clone "$repo_url" "$repo_name"
}

# Function to run a command in a repository
run_command() {
  local repo_path=$1
  local command=$2
  local output_dir=$3
  local exit_code_file=$output_dir/exit_code.txt
  local output_file=$output_dir/output.txt

  cd "$repo_path" || exit 1

  # Run the command and store the output and exit code
  $command > "$output_file" 2>&1
  echo $? > "$exit_code_file"
}

# Function to modify the stencil.lock file
modify_stencil_lock() {
  local repo_path=$1
  local branch_name=$2

  cd "$repo_path" || exit 1

  # Modify the devbase version in stencil.lock
  sed -i "s/devbase:.*/devbase: $branch_name/" stencil.lock
}

# Function to compare the output and exit code between runs
compare_results() {
  local output_dir=$1
  local diff_count=0

  for dir in "$output_dir"/*/; do
    local exit_code_file=$dir/exit_code.txt
    local output_file=$dir/output.txt

    # Compare the exit code
    if [[ $(cat "$exit_code_file") != 0 ]]; then
      ((diff_count++))
    fi

    # Compare the output
    if ! diff -q "$output_file" "$output_dir"/baseline/output.txt >/dev/null; then
      ((diff_count++))
    fi
  done

  echo "Number of repos with different output/exit code: $diff_count"
}

# Main script

# Set the command to run in the repositories
command="make lint"

# Set the branch name for devbase version change
branch_name="golangci-lint"

# Create a temporary directory to store the output
output_dir=$(mktemp -d)

# Get the list of repositories (1707 currently lol) in the organization
repo_list=$(curl -s -H "Authorization: Bearer $access_token" "https://api.github.com/orgs/getoutreach/repos?per_page=10&page=9" | jq -r '.[].name')

stenciled_repos=()

# Create a temporary file
tempfile=$(mktemp)

# Loop through each repository
for repo_name in $repo_list; do
  (
    if (( $(curl -s -H "Authorization: Bearer $access_token" "https://api.github.com/repos/getoutreach/$repo_name/contents/stencil.lock" |  grep -c "stencil.lock") > 0 )); then
      echo "$repo_name is stenciled"
      echo "$repo_name" >> "$tempfile"
    else
      echo "$repo_name is not stenciled"
    fi
  ) &
done
wait

# Read the stenciled repos from the temporary file into an array
stenciled_repos=()
while IFS= read -r line; do
  stenciled_repos+=("$line")
done < "$tempfile"

# Remove the temporary file
rm "$tempfile"

echo "Stenciled repos: ${stenciled_repos[*]}"

  # if download_url=$(curl -s -H "Authorization: Bearer $access_token" "https://api.github.com/repos/getoutreach/$repo_name/contents/stencil.lock" | jq -r '.download_url'); then

  # # Download the repository
  # download_repo "$repo_url"
  # echo "Downloaded repository: $(basename "$repo_url" .git)"

  # # Get the repository name
  # repo_name=$(basename "$repo_url" .git)
  # echo "Processing repository: $repo_name"

  # # Run the command and store the output and exit code
  # run_command "$repo_name" "$command" "$output_dir/baseline"
  # echo "Executed command in: $repo_name"

  # # Modify the stencil.lock file
  # modify_stencil_lock "$repo_name" "$branch_name"
  # echo "Modified stencil.lock in: $repo_name"

  # # Run the command again and store the output and exit code
  # run_command "$repo_name" "$command" "$output_dir/modified"
  # echo "Executed \"$command\" in: $repo_name"
# done

# # Compare the output and exit code between runs
# compare_results "$output_dir"
