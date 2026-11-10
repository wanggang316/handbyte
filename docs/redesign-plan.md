# Redesign plan: clean, apps and storage for the CLI and the macOS app

Status: design agreed, 2026-11-10, revision 4. Steps 1 to 5 are
implemented on the branch `refactor/new-version` of both repositories; see
"Progress" for what is done and what is not. The section "Decisions" lists
what was decided.
Revision 2 added one-step cleaning, the waiting states, a lighter visual
direction and the logo. Revision 3 records the decisions, adds the system
areas to Clean, describes the removal modes and changes the logo.
Research date: 2026-11-10. Baseline: handbyte `ac17ede`, handbyte-macos
`cae90e7`, Mole `28e68bd`, Pearcleaner `7724df7`.
Visual design: <https://claude.ai/artifact/UakSsGMe8KGWjUKtWb9yop> (private
canvas: five app screens, five terminal screens, one token and state sheet,
one logo sheet. The Smart Clean screens play the scan as an animation).

## Recommendation

Grow handbyte from one cleanup list into three tools that share one safety
boundary and one design language:

1. **Clean**: one step for the safe items (Smart Clean), with "Choose items"
   and "Deep scan" for control. More categories and finer rules, each with an
   explicit rule, never a wildcard sweep.
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

- One primary action for each screen. It is the only filled ink control.
- Three tiers with fixed names, glyphs and colors: **Safe** (●), **Review**
  (◐ in the app, ▲ in the terminal), **Protected** (▪ / ■). A tier is never
  shown by color alone.
- Safe items are selected by default. Review items are never selected by
  default. Protected items cannot be selected.
- Each row gives the name first, in plain words. The path, the rule ID and the
  evidence are in the detail pane.
- Storage and Apps never delete by themselves. They hand items to the same
  review screen and the same executor that Clean uses.

## Feature design

### Clean

Keep the present eligibility policy and the recheck before each action.

#### Three depths

| Mode | Entry | Scope | Steps |
| --- | --- | --- | --- |
| **Smart Clean** | "Clean now" on Overview, `handbyte clean` | Safe items of the fast areas | Scan, one confirmation with the total, clean, result. `--yes` and a setting remove the confirmation. |
| **Choose items** | Link below the main button, `r` in the terminal, `handbyte clean --review` | All found items. Safe items start selected. | The grouped review list with the detail pane |
| **Deep scan** | Link below the main button, `d` in the terminal, `handbyte clean --deep` | Adds the slow and costly areas: project build output on all roots, installers, old downloads, iOS backups, Xcode archives, Docker, leftovers of removed apps | Always ends in "Choose items", because these are Review items |

Smart Clean never takes a Review item. After it finishes, the result names
what stays ("5.2 GB more needs review") and links to "Choose items".

#### Removal mode

Decided: by data class. What Mole `28e68bd` does:

- `mo clean` has no confirmation and no selection. It asks only for the
  administrator password, then scans and deletes section by section. It
  deletes permanently, and it also empties `~/.Trash` unless the user
  protects it.
- `mo purge` and `mo installer` delete permanently after a selection.
  `mo uninstall`, `mo analyze` and `mo slim` use the Trash after a
  confirmation. `mo uninstall --permanent` skips the Trash.
- Its audit keeps this split on purpose: "Changing every cleanup to Trash
  would change its existing reclaim-space contract."
- It has no restore. In place of one it has `--dry-run`, a whitelist, a path
  validator, a refusal when the owner process runs or holds an open handle
  on a cache, and an operation log.

The split in Mole follows what the data is. Data that its owner makes again
is deleted. Data that cannot be made again goes to the Trash. The proposal
uses the same rule and states the class in each catalog rule:

| Class | Examples | Removal | Recovery |
| --- | --- | --- | --- |
| Rebuildable | Caches, logs, build data, package manager stores | Direct | None needed. The owner makes it again. |
| Not rebuildable | Apps, leftovers with settings, installers, downloads, backups, archives, files from the Storage collector | Trash | Restore from History |

Smart Clean takes only rebuildable Safe items, so it removes them directly
and the freed number is real. Three points go further than Mole: the scan
ends before any removal, the user sees the total and confirms once, and
Smart Clean never empties `~/.Trash`. The option `--trash` and the setting
"Always use the Trash" change direct removal to the Trash; then the result
offers "Empty these items now".

#### Finer rules

Mole finds more because it has about 15 sections and several hundred
app-specific rules. The catalog grows in the same direction, but each rule
names exact subpaths, an owner bundle ID and a tier:

