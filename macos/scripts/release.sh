#!/usr/bin/env bash
# Build a signed, notarized Handbyte release: archive, notarize, DMG and
# Sparkle appcast. Outputs go to macos/dist/.
#
# Usage: release.sh [--version X.Y.Z] [--notes FILE]
#
#   --version  Expected marketing version. Fails when it differs from
#              MARKETING_VERSION in project.yml (CI passes the tag version).
#   --notes    Markdown release notes for this version. Default: the
#              "## [X.Y.Z]" section of macos/CHANGELOG.md.
#
# Environment:
#   DERIVED_DATA              Xcode derived data path (default: .dd)
#   CODESIGN_IDENTITY         SHA-1 or name of the Developer ID Application
#                             identity (default: the first one in the keychain)
#   NOTARY_KEYCHAIN_PROFILE   notarytool keychain profile (default: handbyte-notary)
#   AC_API_KEY_ID, AC_API_ISSUER_ID, AC_API_KEY_P8
#                             App Store Connect API key (P8 as base64); used
#                             instead of the keychain profile when all are set (CI).
#   SPARKLE_PRIVATE_KEY       EdDSA private key for generate_appcast (CI).
#                             Default: the key in the login keychain.
#   DOWNLOAD_URL_PREFIX       Where the DMG is published
#                             (default: GitHub release for tag app/v$VERSION)
#   PREVIOUS_APPCAST          Published feed to extend (default: ../docs/appcast.xml)
#
# Notarization runs only when credentials exist; otherwise the step prints
# SKIPPED and the unnotarized artifacts are still produced.
set -euo pipefail

MACOS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_DIR="$(cd "$MACOS_DIR/.." && pwd)"
# shellcheck source=release-lib.sh
source "$MACOS_DIR/scripts/release-lib.sh"

usage_error() {
  echo "release.sh: $1" >&2
  sed -n '5,10p' "$0" >&2
  exit 2
}

