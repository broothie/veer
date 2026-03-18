package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func testModel(files []FileDiff) model {
	m := model{
		cfg:            config{Context: 3, SidebarWidth: defaultSidebarWidth},
		files:          files,
		tree:           buildTree(files),
		focus:          focusFiles,
		width:          120,
		height:         40,
		sidebarWidth:   defaultSidebarWidth,
		viewport:       viewport.New(80, 36),
		cwd:            "~/proj",
		branch:         "main",
		sha:            "abc1234",
		message:        "test commit",
		selectedCommit: -1,
		commitMsg:      textarea.New(),
	}
	content := m.buildDiffContent()
	m.viewport.SetContent(content)
	return m
}

var twoFiles = []FileDiff{
	{Path: "a.go", Lines: []DiffLine{{Type: LineAdded, NewNum: 1, Content: "pkg a"}}, Added: 1, Unstaged: true},
	{Path: "b.go", Lines: []DiffLine{{Type: LineAdded, NewNum: 1, Content: "pkg b"}}, Added: 1, Unstaged: true},
}

// --- setCursor ---

func TestSetCursor(t *testing.T) {
	m := testModel(twoFiles)

	m.setCursor(1)
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1", m.cursor)
	}
}

func TestSetCursor_NopOnSameIndex(t *testing.T) {
	m := testModel(twoFiles)
	gen := m.diffGen

	m.setCursor(0) // already at 0
	if m.diffGen != gen {
		t.Error("diffGen should not change when cursor doesn't move")
	}
}

func TestSetCursor_BoundsCheck(t *testing.T) {
	m := testModel(twoFiles)

	m.setCursor(-1)
	if m.cursor != 0 {
		t.Error("negative index should be ignored")
	}

	m.setCursor(99)
	if m.cursor != 0 {
		t.Error("out-of-range index should be ignored")
	}
}

// --- handleKey ---

func TestHandleKey_Quit(t *testing.T) {
	m := testModel(twoFiles)

	for _, key := range []string{"q", "ctrl+c"} {
		_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if key == "q" {
			// tea.KeyMsg for "q" is runes
			result, cmd2 := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
			_ = result
			if cmd2 == nil {
				t.Error("q should return a quit command")
			}
		}
		_ = cmd
	}
}

func TestHandleKey_JK_SidebarNavigation(t *testing.T) {
	m := testModel(twoFiles)
	m.focus = focusFiles

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = result.(model)
	if m.cursor != 1 {
		t.Errorf("j: cursor = %d, want 1", m.cursor)
	}

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = result.(model)
	if m.cursor != 0 {
		t.Errorf("k: cursor = %d, want 0", m.cursor)
	}
}

func TestHandleKey_G_FirstLast(t *testing.T) {
	m := testModel(twoFiles)
	m.focus = focusFiles

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = result.(model)
	if m.cursor != 1 {
		t.Errorf("G: cursor = %d, want 1", m.cursor)
	}

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = result.(model)
	if m.cursor != 0 {
		t.Errorf("g: cursor = %d, want 0", m.cursor)
	}
}

func TestHandleKey_Tab_CyclesFocus(t *testing.T) {
	m := testModel(twoFiles)
	m.focus = focusFiles
	m.commits = []CommitInfo{{SHA: "abc1234", FullSHA: "abc1234full", Message: "test"}}

	// files -> commits
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(model)
	if m.focus != focusCommits {
		t.Errorf("tab from files: focus = %d, want focusCommits", m.focus)
	}

	// commits -> diff
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(model)
	if m.focus != focusDiff {
		t.Errorf("tab from commits: focus = %d, want focusDiff", m.focus)
	}

	// diff -> files
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(model)
	if m.focus != focusFiles {
		t.Errorf("tab from diff: focus = %d, want focusFiles", m.focus)
	}
}

func TestHandleKey_ShiftTab_CyclesBackward(t *testing.T) {
	m := testModel(twoFiles)
	m.commits = []CommitInfo{{SHA: "abc1234", FullSHA: "abc1234full", Message: "test"}}

	// diff -> commits
	m.focus = focusDiff
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = result.(model)
	if m.focus != focusCommits {
		t.Errorf("shift+tab from diff: focus = %d, want focusCommits", m.focus)
	}

	// commits -> files
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = result.(model)
	if m.focus != focusFiles {
		t.Errorf("shift+tab from commits: focus = %d, want focusFiles", m.focus)
	}

	// files -> diff
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = result.(model)
	if m.focus != focusDiff {
		t.Errorf("shift+tab from files: focus = %d, want focusDiff", m.focus)
	}
}

func TestHandleKey_Tab_SkipsCommitsWhenEmpty(t *testing.T) {
	m := testModel(twoFiles)
	m.focus = focusFiles

	// No commits: files -> diff (skip commits)
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(model)
	if m.focus != focusDiff {
		t.Errorf("tab with no commits: focus = %d, want focusDiff", m.focus)
	}
}

