# handbyte

An interactive macOS disk cleanup CLI.

`handbyte` scans the well-known noisy paths on a macOS dev machine — Xcode
DerivedData, iOS DeviceSupport, language caches, project build artifacts,
container VM data — shows them in a multi-select TUI, and lets you delete
selected items safely (move-to-Trash by default).

## Install

One-liner (downloads the latest release, installs to `/usr/local/bin`):

```sh
curl -fsSL https://handbyte.gumpw.com/install.sh | bash
```

Pin a version, or install to a user-writable prefix:

```sh
curl -fsSL https://handbyte.gumpw.com/install.sh | VERSION=v0.2.0 bash
curl -fsSL https://handbyte.gumpw.com/install.sh | PREFIX=$HOME/.local bash
```

From source (requires Go 1.26+):

```sh
go install github.com/wanggang316/handbyte@latest   # or
make install                                        # → $HOME/.local/bin/handbyte
```

## Uninstall

`handbyte` writes no config files and no caches. Removing the binary uninstalls it.

```sh
# One-liner (same script, UNINSTALL=1 mode).
curl -fsSL https://handbyte.gumpw.com/install.sh | UNINSTALL=1 bash

# If you installed to a custom prefix:
curl -fsSL https://handbyte.gumpw.com/install.sh | UNINSTALL=1 PREFIX=$HOME/.local bash

# From source:
make uninstall                          # uses PREFIX (default $HOME/.local)

# Manual:
sudo rm "$(command -v handbyte)"
```

## Usage

```sh
handbyte                 # interactive TUI
handbyte --dry-run       # preview what would be deleted
handbyte --rm            # delete directly (skip Trash, unrecoverable)
handbyte --min=200M      # ignore items smaller than 200 MiB
handbyte --roots=~/work  # project scan roots (default: ~/dev ~/Projects ~/Code ~/workspace)
handbyte --no-projects   # skip project artifact scan
```

## Key bindings

| Key | Action |
|---|---|
| j / ↓, k / ↑ | navigate |
| g / G | jump to top / bottom |
| space | toggle selection |
| a | toggle whole group |
| A | toggle all auto-safe items |
| c | clear selection |
| enter | collapse / expand group |
| d | delete selected (with confirm) |
| q / Ctrl-C | quit |
| ? | help |

## Safety levels

| Badge | Meaning |
|---|---|
| `safe`   | Regenerable cache. Deleting costs only rebuild time. |
| `review` | Project artifacts / large stores. Restorable but worth a glance. |
| `hold`   | User data or live state (Downloads, container data). Confirm carefully. |

## What it scans

- Xcode DerivedData, iOS / watchOS / tvOS DeviceSupport, Archives, CoreSimulator caches
- Language caches: `go-build`, Cargo, npm, pnpm, yarn, gradle, maven, SwiftPM
- Tool caches: Playwright, Cypress, JetBrains, Homebrew, Codex, `~/.cache/*`
- Per-app `~/Library/Caches/*`, `~/Library/Logs/*`, `~/Library/Containers/*`
- VM data: OrbStack, Apple Container, Claude vm_bundles, Docker
- Trash, Downloads
- Project artifacts under `~/dev` / `~/Projects` / `~/Code` / `~/workspace`:
  `node_modules`, `target`, `build`, `.build`, `dist`, `.next`, `.nuxt`,
  `.turbo`, `.parcel-cache`, `.gradle`, `.dart_tool`, `DerivedData`, `Pods`,
  `__pycache__`, `.pytest_cache`, `.mypy_cache`, `.ruff_cache`, `venv`, `.venv`
