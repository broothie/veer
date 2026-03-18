# AGENTS.md

This file provides guidance to coding agents working in this repository.

## Build & Test

```bash
go build .          # produces ./veer binary
go test ./...       # run all tests
go test -run TestName ./...  # run a single test
```

No Makefile, linter config, or release tooling — just standard Go commands. Uses `mise` for Go version management (`mise.toml`).

## Architecture

Veer is a live-diffing fullscreen TUI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Elm architecture). It watches the git working tree and displays color-coded diffs that refresh every 500ms.

### File Layout

- **main.go** — CLI entry point, flag parsing via [kong](https://github.com/alecthomas/kong), `config` struct
- **model.go** — Bubble Tea model: state, `Update()` message handling, keyboard/mouse input, focus management
- **diff.go** — Diff types (`DiffResult`, `FileDiff`, `DiffLine`), `Repo` interface, `fetchDiff()` logic, unified diff parsing
- **repo.go** — `gitRepo` struct implementing `Repo` via [go-git](https://github.com/go-git/go-git) (no shelling out to `git`)
- **render.go** — Viewport content rendering: diff lines, file headers, status bar, scrollbars
- **sidebar.go** — File tree + commit list rendering, tree building from flat file paths

### Key Patterns

**Repo interface** (`diff.go`): Abstracts all git operations. `gitRepo` in `repo.go` is the real implementation; `fakeRepo` in `diff_test.go` enables unit testing without a git repo.

**Three focus areas**: `focusFiles` / `focusCommits` / `focusDiff` — determines which pane receives keyboard/mouse input. Tab cycles between them.

**Generation counter**: `diffGen` / `lastBuiltGen` on the model skip redundant viewport rebuilds when files haven't changed.

**Continuous scroll**: All files render into a single viewport with `fileOffsets[]` tracking where each file starts. `syncCursorToScroll()` keeps the sidebar cursor in sync with scroll position.

**Sidebar split**: File tree occupies the top, a separator line divides it from the commit list at the bottom. `sidebarSplit()` computes the split; `commitListHeight()` caps the commit area at 1/3 of main height.

### Data Flow

1. `tickCmd` fires every N ms → `fetchCmd` opens repo, calls `fetchDiff()` + `repo.Log()`
2. `diffResultMsg` arrives → model updates files, tree, branch info (only in working-tree mode)
3. `rebuildDiffContent()` regenerates viewport string if `diffGen` changed
4. `View()` composes: header | sidebar+scrollbar | viewport+scrollbar | status bar

### Staging and Committing

- **stage.go** — Git staging/commit operations that shell out to `git` (not go-git). `stageFileCmd`, `unstageFileCmd`, `unstageAllCmd`, `stageHunkCmd`, `commitStagedCmd`. Uses `buildPatch()` to construct unified diffs for hunk-level staging via `git apply --cached`.
- **Hunk struct** (`diff.go`): Groups raw diff lines per hunk, preserved during parsing for patch reconstruction. Each hunk tracks its `Section` ("staged"/"unstaged") set by `appendDiffSection`.
- **hunkRefs** (`model.go`/`render.go`): Maps viewport lines to `(fileIdx, hunkIdx)` pairs, built during `buildDiffContent()`. Used to determine which hunk to stage when `s` is pressed in the diff view.
- **Commit message input**: `textinput.Model` from bubbles, rendered in the sidebar between file tree and commit list. `focusCommitMsg` focus area handles key delegation to the textinput.
- **Four focus areas**: `focusFiles` / `focusCommitMsg` / `focusCommits` / `focusDiff`
- Staging disabled when viewing historical commits or ref-based diffs (`isWorkingTree()` guard).

### Ref-based Diffing

`veer <ref>` diffs working tree against an arbitrary ref. Uses `object.DiffTree(refTree, headTree)` to find changed files, unions with `Status()` for worktree changes, then diffs ref content vs worktree content per file.
