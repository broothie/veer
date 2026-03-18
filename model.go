package main

import (
	"fmt"
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
	styleBold   = lipgloss.NewStyle().Bold(true)
	styleActive = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
	styleBranch = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleSHA    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleDir    = lipgloss.NewStyle().Faint(true)

	styleSidebar = lipgloss.NewStyle().
			BorderRight(true).
			BorderStyle(lipgloss.BlockBorder()).
			BorderForeground(lipgloss.Color("237")).
			PaddingRight(sidebarPad)
)

// treeEntry is a row in the sidebar: either a directory header or a file.
type treeEntry struct {
	name    string
	fileIdx int // -1 for directory headers
	depth   int
}

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
		result, err := fetchDiff(args)
		return diffResultMsg{result, err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpWidth := max(1, m.width-sidebarWidth-sidebarPad-1)
		vpHeight := m.mainHeight()
		m.viewport = viewport.New(vpWidth, vpHeight)
		m.viewport.SetContent(m.buildDiffContent())

	case tickMsg:
		return m, tea.Batch(fetchCmd(m.gitArgs), tickCmd())

	case diffResultMsg:
		m.err = msg.err
		if msg.result != nil {
			m.branch = msg.result.Branch
			m.sha = msg.result.SHA
			m.message = msg.result.Message

			prevPath := ""
			if m.cursor < len(m.files) {
				prevPath = m.files[m.cursor].Path
			}

			m.files = msg.result.Files
			m.tree = buildTree(m.files)

			found := false
			if prevPath != "" {
				for i, f := range m.files {
					if f.Path == prevPath {
						m.cursor = i
						found = true
						break
					}
				}
			}
			if !found && m.cursor >= len(m.files) {
				m.cursor = max(0, len(m.files)-1)
			}
			m.viewport.SetContent(m.buildDiffContent())
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
			if m.cursor < len(m.files)-1 {
				m.cursor++
				m.viewport.SetContent(m.buildDiffContent())
				m.viewport.GotoTop()
			}
		} else {
			m.viewport.LineDown(1)
		}

	case "k", "up":
		if m.sidebarFocused {
			if m.cursor > 0 {
				m.cursor--
				m.viewport.SetContent(m.buildDiffContent())
				m.viewport.GotoTop()
			}
		} else {
			m.viewport.LineUp(1)
		}

	case "g":
		if m.sidebarFocused {
			m.cursor = 0
			m.viewport.SetContent(m.buildDiffContent())
			m.viewport.GotoTop()
		} else {
			m.viewport.GotoTop()
		}

	case "G":
		if m.sidebarFocused {
			if last := len(m.files) - 1; last >= 0 {
				m.cursor = last
				m.viewport.SetContent(m.buildDiffContent())
				m.viewport.GotoTop()
			}
		} else {
			m.viewport.GotoBottom()
		}

	case "ctrl+d":
		if !m.sidebarFocused {
			m.viewport.HalfViewDown()
		}

	case "ctrl+u":
		if !m.sidebarFocused {
			m.viewport.HalfViewUp()
		}

	case "ctrl+f":
		if !m.sidebarFocused {
			m.viewport.ViewDown()
		}

	case "ctrl+b":
		if !m.sidebarFocused {
			m.viewport.ViewUp()
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
			// Map click Y (relative to main area) to a tree entry.
			row := m.sidebarOffset + (msg.Y - headerHeight)
			if row >= 0 && row < len(m.tree) {
				entry := m.tree[row]
				if entry.fileIdx >= 0 {
					m.cursor = entry.fileIdx
					m.sidebarFocused = false
					m.viewport.SetContent(m.buildDiffContent())
					m.viewport.GotoTop()
				}
			}
		} else {
			m.sidebarFocused = false
		}

	case tea.MouseButtonWheelUp:
		if inSidebar {
			if m.cursor > 0 {
				m.cursor--
				m.viewport.SetContent(m.buildDiffContent())
			}
		} else {
			m.viewport.LineUp(3)
		}

	case tea.MouseButtonWheelDown:
		if inSidebar {
			if m.cursor < len(m.files)-1 {
				m.cursor++
				m.viewport.SetContent(m.buildDiffContent())
			}
		} else {
			m.viewport.LineDown(3)
		}
	}

	return m, nil
}