- Browsers: Safari, Chrome, Edge, Arc, Brave, Firefox, Vivaldi and others,
  for each profile, cache directories only.
- Developer tools: add Bun, pnpm, pip, uv, Poetry, CocoaPods, Flutter, Bazel,
  Composer, Terraform, VS Code and JetBrains logs and caches.
- Apps: named cache rules for chat, meeting, music, design and cloud-drive
  apps, most used first. Each rule lists what it must not touch.
- A new rule needs a positive fixture, a protected fixture and a "running
  owner" fixture before it can be Safe.

The first screen groups the result into six areas (Xcode, package managers,
app caches, browsers, logs, project build output). Each area opens to its
items.

#### Categories

Add categories in this order. Each one is a named rule in the catalog.

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
| System caches | Named rules below `/Library/Caches`. No `com.apple.*` rule without a fixture. | Safe | Admin rights. Owner not running. |
| System logs | `/Library/Logs`, rotated files in `/private/var/log`, `/Library/Logs/DiagnosticReports`, files older than 30 days | Safe | Admin rights. Keep the directories and the active log files. |
| System diagnostics data | `/private/var/db/diagnostics`, `/private/var/db/powerlog` | Review | Admin rights. Not while a diagnosis runs. |
| Update files | `/Library/Updates`, "Install macOS" apps | Review | Admin rights. Not while an update is pending. |

#### Waiting states

A scan must never show an empty screen. The rules are the same in the app and
in the terminal.

1. **Show the last result at once.** Overview opens with the cached number
   and the line "Found 2 minutes ago · updates in the background". A new scan
   replaces the number when it is ready.
2. **Fast areas first.** Known paths are measured in parallel and report in
   the first second. Project discovery runs last. Deep areas run only on
   request.
3. **One line for each area, three states.** Waiting (dim dot), scanning
   (spinner, the current path, a size with a trailing `+`), complete (check
   mark, size, a short summary such as "12 apps").
4. **One large number that grows.** "12.4 GB found so far", then "18.4 GB
   safe to clean", then "18.4 GB freed". The same position is used in all
   phases, so the screen does not jump.
5. **Evidence of speed.** A thin progress line by areas, and one meta line
   with files checked, files per second and elapsed time.
6. **Stop keeps the result.** "Stop and use what was found" goes to the
   confirmation with the areas that are complete. The scan continues when the
   user changes tabs.
7. **Cleaning uses the same list.** Each line changes to "Cleaned" in turn.
   The free-space value counts up. The result gives the time, the change of
   free space, the skipped items with their reason and the next step.

Targets to measure in the first step, not promises: the first area result in
0.3 s, a Smart scan in less than 10 s on a developer Mac with a warm cache,
and no frame without movement for more than 1 s.

Flow for "Choose items" and "Deep scan": the same scan screen, then the
grouped review list, then confirmation, then the same result page with
"Review remaining" and the log.

System areas are in scope, because the main tools of this type have them.
They are a separate area in the result ("System", needs an administrator),
they use the same named rules, and Smart Clean includes them only when admin
rights are available. See "Admin rights".

Not planned: language files, universal binary thinning, `/private/var/folders`
and any "optimize" action. They break code signatures or remove data that
running processes use.

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
rights follow the section "Admin rights".

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

#### Admin rights

Some targets belong to root: the system areas of Clean, and the parts of an
app in `/Library/LaunchDaemons`, `/Library/PrivilegedHelperTools` and
`/Library/Application Support`. A process of the user cannot remove them.
There are three ways to get the right:

| Way | How | Cost |
| --- | --- | --- |
| No rights | List the items. "Show in Finder" lets the user remove them. | Not one step. A launch daemon stays loaded until restart. |
| One-time authorization | CLI: `sudo`, asked once for each run, as Mole does. App: the system password dialog for one run of a fixed, validated command. | A password prompt for each run. No code stays installed. |
| Privileged helper | A small root daemon that the app installs once (`SMAppService`). The app sends requests through XPC. | No more prompts. A root service stays installed, and each request must be validated against the catalog. A defect in it is a privilege escalation. |

Proposal: one-time authorization in both products. The root side accepts
only rule IDs and journal entry IDs, never paths or commands, and applies
the same executor checks. The helper is not built unless the prompt proves
to be a real problem. To test first: the app dialog on the supported macOS
versions, and that a user process cannot replace the executed binary.

## CLI design

### Commands

