# Changelog

All notable changes to the Handbyte macOS app are documented in this file.
The Go CLI in the repository root has its own release notes on GitHub.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Before 1.0, a breaking change bumps the minor version. Releases are dated.

The release workflow publishes the section of the released version as the
GitHub Release body and as the "What's New" notes in the in-app updater.

## [Unreleased]

### Added

### Changed

### Deprecated

### Removed

### Fixed

### Security

## [0.1.0] - 2026-11-09

### Added

- **Review and clean caches and build output in a native window.** Handbyte
  scans the same locations as the `handbyte` CLI, groups what it finds by
  application and project, and explains what each item is before you act.
- **Nothing is removed without a confirmation.** Items move to the Trash with
  Finder "Put Back" information by default. Permanent deletion is a separate
  setting and asks again before every cleanup.
- **Safety tiers tell apart what can be cleaned at once and what needs a
  look.** Items that a running app still uses or that are not regenerable are
  held back and shown with the reason.
- **Settings for the minimum size, project roots and exclusions.** Mark any
  item "Never Clean This" from its detail pane.
- **In-app updates.** Handbyte checks for new versions and installs them with
  Sparkle.