func (m model) buildDiffContent() string {
	if m.cursor >= len(m.files) {
		return ""
	}

	f := m.files[m.cursor]
	if len(f.Lines) == 0 {
		return ""
	}

	// Determine line number column width from the largest line number.
	maxNum := 0
	for _, dl := range f.Lines {
		if dl.OldNum > maxNum {
			maxNum = dl.OldNum
		}
		if dl.NewNum > maxNum {
			maxNum = dl.NewNum
		}
	}
	numWidth := len(fmt.Sprint(maxNum))
	if numWidth < 3 {
		numWidth = 3
	}

	var sb strings.Builder
	for _, dl := range f.Lines {
		sb.WriteString(renderDiffLine(dl, numWidth))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func renderDiffLine(dl DiffLine, numWidth int) string {
	// gutterWidth is: space + number + space + marker + space
	// e.g. " 123 + " or " 123   "
	switch dl.Type {
	case LineSeparator:
		gutter := styleGutter.Render(fmt.Sprintf(" %*s   ", numWidth, "…"))
		return gutter
	case LineContext:
		gutter := styleGutter.Render(fmt.Sprintf(" %*d   ", numWidth, dl.NewNum))
		return gutter + dl.Content
	case LineAdded:
		gutter := styleGutter.Render(fmt.Sprintf(" %*d + ", numWidth, dl.NewNum))
		return gutter + styleAddLine.Render(dl.Content)
	case LineRemoved:
		gutter := styleGutter.Render(fmt.Sprintf(" %*d - ", numWidth, dl.OldNum))
		return gutter + styleRemLine.Render(dl.Content)
	default:
		return ""
	}
}

// --- View rendering ---

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

	main := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content)
	status := m.renderStatus()

	return lipgloss.JoinVertical(lipgloss.Left, header, main, status)
}

func (m model) renderHeader() string {
	var parts []string

	if m.cwd != "" {
		parts = append(parts, styleFaint.Render(m.cwd))
	}
	if m.branch != "" {
		parts = append(parts, styleBranch.Render(m.branch))
	}
	if m.sha != "" {
		parts = append(parts, styleSHA.Render(m.sha))
	}
	if m.message != "" {
		parts = append(parts, m.message)
	}

	sep := styleFaint.Render(" · ")

	// Build the line without the message to see how much room is left for it.
	base := strings.Join(parts[:len(parts)-1], sep)
	if m.message == "" {
		base = strings.Join(parts, sep)
	}

	line := strings.Join(parts, sep)
	avail := m.width - lipgloss.Width(base) - lipgloss.Width(sep) - 2 // 1 leading space + margin

	if m.message != "" && avail < len(m.message) {
		if avail > 3 {
			truncated := make([]string, len(parts)-1)
			copy(truncated, parts[:len(parts)-1])
			truncated = append(truncated, m.message[:avail-1]+"…")
			line = strings.Join(truncated, sep)
		} else {
			// No room for the message at all.
			line = base
		}
	}

	return " " + line + "\n"
}

func (m model) renderSidebar(height int) string {
	var lines []string

	if len(m.tree) == 0 {
		msg := "no changes"
		if m.err != nil {
			msg = "error"
		}
		lines = append(lines, styleFaint.Render(msg))
	} else {
		start := m.sidebarOffset
		end := min(start+height, len(m.tree))

		for _, entry := range m.tree[start:end] {
			lines = append(lines, m.renderTreeEntry(entry))
		}
	}

	return styleSidebar.
		Width(sidebarWidth + sidebarPad).
		Height(height).
		Render(strings.Join(lines, "\n"))
}