func TestHandleKey_Tab_NoToggleWhenEmpty(t *testing.T) {
	m := testModel(nil)
	m.focus = focusFiles

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(model)
	// No files and no commits: should stay on focusFiles or go to commits
	// With no commits and no files, tab from files goes nowhere useful
	if m.focus == focusDiff {
		t.Error("tab should not switch to diff when no files")
	}
}

func TestHandleKey_Enter_OpensDiff(t *testing.T) {
	m := testModel(twoFiles)
	m.focus = focusFiles

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(model)
	if m.focus != focusDiff {
		t.Error("enter should switch focus to diff")
	}
}

func TestHandleKey_ShiftTab_DiffToFiles(t *testing.T) {
	m := testModel(twoFiles)
	m.focus = focusDiff

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = result.(model)
	if m.focus != focusFiles {
		t.Error("shift+tab from diff (no commits) should switch focus to files")
	}
}

// --- Update: diffResultMsg ---

func TestUpdate_DiffResult_SyncsFromScroll(t *testing.T) {
	m := testModel(twoFiles)

	// After a refresh, cursor should be synced from scroll position.
	msg := diffResultMsg{
		result: &DiffResult{
			Branch:  "main",
			SHA:     "def5678",
			Message: "update",
			Files:   twoFiles,
		},
	}

	result, _ := m.Update(msg)
	m = result.(model)

	// At YOffset 0, cursor should be 0 (first file).
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (synced from top scroll position)", m.cursor)
	}
}

func TestUpdate_DiffResult_ClampsCursor(t *testing.T) {
	m := testModel(twoFiles)
	m.cursor = 1

	// File list shrinks to 1 file.
	msg := diffResultMsg{
		result: &DiffResult{
			Files: twoFiles[:1],
		},
	}

	result, _ := m.Update(msg)
	m = result.(model)

	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (should clamp when files shrink)", m.cursor)
	}
}

// --- Update: tickMsg with fetch guard ---

func TestUpdate_TickMsg_SkipsWhenFetching(t *testing.T) {
	m := testModel(twoFiles)
	m.fetching = true

	result, cmd := m.Update(tickMsg{})
	m = result.(model)

	if !m.fetching {
		t.Error("fetching should remain true")
	}
	// Should return a tick cmd but not a fetch cmd.
	if cmd == nil {
		t.Error("should still schedule next tick")
	}
}

func TestUpdate_TickMsg_FetchesWhenIdle(t *testing.T) {
	m := testModel(twoFiles)
	m.fetching = false

	result, cmd := m.Update(tickMsg{})
	m = result.(model)

	if !m.fetching {
		t.Error("fetching should be set to true")
	}
	if cmd == nil {
		t.Error("should return batch cmd")
	}
}

// --- renderHeader ---

func TestRenderHeader_ContainsParts(t *testing.T) {
	m := testModel(twoFiles)
	header := m.renderHeader()

	if !strings.Contains(header, "proj") {
		t.Error("header should contain cwd")
	}
	if !strings.Contains(header, "abc1234") {
		t.Error("header should contain SHA")
	}
	if !strings.Contains(header, "test commit") {
		t.Error("header should contain message")
	}
}

func TestRenderHeader_TruncatesLongMessage(t *testing.T) {
	m := testModel(twoFiles)
	m.width = 50
	m.message = "this is a very long commit message that should be truncated"

	header := m.renderHeader()
	if lipgloss.Width(header) > m.width {
		t.Errorf("header width %d exceeds terminal width %d", lipgloss.Width(header), m.width)
	}
}

func TestRenderHeader_NoMessage(t *testing.T) {
	m := testModel(twoFiles)
	m.message = ""
	header := m.renderHeader()
	if strings.Contains(header, "test commit") {
		t.Error("should not contain message when empty")
	}
}

func TestRenderHeader_ShowsBranchAndTruncatesCWD(t *testing.T) {
	m := testModel(twoFiles)
	m.width = 80
	m.cwd = "/Users/andrewbooth/Developer/github.com/broothie/veer"
	m.branch = "claude/optimize-monorepo-performance-aLsBS"

	header := m.renderHeader()
	if !strings.Contains(header, "optimize-monorepo-performance-aLsBS") {
		t.Error("header should contain the branch when width is constrained")
	}
	if strings.Contains(header, "/Users/andrewbooth/Developer/github.com/broothie/veer") {
		t.Error("header should truncate the cwd when space is constrained")
	}
}

// --- renderStatus ---

func TestRenderHeader_WithFilesDelta(t *testing.T) {
	m := testModel(twoFiles)
	header := m.renderHeader()

	if !strings.Contains(header, "+2") {
		t.Error("header should show additions")
	}
	if !strings.Contains(header, "-0") {
		t.Error("header should show removals")
	}
}

