#!/usr/bin/env bash
# Build a signed, notarized Handbyte release: archive, export, notarize,
# DMG and Sparkle appcast. Outputs go to macos/dist/.
#
# Usage: release.sh [--version X.Y.Z] [--notes FILE]
#
#   --version  Marketing version. Default: MARKETING_VERSION in project.yml.
#   --notes    Release notes file (.html or .md) for this version. Default: a
#              page generated from the commits since the previous app/v* tag.
#
# Environment:
#   DERIVED_DATA              Xcode derived data path (default: .dd)
#   CODESIGN_IDENTITY         Full Developer ID Application identity
#                             (default: the one found in the keychain)
#   NOTARY_KEYCHAIN_PROFILE   notarytool keychain profile (default: handbyte-notary)
#   NOTARY_API_KEY_PATH, NOTARY_API_KEY_ID, NOTARY_API_ISSUER
#                             App Store Connect API key; used instead of the
#                             keychain profile when all three are set (CI).
#   SPARKLE_PRIVATE_KEY       EdDSA private key for generate_appcast (CI).
#                             Default: the key in the login keychain.
#   DOWNLOAD_URL_PREFIX       Where the DMG is published
#                             (default: GitHub release for tag app/v$VERSION)
#   RELEASE_NOTES_URL_PREFIX  Where release notes are published
#                             (default: https://handbyte.gumpw.com/releases/)
#   PREVIOUS_APPCAST          Existing feed to extend (default: ../docs/appcast.xml)
#
# Notarization runs only when credentials exist; otherwise the step prints
# SKIPPED and the unnotarized artifacts are still produced.
set -euo pipefail

MACOS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_DIR="$(cd "$MACOS_DIR/.." && pwd)"
# shellcheck source=release-lib.sh
source "$MACOS_DIR/scripts/release-lib.sh"

VERSION=""
NOTES_FILE=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION=$2; shift 2 ;;
    --notes) NOTES_FILE=$2; shift 2 ;;
    -h|--help) sed -n '2,30p' "$0"; exit 0 ;;
    *) echo "release.sh: unknown argument '$1'" >&2; exit 2 ;;
  esac
done

DERIVED_DATA="${DERIVED_DATA:-.dd}"
NOTARY_KEYCHAIN_PROFILE="${NOTARY_KEYCHAIN_PROFILE:-handbyte-notary}"
RELEASE_NOTES_URL_PREFIX="${RELEASE_NOTES_URL_PREFIX:-https://handbyte.gumpw.com/releases/}"
PREVIOUS_APPCAST="${PREVIOUS_APPCAST:-$REPO_DIR/docs/appcast.xml}"
SCHEME=Handbyte
APP_NAME=Handbyte

DIST="$MACOS_DIR/dist"
BUILD="$DIST/build"
UPDATES="$DIST/updates"
ARCHIVE="$BUILD/$APP_NAME.xcarchive"
EXPORT="$BUILD/export"
APP="$EXPORT/$APP_NAME.app"

step() { printf '\n==> %s\n' "$*"; }
run_logged() {
  # run_logged <log-name> <command...>: keep xcodebuild noise in a log file.
  local log="$DIST/logs/$1.log"
  shift
  if ! "$@" >"$log" 2>&1; then
    echo "release.sh: command failed, last lines of $log:" >&2
    tail -n 40 "$log" >&2
    return 1
  fi
}

cd "$MACOS_DIR"
mkdir -p "$DIST/logs" "$UPDATES"

# --- version ---------------------------------------------------------------
PROJECT_VERSION="$(release_marketing_version "$MACOS_DIR/project.yml")"
if [[ -z "$VERSION" ]]; then
  VERSION="$PROJECT_VERSION"
elif [[ "$VERSION" != "$PROJECT_VERSION" ]]; then
  echo "release.sh: warning: --version $VERSION differs from project.yml MARKETING_VERSION $PROJECT_VERSION" >&2
