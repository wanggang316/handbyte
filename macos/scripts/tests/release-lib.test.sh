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

# --- release_marketing_version -------------------------------------------
cat > "$TMP/project.yml" <<'YML'
targets:
  Handbyte:
    settings:
      base:
        CURRENT_PROJECT_VERSION: "1"
        MARKETING_VERSION: "0.3.1"
YML
assert_eq "marketing version is read from project.yml" \
  "0.3.1" "$(release_marketing_version "$TMP/project.yml")"

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

# --- release_build_number --------------------------------------------------
git -C "$TMP" init -q repo
for i in 1 2 3; do
  git -C "$TMP/repo" -c user.name=t -c user.email=t@t commit -q --allow-empty -m "c$i"
done
assert_eq "build number is the commit count" \
  "3" "$(release_build_number "$TMP/repo")"

# --- release_notes_html ----------------------------------------------------
NOTES="$(printf 'Fix <Trash> move\nAdd "Put Back" & journal\n' | release_notes_html "1.2.3")"
assert_eq "release notes are a full HTML document" \
  "<!DOCTYPE html>" "$(printf '%s\n' "$NOTES" | head -1)"
case "$NOTES" in
  *"<title>Handbyte 1.2.3</title>"*) echo "ok   release notes carry the version in the title" ;;
  *) echo "FAIL release notes carry the version in the title"; FAILED=1 ;;
esac
case "$NOTES" in
  *"<li>Fix &lt;Trash&gt; move</li>"*"<li>Add &quot;Put Back&quot; &amp; journal</li>"*)
    echo "ok   release notes escape HTML in commit subjects" ;;
  *) echo "FAIL release notes escape HTML in commit subjects"; printf '%s\n' "$NOTES"; FAILED=1 ;;
esac
EMPTY="$(printf '' | release_notes_html "1.2.3")"
case "$EMPTY" in
  *"<li>"*) echo "FAIL empty input yields no list items"; FAILED=1 ;;
  *"No changes recorded."*) echo "ok   empty input yields a placeholder paragraph" ;;
  *) echo "FAIL empty input yields a placeholder paragraph"; FAILED=1 ;;
esac

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
