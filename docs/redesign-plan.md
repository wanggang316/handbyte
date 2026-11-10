# Redesign plan: clean, apps and storage for the CLI and the macOS app

Status: proposal, 2026-11-10. Not approved. The section "Open decisions" lists
what must be decided before implementation starts.
Research date: 2026-11-10. Baseline: handbyte `ac17ede`, handbyte-macos
`cae90e7`, Mole `28e68bd`, Pearcleaner `7724df7`.
Visual design: <https://claude.ai/artifact/UakSsGMe8KGWjUKtWb9yop> (private
canvas: four app screens, four terminal screens, one token and state sheet).

## Recommendation

Grow handbyte from one cleanup list into three tools that share one safety
boundary and one design language:

1. **Clean**: more categories, each with an explicit rule, never a wildcard
   sweep. Trash and restore stay the default.
2. **Apps**: size, last use and source for each app, and an uninstall that
   lists every related file with a confidence tier.
3. **Storage**: a volume overview and a drill-down view on a new scan engine
   that counts hard links and full clones once.

Keep the two native implementations (Go CLI, Swift app). Move the rule data
into one shared catalog and add shared conformance fixtures, so that the three
new engines do not drift.

Build the CLI and the app from the same information architecture, the same
tier vocabulary and the same token set. The canvas above shows both.

## Research basis

Three surveys were made from primary sources (source code, READMEs, vendor
pages, local man pages and SDK headers). Items marked *not verified* come from
secondary sources and need a test before the design depends on them.

### What the market does

| Area | Reference | What to take | What to avoid |
| --- | --- | --- | --- |
| Clean | Mole, CleanMyMac, MacSai, mac-cleaner-cli, DevCleaner, npkill | Scan, summary by category, grouped review, result page with a log. Written exceptions for smart selection (newest backup, newest DeviceSupport, recent artifacts stay unselected). A visible reason for each skipped item. | Permanent deletion without restore (Mole `clean`, `purge`, `installer`). Wildcard sweeps of `~/Library/Caches` (Mole issues 174, 577, 1941, 1390). System "optimize" actions (Mole issues 136, 862). All-or-nothing cleanup (Mole issue 859). |
| Apps | Mole `uninstall`, Pearcleaner, Nektony, CleanMyMac, `brew --zap` | Exact path templates with bundle ID dot boundaries. User data kept by default. Grouping by unused, large, source. Refusal with guidance for system apps and security software. | Substring matching after normalization (Pearcleaner needs 27 per-app conditions to correct it). Selecting a whole vendor folder. A root helper that runs arbitrary commands. |
| Storage | DaisyDisk, GrandPerspective, gdu, ncdu, dua, Mole `analyze` | Sorted list with share bars in the terminal. Map plus list in the app. A collector that gathers items before one confirmation. Cached result first, refresh in the background. An explicit "not scanned" amount. | Totals larger than the disk because of clones and hard links. Scans that enter iCloud folders. An unbounded cache (Mole recorded 7.82 GB). |

Mole is GPL-3.0. Pearcleaner is Apache-2.0 with the Commons Clause and its
README says the project is on hold. Use both as references only. Do not copy
their code or rule tables.

### Facts that shape the design

- `getattrlistbulk` returns allocated size, link count and, with
  `FSOPT_ATTR_CMN_EXTENDED`, the clone ID, extended flags and private size.
  *Not verified*: that the bulk call returns the clone attributes on every
  supported macOS version. DaisyDisk states that clone detection needs
  macOS 14.
- `ATTR_CMNEXT_PRIVATESIZE` is the number of bytes that a deletion frees
  immediately. Use it for "frees now" in confirmations.
- A scan from `/` must not also enter `/System/Volumes/Data`. Firmlinks cause
  double counting.
- A scanner thread can set `IOPOL_TYPE_VFS_MATERIALIZE_DATALESS_FILES` to off.
  Files with `SF_DATALESS` then stay in iCloud and count as zero.