fi
BUILD_NUMBER="$(release_build_number "$REPO_DIR")"
DOWNLOAD_URL_PREFIX="${DOWNLOAD_URL_PREFIX:-https://github.com/wanggang316/handbyte/releases/download/app/v$VERSION/}"
DMG="$UPDATES/$APP_NAME-$VERSION.dmg"
ZIP="$BUILD/$APP_NAME-$VERSION.zip"

step "Release $APP_NAME $VERSION (build $BUILD_NUMBER)"

# --- signing identity --------------------------------------------------------
if [[ -z "${CODESIGN_IDENTITY:-}" ]]; then
  CODESIGN_IDENTITY="$(security find-identity -v -p codesigning | sed -n 's/.*"\(Developer ID Application:[^"]*\)".*/\1/p' | head -1)"
fi
if [[ -z "$CODESIGN_IDENTITY" ]]; then
  echo "release.sh: no 'Developer ID Application' identity in the keychain" >&2
  exit 1
fi
echo "Signing identity: $CODESIGN_IDENTITY"

# --- notarization credentials ------------------------------------------------
NOTARY_ARGS=()
if [[ -n "${NOTARY_API_KEY_PATH:-}" && -n "${NOTARY_API_KEY_ID:-}" && -n "${NOTARY_API_ISSUER:-}" ]]; then
  NOTARY_ARGS=(--key "$NOTARY_API_KEY_PATH" --key-id "$NOTARY_API_KEY_ID" --issuer "$NOTARY_API_ISSUER")
  echo "Notarization: App Store Connect API key $NOTARY_API_KEY_ID"
elif xcrun notarytool history --keychain-profile "$NOTARY_KEYCHAIN_PROFILE" >/dev/null 2>&1; then
  NOTARY_ARGS=(--keychain-profile "$NOTARY_KEYCHAIN_PROFILE")
  echo "Notarization: keychain profile $NOTARY_KEYCHAIN_PROFILE"
else
  echo "Notarization: SKIPPED (no keychain profile '$NOTARY_KEYCHAIN_PROFILE' and no NOTARY_API_KEY_* variables)"
fi

notarize() {
  # notarize <submission> [<staple-target>]: submit, wait, staple.
  # An .app is submitted as a zip but stapled as a bundle, hence two paths.
  local submission=$1 target=${2:-$1}
  if [[ ${#NOTARY_ARGS[@]} -eq 0 ]]; then
    echo "SKIPPED notarization of $(basename "$target") (no credentials)"
    return 0
  fi
  xcrun notarytool submit "$submission" "${NOTARY_ARGS[@]}" --wait --timeout 30m
  xcrun stapler staple "$target"
  xcrun stapler validate "$target"
}

# --- archive + export --------------------------------------------------------
step "Generate Xcode project"
xcodegen generate

step "Archive ($SCHEME, Release)"
rm -rf "$ARCHIVE"
run_logged archive xcodebuild -scheme "$SCHEME" -configuration Release -destination 'platform=macOS' \
  -derivedDataPath "$DERIVED_DATA" -archivePath "$ARCHIVE" archive \
  MARKETING_VERSION="$VERSION" CURRENT_PROJECT_VERSION="$BUILD_NUMBER" \
  CODE_SIGN_IDENTITY="$CODESIGN_IDENTITY" OTHER_CODE_SIGN_FLAGS="--timestamp"

step "Export with Developer ID"
rm -rf "$EXPORT"
run_logged export xcodebuild -exportArchive -archivePath "$ARCHIVE" \
  -exportOptionsPlist "$MACOS_DIR/ExportOptions.plist" -exportPath "$EXPORT"
codesign --verify --deep --strict "$APP"
codesign -dvv "$APP" 2>&1 | grep -E '^(Identifier|Authority|TeamIdentifier|CodeDirectory)' || true

# --- notarize + staple the app ------------------------------------------------
step "Notarize app"
rm -f "$ZIP"
ditto -c -k --keepParent "$APP" "$ZIP"
notarize "$ZIP" "$APP"
echo "Gatekeeper (app):"
spctl -a -t exec -vv "$APP" 2>&1 || true

# --- DMG -----------------------------------------------------------------------
step "Build DMG"
if ! command -v create-dmg >/dev/null 2>&1; then
  brew install create-dmg
fi
DMG_ROOT="$BUILD/dmg-root"
rm -rf "$DMG_ROOT" "$DMG"
mkdir -p "$DMG_ROOT"
cp -R "$APP" "$DMG_ROOT/"
create-dmg \
  --volname "$APP_NAME" \
  --window-pos 200 120 --window-size 600 400 --icon-size 128 \
  --icon "$APP_NAME.app" 150 190 \
  --hide-extension "$APP_NAME.app" \
  --app-drop-link 450 190 \
  --no-internet-enable \
  --codesign "$CODESIGN_IDENTITY" \
  "$DMG" "$DMG_ROOT"
codesign --verify --strict "$DMG"

step "Notarize DMG"
notarize "$DMG"
echo "Gatekeeper (dmg):"
spctl -a -t open --context context:primary-signature -vv "$DMG" 2>&1 || true

# --- release notes ------------------------------------------------------------
step "Release notes"
rm -f "$UPDATES/$APP_NAME-$VERSION".{html,md,txt}
if [[ -n "$NOTES_FILE" ]]; then
  cp "$NOTES_FILE" "$UPDATES/$APP_NAME-$VERSION.${NOTES_FILE##*.}"
else
  PREVIOUS_TAG="$(git -C "$REPO_DIR" tag -l 'app/v*' --sort=-v:refname | grep -v -x "app/v$VERSION" | head -1 || true)"
  RANGE="${PREVIOUS_TAG:+$PREVIOUS_TAG..}HEAD"
  git -C "$REPO_DIR" log --no-merges --format=%s "$RANGE" -- macos \
    | release_notes_html "$VERSION" > "$UPDATES/$APP_NAME-$VERSION.html"
  echo "Generated from commits in $RANGE touching macos/"
fi

# --- appcast -----------------------------------------------------------------------
step "Sparkle appcast"
SPARKLE_BIN="$(find "$DERIVED_DATA/SourcePackages/artifacts" -type d -path '*/sparkle/Sparkle/bin' | head -1)"
if [[ -z "$SPARKLE_BIN" ]]; then
  echo "release.sh: Sparkle tools not found under $DERIVED_DATA/SourcePackages/artifacts" >&2
  exit 1
fi
# Always start from the published feed so a rerun of the same version does
# not keep the item generate_appcast wrote for the previous build.
rm -f "$UPDATES/appcast.xml"
if [[ -f "$PREVIOUS_APPCAST" ]]; then
  cp "$PREVIOUS_APPCAST" "$UPDATES/appcast.xml"
  echo "Extending published feed $PREVIOUS_APPCAST"
fi
APPCAST_ARGS=(
  --download-url-prefix "$DOWNLOAD_URL_PREFIX"
  --release-notes-url-prefix "$RELEASE_NOTES_URL_PREFIX"
  --link "https://handbyte.gumpw.com"
  "$UPDATES"
)
if [[ -n "${SPARKLE_PRIVATE_KEY:-}" ]]; then
  printf '%s' "$SPARKLE_PRIVATE_KEY" | "$SPARKLE_BIN/generate_appcast" --ed-key-file - "${APPCAST_ARGS[@]}"
else
  "$SPARKLE_BIN/generate_appcast" "${APPCAST_ARGS[@]}"
fi

# --- collect outputs ---------------------------------------------------------------
step "Outputs"
mkdir -p "$DIST/release-notes"
cp "$UPDATES/appcast.xml" "$DIST/appcast.xml"
cp "$DMG" "$DIST/"
cp "$UPDATES/$APP_NAME-$VERSION".* "$DIST/release-notes/" 2>/dev/null || true
rm -f "$DIST/release-notes/$APP_NAME-$VERSION.dmg"
ls -la "$DIST/$APP_NAME-$VERSION.dmg" "$DIST/appcast.xml" "$DIST/release-notes/"
