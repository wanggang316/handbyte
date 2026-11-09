#!/usr/bin/env bash
# Compares the Go CLI's `--scan-only` output with `hbscan --json` on this
# machine. Manual developer tool; read-only; not a CI gate.
#
# usage: scripts/parity.sh [--go-bin <handbyte>] [--min <size>] [--roots <a,b>] [--no-projects] [--workers <n>]
#
# Both scanners run against the real home directory. Expected differences:
# the Go CLI lists ~/Library/Caches children twice when a specific preset also
# matches them, and it does not deduplicate hard links.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
macos_dir="$repo_root/macos"
go_bin=""
pass_args=()

while [ $# -gt 0 ]; do
    case "$1" in
        --go-bin) go_bin="$2"; shift 2 ;;
        --go-bin=*) go_bin="${1#--go-bin=}"; shift ;;
        --min|--roots|--workers) pass_args+=("$1=$2"); shift 2 ;;
        --min=*|--roots=*|--workers=*|--no-projects) pass_args+=("$1"); shift ;;
        -h|--help) sed -n '2,10p' "$0"; exit 0 ;;
        *) echo "parity: unknown argument $1" >&2; exit 2 ;;
    esac
done

work="$(mktemp -d "${TMPDIR:-/tmp}/handbyte-parity.XXXXXX")"
trap 'rm -rf "$work"' EXIT

echo "parity: building hbscan" >&2
swift build -c release --package-path "$macos_dir" --product hbscan >/dev/null
hbscan="$(swift build -c release --package-path "$macos_dir" --show-bin-path)/hbscan"

echo "parity: running Go scanner" >&2
if [ -n "$go_bin" ]; then
    "$go_bin" --scan-only "${pass_args[@]+"${pass_args[@]}"}" > "$work/go.json"
else
    (cd "$repo_root" && go run . --scan-only "${pass_args[@]+"${pass_args[@]}"}") > "$work/go.json"
fi

echo "parity: running hbscan" >&2
"$hbscan" --json "${pass_args[@]+"${pass_args[@]}"}" > "$work/swift.json"

python3 -I - "$work/go.json" "$work/swift.json" <<'PY'
import json
import sys

go = json.load(open(sys.argv[1]))
sw = json.load(open(sys.argv[2]))

go_items = {}
go_duplicates = []
for item in go.get("items") or []:
    if item["Path"] in go_items:
        go_duplicates.append(item)
    go_items[item["Path"]] = item
sw_items = {c["path"]: c for c in sw["candidates"]}

both = sorted(set(go_items) & set(sw_items))
only_go = sorted(set(go_items) - set(sw_items))
only_sw = sorted(set(sw_items) - set(go_items))

exact = [p for p in both if go_items[p]["Size"] == sw_items[p]["size"]["allocatedBytes"]]
differ = [p for p in both if p not in exact]

def fmt(n):
    return f"{n:,}"

print(f"Go items: {len(go_items)} unique paths ({len(go_duplicates)} listed twice); hbscan candidates: {len(sw_items)}")
print(f"Paths on both sides: {len(both)}; identical size: {len(exact)}; different size: {len(differ)}")
print(f"Only in Go: {len(only_go)}; only in hbscan: {len(only_sw)}")
print(f"hbscan partial candidates: {sw['partialCount']}; categories: {sw['categories']}")

if differ:
    print("\nSize differences (Go - hbscan):")
    for p in differ:
        g, s = go_items[p], sw_items[p]
        flags = []
        if s["size"]["partial"]:
            flags.append("partial")
        if g["FileCount"] != s["size"]["fileCount"]:
            flags.append(f"files {g['FileCount']} vs {s['size']['fileCount']}")
        print(f"  {fmt(g['Size'] - s['size']['allocatedBytes']):>16}  {g['Source']:<20} {s['rule']['id']:<20} {p}  {' '.join(flags)}")

if go_duplicates:
    print("\nListed twice by Go (same path, two sources):")
    for item in go_duplicates:
        print(f"  {item['Source']:<20} {item['Path']}")

if only_go:
    print("\nOnly in Go:")
    for p in only_go:
        print(f"  {fmt(go_items[p]['Size']):>16}  {go_items[p]['Source']:<20} {p}")

if only_sw:
    print("\nOnly in hbscan:")
    for p in only_sw:
        print(f"  {fmt(sw_items[p]['size']['allocatedBytes']):>16}  {sw_items[p]['rule']['id']:<20} {p}")
PY
