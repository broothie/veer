package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var styleSidebar = lipgloss.NewStyle()

// treeEntry is a row in the sidebar: either a directory header or a file.
type treeEntry struct {
	name    string
	fileIdx int // -1 for directory headers
	depth   int
}

func (m model) renderSidebar(height int) string {
	fileH, msgH, commitH := m.sidebarSplit()
	lines := make([]string, 0, height)

	fileLines := strings.Split(m.renderFileTree(fileH), "\n")
	lines = append(lines, fileLines...)
	for len(lines) < fileH {
		lines = append(lines, "")
	}

	if msgH > 0 {
		msgLines := strings.Split(m.renderCommitInput(), "\n")
		lines = append(lines, msgLines...)
		for len(lines) < fileH+msgH {
			lines = append(lines, "")
		}
	}

	commitLines := strings.Split(m.renderHistoryBody(commitH), "\n")
	lines = append(lines, commitLines...)
	for len(lines) < height {
		lines = append(lines, "")
	}

	return styleSidebar.
		Width(m.sidebarWidth).
		Height(height).
		Render(strings.Join(lines[:height], "\n"))
}

func (m model) renderHistoryBody(commitH int) string {
	lines := make([]string, 0, commitH)
	commitSection := m.renderCommitList(commitH)
	commitLines := strings.Split(commitSection, "\n")
	lines = append(lines, commitLines...)
	for len(lines) < commitH {
		lines = append(lines, "")
	}

	return styleSidebar.
		Width(m.sidebarWidth).
		Height(commitH).
		Render(strings.Join(lines[:commitH], "\n"))
}

func (m model) renderCommitInput() string {
	if m.focus == focusCommitMsg {
		m.commitMsg.SetWidth(m.sidebarWidth)
		view := m.commitMsg.View()
		viewLines := strings.Split(view, "\n")
		result := append([]string(nil), viewLines...)
		for len(result) < 3 {
			result = append(result, "")
		}
		return strings.Join(result[:3], "\n")
	}

	lines := []string{
		styleFaint.Render(" c: type message"),
		"",
		"",
	}
	return strings.Join(lines, "\n")
}

func (m model) renderFileTree(height int) string {
	if len(m.tree) == 0 {
		msg := "no changes"
		if m.err != nil {
			msg = "error"
		}
		return lipgloss.NewStyle().
			Width(m.sidebarWidth).
			Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Render(styleFaint.Render(msg))
	}

	var lines []string
	start := m.sidebarOffset
	end := min(start+height, len(m.tree))

	for _, entry := range m.tree[start:end] {
		lines = append(lines, m.renderTreeEntry(entry))
	}

	return strings.Join(lines, "\n")
}

func (m model) renderCommitList(height int) string {
	if height < 1 {
		return ""
	}

	var lines []string

	total := len(m.commits) + 1 // +1 for HEAD entry

	start := m.commitOffset
	end := min(start+height, total)

	for i := start; i < end; i++ {
		if i == 0 {
			headLabel := "HEAD"
			if m.branch != "" {
				headLabel += " (" + m.branch + ")"
			}
			selected := m.commitCursor == 0
			if m.selectedCommit == -1 {
				headLabel = renderSidebarStyled("● ", styleActive, selected) + renderSidebarText(headLabel, selected)
			} else {
				headLabel = renderSidebarStyled("○ ", styleFaint, selected) + renderSidebarText(headLabel, selected)
			}
			lines = append(lines, renderSelectableSidebarRow(headLabel, m.sidebarWidth, selected))
		} else {
			ci := i - 1
			if ci >= len(m.commits) {
				continue
			}
			c := m.commits[ci]
			selected := m.commitCursor == i

			sha := renderSidebarStyled(c.SHA, styleSHA, selected)

			// Truncate message to fit.
			usedWidth := lipgloss.Width(c.SHA) + 3 // 3 for "● " + space
			msgMax := m.sidebarWidth - usedWidth
			msg := c.Message
			if len(msg) > msgMax && msgMax > 3 {
				msg = msg[:msgMax-1] + "…"
			} else if msgMax <= 3 {
				msg = ""
			}

			var marker string
			if m.selectedCommit == ci {
				marker = renderSidebarStyled("● ", styleActive, selected)
			} else {
				marker = renderSidebarStyled("○ ", styleFaint, selected)
			}

			line := marker + sha + renderSidebarText(" "+msg, selected)
			lines = append(lines, renderSelectableSidebarRow(line, m.sidebarWidth, selected))
		}
	}

	return strings.Join(lines, "\n")
}

