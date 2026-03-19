# veer

[![Go Report Card](https://goreportcard.com/badge/github.com/broothie/veer)](https://goreportcard.com/report/github.com/broothie/veer)
[![codecov](https://codecov.io/gh/broothie/veer/branch/main/graph/badge.svg)](https://codecov.io/gh/broothie/veer)
[![gosec](https://github.com/broothie/veer/actions/workflows/ci.yml/badge.svg)](https://github.com/broothie/veer/actions/workflows/ci.yml)

A live-diffing TUI for coding with AI.

Veer watches your working tree for changes and displays a live, color-coded diff in a fullscreen terminal interface. It's designed to run alongside an AI coding session so you can see exactly what's being changed as it happens.

## Features

- Live-updating diff that refreshes every 500ms
- File tree sidebar with per-file `+N -M` line counts
- Line numbers with colored gutter (green/red background for adds/removes)
- Chunk separators (`…`) instead of raw `@@` hunk headers
- Mouse support: click files to select, scroll wheel in both panes
- Vim-style keyboard navigation
- Respects `.gitignore` (including global excludes)
- Shows untracked files as all-additions diffs
- Header bar with branch, commit SHA, commit message, and working directory

## Install

### Homebrew

```
brew install broothie/tap/veer
```

### Go

```
go install github.com/broothie/veer@latest
```

## Usage

```
veer                     # show all working tree changes
veer main                # diff working tree against a ref
veer src/ lib/           # filter to specific paths
veer main src/           # combine ref and path filter
veer -s                  # show only staged changes
veer -U 5               # 5 lines of context
veer -n 1s              # refresh every 1 second
veer -w 40              # initial sidebar width of 40 (default 35)
veer -t monokai         # use monokai syntax theme (default dracula)
veer --dump-view > /tmp/veer.txt  # render one frame to stdout and exit
veer --no-syntax-highlight  # disable syntax highlighting
veer -d                 # enable debug logging to ~/.veer/debug.log
```

## TODO

- [x] Line numbers
- [x] Git history
- [x] CLI flags
- [x] Thicker sidebars
- [x] Resizable sidebar
- [x] Scrollbar
- [x] Continuous scroll
- [x] Refactor everything
- [ ] Expandable ellipsis
- [x] Rethink keybinds
- [x] Hunk/file staging and committing
- [ ] Display empty commits
- [ ] Dynamic sidebar width based on window size
- [ ] Resizable git history pane
- [x] Flag to disable syntax highlighting
- [ ] Fix version flag output (`veer -v` currently prints `dev`)
- [ ] Fix header truncation
- [ ] Fix line wrapping
- [ ] Fix pane borders
- [ ] Fix line numbers changing upon file/hunk staging
- [ ] Add linux to goreleaser
- [ ] Ensure README matches implementation
- [ ] Flesh out tests
- [x] Run go vet in CI
- [ ] Add GitHub info (issues, discussions, etc.)
- [ ] Show focus indicator in footer
- [x] Set up goreleaser
- [x] Syntax highlighting
