# ghab -- GitHub browser TUI (BUILD spec)

**SOURCE OF TRUTH for the ghab build.** Supersedes nothing; genesis doc. Branch: `ghab/genesis`.

## What it is

A terminal GitHub browser: browse ANY repo like the github.com page (readme / code tree / releases / issues tabs), hop to any user's profile and their repos, clone from inside, open files in `micro`. GitHub-first (remote browsing), NOT a local-git tool (that's lazygit) and NOT a work dashboard (that's gh-dash).

Runs on both hosts (Mac + Pi, aarch64). Single static Go binary, cross-compiled from the Mac. Rides the existing `gh auth login` -- zero token UX.

Prior art (researched 2026-07-12): nothing occupies this niche. Closest: githut (unvetted), ghgrab (no social layer), gh-eco (pinned-only profiles). gh-dash's section/keybinding/theming architecture is the pattern to imitate, not extend.

## Stack (pinned -- do not substitute)

- Go >= 1.22, module `github.com/Sh-ui/ghab`, source rooted at `ghab/` in the vault
- `github.com/charmbracelet/bubbletea` **v1 line** (NOT v2 / charm.land imports)
- `github.com/charmbracelet/bubbles` (list, viewport, textinput, spinner, help)
- `github.com/charmbracelet/lipgloss` v1
- `github.com/charmbracelet/glamour` (readme render; JSON stylesheet via `glamour.WithStylePath`)
- `github.com/alecthomas/chroma/v2` (code view syntax highlight, `formatters/tty` truecolor ANSI)
- `github.com/cli/go-gh/v2` (`pkg/api` RESTClient -- auth resolution free: GH_TOKEN env then gh's stored OAuth token)
- `github.com/BurntSushi/toml` (config)
- No other deps without a reason recorded here.

## Layout

```
ghab/
  BUILD.md            this file
  go.mod / go.sum
  main.go             flag parsing (--check-config, --version, [owner[/repo]] arg), tea.NewProgram
  internal/
    config/           config port: load, defaults, fail-soft validate, palette resolver
    gh/               data layer: typed fetchers over go-gh REST, caching
    ui/               tea model tree: app shell, screens, tabs, components
      style/          lipgloss styles built FROM config theme (no hex literals in ui code)
  styles/
    readme-ombre-dark.json    glamour stylesheet, tracked
    readme-ombre-light.json
  build.sh            builds darwin-arm64 + linux-arm64 into dist/ (gitignored)
  .gitignore          dist/
config/ghab/config.toml     tracked default config (vault config root -- host-neutral)
```

## Config port (REQUIREMENT, v1, not a retrofit)

File: `~/.config/ghab/config.toml` (XDG_CONFIG_HOME respected). Vault tracks the canonical default at `config/ghab/config.toml`; installer symlinks it into place (wiring happens at merge -- build must not assume the symlink exists). Re-read at launch (natural start point). **Fail-soft everywhere**: a bad key/value logs one warning line to stderr and keeps the compiled default; never crash on config. `ghab --check-config` prints resolved config + palette resolution + warnings, exit 1 only on unreadable file syntax.

```toml
[palette]
# canonical ombre palette -- resolved by NAME, never forked. "auto" searches:
# $GHAB_ROOT/config/ombre-palette.json, ~/dev-root/config/ombre-palette.json,
# ~/Developer/dev-root/config/ombre-palette.json. Missing -> compiled fallback hexes + warn.
file = "auto"

[theme]
# NEUTRALS follow the terminal mode (house rule: never pin cream on cream).
# ANSI slots: fg = 15, dim = 7, muted = 8. Background = terminal default (unset).
# ACCENTS are palette names resolved from [palette].file, or literal #hex.
accent        = "coffee"    # active tab underline, focused-frame rule, spinner
accent_alt    = "teal"      # links, repo names
open_marker   = "green"     # open issue bullet
closed_marker = "red"       # closed issue bullet
release_tag   = "yellow"    # release tag chips
frame         = "ansi:8"    # box borders (slate slot)
selection_bg  = "ansi:8"    # fallback if terminal lacks reverse-video finesse; see style.go

[readme]
# glamour stylesheet paths; "auto" = tracked styles/ files resolved next to the palette search,
# fallback = glamour built-in "notty"-safe dark. Mode picked via ~/.config/ghab/color-mode
# (values: dark|light; missing file -> dark).
style_dark  = "auto"
style_light = "auto"

[behavior]
clone_dir   = "~/Developer"        # Pi override in its own config: ~/src
editor      = "micro"              # open-file hook; %s substituted with path, else appended
open_url    = "auto"               # "auto" = wend if on PATH, else open/xdg-open
page_size   = 30                   # issues/releases page length
cache_ttl   = "5m"                 # in-memory per-endpoint TTL
# search/issues queries behind "my PRs" (issue #11). GitHub search has no OR between
# qualifiers, so the authored-or-review-requested scope is two queries, merged and
# de-duplicated client-side. Accepts a single string (one query) or an array of strings.
my_prs_query       = ["is:pr is:open author:@me", "is:pr is:open review-requested:@me"]
diff_context_lines = 3             # unchanged lines shown per side of a PR diff hunk; 0 = no trimming

[keys]
# every binding remappable; defaults below are the compiled defaults (one key per line -- TOML)
quit = "q"
help = "?"
search = "s"
back = "esc"
tab_next = "l"
tab_prev = "h"
down = "j"
up = "k"
open = "enter"
profile = "u"
clone = "c"
edit = "e"
web = "o"
refresh = "r"
my_prs = "ctrl+p"
```

Palette JSON shape (do not fork values -- read the file): `{"cream":"#F8F2E9","slate":"#797B86","umbra":"#1F1E1D","coffee":"#AD774E","teal":"#00ACC1","green":"#43A047","red":"#ED4B40","yellow":"#BF9800", ...}`. Compiled fallback = this exact set, used only when no palette file is found.

## House style (docs/desktop/tui-visual-style.md -- binding)

- Square corners everywhere: `lipgloss.NormalBorder()`, never rounded.
- Title rides the top border of framed panes (`┌─ ghab ─┐` pattern).
- Slate keybind-hint footer on EVERY screen: `j/k move  l/h tab  enter open  u profile  c clone  ? help  q quit` (contextual, from live keymap).
- Selection = full-width hard-edged bar (ANSI 8 bg or reverse), not colored text.
- Accents as small markers (bullets, tag chips, underlines) -- never fills.
- Neutrals via ANSI slots so light/dark terminal modes both work with zero flip wiring. NO truecolor neutrals in ui code. Accents may be truecolor (luma-balanced for both grounds).
- Flat: no gradients, no shadows.

## Data layer (`internal/gh`)

One `Client` wrapping `api.DefaultRESTClient()`. Typed structs only for fields the UI shows. In-memory cache keyed by URL, TTL from config; `r` key busts cache for current view.

| fetch | endpoint | notes |
|---|---|---|
| Repo meta | `repos/{o}/{r}` | stars, desc, default_branch, topics, license, fork-of |
| Tree | `repos/{o}/{r}/git/trees/{branch}?recursive=1` | ONE call per repo; if `truncated:true`, lazy per-dir via `contents/{path}` |
| File | `repos/{o}/{r}/contents/{path}` w/ `Accept: application/vnd.github.raw+json` | cap at 1 MB; binary sniff -> "binary file (n KB)" placeholder |
| Readme | `repos/{o}/{r}/readme` raw | glamour renders client-side |
| Releases | `repos/{o}/{r}/releases?per_page={page_size}` | assets inline; note arch-matching asset (linux-arm64 etc.) with a marker |
| Issues | `repos/{o}/{r}/issues?state=all&per_page=...` | entries with `pull_request` key are PRs -- split into Issues / PRs lists |
| Issue detail | `.../issues/{n}` + `.../issues/{n}/comments` | body + comments through glamour |
| User | `users/{u}` + `users/{u}/repos?sort=updated&per_page=100` | profile hop |
| Search | `search/repositories?q=...` | HARD debounce: fire only on enter, never per-keystroke (30 req/min bucket) |
| My PRs | `search/issues?q=...` once per `{my_prs_query}` entry | cross-repo, issue #11; results merged + de-duplicated by repo+number (`gh.MergeSearchIssues`), same 30 req/min bucket -- fires once per screen push, never re-fires on scroll |
| PR files | `.../pulls/{n}/files?per_page=...` | filename, status, +/- counts, per-file unified diff ("patch"); GitHub omits `patch` for binary files, pure renames, AND oversized text diffs -- `PullFile.Omission` tells the three apart |
| PR review comments | `.../pulls/{n}/comments?per_page=100` | line-anchored (path + line/original_line + side); grouped client-side by file:line, see `gh.GroupReviewThreads` |

Rate limits are a non-issue at human browsing pace (5000/hr authed) except search -- hence enter-to-search. Errors: 404 -> "not found or no access"; network fail -> inline error line + retry key, never a crash.

## UI (tea model tree)

App shell = one root model owning a screen stack (push/pop, `esc` pops). Screens:

1. **Home** -- textinput: `owner/repo` jumps straight in, bare `owner` -> profile, anything else -> repo search results list. Recent-visits list below (session-only for M1; persisted later).
2. **Repo** -- header (owner/name, stars, desc, branch) + tab bar: `readme | code | releases | issues | prs`. Tab content in a viewport. Actions: `c` clone, `u` owner profile, `o` open on web, `r` refresh.
3. **Code tab** -- left pane tree (collapsed dirs, lazy expand), right pane file preview (chroma, line-number gutter). `enter` on file focuses preview; `e` opens in editor.
4. **File view / editor hook** -- `e` writes blob to `$XDG_CACHE_HOME/ghab/{owner}/{repo}/{path}` (real filename so micro gets syntax), then `tea.ExecProcess(exec.Command(editor, path))`, resume TUI on exit.
5. **Releases tab** -- list release -> expand body (glamour) + assets; asset `enter` downloads to clone_dir/downloads with progress.
6. **Issues / PRs tabs** -- list (state bullet in open/closed marker colors, #num, title, author, age) -> detail view w/ comments. A PR row's detail is the fuller `prDetailModel` (conversation / files-changed / diff / review threads) -- see "PR-view extension" below.
7. **Profile** -- user card (name, bio, followers, location) + their repos list (sorted updated) -> `enter` enters repo. THIS IS THE HOP -- must be seamless both directions.
8. **Help** -- full keymap overlay, generated from live (config-merged) keymap.
9. **My PRs** -- cross-repo screen (issue #11): every open PR authored by or requesting review from the signed-in user, across every repo, one search fetch per push. Row grammar: state bullet, repo, #num, title, age. `enter` pushes the PR detail screen (same `prDetailModel` as the per-repo PRs tab).

Async pattern: every fetch = tea.Cmd returning a typed msg; spinner while pending; all list state survives tab switches (models kept per tab, not rebuilt).

Clone = `gh repo clone {o}/{r} {clone_dir}/{r}` via ExecProcess (inherits gh auth + protocol config); on success show path + hint `cd` line printed after quit.

## PR-view extension (issue #11, branch `ghab/pr-view`)

Motivated verbatim by Ian's issue #11: "Viewing my pull requests and going through them in gh -- gh-dash sucks ... I need to be able to browse and read pull requests on the lab pie without having to go into Piper and right now that's not easy." **Mac-built and gate-verified only -- no device pass yet** (label per every claim below: applied, device-UNVERIFIED, needs a both-modes visual check per tui-visual-style.md before it's called done).

**Cross-repo my-PRs screen** (`internal/ui/myprs.go`, `MyPRsScreen`) -- pushed from Home via the new `my_prs` key (default `ctrl+p`, a chord rather than a bare letter so it works no matter what's already typed into the input; a bare `p` was tried first and rejected -- gated to an empty input it still ate the first keystroke of any owner/repo or search value starting with "p", which is a far more common leading letter than the pre-existing `q`-for-quit idiom this pattern otherwise follows). One `search/issues` fetch per `[behavior].my_prs_query` entry (default `is:pr is:open author:@me` + `is:pr is:open review-requested:@me`) lists every open PR authored by or requesting review from the signed-in user, across every repo -- ghab's only screen that isn't scoped to one repo. Two queries rather than one because GitHub's search syntax has no OR between qualifiers: `author:@me review-requested:@me` in a single query means BOTH and matches nothing, while the one-query `involves:@me` is a wider scope that also pulls in PRs the user merely commented on or was mentioned in. The pages merge and de-duplicate by repo+number, most-recently-updated first. Same hard search-endpoint debounce as the existing repo search (fires once per push, never per keystroke). Row grammar: state bullet, repo, `#num`, title, age. `enter` pushes the PR detail screen.

**PR detail** (`internal/ui/prdetail.go`, `prDetailModel`) -- a reusable sub-model, not a `Screen` itself, used in two places per the build brief ("new view AND existing per-repo PRs tab"): `PRDetailScreen` (`prdetail_screen.go`) wraps one for the my-PRs hop; `issuesTab` (kind `issueKindPR`) owns one directly, swapping it in for the old body+comments-only detail the PRs tab used to show (real issues, kind `issueKindIssue`, are untouched -- they have no files/diff and keep the original detail view exactly as M3 shipped it). Three sub-views, cycled with `tab` (conversation <-> files; diff is entered from a file, `esc` steps it back to files, `esc` from conversation/files exits detail to the list -- same "back pops one level" convention as the rest of the screen stack):

- **Conversation** -- unchanged from M3: title/state/author/age header + body + comments through glamour.
- **Files changed** (`repos/{o}/{r}/pulls/{n}/files`) -- one row per file: a one-letter status marker (`A`/`D`/`M`/`R`, colored from the existing open/closed/accent theme slots -- no new colors invented) + filename (`old -> new` for a rename) + `+adds -dels`, plus `(binary)` / `(renamed, no changes)` / `(diff not loaded)` when GitHub omits the patch. `enter` opens that file's diff.
- **Diff** -- GitHub's per-file unified diff (`patch` field), parsed by `gh.ParsePatch` and trimmed by `gh.TrimContext` (`[behavior].diff_context_lines`, default 3) into add/remove/hunk/context lines, colored via the existing open/closed/accent theme slots (never a new accent). Review threads (see below) render inline right after the diff line they anchor to. Rendered into a bubbles `viewport`: **wide lines truncate rather than wrap, and h-scroll (`h`/`l` or the arrow keys) reveals the truncated part** -- bubbles v1.0.0 ships h-scroll disabled by default (`horizontalStep` is 0, making the built-in keymap's `h`/`l` a no-op until set), so `prDetailModel` calls `diffVp.SetHorizontalStep` at construction to turn it on. There is no `[keys]` slot for the `h`/`l` binding itself (same "no natural config slot" precedent as the `tab` pane-focus key), so it's the viewport's own hardcoded keymap. When this view is open inside the per-repo PRs tab, `RepoScreen` must not steal `h`/`l` for tab-switching first -- see `issuesTab.wantsHorizontalKeys` / `RepoScreen.activeWantsHorizontalKeys`.
- **Review threads** (`repos/{o}/{r}/pulls/{n}/comments`) -- grouped by file, then by (side, line) via `gh.GroupReviewThreads`, rendered through glamour, indented under the diff line they match. A comment whose line was trimmed out of the shown context (or is otherwise stale) still renders -- in a trailer at the end of that file's diff -- rather than being silently dropped.

**Edge cases handled**: every reason GitHub omits `patch` gets its own placeholder instead of an empty diff pane, classified by `PullFile.Omission` -- a pure rename, a binary file (no patch AND no line counts, so there is no text diff to show), and a text diff GitHub withheld for size (no patch but real +/- counts, labeled "diff not loaded", never "binary"); a refresh with a diff open re-points at the refetched file and rebuilds the render, so an open diff can never show pre-refresh content; a malformed hunk header falls back to line-number 0 rather than erroring (fail-soft, matching config's philosophy); review comments on a since-force-pushed line fall back to `original_line` for grouping.

**Config-first** (`[behavior]` in `config/ghab/config.toml`): `my_prs_query` (the search scope -- one string for a single query, or an array to widen it; swap in `assignee:@me`, `involves:@me`, a `org:` filter, whatever fits) and `diff_context_lines` (0 disables trimming). Both fail-soft (bad value keeps the compiled default + warns, verified via `--check-config`); `my_prs` joins `[keys]` the same way (default `ctrl+p`, remappable to anything `matchesKey`'s `msg.String()` comparison understands -- including back to a bare letter, if a future config wants that trade-off back). Read-only throughout -- no approve/comment/merge, per the genesis non-goals below.

**App-level bug found and fixed while building this** (`internal/ui/app.go`, `App.Update`'s `pushScreenMsg` case): bubbletea delivers `tea.WindowSizeMsg` exactly once at program start (plus on a real terminal resize) -- it does not replay on every screen push. Every screen pushed AFTER that first message has already landed -- i.e. essentially all interactive navigation: typing into Home, a search/profile hop, opening a PR -- never learned the terminal's real size, so its own cached width/height stayed at zero forever and every sub-pane it laid out rendered degenerate (0-width code-tab boxes, blank issues/releases/prs lists), even though the outer frame still looked fine (`View`'s width/height come from the App as render parameters, not from the screen's own state). Only the CLI `ghab owner/repo` jump-arg path worked, because that path's target screen is already on the stack when the one real message arrives. Confirmed both the break and the fix live (PTY-driven, real `gh api` calls, this repo) -- pushing `RepoScreen` from Home's typed input rendered blank code/issues/prs panes before the fix and real content after it. Fixed by replaying the last known size into a freshly pushed screen before its `Init()` cmds run. This would otherwise have made `MyPRsScreen`/`PRDetailScreen` (built here, reached exactly this way) non-functional in practice despite passing every gate.

**Fixes from the cold audit of this extension** (post-M6, pre-merge, still branch `ghab/pr-view`): diff h-scroll was a documented no-op (see the Diff bullet above -- now fixed via `SetHorizontalStep`); `PRDetailScreen`'s `bodyRows` reserved its own header+blank but not the app's footer line, clipping the owner/repo/title header off the top of every sub-view (now `height-3`, matching `MyPRsScreen`'s convention); `my_prs`'s default binding changed from a bare `p` to `ctrl+p` for the reason described above; `gh.ParsePatch` was treating a patch's `\ No newline at end of file` marker as a real context line and advancing both line counters, skewing every subsequent line number in that hunk (now recognized and skipped); `applyDiffContent`'s (file, width) cache computed `buildDiffContent()` on both the hit and miss path, i.e. cached nothing (now actually short-circuits on a hit); the cross-repo my-PRs flow's stale-response guard matched PR number only, so opening `repoA#5` then `repoB#5` before `repoA`'s fetch landed could paint `repoB`'s screen with `repoA`'s content on that number collision (now matches owner+repo+number). Not fixed (left for a deliberate follow-up, not silently patched): `PullFiles`/`MyPRs` fetch a single page and can silently truncate large PRs/queries (the files-changed header's `"%d files changed"` count is one such visible-but-wrong case) -- a real fix needs either a page bump plus a "showing first N" indicator or actual Link-header pagination, which is more than a one-line change; the list-row selection-bg-bar convention note below is explicitly Ian's call, untouched.

**Convention note (flagged, not resolved here)**: this extension's new list rows (my-PRs, files-changed) keep ghab's existing full-width `selection_bg` bar for the cursor row, matching every other list already in this app (issues, releases, code tree, repo browser) -- not the newer "▸ + bold, no background bars" TUI grammar noted in some of Ian's other recent tools. Introducing a second selection idiom inside one app seemed worse than a brief inconsistency across the fleet; if Ian wants the newer grammar, it should land as one consistency pass over every ghab list at once, not piecemeal per feature.

## Milestones

- **M1 skeleton**: config port complete (load/validate/palette/`--check-config`), app shell + tab chrome + footer + home screen, repo meta fetch + header. Acceptance: `ghab Sh-ui/example` shows themed repo header w/ working tabs (empty bodies), `ghab --check-config` behaves per spec, bad config values warn + default.
- **M2 code+readme**: tree pane, file preview w/ chroma + gutter, readme tab through glamour w/ ombre stylesheets, editor hook. Acceptance: browse this vault's tree, open a file in micro, readme renders styled.
- **M3 releases+issues+prs**: lists, detail views, asset download w/ arch marker.
- **M4 hop+search**: profile screen, user repos, home search, screen stack polish (deep back-chains).
- **M5 ship**: clone flow, wend `o` hook, build.sh + deploy to Pi, install symlink wiring, docs page + tasks + ledger (via the docs pipeline), BOTH-MODE visual check on device (light + dark -- the tui-visual-style bar).
- **M6 PR-view extension** (issue #11, branch `ghab/pr-view`): my-PRs cross-repo screen, files-changed + diff + review-thread PR detail (both the new screen and the existing per-repo PRs tab). Mac-built and gate-verified (`go build`/`go test`/`go vet` all green); **device-unverified** -- no both-modes visual pass on the device yet. See "PR-view extension" below.

## Deployment (two hosts, one artifact each)

`ghab/build.sh` on the Mac builds `dist/ghab-darwin-<hostarch>` (this Mac is Intel -> amd64) and `dist/ghab-linux-arm64`. `dist/` is gitignored; binaries move by scp, not git. Pi deploy: `scp ghab/dist/ghab-linux-arm64 lab:dev-root/ghab/dist/` then `bin/install-pi.sh` symlinks it to `/usr/local/bin/ghab` (flit's skip-if-unbuilt pattern) and links `~/.config/ghab/config.toml` -> `config/ghab/config.toml`. Mac: `bin/install-mac.sh` does the same with the darwin binary. Config + readme-style "auto" search resolves through the vault clone on each host once this branch merges; until then the compiled defaults + palette fallback keep the binary functional (fail-soft by design).

## Non-goals (genesis)

No writes to GitHub (no issue creation, no comments, no stars) -- read-only browser first. No notifications (gh-notify exists). No local git operations beyond clone. No multi-forge abstraction.