EXPECTED_VERSION=""
NOTES_FILE=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) [[ $# -ge 2 ]] || usage_error "--version needs a value"; EXPECTED_VERSION=$2; shift 2 ;;
    --notes) [[ $# -ge 2 ]] || usage_error "--notes needs a file"; NOTES_FILE=$2; shift 2 ;;
    -h|--help) sed -n '2,28p' "$0"; exit 0 ;;
    *) echo "release.sh: unknown argument '$1'" >&2; exit 2 ;;
  esac
done

DERIVED_DATA="${DERIVED_DATA:-.dd}"
NOTARY_KEYCHAIN_PROFILE="${NOTARY_KEYCHAIN_PROFILE:-handbyte-notary}"
PREVIOUS_APPCAST="${PREVIOUS_APPCAST:-$REPO_DIR/docs/appcast.xml}"
SCHEME=Handbyte
APP_NAME=Handbyte

DIST="$MACOS_DIR/dist"
BUILD="$DIST/build"
UPDATES="$DIST/updates"
ARCHIVE="$BUILD/$APP_NAME.xcarchive"
APP="$BUILD/$APP_NAME.app"

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

cleanup_files=()
cleanup() {
  if [[ ${#cleanup_files[@]} -gt 0 ]]; then
    rm -f "${cleanup_files[@]}"
  fi
}
trap cleanup EXIT

cd "$MACOS_DIR"
rm -rf "$DIST"
mkdir -p "$DIST/logs" "$UPDATES" "$BUILD"

# --- version ---------------------------------------------------------------
VERSION="$(release_marketing_version "$MACOS_DIR/project.yml")"
BUILD_NUMBER="$(release_build_number "$MACOS_DIR/project.yml")"
if [[ -n "$EXPECTED_VERSION" && "$EXPECTED_VERSION" != "$VERSION" ]]; then
  echo "release.sh: --version $EXPECTED_VERSION differs from project.yml MARKETING_VERSION $VERSION; run scripts/bump-version.sh first" >&2
  exit 1
fi
DOWNLOAD_URL_PREFIX="${DOWNLOAD_URL_PREFIX:-https://github.com/wanggang316/handbyte/releases/download/app/v$VERSION/}"
DMG="$UPDATES/$APP_NAME-$VERSION.dmg"
NOTES="$UPDATES/$APP_NAME-$VERSION.md"

step "Release $APP_NAME $VERSION (build $BUILD_NUMBER)"

# --- release notes first, so a missing section fails before the build ----------
if [[ -n "$NOTES_FILE" ]]; then
  cp "$NOTES_FILE" "$NOTES"
else
  release_changelog_section "$MACOS_DIR/CHANGELOG.md" "$VERSION" > "$NOTES"
fi
echo "Release notes: $(wc -l < "$NOTES" | tr -d ' ') lines"

# --- signing identity --------------------------------------------------------
if [[ -z "${CODESIGN_IDENTITY:-}" ]]; then
  CODESIGN_IDENTITY="$(security find-identity -v -p codesigning | grep 'Developer ID Application' | head -1 | awk '{print $2}')"
fi
if [[ -z "$CODESIGN_IDENTITY" ]]; then
  echo "release.sh: no 'Developer ID Application' identity in the keychain" >&2
  exit 1
fi
export CODESIGN_IDENTITY
echo "Signing identity: $CODESIGN_IDENTITY"

# --- notarization credentials ------------------------------------------------
NOTARY_ARGS=()
if [[ -n "${AC_API_KEY_ID:-}" && -n "${AC_API_ISSUER_ID:-}" && -n "${AC_API_KEY_P8:-}" ]]; then
  KEY_PATH="$(mktemp -t handbyte-notary).p8"
  cleanup_files+=("$KEY_PATH")
  # Strip the whitespace that pasted secrets pick up before decoding.
  printf '%s' "$AC_API_KEY_P8" | tr -d ' \n\r\t' | base64 --decode > "$KEY_PATH"
  NOTARY_ARGS=(--key "$KEY_PATH" --key-id "$AC_API_KEY_ID" --issuer "$AC_API_ISSUER_ID")
  echo "Notarization: App Store Connect API key $AC_API_KEY_ID"
elif xcrun notarytool history --keychain-profile "$NOTARY_KEYCHAIN_PROFILE" >/dev/null 2>&1; then
  NOTARY_ARGS=(--keychain-profile "$NOTARY_KEYCHAIN_PROFILE")
  echo "Notarization: keychain profile $NOTARY_KEYCHAIN_PROFILE"
else
  echo "Notarization: SKIPPED (no keychain profile '$NOTARY_KEYCHAIN_PROFILE' and no AC_API_* variables)"
fi

notarize() {
  # notarize <submission> [<staple-target>]: submit, wait, staple.
  # An .app is submitted as a zip but stapled as a bundle, hence two paths.
  local submission=$1 target=${2:-$1} log status id
  if [[ ${#NOTARY_ARGS[@]} -eq 0 ]]; then
    echo "SKIPPED notarization of $(basename "$target") (no credentials)"
    return 0
  fi
  log="$DIST/logs/notarize-$(basename "$target").json"
  xcrun notarytool submit "$submission" "${NOTARY_ARGS[@]}" --wait --timeout 30m --output-format json > "$log"
  status="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("status",""))' "$log")"
  id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("id",""))' "$log")"
  echo "Notarization of $(basename "$target"): $status ($id)"
  if [[ "$status" != "Accepted" ]]; then
    xcrun notarytool log "$id" "${NOTARY_ARGS[@]}" >&2 || true
    return 1
  fi
  xcrun stapler staple "$target"
  xcrun stapler validate "$target"
}

# --- archive -------------------------------------------------------------------
step "Generate Xcode project"
xcodegen generate

step "Archive ($SCHEME, Release)"
run_logged archive xcodebuild -scheme "$SCHEME" -configuration Release -destination 'generic/platform=macOS' \
  -derivedDataPath "$DERIVED_DATA" -archivePath "$ARCHIVE" archive \
  CODE_SIGN_STYLE=Manual CODE_SIGN_IDENTITY="$CODESIGN_IDENTITY" OTHER_CODE_SIGN_FLAGS="--timestamp"

# -exportArchive needs an Apple ID signed in to Xcode, which a CI runner has
# not got. Developer ID distribution does not re-sign, so the signed bundle is
# taken from the archive as it is.
step "Extract and re-sign nested code"
cp -R "$ARCHIVE/Products/Applications/$APP_NAME.app" "$APP"
scripts/resign-nested.sh "$APP"
if codesign -dv "$APP" 2>&1 | grep -q 'Signature=adhoc'; then
  echo "release.sh: the archive is ad-hoc signed, not Developer ID" >&2
  exit 1
fi
codesign -dvv "$APP" 2>&1 | grep -E '^(Identifier|Authority|TeamIdentifier)' || true
BUILT_VERSION="$(/usr/libexec/PlistBuddy -c 'Print CFBundleShortVersionString' "$APP/Contents/Info.plist")"
BUILT_BUILD="$(/usr/libexec/PlistBuddy -c 'Print CFBundleVersion' "$APP/Contents/Info.plist")"
if [[ "$BUILT_VERSION" != "$VERSION" || "$BUILT_BUILD" != "$BUILD_NUMBER" ]]; then
  echo "release.sh: built $BUILT_VERSION ($BUILT_BUILD), expected $VERSION ($BUILD_NUMBER)" >&2
  exit 1
fi

# --- notarize + staple the app ------------------------------------------------
step "Notarize app"
ZIP="$BUILD/$APP_NAME-$VERSION.zip"
ditto -c -k --keepParent "$APP" "$ZIP"
notarize "$ZIP" "$APP"
echo "Gatekeeper (app):"
spctl -a -t exec -vv "$APP" 2>&1 || true

# --- DMG -----------------------------------------------------------------------
step "Build DMG"
if ! command -v create-dmg >/dev/null 2>&1; then
  brew install create-dmg
fi
ICONSET="$BUILD/volume.iconset"
rm -rf "$ICONSET"
mkdir -p "$ICONSET"
cp App/Resources/Assets.xcassets/AppIcon.appiconset/icon_*.png "$ICONSET/"
iconutil -c icns "$ICONSET" -o "$BUILD/volume.icns"
DMG_ROOT="$BUILD/dmg-root"
rm -rf "$DMG_ROOT"
mkdir -p "$DMG_ROOT"
cp -R "$APP" "$DMG_ROOT/"
create-dmg \
  --volname "$APP_NAME" \
  --volicon "$BUILD/volume.icns" \
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
# Stapling changes the file, so the checksum is taken last.
(cd "$UPDATES" && shasum -a 256 "$(basename "$DMG")" > "$(basename "$DMG").sha256")

# --- appcast -----------------------------------------------------------------------
step "Sparkle appcast"
SPARKLE_BIN="$(find "$DERIVED_DATA/SourcePackages/artifacts" -type d -path '*/sparkle/Sparkle/bin' | head -1)"
if [[ -z "$SPARKLE_BIN" ]]; then
  echo "release.sh: Sparkle tools not found under $DERIVED_DATA/SourcePackages/artifacts" >&2
  exit 1
fi
# Start from the published feed so earlier versions stay in it; the .md next
# to the DMG becomes the item's embedded release notes.
if [[ -f "$PREVIOUS_APPCAST" ]]; then
  cp "$PREVIOUS_APPCAST" "$UPDATES/appcast.xml"
  echo "Extending published feed $PREVIOUS_APPCAST"
fi
APPCAST_ARGS=(
  --download-url-prefix "$DOWNLOAD_URL_PREFIX"
  --link "https://handbyte.gumpw.com"
  --embed-release-notes
  "$UPDATES"
)
if [[ -n "${SPARKLE_PRIVATE_KEY:-}" ]]; then
  printf '%s' "$SPARKLE_PRIVATE_KEY" | tr -d ' \n\r\t' | "$SPARKLE_BIN/generate_appcast" --ed-key-file - "${APPCAST_ARGS[@]}"
else
  "$SPARKLE_BIN/generate_appcast" "${APPCAST_ARGS[@]}"
fi
grep -q "sparkle:version>$BUILD_NUMBER<\|sparkle:version=\"$BUILD_NUMBER\"" "$UPDATES/appcast.xml" \
  || { echo "release.sh: appcast has no item for build $BUILD_NUMBER" >&2; exit 1; }

# --- collect outputs ---------------------------------------------------------------
# Handbyte.dmg is a stable-named copy for a release-independent download link.
step "Outputs"
cp "$UPDATES/appcast.xml" "$DIST/appcast.xml"
cp "$DMG" "$DMG.sha256" "$DIST/"
cp "$DMG" "$DIST/$APP_NAME.dmg"
cp "$NOTES" "$DIST/"
ls -la "$DIST"/*.dmg "$DIST"/*.sha256 "$DIST/appcast.xml" "$DIST/$APP_NAME-$VERSION.md"