```text
handbyte                  Home screen (interactive)
handbyte clean            Smart Clean         [--review] [--deep] [--yes] [--dry-run] [--json]
handbyte apps             App list            [--unused] [--json]
handbyte uninstall <app>  Uninstall review    [--with-data] [--dry-run]
handbyte analyze [path]   Storage drill-down  [--json]
handbyte history          Past actions        [--json]
handbyte restore <id>     Restore a Trash action
handbyte config           Roots and protected paths
```

`handbyte clean` is not a full-screen program. It prints the area lines in
place, as Mole does, and stays in the scrollback: scan, a three-line summary
(safe, needs review, skipped), one key to confirm, the cleaning lines and the
result. Enter confirms here, because only Safe items are in scope. `r` opens
the full-screen "Choose items" view and `d` starts a deep scan.

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

Color has meaning and is never the only signal: a white chip for the primary
action, a neutral band for the cursor row, green for Safe, progress and done,
amber for Review, grey for Protected, red only in a permanent deletion
confirmation. Storage categories use the same hues as
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
- Enter confirms Smart Clean only. Confirmations that include Review items,
  apps or collected files need `y`. A permanent deletion of such items needs
  the typed word.
- The first run shows three lines: what a tier means, that nothing is
  preselected, and that the Trash is the default.
- Mouse: click, wheel and row highlight under the pointer are on. A setting
  turns them off for users who select terminal text with the mouse.

## macOS app design

### Structure

- No sidebar. A slim top bar holds the mark, five pill tabs and the command
  field. The window is one column, so it feels like a utility, not a
  workspace. The default window is small (about 880 by 620).
- Overview is one centered statement: the number that can be cleaned, one
  pill button, two text links for the deeper modes, then the volume meter and
  three cards.
- "Choose items", Apps and Storage share one screen pattern: header with
  summary, filter row (segmented control and search), list, inspector on the
  right for the focused item, action bar.
- A command palette on ⌘K lists sections and actions. The keys match the CLI
  where a match exists (`/` filter, Space select, ⌘R rescan).
- The system frame stays native (window, toolbar, materials). Custom
  tokens apply to the content, so that the app fits the macOS release it runs
  on.

### Design system

A new `HandbyteDesign` target holds the tokens and the components. No view
uses a raw color, font or duration.

- Direction: ink on paper with one green signal. The first proposal used a
  violet accent on graphite with a sidebar. It looked like a project tool and
  felt heavy, so it was dropped.
- Color: white or near-black ground, soft grey fills in place of outlines.
  The primary action is ink (black on light, white on dark). Green is the
  only brand color and means safe, progress and done. Amber and grey are the
  other two tiers. Six category colors are used in meters and maps only.
  Each token has a light and a dark value. Text contrast is at least 4.5:1.
- Type: SF Pro in seven steps (hero 64, title 20, head 14, body 13, meta 12,
  label 11 upper case, mono 12). All numbers use tabular figures. The hero
  number is the main visual element of Overview and Smart Clean.
- Shape: full pills for the main button, tabs, search and segmented controls.
  Radius 8 for rows and small buttons, 16 for cards. Cards have a fill and no
  border. No shadow, except on the palette and on sheets.
- Speed in the UI: no modal sheet in Smart Clean, no page change between scan,
  confirm, clean and result, and state changes of 120 to 220 ms.
- Components: button (primary, secondary, ghost, danger), row, checkbox, tier
  badge, segmented control, search field, size bar, meter, treemap tile,
  toast, empty state.

### Hover and micro-interaction

Each interactive element has five defined states: rest, hover, pressed, focus,
disabled. The token sheet on the canvas shows them.

| Element | Hover | Other feedback |
| --- | --- | --- |
| Tab | Tint. The icon rises 1 pt. | The current tab keeps the tint and uses a heavier weight |
| Button | Stronger fill. The primary button gains a soft ring. The large pill rises 1 pt. | Pressed: 1 pt down, 98.5% scale |
| Text link | The underline and the text darken | |
| List row | Tint. The checkbox border brightens. Reveal and Protect buttons slide in. The size brightens. | Focused row: accent tint. Protected row: dashed box and a reason. |
| Card | Stronger border, 1 pt lift. The arrow moves 3 pt. | |
| Meter segment, treemap tile | Brighter, slightly larger, tooltip with name and size | A click focuses the matching list row |
| Checkbox | | Accent fill with a short pop. The total in the action bar counts to the new value. |
| Scan | | See "Waiting states". Area lines change state in place. The spinner, the skeleton and the live dot are the only looping animations. |
| Cleanup result | | The free-space number counts up once. Actions that used the Trash show Restore for 8 seconds. |

