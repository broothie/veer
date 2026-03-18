package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleScrollThumb = lipgloss.NewStyle().Background(lipgloss.Color("244"))
	styleScrollTrack = lipgloss.NewStyle().Background(lipgloss.Color("236"))
)

const (
	scrollThumbChar = "┃"
	scrollTrackChar = "│"
)

// renderScrollbar renders a vertical scrollbar column of the given height.
// Returns empty string if all content is visible.
func renderScrollbar(height, total, offset int) string {
	if total <= height {
		return ""
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
	sep := styleFaint.Render(" · ")

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

	line := strings.Join(parts, sep)

	// Append commit message, truncating if needed.
	if m.message != "" {
		prefix := line
		if len(parts) > 0 {
			prefix += sep
		}
		avail := m.width - lipgloss.Width(prefix) - 1 // 1 for leading space
		if avail > 3 {
			msg := m.message
			if len(msg) > avail {
				msg = msg[:avail-1] + "…"
			}
			line = prefix + msg
		}
	}

	return " " + line + "\n"
}

func (m model) buildDiffContent() string {
	if len(m.files) == 0 {
		vpWidth := max(1, m.width-sidebarWidth-sidebarPad-1-1) // -1 for scrollbar
		vpHeight := m.mainHeight()
		return lipgloss.NewStyle().
			Width(vpWidth).
			Height(vpHeight).
			Align(lipgloss.Center, lipgloss.Center).
			Render(styleFaint.Render("no changes"))
	}

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
		maxNum = max(maxNum, dl.OldNum)
		maxNum = max(maxNum, dl.NewNum)
	}
	numWidth := max(3, len(fmt.Sprint(maxNum)))

	var sb strings.Builder
	for _, dl := range f.Lines {
		sb.WriteString(renderDiffLine(dl, numWidth))
		sb.WriteByte('\n')
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
	default:
		return ""
	}
}

func (m model) renderStatus() string {
	if m.err != nil {
		return styleFaint.Render(" error: " + m.err.Error())
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

	if m.sidebarFocused {
		parts = append(parts, "enter/l: open  tab: switch  q: quit")
	} else {
		parts = append(parts, "h/tab: files  j/k ↑↓  ^d/^u  q: quit")
	}

	return "\n" + styleFaint.Render(" "+strings.Join(parts, "  ·  "))
}
