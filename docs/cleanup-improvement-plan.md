# Cleanup improvement plan

Status: proposal for review; no product changes implemented.
Research date: 2026-10-28.
Baseline: handbyte `e91b666`; Mole CLI `V1.65.0` (`308bf9911179ddfa40e5df9fdc42095a18ba5cfc`).

## Recommendation

Make handbyte help users decide what to remove, why, and at what cost. Keep the
Go CLI and Bubble Tea UI. Add application-aware and project-aware cleanup in
small vertical slices, after correcting safety and accounting.

Depth means finding disposable objects inside a large store, not making the
store itself a deletion target. More reported bytes is not the success metric.
Keep scan-only inspection useful even when cleanup is blocked.

## Research and evidence

Mole was installed through Homebrew (1.65.0). Its source was checked out at the
release tag. handbyte was built from the baseline into a temporary directory.
The research used the real home directory for read-only scanning and a separate
temporary HOME for small fixtures. No cleanup was applied to user data. Mole
may write its own logs/cache during preview. No sudo credentials were supplied
to Mole; system-only cleanup preview was therefore incomplete.

Homebrew also updated its metadata/runtime and reinstalled pkgconf for the
current macOS version during installation. The Mole native paid app was not
installed or evaluated.

### Observed handbyte behavior

| Finding | Evidence | Impact |
| --- | --- | --- |
| Duplicate candidates | Live `--scan-only --no-projects` returned 57 rows, 99.69 GiB summed, 96.61 GiB after exact-path deduplication. Four paths appeared twice: SwiftPM, JetBrains, Playwright and Homebrew caches. | About 3.08 GiB was counted twice. Selection can also schedule one path more than once. |
| Static names imply cleanup eligibility | An isolated Git-indexed `build/sample.bin` appeared as a review candidate. A `target` directory without Cargo metadata was described as Rust output. | A familiar basename is insufficient evidence of disposable content. |
| Whole data stores are candidates | Presets include Downloads, sandbox app containers, Docker configuration and VM data. The live VM category alone reported 47.80 GiB. | The user sees large numbers without a safe way to remove individual unused objects. |
| Preview reports successful cleanup | A fixture selected one unique 1 MiB Go cache shown in two categories. Confirmation showed two items/2 MiB. The observed dry-run result showed four cleaned items/4 MiB. | Preview, duplicate paths and late progress messages all affect trust. Four is an observed event-order outcome, not a deterministic count. |
| Long rows lose useful information | At 100 columns by 28 rows, descriptions were clipped and the footer wrapped; the list header was pushed out of the visible terminal area. | The user cannot reliably see totals, consequences and controls together. |

These live sizes are a snapshot of paths accessible to that scan, not a complete
disk inventory or an estimate of reclaimable space. Scan errors are currently
swallowed. Do not compare them directly with Mole's differently scoped totals.

### Source-confirmed gaps

- `internal/scanner/scanner.go`: preset and generic children are appended without
  deduplication; project discovery matches directory names; `du` discards errors.
  Recent file modification is collected but does not determine eligibility.
  Progress reads `len(items)` without the mutex used for append; check with the
  race detector as part of the scanner change.
- `internal/scanner/presets.go`: all direct Library cache directories receive the
  same safe label. Broad categories mix recoverable caches with personal state.
- `internal/cleaner/cleaner.go`: the executor receives paths and sizes, not the
  policy evidence. It has no shared final eligibility gate. Both dry-run and
  Trash success count the scan size as bytes freed. A direct rename to Trash
  does not itself write Finder restoration metadata. A timestamp collision in
  the Trash naming fallback is not rechecked.
- `internal/ui/ui.go`: progress appends results while completion replaces them;
  a later progress message can append a result again. Confirmation shows at most
  eight items. Stored scan errors are not rendered. Cleanup uses an uncancellable
  background context while the global quit handler remains available.
- No Go test files exist in the baseline. Add focused behavior tests with each
  implementation slice, not a large test-only rewrite.

### Observed Mole behavior and useful references

- Main menu separates Clean, Uninstall, Optimize, Analyze and Status by intent.
- Analyze was used to enter a fixture project directory. It shows relative sizes,
  a path, child directories, and controls for filter, open, preview and refresh.
- Purge dry-run was opened on the fixture. It grouped the remaining target under
  the project, displayed its full path and `<1d` age, and left it unselected.
  The Git-indexed build was absent. No deletion was confirmed.
- Live Clean dry-run showed its protected paths, category-by-category progress,
  owner-command previews, and inspection-only hints for Rust toolchains and VM
  data. It recommended inspecting Docker data instead of deleting the VM store.
