#!/usr/bin/env bash
# Helper functions for macos/scripts/release.sh. Source this file; do not run it.
# Tests: macos/scripts/tests/release-lib.test.sh

# Print MARKETING_VERSION from an xcodegen project.yml.
release_marketing_version() {
  local project_yml=$1 version
  version="$(sed -n -E 's/^[[:space:]]*MARKETING_VERSION:[[:space:]]*"?([^"[:space:]]+)"?[[:space:]]*$/\1/p' "$project_yml" | head -1)"
  if [[ -z "$version" ]]; then
    echo "release-lib: MARKETING_VERSION not found in $project_yml" >&2
    return 1
  fi
  printf '%s\n' "$version"
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

# Print the build number for a checkout: the number of commits reachable from HEAD.
release_build_number() {
  local repo=$1
  git -C "$repo" rev-list --count HEAD
}

html_escape() {
  sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' -e 's/"/\&quot;/g'
}

# Read one change per line on stdin and print a standalone HTML release notes page.
# A full document (with DOCTYPE) makes generate_appcast publish it as a
# sparkle:releaseNotesLink instead of embedding it.
release_notes_html() {
  local version=$1 items="" line
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    items+="    <li>$(printf '%s' "$line" | html_escape)</li>"$'\n'
  done
  cat <<HTML
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Handbyte $version</title>
<style>
  body { font: 14px/1.5 -apple-system, system-ui, sans-serif; margin: 20px; color: #1d1d1f; }
  h1 { font-size: 18px; margin: 0 0 12px; }
  ul { padding-left: 20px; }
  @media (prefers-color-scheme: dark) { body { color: #f5f5f7; background: #1e1e1e; } }
</style>
</head>
<body>
<h1>Handbyte $version</h1>
HTML
  if [[ -n "$items" ]]; then
    printf '<ul>\n%s</ul>\n' "$items"
  else
    printf '<p>No changes recorded.</p>\n'
  fi
  cat <<HTML
</body>
</html>
HTML
}
