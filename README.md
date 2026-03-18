# veer

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

```
go install github.com/broothie/veer@latest
```

## Usage

```
veer                # show all unstaged changes
veer -- src/        # filter to a specific path
```

## Keybindings

| Key | Sidebar | Diff |
|-----|---------|------|
| `j` / `k` | navigate files | scroll |
| `enter` / `l` | open diff | — |
| `h` | — | back to sidebar |
| `tab` | switch panes | switch panes |
| `g` / `G` | first / last file | top / bottom |
| `ctrl+d` / `ctrl+u` | — | half-page down / up |
| `ctrl+f` / `ctrl+b` | — | full-page down / up |
| `q` | quit | quit |

Mouse: click to select files, scroll wheel to navigate or scroll.

## TODO

- [x] Line numbers
- [ ] Git history
- [ ] CLI flags
- [x] Thicker sidebars
- [ ] Collapsible sidebar
- [ ] Scrollbar
- [ ] Continuous scroll
- [ ] Refactor everything
