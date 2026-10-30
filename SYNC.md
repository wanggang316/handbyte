# CLI and app rule synchronization

The Go CLI (`internal/scanner/presets.go`) and the macOS app
(`macos/Sources/HandbyteCore/Catalog.swift`) are separate implementations of
the same cleanup scope. They stay aligned through identical rule ids: the id a
rule carries in the app is the exact string the CLI prints as `Source`.
`macos/scripts/sync-check.sh` compares the two id sets and fails CI on any
difference. This file is the human-maintained record of where each rule stands.

States:

- **Pending**: planned on this side, not implemented yet.
- **Implemented**: present on this side with the same scope as the other side.
- **Intentional difference**: present on both sides but behaving differently on
  purpose, or deliberately absent on one side; the note says why.
- **Not applicable**: the rule cannot exist on this side.

Any state other than Implemented on both sides needs a note. Add a row when a
rule is added to either side; remove the row when it is removed from both.

| Rule id | CLI | App | Note |
|---|---|---|---|
| `xcode-derived` | Implemented | Implemented | |
| `xcode-devicesupport` | Implemented | Implemented | |
| `xcode-archives` | Implemented | Implemented | |
| `coresim-cache` | Implemented | Implemented | |
| `go-build` | Implemented | Implemented | |
| `go-mod-cache` | Implemented | Implemented | |
| `cargo-registry` | Implemented | Implemented | |
| `cargo-git` | Implemented | Implemented | |
| `rustup` | Implemented | Implemented | |
| `npm-cache` | Implemented | Implemented | |
| `pnpm-store` | Implemented | Implemented | |
| `yarn-cache` | Implemented | Implemented | |
| `gradle-cache` | Implemented | Implemented | |
| `maven-cache` | Implemented | Implemented | |
| `swiftpm-cache` | Implemented | Implemented | |
| `playwright` | Implemented | Implemented | |
| `cypress` | Implemented | Implemented | |
| `brew-cache` | Implemented | Implemented | |
| `jetbrains` | Implemented | Implemented | |
| `codex` | Implemented | Implemented | |
| `xdg-cache` | Implemented | Implemented | |
| `library-caches` | Intentional difference | Implemented | The CLI also lists a child of `~/Library/Caches` that a more specific preset matches (for example `go-build`), so that directory appears twice. The app deduplicates by file identity and keeps the more specific rule. |
| `library-logs` | Implemented | Implemented | |
| `containers` | Implemented | Implemented | |
| `orbstack` | Implemented | Implemented | |
| `apple-container` | Implemented | Implemented | |
| `claude-vm` | Implemented | Implemented | |
| `docker` | Implemented | Implemented | |
| `trash` | Implemented | Implemented | |
| `downloads` | Implemented | Implemented | |
| `artifact:node_modules` | Implemented | Implemented | |
| `artifact:target` | Implemented | Implemented | |
| `artifact:build` | Implemented | Implemented | |
| `artifact:.build` | Implemented | Implemented | |
| `artifact:dist` | Implemented | Implemented | |
| `artifact:.next` | Implemented | Implemented | |
| `artifact:.nuxt` | Implemented | Implemented | |
| `artifact:.turbo` | Implemented | Implemented | |
| `artifact:.parcel-cache` | Implemented | Implemented | |
| `artifact:.gradle` | Implemented | Implemented | |
| `artifact:.dart_tool` | Implemented | Implemented | |
| `artifact:DerivedData` | Implemented | Implemented | |
| `artifact:Pods` | Implemented | Implemented | |
| `artifact:__pycache__` | Implemented | Implemented | |
| `artifact:.pytest_cache` | Implemented | Implemented | |
| `artifact:.mypy_cache` | Implemented | Implemented | |
| `artifact:.ruff_cache` | Implemented | Implemented | |
| `artifact:.venv` | Implemented | Implemented | |
| `artifact:venv` | Implemented | Implemented | |

Scan-wide differences that belong to no single rule:

| Behaviour | CLI | App | Note |
|---|---|---|---|
| Hard links | Intentional difference | Implemented | The CLI counts every hard link to a file, so a file with two links is counted twice. The app counts an inode once per scan, so the sum over all candidates equals the bytes a cleanup can free. `macos/scripts/parity.sh` reports both effects as expected size differences. |
| Operations journal | Intentional difference | Intentional difference | The CLI appends JSON Lines to `actions.jsonl` and reads it back for `restore`. The app appends tab-separated lines to `~/Library/Logs/handbyte/operations.log` as an audit trail only. Neither side reads the other file. |
| Settings file | Intentional difference | Intentional difference | The CLI stores roots and exclusions in its own `config.json`. The app stores deletion mode, minimum size, roots and exclusions in `~/Library/Application Support/Handbyte/settings.json`. Neither side reads the other file. |