- `kMDItemLastUsedDate` is updated only for launches through LaunchServices
  and can be null. The UI needs an "unknown" state that is different from
  "never opened".
- Homebrew cask `zap` data is BSD-2-Clause. The cask JSON has no bundle ID
  field, and the Homebrew documentation says that `zap` can remove shared
  resources. It is a medium-confidence hint only.
- *Not verified*: the TCC prompts for other apps' containers without Full Disk
  Access, the App Management prompt when an app moves another app from
  `/Applications`, and that a CLI inherits the Full Disk Access of its
  terminal. Test all three on a clean macOS 14 system before the Apps work
  starts.

## Product structure

The CLI and the app have the same five sections in the same order.

| Section | Question it answers | Primary action |
| --- | --- | --- |
| Overview | How full is this Mac and what can I do about it? | Run a scan |
| Clean | What can I remove that rebuilds by itself? | Move selected items to the Trash |
| Apps | Which apps do I not use, and what does each one keep? | Uninstall with related files |
| Storage | Where did the space go? | Open a folder, collect items for review |
| History | What did handbyte do, and can I undo it? | Restore |

Rules for every section:

- One primary action for each screen. It is the only accent-colored control.
- Three tiers with fixed names, glyphs and colors: **Safe** (●), **Review**
  (◐ in the app, ▲ in the terminal), **Protected** (▪ / ■). A tier is never
  shown by color alone.
- No item starts selected. "Select all safe" is one explicit action.
- Each row gives the name first, in plain words. The path, the rule ID and the
  evidence are in the detail pane.
- Storage and Apps never delete by themselves. They hand items to the same
  review screen and the same executor that Clean uses.

## Feature design

### Clean

Keep the present eligibility policy and the recheck before each action. Add
categories in this order. Each one is a named rule in the catalog.

| Category | Target | Default tier | Guard |
| --- | --- | --- | --- |
| User logs and crash reports | `~/Library/Logs`, `DiagnosticReports`, files older than 30 days | Safe | Keep the directories |
| Installers | `.dmg`, `.pkg`, `.xip`, `.iso` in `~/Downloads` and `~/Desktop` | Review | Listed file by file |
| Device firmware | `~/Library/iTunes/* Software Updates/*.ipsw` | Safe | |
| Browser caches | `Cache`, `Code Cache`, `GPUCache`, `Service Worker/CacheStorage` for each profile | Safe | The browser is not running. Cookies, history and profiles are never targets. |
| Xcode | Old DeviceSupport versions, DocumentationCache, unavailable simulators through `simctl` | Safe | The newest DeviceSupport for each platform stays unselected |
| Xcode archives | `~/Library/Developer/Xcode/Archives` | Review | Listed archive by archive |
| Known app caches | An allowlist of rules with owner bundle IDs | Safe | Owner not running. Unknown caches stay inspection-only. |
| Docker | Build cache and dangling images through the `docker` command | Review | Local context only. Volumes are never targets. |
| Mail downloads | Files older than 30 days | Review | Mail is not running |
| iOS backups | `MobileSync/Backup` | Review | The newest backup for each device is protected |
| Trash | `~/.Trash` | Review | Permanent. Needs its own confirmation. |
| Local snapshots | `tmutil listlocalsnapshots` | Protected | Shown as information only |

Flow: scan, then a summary with one tile for each category (size, tier split,
Review and Clean entry points), then the grouped review list, then
confirmation, then a result page with Restore, "Review remaining" and the log.

Not planned: `/Library/Caches`, `/private/var`, language files, universal
binary thinning and any "optimize" action. They need root or break code
signatures, and they caused the most severe reports against other tools.

### Apps

Inventory: `/Applications`, `~/Applications` and Homebrew Caskroom links.
Skip nested apps and symlinks into `/System`.

Facts for each app, in order of value: size of the bundle and size of its
data, last used date (or unknown), source (App Store, Homebrew, Setapp,
direct), running state, system protection, architecture, background items,
signing team. No thinning and no language pruning.

