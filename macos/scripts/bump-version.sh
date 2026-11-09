#!/usr/bin/env bash
# Set MARKETING_VERSION and CURRENT_PROJECT_VERSION in macos/project.yml.
#
# project.yml is the single source of truth the release workflow checks
# against: tag app/vX.Y.Z must match MARKETING_VERSION. The build number uses
# the YYYYMMDDNNN scheme (20261109001, 20261109002, ...); the published
# appcast supplies the highest build already shipped so a new build is always
# higher, even after a squash merge or on another branch.
#
# Usage:
#   bump-version.sh <X.Y.Z>          # version; build = next in today's sequence
#   bump-version.sh <X.Y.Z> <BUILD>  # set both explicitly (BUILD > current)
#   bump-version.sh --print          # show current values and the next candidates
#
# Environment:
#   APPCAST_URL  Published feed to read the highest build from
#                (default: https://handbyte.gumpw.com/appcast.xml)
set -euo pipefail

MACOS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=release-lib.sh
source "$MACOS_DIR/scripts/release-lib.sh"

PROJECT_YML="$MACOS_DIR/project.yml"
APPCAST_URL="${APPCAST_URL:-https://handbyte.gumpw.com/appcast.xml}"

die() { echo "bump-version: $*" >&2; exit 1; }

semver_check() {
  [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "version '$1' must match X.Y.Z"
}

# True when $1 > $2, both X.Y.Z.
semver_gt() {
  local a1 a2 a3 b1 b2 b3
  IFS=. read -r a1 a2 a3 <<<"$1"
  IFS=. read -r b1 b2 b3 <<<"$2"
  (( a1 != b1 )) && { (( a1 > b1 )); return; }
  (( a2 != b2 )) && { (( a2 > b2 )); return; }
  (( a3 > b3 ))
}

published_max_build() {
  local feed
  feed="$(mktemp)"
  if curl -fsSL --max-time 10 "$APPCAST_URL" -o "$feed" 2>/dev/null; then
    release_appcast_max_build "$feed"
  fi
  rm -f "$feed"
}

print_state() {
  local version build published next
  version="$(release_marketing_version "$PROJECT_YML")"
  build="$(release_build_number "$PROJECT_YML")"
  published="$(published_max_build)"
  next="$(release_next_build "$build" "$published" "$(date +%Y%m%d)")"
  IFS=. read -r x y z <<<"$version"
  cat <<EOT
project.yml: $PROJECT_YML

  MARKETING_VERSION       = $version
  CURRENT_PROJECT_VERSION = $build
  Published max build     = ${published:-<none>}

next build: $next
next patch: $x.$y.$((z + 1))
next minor: $x.$((y + 1)).0
next major: $((x + 1)).0.0
EOT
}

bump() {
  local new_version=$1 new_build=${2:-} version build published
  semver_check "$new_version"
  version="$(release_marketing_version "$PROJECT_YML")"
  build="$(release_build_number "$PROJECT_YML")"
  semver_gt "$new_version" "$version" || die "new version $new_version is not greater than $version"

  if [[ -z "$new_build" ]]; then
    published="$(published_max_build)"
    new_build="$(release_next_build "$build" "$published" "$(date +%Y%m%d)")"
  else
    [[ "$new_build" =~ ^[0-9]+$ ]] || die "build '$new_build' must be a positive integer"
  fi
  (( new_build > build )) || die "new build $new_build must be greater than $build"

  local tmp
  tmp="$(mktemp "$PROJECT_YML.XXXXXX")"
  sed -E \
    -e "s/^([[:space:]]*MARKETING_VERSION:[[:space:]]*)\"[^\"]*\"/\1\"$new_version\"/" \
    -e "s/^([[:space:]]*CURRENT_PROJECT_VERSION:[[:space:]]*)\"[^\"]*\"/\1\"$new_build\"/" \
    "$PROJECT_YML" > "$tmp"
  if [[ "$(release_marketing_version "$tmp")" != "$new_version" || "$(release_build_number "$tmp")" != "$new_build" ]]; then
    rm -f "$tmp"
    die "project.yml rewrite failed; file left unchanged"
  fi
  mv "$tmp" "$PROJECT_YML"
  echo "MARKETING_VERSION:       $version -> $new_version"
  echo "CURRENT_PROJECT_VERSION: $build -> $new_build"
}

case "${1:-}" in
  ""|-h|--help) sed -n '2,17p' "$0" ;;
  --print) print_state ;;
  *) bump "$@" ;;
esac
