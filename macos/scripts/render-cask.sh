#!/usr/bin/env bash
# Stamp version and sha256 into Casks/handbyte.rb and print the result.
#
# Usage: render-cask.sh <X.Y.Z> <sha256> [<output-path>]
set -euo pipefail

version="${1:?usage: render-cask.sh <X.Y.Z> <sha256> [<output>]}"
sha256="${2:?usage: render-cask.sh <X.Y.Z> <sha256> [<output>]}"
output="${3:-/dev/stdout}"

[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "render-cask: version must be X.Y.Z (got $version)" >&2; exit 1; }
[[ "$sha256" =~ ^[a-f0-9]{64}$ ]] || { echo "render-cask: sha256 must be 64 lowercase hex characters" >&2; exit 1; }

template="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/Casks/handbyte.rb"
[[ -f "$template" ]] || { echo "render-cask: missing $template" >&2; exit 1; }

# Python asserts exactly one replacement per field, which catches template drift.
python3 - "$template" "$version" "$sha256" > "$output" <<'PYEOF'
import re, sys
path, version, sha256 = sys.argv[1:4]
src = open(path, encoding="utf-8").read()
src, n = re.subn(r'^(  version )"[^"]+"', rf'\g<1>"{version}"', src, count=1, flags=re.M)
if n != 1:
    sys.exit("render-cask: expected exactly one version line")
src, n = re.subn(r'^(  sha256 )"[a-f0-9]{64}"', rf'\g<1>"{sha256}"', src, count=1, flags=re.M)
if n != 1:
    sys.exit("render-cask: expected exactly one sha256 line")
sys.stdout.write(src)
PYEOF
