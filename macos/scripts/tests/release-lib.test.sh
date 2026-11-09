#!/usr/bin/env bash
# Tests for macos/scripts/release-lib.sh. Run: make -C macos test-scripts
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../release-lib.sh
source "$HERE/../release-lib.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
FAILED=0

assert_eq() {
  local name=$1 expected=$2 actual=$3
  if [[ "$expected" == "$actual" ]]; then
    echo "ok   $name"
  else
    echo "FAIL $name"
    echo "     expected: $expected"
    echo "     actual:   $actual"
    FAILED=1
  fi
}

assert_fails() {
  local name=$1
  shift
  if "$@" >/dev/null 2>&1; then
    echo "FAIL $name (command succeeded)"
    FAILED=1
  else
    echo "ok   $name"
  fi
}

# --- project.yml settings --------------------------------------------------
cat > "$TMP/project.yml" <<'YML'
targets:
  Handbyte:
    settings:
      base:
        CURRENT_PROJECT_VERSION: "20261109001"
        MARKETING_VERSION: "0.3.1"
YML
assert_eq "marketing version is read from project.yml" \
  "0.3.1" "$(release_marketing_version "$TMP/project.yml")"
assert_eq "build number is read from project.yml" \
  "20261109001" "$(release_build_number "$TMP/project.yml")"

printf 'name: X\n' > "$TMP/empty.yml"
assert_fails "marketing version fails when the key is missing" \
  release_marketing_version "$TMP/empty.yml"

# --- release_version_from_tag --------------------------------------------
assert_eq "app tag maps to its version" \
  "1.2.3" "$(release_version_from_tag "app/v1.2.3")"
assert_eq "pre-release suffix is preserved" \
  "1.2.3-beta.1" "$(release_version_from_tag "app/v1.2.3-beta.1")"
assert_fails "CLI tag is rejected" release_version_from_tag "v1.2.3"
assert_fails "non-numeric version is rejected" release_version_from_tag "app/vlatest"

# --- release_next_build ------------------------------------------------------
assert_eq "first build of a day starts at 001" \
  "20261109001" "$(release_next_build 1 "" 20261109)"
assert_eq "same-day build continues the sequence" \
  "20261109003" "$(release_next_build 20261109002 "" 20261109)"
assert_eq "published build is the floor when it is higher" \
  "20261109006" "$(release_next_build 20261109002 20261109005 20261109)"
assert_eq "new day resets the sequence" \
  "20261110001" "$(release_next_build 20261109007 20261109009 20261110)"

# --- release_appcast_max_build ------------------------------------------------
cat > "$TMP/appcast.xml" <<'XML'
<item><sparkle:version>20261101002</sparkle:version></item>
<item><sparkle:version>20261109001</sparkle:version></item>
XML
assert_eq "highest published build is found" \
  "20261109001" "$(release_appcast_max_build "$TMP/appcast.xml")"
assert_eq "missing appcast yields no build" \
  "" "$(release_appcast_max_build "$TMP/none.xml")"

# --- release_changelog_section ------------------------------------------------
cat > "$TMP/CHANGELOG.md" <<'MD'
# Changelog

## [Unreleased]

### Added

## [0.2.0] - 2026-11-20

### Added

- **Second** release.

## [0.1.0] - 2026-11-09

### Added

- **First** release.
- Another line.
MD
assert_eq "changelog section is extracted without its heading" \
  "$(printf '### Added\n\n- **Second** release.')" \
  "$(release_changelog_section "$TMP/CHANGELOG.md" 0.2.0)"
assert_eq "last changelog section runs to the end of the file" \
  "$(printf '### Added\n\n- **First** release.\n- Another line.')" \
  "$(release_changelog_section "$TMP/CHANGELOG.md" 0.1.0)"
assert_fails "missing changelog section fails" \
  release_changelog_section "$TMP/CHANGELOG.md" 9.9.9
assert_fails "empty Unreleased section fails" \
  release_changelog_section "$TMP/CHANGELOG.md" Unreleased

# --- release.sh argument parsing -------------------------------------------
# Parsing happens before release.sh touches the file system, so this is safe.
set +e
bash "$HERE/../release.sh" --version >"$TMP/usage.out" 2>&1
STATUS=$?
set -e
assert_eq "release.sh --version without a value exits 2" "2" "$STATUS"
case "$(cat "$TMP/usage.out")" in
  *"Usage: release.sh"*) echo "ok   release.sh --version without a value prints usage" ;;
  *) echo "FAIL release.sh --version without a value prints usage"; cat "$TMP/usage.out"; FAILED=1 ;;
esac

if [[ $FAILED -ne 0 ]]; then
  echo "release-lib tests FAILED"
  exit 1
fi
echo "release-lib tests passed"
