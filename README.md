# ghab

Browse GitHub like the website, in your terminal.

ghab is a TUI that treats github.com's mental model -- a repo page with
tabs, a profile page with repos, links you follow -- as something a
terminal can do better. Point it at any repo and you get the familiar
layout: readme, code tree, releases, issues, and PRs, each a tab. Hit a
username and you're on their profile, scrolling their repos, descending
into any of them. The hop chains: repo -> owner -> another repo -> its
owner, with `esc` walking back up the stack.

It is GitHub-first (remote browsing), not a local-git tool (that's
lazygit) and not a work dashboard (that's gh-dash).

## What it does

- **Repo screen with five tabs** -- readme (rendered through glamour),
  code (tree + syntax-highlighted file preview), releases (notes,
  assets, streaming download with the build for your machine marked),
  issues, and PRs.
- **PR view** -- conversation, files changed, per-file diffs with
  review threads anchored to their lines; plus a cross-repo "my PRs"
  screen (authored or review-requested, configurable query).
- **Profile hop** -- `u` from any repo ascends to the owner's profile;
  `enter` descends into any of their repos.
- **Search** -- `?query` from the home screen searches repos.
- **Clone** -- `c` clones the current repo (via `gh repo clone`) into
  your configured clone directory.
- **Open in editor** -- `e` on any file in the code tab suspends the
  TUI into your editor of choice.
- **Open on web** -- `o` hands the current page to the browser when the
  terminal isn't enough after all.

## Install

```
go install github.com/Sh-ui/ghab@latest
```

Requires Go 1.25+. Or clone and `go build` (the checked-in `build.sh`
cross-compiles static binaries for macOS and Linux, amd64 and arm64,
into `dist/`); prebuilt binaries, when published, appear on the
releases page.

### Auth

ghab rides your existing GitHub CLI login: it authenticates through
[go-gh](https://github.com/cli/go-gh), which resolves `GH_TOKEN` from
the environment, then the token `gh auth login` already stored. ghab
itself handles zero credentials -- nothing to configure, nothing new to
store. If `gh` works, ghab works.

## Usage

```
ghab                  home screen: recents + input
ghab owner/repo       jump straight into a repo
ghab --check-config   print the resolved config + warnings and exit
ghab --version        print version and exit
```

The home input takes `owner/repo` (open the repo), `owner` (open the
profile), or `?query` (search repos).

Default keys (all remappable in config):

| key | action | key | action |
| --- | --- | --- | --- |
| `enter` | open | `esc` | back |
| `h` / `l` | prev / next tab | `j` / `k` | down / up |
| `u` | owner profile | `c` | clone |
| `e` | open in editor | `o` | open on web |
| `s` | search | `r` | refresh |
| `ctrl+p` | my PRs | `?` | help |
| `q` | quit | | |

## Config

Everything tunable lives in `~/.config/ghab/config.toml`
(`$XDG_CONFIG_HOME` respected). No file is required: every key has a
compiled default, and invalid values fail soft -- a warning on stderr,
the default kept. `ghab --check-config` is the validator: it prints the
fully resolved config, where the palette came from, and every warning.

```toml
[palette]
# "auto" looks for ~/.config/ghab/ombre-palette.json (a JSON object of
# name -> "#hex"), else uses the compiled palette. Or an explicit path.
file = "auto"

[theme]
# Each slot takes a palette name, "#hex", or "ansi:N".
accent = "coffee"
accent_alt = "teal"
open_marker = "green"
closed_marker = "red"
release_tag = "yellow"
frame = "ansi:8"
selection_bg = "ansi:8"

[readme]
# Glamour stylesheets for the readme tab. "auto" looks for
# ~/.config/ghab/styles/readme-ombre-{dark,light}.json (the styles/
# directory in this repo ships a ready-made pair to copy there), else
# falls back to glamour's built-in style of the same mode.
style_dark = "auto"
style_light = "auto"
# "dark" or "light" pin the mode; "auto" reads the one-word file
# ~/.config/ghab/color-mode, so terminal theme switchers can flip it.
color_mode = "auto"

[behavior]
clone_dir = "~/Developer"
editor = "micro"
open_url = "auto"           # or an opener command
page_size = 30
cache_ttl = "5m"
diff_context_lines = 3
# String or array of strings; results merge and de-duplicate.
my_prs_query = ["is:pr is:open author:@me", "is:pr is:open review-requested:@me"]

[keys]
# Any of: quit help search back tab_next tab_prev down up open
# profile clone edit web refresh my_prs
quit = "q"
```

## Status

Honest state of things: ghab is a personal tool, extracted from the
monorepo it grew up in and published because it turned out genuinely
useful. It is used daily on macOS and linux-arm64 terminals; build,
vet, and tests (including -race) are green, and the config surface is
validated by `--check-config`. It is not 1.0: expect rough edges in
less-traveled corners (huge monorepos truncate the code tree to lazy
per-directory loading, exotic diffs may render imperfectly), and the
UI is keyboard-only by design. Issues and PRs welcome.

The original build spec lives in [docs/BUILD.md](docs/BUILD.md) -- it
documents the architecture and milestones as they were built, and is
kept as a design record rather than current-state docs; where it and
this README disagree, the README wins.

## License

[MIT](LICENSE), (c) Ian Schuepbach.
