package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleScrollThumb = lipgloss.NewStyle().Background(lipgloss.Color("244"))
	styleScrollTrack = lipgloss.NewStyle().Background(lipgloss.Color("237"))
	styleBar         = lipgloss.NewStyle().Background(lipgloss.Color("237"))
)

const (
	scrollThumbChar = "█"
	scrollTrackChar = " "
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

// renderEmptyColumn renders a 1-char-wide column of spaces to fill reserved layout space.
func renderEmptyColumn(height int) string {
	var sb strings.Builder
	for i := range height {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteByte(' ')
	}
	return sb.String()
}

// renderBorder renders a thin vertical border column of the given height.
func renderBorder(height int) string {
	var sb strings.Builder
	for i := range height {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(styleFaint.Render("│"))
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
		parts = append(parts, styleSHA.Inherit(styleBar).Render(m.sha))
	}
	if len(m.files) > 0 {
		totalAdd, totalRem := 0, 0
		for _, f := range m.files {
			totalAdd += f.Added
			totalRem += f.Removed
		}
		delta := styleAdd.Inherit(styleBar).Render(fmt.Sprintf("+%d", totalAdd)) +
			styleBar.Render(" ") +
			styleRem.Inherit(styleBar).Render(fmt.Sprintf("-%d", totalRem))
		parts = append(parts, delta)
	}

	line := strings.Join(parts, styleBar.Render(sep))

	// Append commit message, truncating if needed.
	// Render with explicit bar background to prevent SHA style reset from clearing it.
	if m.message != "" {
		sepStr := sep
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
		vpWidth := m.vpWidth()
		vpHeight := m.mainHeight()
		m.fileOffsets = nil
		m.hunkRefs = nil
		return lipgloss.NewStyle().
			Width(vpWidth).
			Height(vpHeight).
			Align(lipgloss.Center, lipgloss.Center).
			Render(styleFaint.Render("no changes"))
	}

	var sb strings.Builder
	m.fileOffsets = make([]int, len(m.files))
	m.hunkRefs = nil
	lineNum := 0

	for i, f := range m.files {
		m.fileOffsets[i] = lineNum

		if len(f.Lines) == 0 {
			continue
		}

		// File path header — full-width bar with delta and status.
		if i > 0 {
			sb.WriteByte('\n')
			m.hunkRefs = append(m.hunkRefs, hunkRef{i, -1})
			lineNum++
		}
		vpWidth := m.vpWidth()

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
		m.hunkRefs = append(m.hunkRefs, hunkRef{i, -1})
		lineNum++

		// Determine line number column width for this file.
		maxNum := 0
		for _, dl := range f.Lines {
			maxNum = max(maxNum, dl.OldNum)
			maxNum = max(maxNum, dl.NewNum)
		}
		numWidth := max(3, len(fmt.Sprint(maxNum)))

		// Batch-highlight all lines for this file.
		highlighted := highlightFile(f.Path, f.Lines)

		// Track hunk index for this file's lines.
		hunkIdx := 0
		seenContent := false

		for j, dl := range f.Lines {
			var section string
			switch dl.Type {
			case LineHeader:
				if seenContent {
					hunkIdx++
				}
				m.hunkRefs = append(m.hunkRefs, hunkRef{i, -1})
			case LineSeparator:
				hunkIdx++
				m.hunkRefs = append(m.hunkRefs, hunkRef{i, -1})
				// Use section of the next hunk for the separator line.
				if hunkIdx < len(f.Hunks) {
					section = f.Hunks[hunkIdx].Section
				}
			default:
				seenContent = true
				m.hunkRefs = append(m.hunkRefs, hunkRef{i, hunkIdx})
				if hunkIdx < len(f.Hunks) {
					section = f.Hunks[hunkIdx].Section
				}
			}

			sb.WriteString(renderDiffLine(dl, numWidth, highlighted[j], section))
			sb.WriteByte('\n')
			lineNum++
		}
	}

	return sb.String()
}

func renderDiffLine(dl DiffLine, numWidth int, hl highlightedLine, section string) string {
	// Gutter staging indicator: S (staged) or M (unstaged).
	var indicator string
	switch section {
	case "staged":
		indicator = styleStaged.Inherit(styleGutter).Render("S")
	case "unstaged":
		indicator = styleSHA.Inherit(styleGutter).Render("M")
	default:
		indicator = styleGutter.Render(" ")
	}

	switch dl.Type {
	case LineSeparator:
		return indicator + styleGutter.Render(fmt.Sprintf(" %*s   ", numWidth, "…"))
	case LineContext:
		return indicator + styleGutter.Render(fmt.Sprintf(" %*d   ", numWidth, dl.NewNum)) + renderHighlighted(hl, dl.Content)
	case LineAdded:
		return indicator + styleGutter.Render(fmt.Sprintf(" %*d + ", numWidth, dl.NewNum)) + renderHighlightedWithBG(hl, dl.Content, lipgloss.Color("22"))
	case LineRemoved:
		return indicator + styleGutter.Render(fmt.Sprintf(" %*d - ", numWidth, dl.OldNum)) + renderHighlightedWithBG(hl, dl.Content, lipgloss.Color("52"))
	case LineHeader:
		return styleGutter.Render(fmt.Sprintf("  %*s   ", numWidth, "…"))
	default:
		return ""
	}
}

func (m model) renderStatus() string {
	if m.err != nil {
		return styleBar.Width(m.width).Render(" error: " + m.err.Error())
	}

	var hint string
	switch m.focus {
	case focusFiles:
		parts := []string{"s: stage", "enter: open", "tab: next", "q: quit"}
		if m.hasStaged() {
			parts = append([]string{"u: unstage all", "c: commit"}, parts...)
		}
		hint = strings.Join(parts, "  ")
	case focusCommitMsg:
		hint = "^d: commit  esc: cancel  tab: next"
	case focusCommits:
		hint = "enter: select  tab: next  q: quit"
	case focusDiff:
		parts := []string{"s: stage hunk", "tab: files", "j/k ↑↓  ^f/^b: page", "q: quit"}
		hint = strings.Join(parts, "  ")
	}

	return styleBar.Width(m.width).Render(styleBar.Render(" " + hint))
}
