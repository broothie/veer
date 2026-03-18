package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	sidebarWidth    = 28
	refreshInterval = 500 * time.Millisecond
)

var (
	styleAdd    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleRem    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleHunk   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleFaint  = lipgloss.NewStyle().Faint(true)
	styleBold   = lipgloss.NewStyle().Bold(true)
	styleActive = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))

	styleSidebar = lipgloss.NewStyle().
			BorderRight(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240"))
)

type (
	tickMsg      struct{}
	diffResultMsg struct {
		files []FileDiff
		err   error
	}
)

type model struct {
	gitArgs        []string
	files          []FileDiff
	cursor         int
	viewport       viewport.Model
	sidebarFocused bool
	width          int
	height         int
	err            error
}

func newModel(args []string) model {
	return model{
		gitArgs:        args,
		sidebarFocused: true,
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
		files, err := fetchDiff(args)
		return diffResultMsg{files, err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpWidth := max(1, m.width-sidebarWidth-1)
		vpHeight := m.height - 1 // 1 line for status bar
		m.viewport = viewport.New(vpWidth, vpHeight)
		m.viewport.SetContent(m.buildDiffContent())

	case tickMsg:
		return m, tea.Batch(fetchCmd(m.gitArgs), tickCmd())

	case diffResultMsg:
		m.err = msg.err
		if msg.err == nil {
			prevPath := ""
			if m.cursor < len(m.files) {
				prevPath = m.files[m.cursor].Path
			}

			m.files = msg.files

			// Try to preserve cursor position on the same file after refresh.
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
			if !found {
				if m.cursor >= len(m.files) {
					m.cursor = max(0, len(m.files)-1)
				}
			}
			m.viewport.SetContent(m.buildDiffContent())
		}

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
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

func (m model) buildDiffContent() string {
	if m.cursor >= len(m.files) {
		return ""
	}
	var sb strings.Builder
	for _, line := range m.files[m.cursor].Lines {
		sb.WriteString(colorDiffLine(line))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func colorDiffLine(line string) string {
	switch {
	case strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++"):
		return styleFaint.Render(line)
	case strings.HasPrefix(line, "@@"):
		return styleHunk.Render(line)
	case strings.HasPrefix(line, "+"):
		return styleAdd.Render(line)
	case strings.HasPrefix(line, "-"):
		return styleRem.Render(line)
	default:
		return line
	}
}

func (m model) View() string {
	if m.width == 0 {
		return ""
	}

	mainHeight := m.height - 1
	sidebar := m.renderSidebar(mainHeight)
	content := m.viewport.View()

	main := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content)
	status := m.renderStatus()

	return lipgloss.JoinVertical(lipgloss.Left, main, status)
}

func (m model) renderSidebar(height int) string {
	var lines []string

	if len(m.files) == 0 {
		msg := "no changes"
		if m.err != nil {
			msg = "error"
		}
		lines = append(lines, styleFaint.Render(msg))
	} else {
		// Scroll the list to keep the cursor visible.
		start := 0
		if m.cursor >= height {
			start = m.cursor - height + 1
		}
		end := min(start+height, len(m.files))

		for i := start; i < end; i++ {
			name := trimPath(m.files[i].Path, sidebarWidth-3)
			entry := "  " + name
			if i == m.cursor {
				entry = "> " + name
				if m.sidebarFocused {
					entry = styleActive.Render(entry)
				} else {
					entry = styleBold.Render(entry)
				}
			}
			lines = append(lines, entry)
		}
	}

	return styleSidebar.
		Width(sidebarWidth).
		Height(height).
		Render(strings.Join(lines, "\n"))
}

func (m model) renderStatus() string {
	if m.err != nil {
		return styleFaint.Render("error: " + m.err.Error())
	}

	var parts []string
	if len(m.files) > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", m.cursor+1, len(m.files)))
		if !m.sidebarFocused {
			parts = append(parts, fmt.Sprintf("%.0f%%", m.viewport.ScrollPercent()*100))
		}
	}

	if m.sidebarFocused {
		parts = append(parts, "enter/l: open  tab: switch  q: quit")
	} else {
		parts = append(parts, "h/tab: files  j/k: scroll  ^d/^u: half page  q: quit")
	}

	return styleFaint.Render(strings.Join(parts, "  ·  "))
}

func trimPath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	return "…" + path[len(path)-maxLen+1:]
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
