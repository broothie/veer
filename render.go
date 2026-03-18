package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleScrollThumb = lipgloss.NewStyle().Background(lipgloss.Color("244"))
	styleScrollTrack = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	styleBar         = lipgloss.NewStyle().Background(lipgloss.Color("237"))
)

const (
	scrollThumbChar = "█"
	scrollTrackChar = " "
)

// renderScrollbar renders a vertical scrollbar column of the given height.
// Always returns a column; renders a plain track when all content is visible.
func renderScrollbar(height, total, offset int) string {
	if total <= height {
		var sb strings.Builder
		for i := range height {
			if i > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(styleScrollTrack.Render(scrollTrackChar))
		}
		return sb.String()
	}

	thumbSize := max(1, height*height/total)
	maxOffset := total - height
	thumbPos := offset * (height - thumbSize) / maxOffset

	var sb strings.Builder
	for i := range height {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if i >= thumbPos && i < thumbPos+thumbSize {
			sb.WriteString(styleScrollThumb.Render(scrollThumbChar))
		} else {
			sb.WriteString(styleScrollTrack.Render(scrollTrackChar))
		}
	}
	return sb.String()
}

func (m model) renderHeader() string {
	sep := " · "

	var parts []string
	if m.cwd != "" {
		parts = append(parts, m.cwd)
	}
	if m.sha != "" {
		parts = append(parts, styleSHA.Render(m.sha))
	}

	line := strings.Join(parts, sep)

	// Append commit message, truncating if needed.
	// Render with explicit bar background to prevent SHA style reset from clearing it.
	if m.message != "" {
		sepStr := ""
		if len(parts) > 0 {
			sepStr = sep
		}
		avail := m.width - lipgloss.Width(line) - lipgloss.Width(sepStr) - 1 // 1 for leading space
		if avail > 3 {
			msg := m.message
			if len(msg) > avail {
				msg = msg[:avail-1] + "…"
			}
			line += styleBar.Render(sepStr + msg)
		}
	}

	return styleBar.Width(m.width).Render(" " + line)
}

func (m *model) buildDiffContent() string {
	if len(m.files) == 0 {
		vpWidth := max(1, m.width-m.sidebarWidth-sidebarPad-1-1) // -1 for scrollbar
		vpHeight := m.mainHeight()
		m.fileOffsets = nil
		return lipgloss.NewStyle().
			Width(vpWidth).
			Height(vpHeight).
			Align(lipgloss.Center, lipgloss.Center).
			Render(styleFaint.Render("no changes"))
	}

	var sb strings.Builder
	m.fileOffsets = make([]int, len(m.files))
	lineNum := 0

	for i, f := range m.files {
		m.fileOffsets[i] = lineNum

		if len(f.Lines) == 0 {
			continue
		}

		// File path header — full-width bar with delta and status.
		if i > 0 {
			sb.WriteByte('\n')
			lineNum++
		}
		vpWidth := max(1, m.width-m.sidebarWidth-sidebarPad-1-1)

		left := " " + f.Path

		var statusStr string
		switch {
		case f.Staged && f.Unstaged:
			statusStr = "SM"
		case f.Staged:
			statusStr = "S"
		default:
			statusStr = "M"
		}
		right := fmt.Sprintf("+%d -%d %s ", f.Added, f.Removed, statusStr)

		gap := max(1, vpWidth-lipgloss.Width(left)-lipgloss.Width(right))
		sb.WriteString(styleFilePath.Render(left + strings.Repeat(" ", gap) + right))
		sb.WriteByte('\n')
		lineNum++

		// Determine line number column width for this file.
		maxNum := 0
		for _, dl := range f.Lines {
			maxNum = max(maxNum, dl.OldNum)
			maxNum = max(maxNum, dl.NewNum)
		}
		numWidth := max(3, len(fmt.Sprint(maxNum)))

		for _, dl := range f.Lines {
			sb.WriteString(renderDiffLine(dl, numWidth))
			sb.WriteByte('\n')
			lineNum++
		}
	}

	return sb.String()
}

func renderDiffLine(dl DiffLine, numWidth int) string {
	switch dl.Type {
	case LineSeparator:
		return styleGutter.Render(fmt.Sprintf(" %*s   ", numWidth, "…"))
	case LineContext:
		return styleGutter.Render(fmt.Sprintf(" %*d   ", numWidth, dl.NewNum)) + dl.Content
	case LineAdded:
		return styleGutter.Render(fmt.Sprintf(" %*d + ", numWidth, dl.NewNum)) + styleAddLine.Render(dl.Content)
	case LineRemoved:
		return styleGutter.Render(fmt.Sprintf(" %*d - ", numWidth, dl.OldNum)) + styleRemLine.Render(dl.Content)
	case LineHeader:
		pad := strings.Repeat("─", numWidth+3)
		return styleHeader.Render(fmt.Sprintf(" %s %s ", pad, dl.Content))
	default:
		return ""
	}
}

func (m model) renderStatus() string {
	if m.err != nil {
		return styleBar.Width(m.width).Render(" error: " + m.err.Error())
	}

	var parts []string
	if len(m.files) > 0 {
		totalAdd, totalRem := 0, 0
		for _, f := range m.files {
			totalAdd += f.Added
			totalRem += f.Removed
		}
		parts = append(parts, fmt.Sprintf("%d files", len(m.files)))
		parts = append(parts, styleAdd.Render(fmt.Sprintf("+%d", totalAdd))+" "+styleRem.Render(fmt.Sprintf("-%d", totalRem)))
		parts = append(parts, fmt.Sprintf("%d/%d", m.cursor+1, len(m.files)))
	}

	switch m.focus {
	case focusFiles:
		parts = append(parts, "enter/l: open  tab: commits  q: quit")
	case focusCommits:
		parts = append(parts, "enter: select  tab: diff  shift+tab: files  q: quit")
	case focusDiff:
		parts = append(parts, "tab: files  j/k ↑↓  ^f/^b: page  q: quit")
	}

	return styleBar.Width(m.width).Render(" " + strings.Join(parts, "  ·  "))
}
