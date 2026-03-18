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

var (
	styleAdd     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleRem     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleAddLine = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Background(lipgloss.Color("22"))
	styleRemLine = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Background(lipgloss.Color("52"))
	styleGutter  = lipgloss.NewStyle().Background(lipgloss.Color("237"))
	styleFaint   = lipgloss.NewStyle().Faint(true)
	styleBold    = lipgloss.NewStyle().Bold(true)
	styleActive  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
	styleBranch  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleSHA     = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleDir     = lipgloss.NewStyle().Faint(true)
	styleStaged  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleHeader   = lipgloss.NewStyle().Faint(true)
	styleFilePath = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("237"))

	styleSidebar = lipgloss.NewStyle().
			BorderRight(true).
			BorderStyle(lipgloss.BlockBorder()).
			BorderForeground(lipgloss.Color("237")).
			PaddingRight(sidebarPad)
)

type (
	tickMsg       struct{}
	diffResultMsg struct {
		result *DiffResult
		err    error
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
	sidebarFocused bool
	width          int
	height         int
	err            error
	fetching       bool
	diffGen        uint64 // incremented when files change
	lastBuiltGen   uint64 // gen when diff content was last built
	fileOffsets    []int  // line offset where each file starts in the viewport
}

func newModel(args []string) model {
	cwd, _ := os.Getwd()
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(cwd, home) {
		cwd = "~" + cwd[len(home):]
	}
	return model{
		gitArgs:        args,
		sidebarFocused: true,
		cwd:            cwd,
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
			return diffResultMsg{nil, err}
		}
		result, err := fetchDiff(repo, args)
		return diffResultMsg{result, err}
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
		if msg.result != nil {
			m.branch = msg.result.Branch
			m.sha = msg.result.SHA
			m.message = msg.result.Message

			prevYOffset := m.viewport.YOffset

			m.files = msg.result.Files
			m.tree = buildTree(m.files)

			m.diffGen++
			m.rebuildDiffContent()

			// Restore scroll position and sync cursor.
			m.viewport.SetYOffset(prevYOffset)
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
	m.viewport.SetContent(m.buildDiffContent())
	m.lastBuiltGen = m.diffGen
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "tab":
		if !m.sidebarFocused || len(m.files) > 0 {
			m.sidebarFocused = !m.sidebarFocused
		}

	case "j", "down":
		if m.sidebarFocused {
			m.setCursor(m.cursor + 1)
		} else {
			m.viewport.LineDown(1)
			m.syncCursorToScroll()
		}

	case "k", "up":
		if m.sidebarFocused {
			m.setCursor(m.cursor - 1)
		} else {
			m.viewport.LineUp(1)
			m.syncCursorToScroll()
		}

	case "g":
		if m.sidebarFocused {
			m.setCursor(0)
		} else {
			m.viewport.GotoTop()
			m.syncCursorToScroll()
		}

	case "G":
		if m.sidebarFocused {
			m.setCursor(len(m.files) - 1)
		} else {
			m.viewport.GotoBottom()
			m.syncCursorToScroll()
		}

	case "ctrl+d":
		if !m.sidebarFocused {
			m.viewport.HalfViewDown()
			m.syncCursorToScroll()
		}

	case "ctrl+u":
		if !m.sidebarFocused {
			m.viewport.HalfViewUp()
			m.syncCursorToScroll()
		}

	case "ctrl+f":
		if !m.sidebarFocused {
			m.viewport.ViewDown()
			m.syncCursorToScroll()
		}

	case "ctrl+b":
		if !m.sidebarFocused {
			m.viewport.ViewUp()
			m.syncCursorToScroll()
		}

	case "l", "enter":
		if m.sidebarFocused && len(m.files) > 0 {
			m.sidebarFocused = false
		}

	case "h":
		if !m.sidebarFocused {
			m.sidebarFocused = true
		}
	}

	return m, nil
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	inSidebar := msg.X <= sidebarWidth+sidebarPad

	switch msg.Button {
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			break
		}
		if inSidebar {
			row := m.sidebarOffset + (msg.Y - headerHeight)
			if row >= 0 && row < len(m.tree) {
				entry := m.tree[row]
				if entry.fileIdx >= 0 {
					m.setCursor(entry.fileIdx)
					m.sidebarFocused = false
				}
			}
		} else {
			m.sidebarFocused = false
		}

	case tea.MouseButtonWheelUp:
		if inSidebar {
			if m.sidebarOffset > 0 {
				m.sidebarOffset--
			}
		} else {
			m.viewport.LineUp(3)
			m.syncCursorToScroll()
		}

	case tea.MouseButtonWheelDown:
		if inSidebar {
			maxOffset := max(0, len(m.tree)-m.mainHeight())
			if m.sidebarOffset < maxOffset {
				m.sidebarOffset++
			}
		} else {
			m.viewport.LineDown(3)
			m.syncCursorToScroll()
		}
	}

	return m, nil
}

func (m model) View() string {
	if m.width == 0 {
		return ""
	}

	header := m.renderHeader()

	mainH := m.mainHeight()

	// Keep cursor's tree row visible in sidebar scroll region.
	cursorRow := m.cursorTreeRow()
	if cursorRow < m.sidebarOffset {
		m.sidebarOffset = cursorRow
	} else if cursorRow >= m.sidebarOffset+mainH {
		m.sidebarOffset = cursorRow - mainH + 1
	}

	sidebar := m.renderSidebar(mainH)
	content := m.viewport.View()

	// Diff scrollbar: total lines in content vs viewport height.
	totalDiffLines := m.viewport.TotalLineCount()
	diffScrollbar := renderScrollbar(mainH, totalDiffLines, m.viewport.YOffset)

	// Sidebar scrollbar (rendered inside renderSidebar via the border area isn't
	// feasible, so we handle it separately if needed).
	sidebarScrollbar := renderScrollbar(mainH, len(m.tree), m.sidebarOffset)

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