Filters: All, Unused (not opened for six months, date known), Largest,
Leftovers (data of apps that are no longer installed; a review list, never
preselected).

Matching of related files uses four confidence levels.

| Level | Rule | Default |
| --- | --- | --- |
| High | The name equals the bundle ID or starts with `<bundle id>.`. A UUID container whose metadata names the bundle ID. A launchd plist whose `Program` is inside the bundle. `.savedState`, `.binarycookies`, `.sfl*` with the bundle ID. | Selected |
| Medium | An exact name variant in a standard root (at least 3 characters, not a generic word). A cask `zap` path. A Group Container that no other installed app declares. | Caches, logs and state selected. User data not selected. |
| Low | A product folder below a vendor folder. Dot directories and XDG paths. A name match in `/Library`. | Shown, not selected |
| Never | A whole vendor folder. A Group Container that another installed app declares. `com.apple.*`. Credentials. Files of security or device-management software. | Not offered |

Guards: validate the bundle ID as reverse DNS, or use name rules only. Demote
every name-derived item when another install of the same family exists. No
substring matching. Recheck each path before the action.

Uninstall: quit the app if it runs, then move the bundle and the selected
items to the Trash as one journal entry, so that one Restore brings all of
them back. "Reset app" removes the data and keeps the bundle. Apps installed
by Homebrew use `brew uninstall`, so that the Homebrew state stays correct.
System apps, security software and apps with system extensions are refused,
and the screen says what to use instead. Items in `/Library` that need admin
rights are listed with a "remove in Finder" action in the first release. A
privileged helper is a later, separate decision.

### Storage

Scan engine (one design, two implementations):

- One `getattrlistbulk` loop for each directory. `fts` is the fallback, and
  `readdir` with `lstat` is used on network volumes.
- A bounded worker pool for directories (CPU count, at most 12). A worker
  walks in place when the pool is full.
- Allocated size is the main number. Logical size is a toggle.
- Files with more than one link count once for each `(device, inode)`. Full
  clones count once for each `(device, clone ID)`. Partial clones count in
  full and carry a "may share" mark.
- The scan does not cross mount points, does not enter dataless directories
  and records each unreadable directory. The parent chain shows "incomplete".
- Results stream about ten times each second. Esc stops the scan and keeps
  the partial result.
- A cache for each subtree with an entry threshold and a size limit. A stale
  result is shown at once and refreshed in the background. A cleanup
  invalidates the ancestors.

Views:

- **Volume overview**: used, free, purgeable and not scanned, with the reason
  for the last one (system volume, snapshots, folders without access).
- **Categories**: Apps, Developer, Documents, Media, System, Other, from path
  rules in the catalog. Developer shows Xcode, package manager caches,
  project build output and container images as sub-rows.
- **Drill-down**: a sorted list with share bars in the terminal. A treemap
  with a side list in the app. A sunburst is not planned, because a treemap
  uses the window better and shows single large files.
- **Largest files** and **old downloads** (age is a hint, not evidence).
- **Links**: a folder that a cleanup rule covers shows "N GB can be cleaned"
  and opens Clean with that filter. An `.app` opens Apps. Other items go to
  the collector, and the collector opens the standard review screen with
  Trash as the only action.

### History and the safety boundary

- One executor for Clean, Apps and the Storage collector. It keeps the present
  checks: identity, path, rule, exclusions, owner process.
- One journal format, read by both products, with an entry type for a group
  (an app and its files). The app gains Restore; today only the CLI has it.
- The result reports four numbers separately: moved to Trash, removed, frees
  now (private size) and the observed change of free space.

## CLI design

### Commands

```text
handbyte                  Home screen (interactive)
handbyte clean            Clean review        [--scope] [--min] [--dry-run] [--rm] [--json]
handbyte apps             App list            [--unused] [--json]
handbyte uninstall <app>  Uninstall review    [--with-data] [--dry-run]
handbyte analyze [path]   Storage drill-down  [--json]
handbyte history          Past actions        [--json]
handbyte restore <id>     Restore a Trash action
handbyte config           Roots and protected paths
```