func (m model) renderTreeEntry(e treeEntry) string {
	indent := strings.Repeat("  ", e.depth)

	if e.fileIdx < 0 {
		// Directory header.
		return styleDir.Render(indent + e.name)
	}

	f := m.files[e.fileIdx]

	// Build delta string: "+N -M"
	delta := fmt.Sprintf("+%d -%d", f.Added, f.Removed)
	coloredDelta := styleAdd.Render(fmt.Sprintf("+%d", f.Added)) + " " + styleRem.Render(fmt.Sprintf("-%d", f.Removed))

	// Available space: sidebarWidth - indent - cursor prefix - gap - delta
	prefix := "  "
	if e.fileIdx == m.cursor {
		prefix = "> "
	}

	nameMaxLen := sidebarWidth - len(indent) - len(prefix) - len(delta) - 1 // 1 for gap
	name := e.name
	if len(name) > nameMaxLen && nameMaxLen > 3 {
		name = name[:nameMaxLen-1] + "…"
	}

	// Pad between name and delta.
	gap := sidebarWidth - len(indent) - len(prefix) - len(name) - len(delta)
	if gap < 1 {
		gap = 1
	}
	padding := strings.Repeat(" ", gap)

	line := indent + prefix + name + padding + coloredDelta

	if e.fileIdx == m.cursor {
		// Re-render with cursor highlighting (bold the name portion).
		highlighted := indent + styleActive.Render(prefix+name) + padding + coloredDelta
		if !m.sidebarFocused {
			highlighted = indent + styleBold.Render(prefix+name) + padding + coloredDelta
		}
		return highlighted
	}

	return line
}

func (m model) renderStatus() string {
	if m.err != nil {
		return styleFaint.Render(" error: " + m.err.Error())
	}

	var parts []string
	if len(m.files) > 0 {
		// Total delta summary.
		totalAdd, totalRem := 0, 0
		for _, f := range m.files {
			totalAdd += f.Added
			totalRem += f.Removed
		}
		parts = append(parts, fmt.Sprintf("%d files", len(m.files)))
		parts = append(parts, styleAdd.Render(fmt.Sprintf("+%d", totalAdd))+" "+styleRem.Render(fmt.Sprintf("-%d", totalRem)))

		parts = append(parts, fmt.Sprintf("%d/%d", m.cursor+1, len(m.files)))
		if !m.sidebarFocused {
			parts = append(parts, fmt.Sprintf("%.0f%%", m.viewport.ScrollPercent()*100))
		}
	}

	if m.sidebarFocused {
		parts = append(parts, "enter/l: open  tab: switch  q: quit")
	} else {
		parts = append(parts, "h/tab: files  j/k ↑↓  ^d/^u  q: quit")
	}

	return "\n" + styleFaint.Render(" "+strings.Join(parts, "  ·  "))
}

// --- Tree building ---

// buildTree converts a sorted list of file diffs into an indented tree of entries.
func buildTree(files []FileDiff) []treeEntry {
	var entries []treeEntry
	var prevParts []string

	for i, f := range files {
		parts := strings.Split(f.Path, "/")
		dirParts := parts[:len(parts)-1]
		fileName := parts[len(parts)-1]

		// Find how much of the directory path is shared with the previous file.
		common := 0
		for common < len(prevParts) && common < len(dirParts) && prevParts[common] == dirParts[common] {
			common++
		}

		// Emit new directory headers for any divergence.
		for d := common; d < len(dirParts); d++ {
			entries = append(entries, treeEntry{
				name:    dirParts[d] + "/",
				fileIdx: -1,
				depth:   d,
			})
		}

		entries = append(entries, treeEntry{
			name:    fileName,
			fileIdx: i,
			depth:   len(dirParts),
		})

		prevParts = dirParts
	}

	return entries
}

// cursorTreeRow returns the tree entry index that corresponds to the current cursor.
func (m model) cursorTreeRow() int {
	for i, e := range m.tree {
		if e.fileIdx == m.cursor {
			return i
		}
	}
	return 0
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