func TestRenderStatus_WithError(t *testing.T) {
	m := testModel(nil)
	m.err = errors.New("test error")
	status := m.renderStatus()

	if !strings.Contains(status, "test error") {
		t.Error("status should show error")
	}
}

func TestRenderStatus_FocusHints(t *testing.T) {
	m := testModel(twoFiles)

	m.focus = focusFiles
	status := m.renderStatus()
	if !strings.Contains(status, "s: stage") {
		t.Error("file-focused status should show stage hint")
	}

	m.focus = focusCommits
	status = m.renderStatus()
	if !strings.Contains(status, "enter: select") {
		t.Error("commit-focused status should show commit hints")
	}

	m.focus = focusDiff
	status = m.renderStatus()
	if !strings.Contains(status, "s: stage hunk") {
		t.Error("diff-focused status should show hunk stage hint")
	}
}

// --- buildDiffContent ---

func TestBuildDiffContent_NoFiles(t *testing.T) {
	m := testModel(nil)
	content := m.buildDiffContent()
	if !strings.Contains(content, "no changes") {
		t.Error("empty file list should show 'no changes'")
	}
}

func TestBuildDiffContent_RendersDiff(t *testing.T) {
	m := testModel(twoFiles)
	content := m.buildDiffContent()
	if !strings.Contains(content, "pkg a") {
		t.Error("should render first file's diff content")
	}
}

func TestBuildDiffContent_AllFilesRendered(t *testing.T) {
	m := testModel(twoFiles)
	content := m.buildDiffContent()
	if !strings.Contains(content, "a.go") || !strings.Contains(content, "b.go") {
		t.Error("continuous scroll should render all files")
	}
	if !strings.Contains(content, "pkg a") || !strings.Contains(content, "pkg b") {
		t.Error("continuous scroll should render all file contents")
	}
}

func TestRenderHighlighted_Disabled(t *testing.T) {
	line := DiffLine{Type: LineContext, NewNum: 1, Content: "package main"}

	initTheme("dracula", true)
	t.Cleanup(func() { theme = nil })

	enabled := highlightFile("main.go", []DiffLine{line})[0]
	if len(enabled.tokens) == 0 {
		t.Fatal("enabled syntax highlighting should tokenize file content")
	}

	initTheme("dracula", false)
	disabled := highlightFile("main.go", []DiffLine{line})[0]
	if len(disabled.tokens) != 0 {
		t.Fatalf("disabled syntax highlighting should skip tokenization, got %d tokens", len(disabled.tokens))
	}
	if got := renderHighlighted(disabled, line.Content); got != line.Content {
		t.Fatalf("disabled syntax highlighting = %q, want %q", got, line.Content)
	}
}

// --- renderDiffLine ---

func TestRenderDiffLine_AllTypes(t *testing.T) {
	tests := []struct {
		name string
		dl   DiffLine
		want string
	}{
		{"context", DiffLine{Type: LineContext, OldNum: 5, NewNum: 5, Content: "hello"}, "hello"},
		{"added", DiffLine{Type: LineAdded, NewNum: 3, Content: "new"}, "new"},
		{"removed", DiffLine{Type: LineRemoved, OldNum: 3, Content: "old"}, "old"},
		{"separator", DiffLine{Type: LineSeparator}, "…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderDiffLine(tt.dl, 3, highlightedLine{}, "")
			if !strings.Contains(result, tt.want) {
				t.Errorf("renderDiffLine(%s) = %q, should contain %q", tt.name, result, tt.want)
			}
		})
	}
}

// --- cursorTreeRow ---

func TestCursorTreeRow(t *testing.T) {
	files := []FileDiff{
		{Path: "dir/a.go"},
		{Path: "dir/b.go"},
	}
	m := testModel(files)
	// tree: dir/ (idx -1), a.go (idx 0), b.go (idx 1)

	m.cursor = 0
	if row := m.cursorTreeRow(); row != 1 {
		t.Errorf("cursor 0 -> row %d, want 1 (after dir header)", row)
	}

	m.cursor = 1
	if row := m.cursorTreeRow(); row != 2 {
		t.Errorf("cursor 1 -> row %d, want 2", row)
	}
}

func TestCursorTreeRow_Empty(t *testing.T) {
	m := testModel(nil)
	if row := m.cursorTreeRow(); row != 0 {
		t.Errorf("empty tree -> row %d, want 0", row)
	}
}

// --- renderSidebar ---

func TestRenderSidebar_NoChanges(t *testing.T) {
	m := testModel(nil)
	sidebar := m.renderSidebar(20)
	if !strings.Contains(sidebar, "no changes") {
		t.Error("empty sidebar should show 'no changes'")
	}
}