Every interactive screen has a `--json` form that prints the same data and
never acts. The present flags (`--scan-only`, `--explore`, `--scope`,
`--history`, `--restore`, `--protect`, `--unprotect`, `--roots`) continue to
work for one minor release and print one line that names the new command.
The `--scan-only` JSON keeps `schema_version: 2`.

### Layout

One frame for all screens: breadcrumb and summary line, optional tabs, body,
status line, key bar. The layout comes from the terminal size on each resize.

| Width | Body | Key bar |
| --- | --- | --- |
| Below 60 columns | One column. Details in two lines above the key bar. Tab opens full details. Tier counts lose their words. | Primary action and three keys, then `? more` |
| 60 to 99 | One column with a detail block of five lines | Two priority groups |
| 100 and above | List on the left, detail pane on the right (about 60/40) | All keys of the screen |

Below 20 rows the header becomes one line. Names are cut in the middle, and
the size column is never cut. Layout uses the rendered cell width, so that CJK
names and long paths align.

### Hierarchy and color

A terminal has one font size. Hierarchy comes from five levels of weight and
tone: bold bright (the number that matters), bold (titles, the current row),
normal (names), dim (meta, paths), dim upper case (group labels).

Color has meaning and is never the only signal: violet for the cursor and the
primary action, green for Safe, amber for Review, grey for Protected, red only
in a permanent deletion confirmation. Storage categories use the same hues as
the app. The palette has a dark and a light variant (Mole issue 1296 reports
unreadable grey on light terminals), falls back to 256 and 16 colors, and
obeys `NO_COLOR`. Without color, glyphs and brackets carry all states.

### Guided interaction

- The home screen is a menu of four verbs. Each one has one sentence and one
  live number. A hint box explains the highlighted entry.
- The key bar always starts with the primary action as a filled chip. Its
  label names the result ("review 3 items", "open dev/").
