package main

import (
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	sidebarWidth    = 30
	sidebarPad      = 1
	refreshInterval = 500 * time.Millisecond
	headerHeight    = 2 // header line + blank line
	statusHeight    = 2 // blank line + status line
)

type focusArea int

const (
	focusFiles   focusArea = iota
	focusCommits
	focusDiff
)

var (
	styleAdd      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleRem      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleAddLine  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Background(lipgloss.Color("22"))
	styleRemLine  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Background(lipgloss.Color("52"))
	styleGutter   = lipgloss.NewStyle().Background(lipgloss.Color("237"))
	styleFaint    = lipgloss.NewStyle().Faint(true)
	styleBold     = lipgloss.NewStyle().Bold(true)
	styleActive   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
	styleBranch   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleSHA      = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleDir      = lipgloss.NewStyle().Faint(true)
	styleStaged   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleHeader   = lipgloss.NewStyle().Faint(true)
	styleFilePath = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("237"))
)

type (
	tickMsg       struct{}
	diffResultMsg struct {
		result  *DiffResult
		commits []CommitInfo
		err     error
	}
	commitDiffMsg struct {
		sha   string
		files []FileDiff
		err   error
	}
)

type model struct {
	gitArgs        []string
	files          []FileDiff
	tree           []treeEntry
	branch         string
	sha            string
	message        string
	cwd            string
	cursor         int
	sidebarOffset  int
	viewport       viewport.Model
	focus          focusArea
	width          int
	height         int
	err            error
	fetching       bool
	diffGen        uint64 // incremented when files change
	lastBuiltGen   uint64 // gen when diff content was last built
	fileOffsets    []int  // line offset where each file starts in the viewport
	commits        []CommitInfo
	commitCursor   int    // 0 = "working tree", 1+ = commits[i-1]
	commitOffset   int
	selectedCommit int    // -1 = working tree, 0+ = index into commits
}