- Live Clean dry-run completed with exit 0: `Potential space: 19.28GB+`, 2,190
  items across six categories. Its `+` estimate and lack of sudo-only coverage
  must be preserved when reporting it. Separate hints included Simulator data,
  DerivedData, pnpm store, OrbStack and AI worktrees. No user cleanup was applied.

Reproduction entry points (scan only):

```sh
handbyte --scan-only --no-projects
handbyte --dry-run --min=1K
mo --version
mo clean --dry-run
mo analyze /path/to/disposable/fixture
mo purge --dry-run
```

The fixture TUI tests used an isolated HOME and disposable 1 MiB files. The
Mole purge roots were configured inside that isolated HOME. Raw live inventories
were kept outside the repository because they expose local paths.
- Source: `lib/clean/dev.sh` resolves configured cache roots and uses owner tools
  for selected ecosystems, including `pnpm store prune`. This is a separate
  execution strategy, not permission to call a package manager blindly.
- Source: `lib/clean/purge_shared.sh` checks Git-tracked files;
  `lib/clean/project.sh` handles project grouping and recent/uncertain activity;
  `lib/core/file_ops.sh` centralizes many final deletion guards.

References: [Mole release](https://github.com/tw93/Mole/releases/tag/V1.65.0),
[developer rules](https://github.com/tw93/Mole/blob/V1.65.0/lib/clean/dev.sh),
[project rules](https://github.com/tw93/Mole/blob/V1.65.0/lib/clean/project.sh),
[shared checks](https://github.com/tw93/Mole/blob/V1.65.0/lib/clean/purge_shared.sh),
[filesystem boundary](https://github.com/tw93/Mole/blob/V1.65.0/lib/core/file_ops.sh).

Learn the behavior and implement it in handbyte's existing Go structure. Do not
copy the large Shell rule set. Mole is a reference, not a safety oracle: its own
[review](https://github.com/tw93/Mole/blob/V1.65.0/SECURITY_AUDIT.md) documents
limitations, and [issue 1870](https://github.com/tw93/Mole/issues/1870) reports
cleanup of packages still used by npx processes. That report was not reproduced
in this research.

## Proposed user flow

1. Choose Quick cleanup, Project cleanup, or Explore storage. Remember scan roots
   and exclusions. Show the scope before starting.
2. Show progressive results. Each category can be complete, partial, unavailable
   or cancelled. A partial size is a lower bound, never an empty directory.
3. Present Recommended, Review, and Protected/Inspect-only groups. Start with
   nothing selected; a single action selects only eligible recommendations.
4. Show application or project names first. Keep path, rule, age, activity,
   rebuild/download cost and recovery method in a detail pane. An unknown owner
   or activity state must be visible. Modification age is not last-use time.
5. Let users search, filter, expand, reveal in Finder and protect an item. Group
   selection must never silently select protected items.
6. Review the complete scrollable selection, grouped by action and consequence.
   Permanent deletion is explicit; Enter must not silently approve a destructive
   dialog. Show why anything became ineligible after scanning.
7. Report previewed, moved to Trash, permanently removed, skipped and failed
   separately. Give item-level reasons and recovery instructions. Offer rescan
   and retry of eligible failed items without repeating successful actions.

Example detail panel (illustrative data, not this machine's measurements):

```text
Old Xcode build data                         2.4 GiB
Project: ExampleApp
File activity: 45 days ago
Owner state: Xcode and build processes not detected
Effect: The next build must recreate this data.
Action: Move to Trash
[Space] Select  [Enter] Details  [O] Reveal  [P] Protect
```

## Implementation sequence

### P0 / PR 1: Trustworthy candidates and results

- Normalize and deduplicate targets; specific rules take precedence over generic
  cache rules. Prevent parent/child overlap in selection and execution.
- Preserve scan coverage and errors. Add correct empty, partial and cancelled
  states. Make progress and final result handling idempotent by run/action ID.
- Replace the single `Freed` meaning with separate preview bytes, bytes moved,
  estimated removed bytes, and observed volume free-space delta. The last value
  is environmental and must not be attributed entirely to this process.
- Add cancellable cleanup: stop before the next action, record completed actions,
  and wait for the current operation's outcome before exiting.

Acceptance: duplicate fixtures appear and execute once; dry-run reports zero
applied actions; late progress cannot change a final count; denied subtrees stay
visible as partial; cancellation leaves a correct partial report. Run focused
unit tests and `go test -race ./...`.

### P0 / PR 2: Shared eligibility and recovery boundary

- Pass structured candidates to the cleaner. Recheck identity, allowed scope,
  exclusions, owner activity and protected descendants immediately before action.
  Changed identity or unavailable required evidence means skip, not guess.
- Block root/home/structural directories, Git-tracked files, nested repositories,
  live data and unknown whole app containers from bulk cleanup. Downloads and
  VM roots become inspect-only entry points.
- Use native Trash semantics with a recorded destination and restoration plan;
  do not silently fall back to permanent deletion. Keep permanently destructive
  owner-tool actions distinct from recoverable Trash actions.
- Store a minimal local action journal; no file contents. Show recovery failure
  and destination collisions explicitly. Do not promise transactionality or
  protection from every concurrent filesystem replacement.

Acceptance: active-owner, failed-probe, symlink-swap, tracked-build, nested-repo,
protected-child and Trash-collision fixtures cannot bypass the execution gate.
Verify native Trash and restore with disposable files on macOS.

### P1 / PR 3: Review experience

- Add owner/project grouping, search, sorting, detail pane and persisted exclusions.
- Make scan progress incremental and allow browsing completed categories.
- Fix terminal layout using actual rendered header/footer/detail heights.
- Replace the eight-row confirmation truncation with a full review view.
- Show action-specific completion and item errors; retain a usable result screen.

Acceptance: exercise 80x24, 100x28 and 120x40 terminals, long and Unicode paths,
empty results, more than eight selected items, mixed actions and partial scans.
Recommended bulk selection never includes protected/inspect-only candidates.

### P1 / PR 4: Project and Xcode depth

- Identify project roots from manifests and Git/worktree metadata. Support custom
  roots, monorepos and nested projects without scanning dependency trees as projects.
- Classify artifacts using ecosystem evidence; check tracked descendants,
  active processes and recent file activity. Initially use 7 days as a conservative
  default for excluding recent artifacts from recommendations, not proof of safety.
- Split DerivedData by project and DeviceSupport by version. Keep Archives and
  dSYMs protected by default; do not treat published-build artifacts as caches.
- List unavailable simulators through supported tools. Add deletion only when
  preview and ownership checks can describe the exact operation.

Acceptance: old idle output is reviewable; recent or active builds are protected
from recommendations; a directory named build containing tracked source is blocked;
worktree roots and branches are never removed or changed.

### P2 / PR 5: Tool, application and AI cleanup

- Add ecosystem adapters for npm/pnpm, Python uv/pip, Go, Cargo and Homebrew in
  small batches. Resolve configured paths; bound probes and owner commands.
- Start application rules with caches/logs whose ownership is clear. Detect the
  app name and running state; avoid blanket safe labels for Library/Caches.
- Separate AI tool download caches and obsolete runtimes from conversation
  history, settings, credentials, models, databases and worktrees. Personal state
  remains protected; inspect-only reporting is useful without deletion support.
- Use file/version/age rules inside known roots. Protect active npx/MCP packages
  and unknown runtime ownership. Keep current and required fallback versions.

Acceptance: each adapter ships positive, protected, busy, unknown and failure
fixtures; preview and execution use the same policy. Required downloads and
loss of offline capability appear before selection.

### P2 / PR 6: Container storage and large-file review

- Start with read-only Docker/OrbStack storage inspection and object categories:
  images, build cache, containers and volumes. Validate the active endpoint is
  local and supported; never clean a remote Docker context implicitly.
- Add selective owner-tool cleanup only when exact targets and consequences can
  be reviewed. Volumes and VM disks stay protected by default. No blanket prune.
- Explore Downloads by individual installers/files and age; never offer the whole
  folder as one cleanup target. Add subtree navigation for other large stores.

Acceptance: every applied container action corresponds to reviewed objects;
remote/unavailable owners stay inspect-only; volumes, credentials and VM images
cannot enter bulk selection.

## Minimal internal design

Extend existing packages rather than introducing a plugin platform:

- Scanner produces candidates and coverage. A candidate includes a stable ID,
  rule/owner identity, target identity, measured size, activity evidence,
  consequence, eligibility and proposed action.
- Shared policy computes and revalidates eligibility. Keep simple path presets
  for simple caches; use small ecosystem adapters only where semantics require it.
- Cleaner executes typed actions with cancellation and emits one terminal result
  per action. The UI renders evidence; it does not decide safety itself.
- Add local configuration for roots/exclusions and a local operation journal.
  Version these formats. Preserve existing CLI flags; document any JSON evolution
  because `--scan-only` may already have consumers.

## Scope and decision

Recommended first release: PRs 1-4. This delivers trustworthy numbers, actionable
review, and useful developer cleanup without spreading into system optimization,
fan control, app updates, or a new GUI. PRs 5-6 follow after the common boundary
is exercised. Native owner commands may be irreversible and may not support
exact previews; unsupported actions must remain inspection-only.

The alternative is to add more preset paths immediately. That increases apparent
coverage but preserves the current uncertainty and deletion granularity. It is
not recommended. Approval of this proposal should precede the behavior and data
model changes; the research itself does not authorize their implementation.
