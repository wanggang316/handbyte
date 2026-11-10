# Changelog

## [Unreleased]

## [0.4.0] - 2026-11-11

### Added

- Smart Clean scans known cleanup locations by area, summarizes eligible items,
  and cleans them after one confirmation. Use `clean --review` to select items
  individually, `--deep` to include project roots, or `--dry-run` to preview.
- `handbyte analyze` shows folder sizes, storage categories, and large files,
  with hard-link and full APFS clone deduplication and partial-clone indicators.
  Selected files and folders can be reviewed and moved to the Trash.
- `handbyte apps` lists application size, source, architecture, and last use,
  with unused-app and leftover-data filters. `handbyte uninstall` reviews
  related files by confidence, supports `--reset` to keep the app and
  `--with-data` to include its data, and offers `--brew` for Homebrew casks.
  Administrator-owned related files are listed but not removed.
- `handbyte clean --system` removes old system logs with `sudo`. Unified log
  erasure requires a separate interactive confirmation, even with `--yes`.
  System cleanup permanently removes data and cannot be undone with `restore`.
- `handbyte restore <group>` restores eligible Trash records from an uninstall
  group. Storage-analysis Trash actions also support restoration by action ID.
  Apps removed by Homebrew cannot be restored through this journal.
- JSON output for storage analysis and app inventories uses schema version 1.
  Cleanup JSON retains schema version 2 and adds `Area` and `Rebuildable` item
  fields. Action records can include `Group`, `Rule`, and `Tool` fields.

### Changed

- Running `handbyte` now opens a home screen for cleanup, storage analysis,
  apps, and history. Terminal views adapt to window width and support mouse
  input, light and dark themes, and `NO_COLOR`. Use `handbyte config mouse off`
  to disable mouse input.
- **Cleanup default changed:** the new `handbyte clean` command permanently
  removes rebuildable data and moves other data to the Trash. Direct removals
  cannot be restored. Use `--trash` to move all selected cleanup items to the
  Trash; `--rm` still requests permanent deletion for all selected items.
- Expanded cleanup rules cover more application caches, browser caches,
  development tools, and logs, with owner-process checks and wildcard paths.
- The CLI and macOS app share action records at
  `~/Library/Application Support/handbyte/actions.jsonl`.

### Deprecated

- Options without a subcommand remain supported but print a migration hint.
  Use `clean`, `analyze`, `history`, `restore`, and `config` in new scripts.
  Legacy cleanup options retain their Trash default. Legacy
  `--explore --scan-only` retains its existing JSON format; the replacement
  `analyze --json` uses the new storage-analysis schema.
