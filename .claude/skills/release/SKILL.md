---
name: release
description: Prepare a Handbyte CLI release PR by writing CHANGELOG.md from verified Git changes and selecting the version. Use when asked to release the CLI or prepare its release notes and PR, not for the separate macOS app or pipeline maintenance.
---

# Handbyte CLI release

Prepare a reviewable release PR. GitHub Actions owns tagging, testing, building,
and publishing after merge. Do not build release archives locally or manually
push a tag as part of this flow.

The contract is `release/vX.Y.Z` → `CHANGELOG.md` section
`## [X.Y.Z] - YYYY-MM-DD` → tag `vX.Y.Z`. The tag supplies `main.version`;
there is no source version constant to bump. CI uses the section body as the
GitHub Release notes and rejects missing, empty, or duplicate sections.

A request to prepare a release authorizes the release branch, commit, push, and
PR. A request for notes only stops at the changelog or draft requested by the
user. Neither request authorizes merging. Explain that merging the release PR
will publish the CLI automatically; merge only with explicit authorization.

## Establish the release range

- Check the repository, current branch, worktree status, and open release PRs.
  This skill applies to `wanggang316/handbyte`, not `handbyte-macos`.
- Fetch `origin/main` and tags. Use a clean checkout; do not stash or overwrite
  unrelated work, or create another worktree without the user's instruction.
- Reuse an existing release PR for the same intent. Do not open a second release
  PR while another version is pending without resolving the intended order.
- Find the latest published stable CLI release with `gh release list` and verify
  its tag is an ancestor of `origin/main`. Do not use an unpublished tag or the
  separate app version as the baseline. If there is no published release,
  inspect the full history; do not invent a previous version.
- Read `git log <previous-tag>..origin/main`, the relevant diffs, merged PRs,
  and existing `[Unreleased]` notes. Confirm changes against the current code.
  Record the reviewed `origin/main` SHA so later changes can be detected.

## Write user-facing changes and choose a version

Write concise English notes under the applicable Keep a Changelog categories:
`Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`, and `Security`.

- Describe the final behavior users get. Consolidate repeated work on the same
  CLI feature; omit superseded implementations, internal refactors, tests, CI,
  and documentation-only work with no user-visible effect.
- Include incompatible commands or flags, JSON/schema changes, cleanup and
  restore behavior, and system requirements. Never infer data recovery or
  safety guarantees from a passing build.
- Cross-check `[Unreleased]`; do not promote it blindly. Preserve entries for
  work outside this release. Preserve historical sections as published.
- If CHANGELOG.md does not exist, create it with `# Changelog`,
  `## [Unreleased]`, and the new dated release section. Do not reconstruct old
  releases without verified evidence. Do not make an empty version release
  merely to ship pipeline or Skill changes.
- Use an explicit user-supplied version if valid. Otherwise derive the bump:
  breaking changes require major (minor while below 1.0), compatible new
  features require minor, and fixes require patch. Do not cross to 1.0 without
  the user's decision. State the choice and its reason in the PR.
- Require an unused `vX.Y.Z` tag newer than the last stable release. Use today's
  local date in the section heading; do not invent a publication date.

## Prepare and open the PR

1. Start `release/vX.Y.Z` from the reviewed `origin/main` with
   `git switch --no-track -c release/vX.Y.Z origin/main`. If already on the
   matching release branch, inspect and continue it. Use the existing checkout.
2. Write the dated changelog section with at least one real change bullet. Keep
   `## [Unreleased]` above it and omit empty categories in the release section.
3. Validate the exact published text:

   ```bash
   python3 scripts/release-notes.py vX.Y.Z
   python3 -B -m unittest discover -s scripts -p 'test_*.py'
   git diff --check
   ```

4. Commit only the release-owned files, normally CHANGELOG.md, with
   `chore(release): prepare vX.Y.Z`. Push explicitly to `release/vX.Y.Z`.
5. Open a non-draft PR to `main` with `gh pr create --body-file`. Include the
   version rationale, reviewed baseline SHA, the exact new changelog body, and:
   **Merging this PR automatically tags, builds, and publishes the CLI.**
6. Report the PR URL and actual check results. If no PR checks are configured,
   say so; the release workflow runs Go tests after merge. Do not report a
   release as published from an open PR or local checks.

Before an authorized merge, fetch main again. Review any added changes and
update the notes and version if needed; do not silently ship unreviewed changes.
Do not move a branch or tag to conceal a version mismatch.

## After an authorized merge

GitHub's `release-merge` workflow tags the merged commit and explicitly calls
`release.yml`. Do not push a second tag: `GITHUB_TOKEN` tag pushes do not trigger
another workflow. Monitor the run associated with this PR's merged commit.

Verify the stable release is public and contains both
`handbyte_vX.Y.Z_darwin_arm64.tar.gz`, `handbyte_vX.Y.Z_darwin_amd64.tar.gz`,
and `checksums.txt`. Compare its notes with the merged changelog section before
reporting the release URL. Do not change app releases, appcast.xml, or Homebrew.

For transient failures, inspect failed logs and rerun failed jobs on the same
commit. Existing matching tags and drafts can be resumed. A missing changelog
in the tagged commit needs a new corrected release; retrying cannot add it.
Never replace published assets or delete/move a pushed tag without explicit
user authorization. Fix a published version with a new release.