func (m model) renderTreeEntry(e treeEntry) string {
	indent := strings.Repeat("  ", e.depth)

	if e.fileIdx < 0 {
		return styleDir.Render(indent + e.name)
	}

	f := m.files[e.fileIdx]

	addStr := fmt.Sprintf("+%d", f.Added)
	remStr := fmt.Sprintf("-%d", f.Removed)
	selected := e.fileIdx == m.cursor

	// Status indicator: S=staged, M=unstaged (modified).
	var status, coloredStatus string
	switch {
	case f.Staged && f.Unstaged:
		status = "SM"
		coloredStatus = renderSidebarStyled("S", styleStaged, selected) + renderSidebarStyled("M", styleSHA, selected)
	case f.Staged:
		status = "S "
		coloredStatus = renderSidebarStyled("S", styleStaged, selected) + renderSidebarText(" ", selected)
	default:
		status = "M "
		coloredStatus = renderSidebarStyled("M", styleSHA, selected) + renderSidebarText(" ", selected)
	}

	delta := addStr + " " + remStr + " " + status
	coloredDelta := renderSidebarStyled(addStr, styleAdd, selected) +
		renderSidebarText(" ", selected) +
		renderSidebarStyled(remStr, styleRem, selected) +
		renderSidebarText(" ", selected) +
		coloredStatus

	var marker string
	if selected {
		marker = renderSidebarStyled("● ", styleActive, selected)
	} else {
		marker = renderSidebarStyled("○ ", styleFaint, selected)
	}
	markerWidth := lipgloss.Width("● ")

	// Use lipgloss.Width for accurate width calculation.
	rowWidth := m.sidebarWidth
	usedWidth := lipgloss.Width(indent) + markerWidth + lipgloss.Width(delta)
	nameMaxLen := rowWidth - usedWidth - 1 // 1 for gap
	name := e.name
	if len(name) > nameMaxLen && nameMaxLen > 3 {
		name = name[:nameMaxLen-1] + "…"
	}

	gap := rowWidth - lipgloss.Width(indent) - markerWidth - lipgloss.Width(name) - lipgloss.Width(delta)
	if gap < 1 {
		gap = 1
	}
	padding := strings.Repeat(" ", gap)

	line := renderSidebarText(indent, selected) + marker + renderSidebarText(name, selected) + renderSidebarText(padding, selected) + coloredDelta
	if selected {
		line = renderSidebarText(indent, selected) + marker + renderSidebarStyled(name, styleBold, selected) + renderSidebarText(padding, selected) + coloredDelta
	}

	return renderSelectableSidebarRow(line, m.sidebarWidth, selected)
}

func renderSelectableSidebarRow(content string, width int, selected bool) string {
	if width < 1 {
		return content
	}

	if !selected {
		return lipgloss.NewStyle().Width(width).Render(content)
	}

	return styleSelectBG.Width(width).Render(content)
}

func renderSidebarText(text string, selected bool) string {
	if selected {
		return styleSelectBG.Render(text)
	}
	return text
}

func renderSidebarStyled(text string, style lipgloss.Style, selected bool) string {
	if selected {
		return style.Inherit(styleSelectBG).Render(text)
	}
	return style.Render(text)
}

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
