---
name: release
description: Cut a Handbyte macOS app release. Write the release notes into macos/CHANGELOG.md, pick the Semantic Versioning bump from them, bump project.yml, open a release PR, and after it merges tag the merged commit app/vX.Y.Z to start the macos-release workflow, then publish the draft release once it passes. Use when shipping a new app build. The Go CLI releases separately with v* tags.
---

# release: Cut a Handbyte app release

An app release is **a release PR, then a tag push**. The PR lands the version
bump and the release notes on `main`. The tag `app/vX.Y.Z` on the merged
commit starts `.github/workflows/macos-release.yml`: it builds, signs,
notarizes, writes the Sparkle `appcast.xml` and opens a **draft** GitHub
Release with the DMG. Publishing the draft starts `macos-publish.yml`, which
copies the appcast to GitHub Pages (the Sparkle feed and the website download
button) and updates the Homebrew cask. The contract CI enforces:

> tag `app/vX.Y.Z` ⇔ `MARKETING_VERSION: "X.Y.Z"` in `macos/project.yml` ⇔
> `macos/CHANGELOG.md` has `## [X.Y.Z] - YYYY-MM-DD` with entries.

CI publishes that CHANGELOG section verbatim as the GitHub Release body and
as the "What's New" notes inside the app, so it is prose for users.

Don't use this for fixing the release pipeline itself (no version bump).
Never push to `main` directly; every release change goes through a PR.

## Process

Run steps 1–6 without stopping: they write only to a release branch and an
open PR. After the user approves the merge in step 7, run steps 7–9 without
stopping, up to the published release.

### 1. Pre-flight

```bash
git status --porcelain                       # empty
git fetch origin --tags
macos/scripts/bump-version.sh --print        # current version, next build, next patch / minor / major
```

Stop if the tree is dirty, a release PR is already open
(`gh pr list --search 'head:release/app-'`), or the target tag exists.

### 2. Write the release notes

`[Unreleased]` in `macos/CHANGELOG.md` is input, not output. Cross-check it
against the commits since the last app tag,
`git log --no-merges --pretty='%h %s' "$(git describe --tags --abbrev=0 --match 'app/v*' origin/main)..origin/main" -- macos`,
and keep only what a user can perceive.

- One entry per user-visible surface, three lines at most: a bold lead-in,
  then at most two sentences. A release reads as 3–8 entries.
- Lead with what the user gets or the symptom that is gone. Leave out
  refactors, CI, tests and dependency bumps, except a minimum-OS change and
  anything removed or breaking.
- No commit prefixes, issue numbers, hashes or type names.

Categories follow Keep a Changelog: `Added`, `Changed`, `Deprecated`,
`Removed`, `Fixed`, `Security`.

### 3. Choose the version

| The release contains | Bump |
|---|---|
| Anything under `Removed`, or a `Changed` entry that breaks settings, the catalog or how a cleanup behaves | **major**, but **minor** while the version is `0.y.z` |
| Anything under `Added`, `Changed`, or `Deprecated` | **minor** |
| Only `Fixed` and `Security` | **patch** |

Take the number from `bump-version.sh --print`. Never bump to `1.0.0` on
your own.

### 4. Branch, edit the changelog, bump

```bash
git switch --no-track -c release/app-vX.Y.Z origin/main
```

Put the notes under `## [X.Y.Z] - YYYY-MM-DD` (today), drop empty
categories, and reseed an empty `## [Unreleased]` with all six headers. Then:

```bash
make -C macos bump-version VERSION=X.Y.Z     # BUILD=N overrides the build number (rare)
make -C macos test-scripts
```

The script owns `macos/project.yml`: it validates the version, takes the next
`YYYYMMDDNNN` build number above the published appcast, and rewrites the two
settings. Never hand-edit them.

### 5. Commit and push

```bash
git add macos/CHANGELOG.md macos/project.yml
git commit -m "Release Handbyte X.Y.Z"
git push -u origin release/app-vX.Y.Z
```

### 6. Open the release PR

```bash
gh pr create --base main --head release/app-vX.Y.Z \
  --title "Release Handbyte X.Y.Z" --body-file - <<'EOT'
Release X.Y.Z (build N): <bump> because <row of step 3>.

<the new CHANGELOG section, verbatim>

Merging this PR does not release. After the merge, tag the merged commit
`app/vX.Y.Z` to start the macos-release workflow.
EOT
```

Not a draft. Wait for `macos-ci`: `gh pr checks <PR> --watch`.

### 7. Merge, the only gate

Report the PR URL, the version and the check result, then wait for the user
to approve the merge.

```bash
gh pr merge <PR> --rebase
git push origin --delete release/app-vX.Y.Z
```

### 8. Tag the merged commit

```bash
SHA=$(gh pr view <PR> --json mergeCommit --jq .mergeCommit.oid)
git fetch origin
git merge-base --is-ancestor "$SHA" origin/main
git show "$SHA:macos/project.yml" | grep 'MARKETING_VERSION: "X.Y.Z"'
git tag -a app/vX.Y.Z -m "Handbyte X.Y.Z" "$SHA"
git push origin app/vX.Y.Z                                   # starts macos-release.yml
```

Always an annotated tag on the merged commit, never on the local branch commit.

### 9. Wait for CI, then publish the release

```bash
RUN=$(gh run list --workflow=macos-release.yml --branch app/vX.Y.Z --limit 1 --json databaseId --jq '.[0].databaseId')
gh run watch "$RUN" --exit-status
gh release view app/vX.Y.Z --json isDraft,assets --jq '{isDraft, assets: [.assets[].name]}'
gh release edit app/vX.Y.Z --draft=false --latest=false
```

Publish only when the run passed and the draft has all four assets:
`Handbyte-X.Y.Z.dmg`, `Handbyte-X.Y.Z.dmg.sha256`, `Handbyte.dmg` and
`appcast.xml`. `--latest=false` keeps `releases/latest` on the CLI, which
`install.sh` reads. Publish from this session, not from CI: a release that
the workflow token publishes does not start `macos-publish.yml`. Then confirm
that run (`gh run list --workflow=macos-publish.yml --limit 1`) and that
<https://handbyte.gumpw.com/appcast.xml> carries the new build.

## Recovery

Roll forward; never force-push `main`. Deleting a pushed tag needs explicit
user approval.

| Symptom | Fix |
|---|---|
| CI: tag version ≠ `MARKETING_VERSION` | The tag is on the wrong commit: with approval, delete it and tag the merged bump commit. |
| CI: no CHANGELOG section | Add it in a PR; then rerun the workflow with `workflow_dispatch` on the same tag. |
| Notarization or another transient failure | `gh run view --log-failed`, then rerun with `workflow_dispatch` on the existing tag; the draft is updated in place. |
| Published release is broken | Mark its heading `[YANKED]` with a one-line reason and ship the fix as the next version. |

`docs/appcast.xml` is written only by `macos-publish.yml`; never edit it by hand.
