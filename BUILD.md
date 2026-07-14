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

Rate limits are a non-issue at human browsing pace (5000/hr authed) except search -- hence enter-to-search. Errors: 404 -> "not found or no access"; network fail -> inline error line + retry key, never a crash.

## UI (tea model tree)

App shell = one root model owning a screen stack (push/pop, `esc` pops). Screens:

1. **Home** -- textinput: `owner/repo` jumps straight in, bare `owner` -> profile, anything else -> repo search results list. Recent-visits list below (session-only for M1; persisted later).
2. **Repo** -- header (owner/name, stars, desc, branch) + tab bar: `readme | code | releases | issues | prs`. Tab content in a viewport. Actions: `c` clone, `u` owner profile, `o` open on web, `r` refresh.
3. **Code tab** -- left pane tree (collapsed dirs, lazy expand), right pane file preview (chroma, line-number gutter). `enter` on file focuses preview; `e` opens in editor.
4. **File view / editor hook** -- `e` writes blob to `$XDG_CACHE_HOME/ghab/{owner}/{repo}/{path}` (real filename so micro gets syntax), then `tea.ExecProcess(exec.Command(editor, path))`, resume TUI on exit.
5. **Releases tab** -- list release -> expand body (glamour) + assets; asset `enter` downloads to clone_dir/downloads with progress.
6. **Issues / PRs tabs** -- list (state bullet in open/closed marker colors, #num, title, author, age) -> detail view w/ comments.
7. **Profile** -- user card (name, bio, followers, location) + their repos list (sorted updated) -> `enter` enters repo. THIS IS THE HOP -- must be seamless both directions.
8. **Help** -- full keymap overlay, generated from live (config-merged) keymap.

Async pattern: every fetch = tea.Cmd returning a typed msg; spinner while pending; all list state survives tab switches (models kept per tab, not rebuilt).

Clone = `gh repo clone {o}/{r} {clone_dir}/{r}` via ExecProcess (inherits gh auth + protocol config); on success show path + hint `cd` line printed after quit.

## Milestones

- **M1 skeleton**: config port complete (load/validate/palette/`--check-config`), app shell + tab chrome + footer + home screen, repo meta fetch + header. Acceptance: `ghab Sh-ui/example` shows themed repo header w/ working tabs (empty bodies), `ghab --check-config` behaves per spec, bad config values warn + default.
- **M2 code+readme**: tree pane, file preview w/ chroma + gutter, readme tab through glamour w/ ombre stylesheets, editor hook. Acceptance: browse this vault's tree, open a file in micro, readme renders styled.
- **M3 releases+issues+prs**: lists, detail views, asset download w/ arch marker.
- **M4 hop+search**: profile screen, user repos, home search, screen stack polish (deep back-chains).
- **M5 ship**: clone flow, wend `o` hook, build.sh + deploy to Pi, install symlink wiring, docs page + tasks + ledger (via the docs pipeline), BOTH-MODE visual check on device (light + dark -- the tui-visual-style bar).

## Deployment (two hosts, one artifact each)

`ghab/build.sh` on the Mac builds `dist/ghab-darwin-<hostarch>` (this Mac is Intel -> amd64) and `dist/ghab-linux-arm64`. `dist/` is gitignored; binaries move by scp, not git. Pi deploy: `scp ghab/dist/ghab-linux-arm64 lab:dev-root/ghab/dist/` then `bin/install-pi.sh` symlinks it to `/usr/local/bin/ghab` (flit's skip-if-unbuilt pattern) and links `~/.config/ghab/config.toml` -> `config/ghab/config.toml`. Mac: `bin/install-mac.sh` does the same with the darwin binary. Config + readme-style "auto" search resolves through the vault clone on each host once this branch merges; until then the compiled defaults + palette fallback keep the binary functional (fail-soft by design).

## Non-goals (genesis)

No writes to GitHub (no issue creation, no comments, no stars) -- read-only browser first. No notifications (gh-notify exists). No local git operations beyond clone. No multi-forge abstraction.