Timing: 120 ms for color, 160 ms for movement, 80 ms for a press. With Reduce
Motion, color changes stay and movement stops. Every hover action also has a
keyboard path and a context menu entry.

### Logo

Direction: **"Bite"**. It is a rounded block of storage with a piece cut
out of the top right corner, and a green rounded square in the cut: the
byte that left. It shows freed space without a broom or a disk, and the word
"bite" echoes "byte".

The first drawing had a small byte that went over the outline of the block,
which is the shape of an unread badge. Its round bite also did not fit the
square byte. The canvas shows the drawings that correct this:

- **B1, Fitted cut** (chosen): the cut is a square with a small inner
  radius, at an equal distance from the byte on both sides. On a 64 grid:
  byte 19 with radius 6.5, gap 4.5, inner radius 11. The two parts together
  make the full rounded square.
- **B2, Fitted cut, outer corner**: the same cut. The outer corner of the
  byte takes a larger radius, nearer to that of the block.
- **A1, Round bite**: a larger round bite (radius 24) with a byte of 17.
  Kept for comparison.

Other directions were tried and set aside: a letter h, three trimmed rows,
and two capsule bars (before and after).

- Colors: ink and one green. No gradient.
- App icon: the mark in white on an ink squircle. A light variant is in the
  canvas.
- Wordmark: the mark, then "handbyte" in lower case, weight 650, tight
  tracking.
- Terminal: two cells, `▙▝`, then the name in bold.

In the Foundation step, redraw `scripts/render-app-icon.swift`, the favicon and the
website header.

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
   frame; the app shell with the top bar and the Overview. No new cleanup
   behavior.
2. **Storage**: the scan engine, the volume overview, the drill-down, largest
   files, the collector, the links into Clean.
3. **Clean**: the new categories in the table order, the summary tiles, the
   result page, the shared journal and Restore in the app.
4. **Apps**: inventory and facts, then matching at the High and Medium levels,
   then uninstall and reset, then leftovers.
5. **Finish**: the command palette, the first-run guide, mouse support in the
   terminal, an accessibility review.

### Progress

Step 1, Foundation, 2026-11-10:

- Shared catalog `catalog/cleanup.json` with validation, embedded in the CLI
  and copied into the app. A golden test fixes the rule set.
- Conformance cases in `testdata/conformance/cleanup`, run by `go test` and
  by `swift test`. One difference is recorded in `SYNC.md` of the app: the
  CLI needs a project marker for build output, the app does not.
- CLI: the commands `clean`, `analyze`, `history`, `restore`, `config`,
  `help` and `version`; the home screen; the frame with the three widths.
  `clean` opens the present review screen. The stream form, `apps` and
  `uninstall` come with their steps.
- App: design tokens and controls, the toolbar with the section tabs, the
  Overview. Clean is the present flow. Apps, Storage and History show a
  description.
- Logo B1 in the app icon, the favicon, the website header and the terminal.

Step 2, Storage, 2026-11-10 (first part):

- Scan engine in both products (`internal/usage`, `HandbyteCore/Usage.swift`)
  with a shared conformance case. It reads with `readdir` and `lstat`, walks
  in parallel, counts a file with several names once, does not follow
  symbolic links, does not enter other volumes or dataless folders, and marks
  the parents of an unreadable folder. On `/opt/homebrew` (167,000 files)
  both report the same totals in about 0.6 s (Swift: release build).
- CLI: `handbyte analyze` shows running totals, then the drill-down with a
  detail pane on a wide terminal, and the largest files. `--json` prints the
  first level and the largest files.
- App: the Storage section with running totals, a treemap with a side list,
  the largest files, and "Review in Clean" for a folder that a rule covers.
- Categories from `catalog/storage.json` (Apps, Developer, Documents,
  Media, Caches and logs, Other) with a shared conformance case. The CLI
  home screen and the app Overview split the used space by them.
- The last result of each folder is kept. The next scan shows the last
  sizes until each folder is measured again.
- The collector, in both products: pick files and folders, review them,
  move them to the Trash. Only items in the home folder; standard folders,
  private data, excluded paths and symbolic links are refused; a folder that
  a cleanup rule covers points to Clean. Each move is journaled; the CLI can
  restore it.
