#!/usr/bin/env bash
# Fails when the rule ids in the Go CLI presets and the Swift catalog differ.
# Reads both sources with grep so no Go or Swift toolchain is needed.
#
# Go ids:    every `Source: "<id>"` in presets.go, plus `artifact:<Name>` for
#            every row of ProjectArtifactPatterns.
# Swift ids: every `preset(id: "<id>"` in Catalog.swift, plus `artifact:<name>`
#            for every `artifact(name: "<name>"`.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
go_presets="$repo_root/internal/scanner/presets.go"
swift_catalog="$repo_root/macos/Sources/HandbyteCore/Catalog.swift"

for f in "$go_presets" "$swift_catalog"; do
    if [ ! -f "$f" ]; then
        echo "sync-check: missing $f" >&2
        exit 2
    fi
done

go_ids() {
    grep -oE 'Source:[[:space:]]*"[^"]+"' "$go_presets" | sed -E 's/.*"([^"]+)"/\1/'
    sed -n '/^var ProjectArtifactPatterns/,/^}/p' "$go_presets" \
        | grep -oE '^[[:space:]]*\{"[^"]+"' | sed -E 's/.*"([^"]+)"/artifact:\1/'
}

swift_ids() {
    grep -oE 'preset\(id: "[^"]+"' "$swift_catalog" | sed -E 's/.*"([^"]+)"/\1/'
    grep -oE 'artifact\(name: "[^"]+"' "$swift_catalog" | sed -E 's/.*"([^"]+)"/artifact:\1/'
}

go_sorted="$(go_ids | sort -u)"
swift_sorted="$(swift_ids | sort -u)"

if [ -z "$go_sorted" ] || [ -z "$swift_sorted" ]; then
    echo "sync-check: extracted an empty id set; the source layout changed" >&2
    exit 2
fi

only_go="$(comm -23 <(printf '%s\n' "$go_sorted") <(printf '%s\n' "$swift_sorted"))"
only_swift="$(comm -13 <(printf '%s\n' "$go_sorted") <(printf '%s\n' "$swift_sorted"))"

if [ -z "$only_go" ] && [ -z "$only_swift" ]; then
    count="$(printf '%s\n' "$go_sorted" | wc -l | tr -d ' ')"
    echo "sync-check: OK, $count rule ids match"
    exit 0
fi

echo "sync-check: rule ids differ between the Go CLI and the Swift catalog" >&2
if [ -n "$only_go" ]; then
    echo "only in Go (internal/scanner/presets.go):" >&2
    printf '  %s\n' $only_go >&2
fi
if [ -n "$only_swift" ]; then
    echo "only in Swift (macos/Sources/HandbyteCore/Catalog.swift):" >&2
    printf '  %s\n' $only_swift >&2
fi
echo "Update the other side or record an intentional difference in SYNC.md." >&2
exit 1
