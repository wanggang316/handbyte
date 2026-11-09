#!/usr/bin/env bash
# Helper functions for macos/scripts/release.sh and bump-version.sh. Source
# this file; do not run it. Tests: macos/scripts/tests/release-lib.test.sh

# Print one quoted build setting from an xcodegen project.yml.
release_project_setting() {
  local project_yml=$1 key=$2 value
  value="$(sed -n -E "s/^[[:space:]]*$key:[[:space:]]*\"?([^\"[:space:]]+)\"?[[:space:]]*\$/\1/p" "$project_yml" | head -1)"
  if [[ -z "$value" ]]; then
    echo "release-lib: $key not found in $project_yml" >&2
    return 1
  fi
  printf '%s\n' "$value"
}

# Print MARKETING_VERSION (X.Y.Z) from project.yml.
release_marketing_version() {
  release_project_setting "$1" MARKETING_VERSION
}

# Print CURRENT_PROJECT_VERSION (the Sparkle build number) from project.yml.
release_build_number() {
  release_project_setting "$1" CURRENT_PROJECT_VERSION
}

# Print the version part of an app release tag such as app/v1.2.3.
release_version_from_tag() {
  local tag=$1
  if [[ "$tag" =~ ^app/v([0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.]+)?)$ ]]; then
    printf '%s\n' "${BASH_REMATCH[1]}"
  else
    echo "release-lib: '$tag' is not an app release tag (expected app/vX.Y.Z)" >&2
    return 1
  fi
}

# Print the next build number in the YYYYMMDDNNN scheme.
#   release_next_build <current> <published-max-or-empty> <today YYYYMMDD>
# The higher of the local and the published build is the floor. On the same
# day the sequence continues; on a new day it starts again at 001. Sparkle
# compares this number, so it must grow with every published build, also when
# MARKETING_VERSION does not change.
release_next_build() {
  local current=$1 published=${2:-} today=$3
  local floor=$current
  if [[ -n "$published" ]] && (( published > floor )); then
    floor=$published
  fi
  if (( floor >= today * 1000 )); then
    printf '%d\n' $((floor + 1))
  else
    printf '%s001\n' "$today"
  fi
}

# Print the highest <sparkle:version> in an appcast file, or nothing.
release_appcast_max_build() {
  local appcast=$1
  [[ -f "$appcast" ]] || return 0
  grep -oE '<sparkle:version>[^<]+' "$appcast" | sed 's/<sparkle:version>//' | sort -n | tail -1 || true
}

# Print the body of the "## [X.Y.Z]" section of a Keep a Changelog file,
# without its heading and without leading or trailing blank lines. A section
# with category headers but no "- " entries counts as missing.
release_changelog_section() {
  local changelog=$1 version=$2 body
  body="$(awk -v v="$version" '
    /^## \[/ { if (found) exit; if (index($0, "## [" v "]") == 1) { found = 1; next } }
    found { print }
  ' "$changelog" | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}' | sed '/./,$!d')"
  if ! grep -q '^- ' <<<"$body"; then
    echo "release-lib: no '## [$version]' section with entries in $changelog" >&2
    return 1
  fi
  printf '%s\n' "$body"
}