- Empty and blocked states say what to do next ("Give your terminal Full Disk
  Access, then press r").
- Destructive confirmations need `y`. Enter never confirms. A permanent
  deletion needs the typed word.
- The first run shows three lines: what a tier means, that nothing is
  preselected, and that the Trash is the default.
- Mouse: click, wheel and row highlight under the pointer are on. A setting
  turns them off for users who select terminal text with the mouse.

## macOS app design

### Structure

- `NavigationSplitView`: sidebar (five sections, a volume meter at the
  bottom), content, and an inspector on the right for the focused item.
- Clean, Apps and Storage share one screen pattern: header with summary,
  filter row (segmented control and search), list, inspector, action bar.
- A command palette on ⌘K lists sections and actions. The keys match the CLI
  where a match exists (`/` filter, Space select, ⌘R rescan).
- The system frame stays native (window, toolbar, sidebar material). Custom
  tokens apply to the content, so that the app fits the macOS release it runs
  on.

### Design system

A new `HandbyteDesign` target holds the tokens and the components. No view
uses a raw color, font or duration.

- Color: neutral graphite surfaces, one violet accent, three tier colors, six
  category colors, each with a dark and a light value. Text contrast is at
  least 4.5:1.
- Type: SF Pro in seven steps (hero 40, title 20, head 14, body 13, meta 12,
  label 11 upper case, mono 12). All numbers use tabular figures.
- Shape: radius 6 for controls and rows, 8 for fields, 12 for cards. Hairline
  borders. No shadow, except on the palette and on sheets.
- Components: button (primary, secondary, ghost, danger), row, checkbox, tier
  badge, segmented control, search field, size bar, meter, treemap tile,
  toast, empty state.

### Hover and micro-interaction

Each interactive element has five defined states: rest, hover, pressed, focus,
disabled. The token sheet on the canvas shows them.

| Element | Hover | Other feedback |
| --- | --- | --- |
| Sidebar item | Tint. The icon moves 1 pt. | The current item keeps the tint and gains an accent rail |
| Button | Lighter fill. The primary button gains a soft ring. | Pressed: 1 pt down, 98.5% scale |
| List row | Tint. The checkbox border brightens. Reveal and Protect buttons slide in. The size brightens. | Focused row: accent tint. Protected row: dashed box and a reason. |
| Card | Stronger border, 1 pt lift. The arrow moves 3 pt. | |
| Meter segment, treemap tile | Brighter, slightly larger, tooltip with name and size | A click focuses the matching list row |
| Checkbox | | Accent fill with a short pop. The total in the action bar counts to the new value. |
| Scan | | Categories fill as results arrive. A partial size has a trailing `+`. |
| Cleanup result | | The free-space number counts up once. A toast offers Restore for 8 seconds. |

Timing: 120 ms for color, 160 ms for movement, 80 ms for a press. With Reduce
Motion, color changes stay and movement stops. Every hover action also has a
keyboard path and a context menu entry.

## Architecture

| Part | CLI repository | App repository |
| --- | --- | --- |
| Catalog | `catalog/*.json` (cleanup rules, leftover location templates, storage categories, protected bundle IDs). The source of truth, embedded with `go:embed`. | A copy as a package resource. `sync-check.sh` compares a hash, not rule IDs. |
| Conformance | `testdata/conformance/`: fixture trees with expected candidates, matches and sizes | The same fixtures, run by `swift test` |
| Scan engine | `internal/usage` (new) | `HandbyteCore/Usage` (replaces the `fts` walk in `DiskUsage.swift`) |
| Clean | `internal/scanner`, `internal/cleaner` (present, extended) | `Scanner`, `Cleaner` (present, extended) |
| Apps | `internal/apps` (new) | `HandbyteCore/Apps` (new) |
| Journal | Version 2 with group entries | The same format. Replaces `operations.log`. |
| UI | `internal/tui`: frame, theme, components, one package for each screen. Replaces the single `ui.go` model. | `HandbyteDesign` (new), one feature folder for each section, one store for each feature |

The scan engine comes first, because Clean sizes, app sizes and Storage all
use it.

## Delivery sequence

Each step is usable alone and ships in both products.

1. **Foundation**: catalog and conformance fixtures; the design tokens in both
   products; the CLI command set with the home screen and the responsive
   frame; the app shell with the sidebar and the Overview. No new cleanup
   behavior.
2. **Storage**: the scan engine, the volume overview, the drill-down, largest
   files, the collector, the links into Clean.
3. **Clean**: the new categories in the table order, the summary tiles, the
   result page, the shared journal and Restore in the app.
4. **Apps**: inventory and facts, then matching at the High and Medium levels,
   then uninstall and reset, then leftovers.
5. **Finish**: the command palette, the first-run guide, mouse support in the
   terminal, an accessibility review.

Acceptance for each step: the conformance fixtures pass in both products;
terminal screens are checked at 58x20, 80x24, 110x28 and 160x45 in a dark and
a light theme and with `NO_COLOR`; app screens are checked in both
appearances, with Reduce Motion and with keyboard only; no test acts on real
user data.

## Open decisions

1. **Core strategy.** Recommended: two native cores, with a shared catalog and
   shared fixtures. Alternative: the app embeds the Go engine and reads JSON
   events from it. That gives one implementation, but it adds a signed helper
   binary and a process boundary for progress and cancellation, and the app
   was moved to an independent Swift core one release ago.
2. **Admin-owned leftovers.** Recommended: list them and open Finder in the
   first release. Alternative: a privileged helper with a narrow, validated
   path interface. That is more complete, and it adds a large security
   surface.
3. **Scope of Clean.** Recommended: user-level rules only, as listed. This
   excludes system caches, which other tools offer.
4. **Storage view in the app.** Recommended: treemap with a side list.
   Alternative: sunburst.
5. **Sequence.** Recommended: Storage before Clean and Apps, because both use
   the engine. Alternative: Apps first, if uninstall is the most wanted
   feature.