func TestRenderSidebar_WithError(t *testing.T) {
	m := testModel(nil)
	m.err = errors.New("fail")
	sidebar := m.renderSidebar(20)
	if !strings.Contains(sidebar, "error") {
		t.Error("sidebar with error should show 'error'")
	}
}

func TestRenderSidebar_ShowsFiles(t *testing.T) {
	m := testModel(twoFiles)
	sidebar := m.renderSidebar(20)
	if !strings.Contains(sidebar, "a.go") || !strings.Contains(sidebar, "b.go") {
		t.Error("sidebar should show file names")
	}
}

// --- renderTreeEntry ---

func TestRenderTreeEntry_Directory(t *testing.T) {
	m := testModel(twoFiles)
	entry := treeEntry{name: "src/", fileIdx: -1, depth: 0}
	result := m.renderTreeEntry(entry)
	if !strings.Contains(result, "src/") {
		t.Error("directory entry should contain dir name")
	}
}

func TestRenderTreeEntry_SelectedFile(t *testing.T) {
	m := testModel(twoFiles)
	m.cursor = 0
	m.focus = focusFiles

	entry := treeEntry{name: "a.go", fileIdx: 0, depth: 0}
	result := m.renderTreeEntry(entry)
	if !strings.Contains(result, "●") {
		t.Error("selected file should have ● marker")
	}
	if !strings.Contains(result, "a.go") {
		t.Error("selected file should contain filename")
	}
}

func TestRenderTreeEntry_UnselectedFile(t *testing.T) {
	m := testModel(twoFiles)
	m.cursor = 0

	entry := treeEntry{name: "b.go", fileIdx: 1, depth: 0}
	result := m.renderTreeEntry(entry)
	if !strings.Contains(result, "○") {
		t.Error("unselected file should have ○ marker")
	}
}

// --- View ---

func TestView_EmptyWhenNoWidth(t *testing.T) {
	m := testModel(twoFiles)
	m.width = 0
	if m.View() != "" {
		t.Error("View should return empty string when width is 0")
	}
}

func TestNewModel_HasInitialLayout(t *testing.T) {
	m := newModel(config{SidebarWidth: defaultSidebarWidth})
	if m.width == 0 || m.height == 0 {
		t.Fatalf("newModel should have a non-zero initial layout, got %dx%d", m.width, m.height)
	}
	if got := m.View(); got == "" {
		t.Fatal("newModel View should not be empty before the first window size message")
	}
}

func TestView_HeightMatchesWindow(t *testing.T) {
	m := testModel(twoFiles)
	if got := len(strings.Split(m.View(), "\n")); got != m.height {
		t.Fatalf("View line count = %d, want %d", got, m.height)
	}
}

func TestHandleKey_Quit_ClosesWatcher(t *testing.T) {
	m := testModel(twoFiles)
	closed := false
	m.watcherClose = func() { closed = true }

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !closed {
		t.Fatal("quit should close the watcher")
	}
	if cmd == nil {
		t.Fatal("quit should return a command")
	}
}

// --- renderScrollbar ---

func TestRenderScrollbar_NoScrollNeeded(t *testing.T) {
	result := renderScrollbar(20, 10, 0)
	if result != "" {
		t.Error("should return empty when content fits")
	}
}

func TestRenderScrollbar_ExactFit(t *testing.T) {
	result := renderScrollbar(20, 20, 0)
	if result != "" {
		t.Error("should return empty when content exactly fits")
	}
}

func TestRenderScrollbar_HasCorrectHeight(t *testing.T) {
	result := renderScrollbar(10, 50, 0)
	lines := strings.Split(result, "\n")
	if len(lines) != 10 {
		t.Errorf("scrollbar height = %d, want 10", len(lines))
	}
}

func TestRenderScrollbar_ThumbMovesWithOffset(t *testing.T) {
	top := renderScrollbar(20, 100, 0)
	bottom := renderScrollbar(20, 100, 80)
	if top == bottom {
		t.Error("scrollbar should look different at top vs bottom")
	}
}

// --- rebuildDiffContent ---

func TestRebuildDiffContent_SkipsWhenCached(t *testing.T) {
	m := testModel(twoFiles)
	m.diffGen = 5
	m.lastBuiltGen = 5

	// Should be a no-op (we can't easily assert this, but it shouldn't panic).
	m.rebuildDiffContent()

	if m.lastBuiltGen != 5 {
		t.Error("lastBuiltGen should not change when cached")
	}
}

func TestRebuildDiffContent_RebuildsOnGenChange(t *testing.T) {
	m := testModel(twoFiles)
	m.diffGen = 2
	m.lastBuiltGen = 1

	m.rebuildDiffContent()

	if m.lastBuiltGen != 2 {
		t.Errorf("lastBuiltGen = %d, want 2", m.lastBuiltGen)
	}
}