func newModel(args []string) model {
	cwd, _ := os.Getwd()
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(cwd, home) {
		cwd = "~" + cwd[len(home):]
	}
	return model{
		gitArgs:        args,
		focus:          focusFiles,
		cwd:            cwd,
		selectedCommit: -1,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchCmd(m.gitArgs), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func fetchCmd(args []string) tea.Cmd {
	return func() tea.Msg {
		repo, err := openRepo()
		if err != nil {
			return diffResultMsg{nil, nil, err}
		}
		result, err := fetchDiff(repo, args)
		commits, _ := repo.Log(50)
		return diffResultMsg{result, commits, err}
	}
}

func commitDiffCmd(sha string) tea.Cmd {
	return func() tea.Msg {
		repo, err := openRepo()
		if err != nil {
			return commitDiffMsg{sha, nil, err}
		}
		files, err := repo.DiffCommit(sha)
		return commitDiffMsg{sha, files, err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpWidth := max(1, m.width-sidebarWidth-sidebarPad-1-1) // -1 for scrollbar
		vpHeight := m.mainHeight()
		m.viewport = viewport.New(vpWidth, vpHeight)
		m.rebuildDiffContent()

	case tickMsg:
		if m.fetching {
			return m, tickCmd()
		}
		m.fetching = true
		return m, tea.Batch(fetchCmd(m.gitArgs), tickCmd())

	case diffResultMsg:
		m.fetching = false
		m.err = msg.err
		if msg.commits != nil {
			m.commits = msg.commits
		}
		// Only update file data if in working tree mode.
		if msg.result != nil && m.selectedCommit == -1 {
			m.branch = msg.result.Branch
			m.sha = msg.result.SHA
			m.message = msg.result.Message

			prevYOffset := m.viewport.YOffset

			m.files = msg.result.Files
			m.tree = buildTree(m.files)

			m.diffGen++
			m.rebuildDiffContent()

			m.viewport.SetYOffset(prevYOffset)
			m.syncCursorToScroll()
		}

	case commitDiffMsg:
		// Only apply if this is still the selected commit.
		if m.selectedCommit >= 0 && m.selectedCommit < len(m.commits) &&
			m.commits[m.selectedCommit].FullSHA == msg.sha {
			m.err = msg.err
			m.files = msg.files
			m.tree = buildTree(m.files)
			m.diffGen++
			m.rebuildDiffContent()
			m.viewport.GotoTop()
			m.syncCursorToScroll()
		}

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	return m, nil
}

func (m model) mainHeight() int {
	return max(1, m.height-headerHeight-statusHeight)
}

// setCursor moves the file cursor and scrolls the viewport to that file.
func (m *model) setCursor(idx int) {
	if idx < 0 || idx >= len(m.files) || idx == m.cursor {
		return
	}
	m.cursor = idx
	if idx < len(m.fileOffsets) {
		m.viewport.SetYOffset(m.fileOffsets[idx])
	}
}

// syncCursorToScroll updates the sidebar cursor to match the current scroll position.
func (m *model) syncCursorToScroll() {
	if len(m.fileOffsets) == 0 {
		return
	}
	yOff := m.viewport.YOffset
	for i := len(m.fileOffsets) - 1; i >= 0; i-- {
		if yOff >= m.fileOffsets[i] {
			m.cursor = i
			return
		}
	}
	m.cursor = 0
}

// rebuildDiffContent rebuilds viewport content only if the generation changed.
func (m *model) rebuildDiffContent() {
	if m.lastBuiltGen == m.diffGen && m.diffGen > 0 {
		return
	}
	content := m.buildDiffContent()
	m.viewport.SetContent(content)
	m.lastBuiltGen = m.diffGen
}

func (m model) inSidebar() bool {
	return m.focus == focusFiles || m.focus == focusCommits
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "tab":
		switch m.focus {
		case focusFiles:
			if len(m.commits) > 0 {
				m.focus = focusCommits
			} else if len(m.files) > 0 {
				m.focus = focusDiff
			}
		case focusCommits:
			if len(m.files) > 0 {
				m.focus = focusDiff
			} else {
				m.focus = focusFiles
			}
		case focusDiff:
			m.focus = focusFiles
		}

	case "j", "down":
		switch m.focus {
		case focusFiles:
			m.setCursor(m.cursor + 1)
		case focusCommits:
			total := len(m.commits) + 1 // +1 for working tree entry
			if m.commitCursor < total-1 {
				m.commitCursor++
			}
		case focusDiff:
			m.viewport.LineDown(1)
			m.syncCursorToScroll()
		}

	case "k", "up":
		switch m.focus {
		case focusFiles:
			m.setCursor(m.cursor - 1)
		case focusCommits:
			if m.commitCursor > 0 {
				m.commitCursor--
			}
		case focusDiff:
			m.viewport.LineUp(1)
			m.syncCursorToScroll()
		}

	case "g":
		switch m.focus {
		case focusFiles:
			m.setCursor(0)
		case focusCommits:
			m.commitCursor = 0
		case focusDiff:
			m.viewport.GotoTop()
			m.syncCursorToScroll()
		}

	case "G":
		switch m.focus {
		case focusFiles:
			m.setCursor(len(m.files) - 1)
		case focusCommits:
			m.commitCursor = len(m.commits) // last entry (commits[len-1])
		case focusDiff:
			m.viewport.GotoBottom()
			m.syncCursorToScroll()
		}

	case "ctrl+d":
		if m.focus == focusDiff {
			m.viewport.HalfViewDown()
			m.syncCursorToScroll()
		}

	case "ctrl+u":
		if m.focus == focusDiff {
			m.viewport.HalfViewUp()
			m.syncCursorToScroll()
		}

	case "ctrl+f":
		if m.focus == focusDiff {
			m.viewport.ViewDown()
			m.syncCursorToScroll()
		}

	case "ctrl+b":
		if m.focus == focusDiff {
			m.viewport.ViewUp()
			m.syncCursorToScroll()
		}

	case "l", "enter":
		switch m.focus {
		case focusFiles:
			if len(m.files) > 0 {
				m.focus = focusDiff
			}
		case focusCommits:
			return m.selectCommit()
		}

	case "h":
		switch m.focus {
		case focusDiff:
			m.focus = focusFiles
		case focusCommits:
			m.focus = focusFiles
		}
	}

	return m, nil
}

// selectCommit handles enter in the commit list.
func (m model) selectCommit() (tea.Model, tea.Cmd) {
	if m.commitCursor == 0 {
		// Select "working tree".
		if m.selectedCommit != -1 {
			m.selectedCommit = -1
			m.focus = focusFiles
			// Working tree data will be refreshed on next tick.
		}
		return m, nil
	}

	idx := m.commitCursor - 1
	if idx >= len(m.commits) {
		return m, nil
	}

	c := m.commits[idx]
	m.selectedCommit = idx
	m.branch = ""
	m.sha = c.SHA
	m.message = c.Message
	m.focus = focusFiles
	return m, commitDiffCmd(c.FullSHA)
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	inSidebar := msg.X <= sidebarWidth+sidebarPad
	_, commitStart := m.sidebarSplit()

	switch msg.Button {
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			break
		}
		if inSidebar {
			row := msg.Y - headerHeight
			if row >= commitStart {
				// Click in commit list area (skip branch header row).
				header := 0
				if m.branch != "" || m.sha != "" {
					header = 1
				}
				commitRow := m.commitOffset + (row - commitStart - header)
				total := len(m.commits) + 1
				if commitRow >= 0 && commitRow < total {
					m.commitCursor = commitRow
					m.focus = focusCommits
					return m.selectCommit()
				}
			} else {
				// Click in file tree area.
				treeRow := m.sidebarOffset + row
				if treeRow >= 0 && treeRow < len(m.tree) {
					entry := m.tree[treeRow]
					if entry.fileIdx >= 0 {
						m.setCursor(entry.fileIdx)
						m.focus = focusDiff
					}
				}
			}
		} else {
			m.focus = focusDiff
		}

	case tea.MouseButtonWheelUp:
		if inSidebar {
			row := msg.Y - headerHeight
			if row >= commitStart {
				if m.commitOffset > 0 {
					m.commitOffset--
				}
			} else {
				if m.sidebarOffset > 0 {
					m.sidebarOffset--
				}
			}
		} else {
			m.viewport.LineUp(3)
			m.syncCursorToScroll()
		}

	case tea.MouseButtonWheelDown:
		if inSidebar {
			row := msg.Y - headerHeight
			if row >= commitStart {
				total := len(m.commits) + 1
				commitH := m.commitListHeight()
				header := 0
				if m.branch != "" || m.sha != "" {
					header = 1
				}
				maxOff := max(0, total-(commitH-header))
				if m.commitOffset < maxOff {
					m.commitOffset++
				}
			} else {
				fileH, _ := m.sidebarSplit()
				maxOffset := max(0, len(m.tree)-fileH)
				if m.sidebarOffset < maxOffset {
					m.sidebarOffset++
				}
			}
		} else {
			m.viewport.LineDown(3)
			m.syncCursorToScroll()
		}
	}

	return m, nil
}

// sidebarSplit returns (fileTreeHeight, commitListStartRow).
// commitListStartRow is relative to the main area top.
func (m model) sidebarSplit() (int, int) {
	mainH := m.mainHeight()
	commitH := m.commitListHeight()
	fileH := mainH - commitH - 1 // -1 for separator
	if fileH < 1 {
		fileH = 1
	}
	return fileH, fileH + 1 // +1 for separator line
}

func (m model) commitListHeight() int {
	mainH := m.mainHeight()
	total := len(m.commits) + 1 // +1 for working tree entry
	header := 0
	if m.branch != "" || m.sha != "" {
		header = 1
	}
	h := min(total+header, mainH/3)
	return max(h, 3) // at least 3 rows
}

func (m model) View() string {
	if m.width == 0 {
		return ""
	}

	header := m.renderHeader()

	mainH := m.mainHeight()

	fileH, _ := m.sidebarSplit()

	// Keep cursor's tree row visible in file tree scroll region.
	cursorRow := m.cursorTreeRow()
	if cursorRow < m.sidebarOffset {
		m.sidebarOffset = cursorRow
	} else if cursorRow >= m.sidebarOffset+fileH {
		m.sidebarOffset = cursorRow - fileH + 1
	}

	// Keep commit cursor visible.
	commitH := m.commitListHeight()
	branchHeader := 0
	if m.branch != "" || m.sha != "" {
		branchHeader = 1
	}
	visibleCommits := commitH - branchHeader
	if m.commitCursor < m.commitOffset {
		m.commitOffset = m.commitCursor
	} else if m.commitCursor >= m.commitOffset+visibleCommits {
		m.commitOffset = m.commitCursor - visibleCommits + 1
	}

	sidebar := m.renderSidebar(mainH)
	content := m.viewport.View()

	// Diff scrollbar.
	totalDiffLines := m.viewport.TotalLineCount()
	diffScrollbar := renderScrollbar(mainH, totalDiffLines, m.viewport.YOffset)

	// Sidebar scrollbar for focused sub-panel.
	var sidebarScrollbar string
	if m.focus == focusCommits {
		total := len(m.commits) + 1
		sidebarScrollbar = renderScrollbar(mainH, total, m.commitOffset)
	} else {
		sidebarScrollbar = renderScrollbar(mainH, len(m.tree), m.sidebarOffset)
	}

	parts := []string{sidebar}
	if sidebarScrollbar != "" {
		parts = append(parts, sidebarScrollbar)
	}
	parts = append(parts, content)
	if diffScrollbar != "" {
		parts = append(parts, diffScrollbar)
	}
	main := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	status := m.renderStatus()

	return lipgloss.JoinVertical(lipgloss.Left, header, main, status)
}
