#!/usr/bin/env bash
# Re-sign every nested framework, XPC service, helper app and executable
# inside a Developer ID signed .app with the same identity, hardened runtime
# and a secure timestamp, deepest first, then re-seal the outer bundle.
#
# xcodebuild signs the embedded Sparkle.framework but leaves the XPC services
# and helpers inside it with their shipped signatures. codesign --deep accepts
# that, Apple's notary service does not.
#
# Usage: CODESIGN_IDENTITY=<sha1 or name> resign-nested.sh <path-to-app>
set -euo pipefail

: "${CODESIGN_IDENTITY:?CODESIGN_IDENTITY must be set}"
APP="${1:?usage: resign-nested.sh <path-to-app>}"
[[ -d "$APP" ]] || { echo "resign-nested: $APP is not a directory" >&2; exit 1; }

sign_path() {
  local path=$1
  local -a args=(-f -s "$CODESIGN_IDENTITY" -o runtime --timestamp)
  case "$path" in
    *.app|*.appex|*.xpc)
      # Keep the helper's own entitlements (Sparkle's XPC services are sandboxed).
      # shellcheck disable=SC2054
      args+=(--preserve-metadata=entitlements,requirements,flags)
      ;;
  esac
  codesign "${args[@]}" "$path"
}

paths=()
for root in Frameworks PlugIns Resources XPCServices Library/LoginItems; do
  dir="$APP/Contents/$root"
  [[ -d "$dir" ]] || continue
  while IFS= read -r -d '' p; do
    paths+=("$p")
  done < <(
    find "$dir" \
      \( -type d \( -name '*.app' -o -name '*.appex' -o -name '*.framework' -o -name '*.xpc' \) \
      -o -type f \( -name '*.dylib' -o -perm -111 \) \) \
      -print0
  )
done

# Deepest first, so each container seals over already-signed children.
if [[ ${#paths[@]} -gt 0 ]]; then
  while IFS=$'\t' read -r _ p; do
    sign_path "$p"
  done < <(
    for p in "${paths[@]}"; do
      slashes="${p//[^\/]/}"
      printf '%s\t%s\n' "${#slashes}" "$p"
    done | sort -rn -k1,1
  )
fi

codesign -f -s "$CODESIGN_IDENTITY" -o runtime --timestamp \
  --preserve-metadata=entitlements,requirements,flags "$APP"
codesign --verify --strict --deep "$APP"