- The app Overview names the purgeable part of the free space.
- Deferred: `getattrlistbulk` and counting APFS clones once. The Go standard
  library and `x/sys/unix` have no wrapper for the call, and the clone
  attributes are not verified on the supported macOS versions (see "Facts
  that shape the design"). Until then a clone counts in full, as `du` does.
  Next: a probe on macOS 14 and 27 that checks `ATTR_CMNEXT_CLONEID` and
  `ATTR_CMNEXT_PRIVATESIZE` from the bulk call, then the Swift engine first.

Step 3, Clean, 2026-11-10:

- Catalog version 2: areas, the data class (`rebuildable`), owner processes
  and owner names, `*` wildcards for browser profiles, and 50 more rules
  (browsers for each profile, developer tools, VS Code and Electron app
  caches, Xcode documentation and device logs, crash reports, device
  firmware, iOS backups, Mail downloads). A preset location that holds the
  location of a more specific rule is not reported. Every preset has a
  positive fixture test; every safe rule names its owner processes.
- Smart Clean in both products: area lines while scanning, the safe total,
  what needs review and what is skipped with the reason, one confirmation,
  the result with the change of free space. The CLI form is a stream in the
  scrollback; `--yes`, `--review`, `--deep`, `--trash`, `--rm`, `--dry-run`.
  Choose items starts with the safe items selected.
- Removal by data class: rebuildable data is removed, all other data goes to
  the Trash. "Always use the Trash" and `--trash` keep the old behavior.
- History with Restore in the app.
- System rules (`catalog/system.json`): system crash reports older than 30
  days and rotated system logs older than 7 days, as fixed `find` commands.
  The CLI runs them with `sudo` (`handbyte clean --system`), the app after
  the administrator dialog. Not done: `/Library/Caches`, update files and
  the unified log store; they need more than an age and a name pattern.

Step 4, Apps, 2026-11-10:

- Inventory in both products: bundle ID, version, source (App Store,
  Homebrew with the cask zap paths, Setapp, direct), running state, last use
  from Spotlight ("unknown" is not "never"), architectures, size.
- File matching by `catalog/apps.json`, with shared conformance cases: a
  bundle ID at a dot boundary is high, the app name or a cask zap path is
  medium, a dot folder is low; a name that belongs more exactly to another
  installed app is not taken; `com.apple.*` is never offered. Caches, logs
  and state are selected, settings for a high match, data only on request.
- Uninstall and reset (keep the app, remove its files). Apps of macOS,
  running apps and apps from Homebrew are refused with the reason; for
  Homebrew the screen names `brew uninstall --cask`. The CLI journals an
  uninstall as one group for `handbyte restore <group>`.
- Leftovers: folders (and `.plist` files in Preferences) named by a bundle
  ID whose vendor, vendor word and app name match no installed app, without
  UUIDs and shared libraries. Review only, nothing selected.
- Not done: background items and system extensions of an app, signing
  team, items in `/Library` that need admin rights, `brew uninstall` run by
  the tool itself.

Step 5, Finish, 2026-11-10:

- App: command palette (⌘K), ⌘1 to ⌘5 for the sections, ⌘R for Clean Now,
  a first-run note on the Overview.
- CLI: three first-run lines on the home screen and before Smart Clean;
  mouse support (wheel, click, a second click opens) in the drill-down and
  the apps list, `handbyte config mouse off`.
- Accessibility: every button of the app has a label (checked with the
  accessibility API). Not done: a full VoiceOver pass and a Reduce Motion
  pass.

Not done in step 1: the terminal screens were checked by tests at 58x20,
80x24, 110x28 and 160x45 without color, and by one run in a pseudo terminal;
a light terminal theme was not checked. The app was checked by screenshots of
the Overview, a planned section and the Clean start screen in the light
appearance; the dark appearance, Reduce Motion and keyboard-only use were
not checked.

Acceptance for each step: the conformance fixtures pass in both products;
terminal screens are checked at 58x20, 80x24, 110x28 and 160x45 in a dark and
a light theme and with `NO_COLOR`; app screens are checked in both
appearances, with Reduce Motion and with keyboard only; no test acts on real
user data.

## Decisions

Decided on 2026-11-10:

- **Core strategy.** Two native cores with a shared catalog and shared
  fixtures. The app does not embed the Go engine.
- **Scope of Clean.** As complete as is reasonable. The system areas that
  the main tools of this type clean are in scope, with named rules and admin
  rights.
- **Storage view in the app.** Treemap with a side list.
- **Sequence.** Storage before Clean and Apps.
- **Logo.** "Bite", drawing B1 (fitted cut).
- **Removal mode.** By data class, as in "Removal mode": direct removal for
  rebuildable items, the Trash for all other items, `--trash` as the option.
- **Admin rights.** One-time authorization in both products. No installed
  helper in the first release.

No decision is open.
