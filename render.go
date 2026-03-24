package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleScrollThumb       = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleScrollThumbActive = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleScrollTrack       = lipgloss.NewStyle().Foreground(lipgloss.Color("239"))
	styleScrollTrackActive = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styleBar               = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	stylePaneBorder        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	stylePaneBorderActive  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	stylePaneTitle         = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	stylePaneTitleActive   = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
)

const (
	scrollThumbChar       = "█"
	scrollThumbCharActive = "▐"
	scrollTrackChar       = "│"
	scrollTrackCharActive = "┃"
)

// renderScrollbar renders a vertical scrollbar column of the given height.
// Returns empty string if all content is visible.
func renderScrollbar(height, total, offset int, active bool) string {
	if total <= height {
		return ""
	}

	thumbSize := max(1, height*height/total)
	maxOffset := total - height
	thumbPos := offset * (height - thumbSize) / maxOffset

	var sb strings.Builder
	thumbStyle := styleScrollThumb
	trackStyle := styleScrollTrack
	thumbChar := scrollThumbChar
	trackChar := scrollTrackChar
	if active {
		thumbStyle = styleScrollThumbActive
		trackStyle = styleScrollTrackActive
		thumbChar = scrollThumbCharActive
		trackChar = scrollTrackCharActive
	}
	for i := range height {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if i >= thumbPos && i < thumbPos+thumbSize {
			sb.WriteString(thumbStyle.Render(thumbChar))
		} else {
			sb.WriteString(trackStyle.Render(trackChar))
		}
	}
	return sb.String()
}

func (m model) renderHeader() string {
	sep := " | "

	var parts []string
	if m.cwd != "" {
		maxCWD := max(16, m.width/3)
		parts = append(parts, truncateLeft(m.cwd, maxCWD))
	}
	if m.branch != "" {
		parts = append(parts, styleBranch.Inherit(styleBar).Render(m.branch))
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

	return styleBar.Width(m.width).Render(" " + line)
}

func truncateLeft(s string, maxWidth int) string {
	if maxWidth <= 0 || lipgloss.Width(s) <= maxWidth {
		return s
	}
	if maxWidth == 1 {
		return "…"
	}
	if maxWidth >= len(s) {
		return s
	}
	return "…" + s[len(s)-maxWidth+1:]
}

func truncateRight(s string, maxWidth int) string {
	if maxWidth <= 0 || lipgloss.Width(s) <= maxWidth {
		return s
	}
	if maxWidth == 1 {
		return "…"
	}
	if maxWidth >= len(s) {
		return s
	}
	return s[:maxWidth-1] + "…"
}

func (m *model) buildDiffContent() string {
	if len(m.files) == 0 {
		vpWidth := m.vpWidth()
		vpHeight := m.paneBodyHeight()
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
	// Gutter staging indicator: S (staged) or M (unstaged), only on changed lines.
	var indicator string
	if dl.Type == LineAdded || dl.Type == LineRemoved {
		switch section {
		case "staged":
			indicator = styleStaged.Inherit(styleGutter).Render("S")
		case "unstaged":
			indicator = styleSHA.Inherit(styleGutter).Render("M")
		default:
			indicator = styleGutter.Render(" ")
		}
	} else {
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
	case LineBinary:
		return indicator + styleGutter.Render(fmt.Sprintf(" %*s   ", numWidth, "")) + styleFaint.Render(dl.Content)
	default:
		return ""
	}
}

func (m model) renderStatus() string {
	if m.err != nil {
		return styleBar.Width(m.width).Render(" error: " + m.err.Error())
	}

	var hint string
	paneHint := m.paneShortcutHint()
	switch m.focus {
	case focusFiles:
		parts := []string{paneHint, "s: stage", "enter: open", "tab: next", "q: quit"}
		if m.hasStaged() {
			parts = append([]string{"u: unstage all", "c: commit"}, parts...)
		}
		hint = strings.Join(parts, " | ")
	case focusCommitMsg:
		hint = "^d: commit | esc: cancel | " + paneHint + " | tab: next"
	case focusCommits:
		hint = paneHint + " | enter: select | tab: next | q: quit"
	case focusDiff:
		parts := []string{paneHint, "s: stage hunk", "tab: files", "j/k ↑↓  ^f/^b: page", "q: quit"}
		hint = strings.Join(parts, " | ")
	}

	return styleBar.Width(m.width).Render(styleBar.Render(" " + hint))
}

func renderPane(title, body, rightOverlay string, width, height int, active bool) string {
	width = max(2, width)
	height = max(2, height)
	innerWidth := max(1, width-2)
	bodyHeight := max(1, height-2)

	borderStyle := stylePaneBorder
	titleStyle := stylePaneTitle
	if active {
		borderStyle = stylePaneBorderActive
		titleStyle = stylePaneTitleActive
	}

	titleText := truncateRight(title, innerWidth)
	topFill := max(0, innerWidth-lipgloss.Width(titleText))
	top := borderStyle.Render("┌") +
		titleStyle.Render(titleText) +
		borderStyle.Render(strings.Repeat("─", topFill)+"┐")
	bottom := borderStyle.Render("└" + strings.Repeat("─", innerWidth) + "┘")

	bodyLines := strings.Split(body, "\n")
	overlayLines := strings.Split(rightOverlay, "\n")
	contentStyle := lipgloss.NewStyle().MaxWidth(innerWidth).Width(innerWidth)
	lines := make([]string, 0, height)
	lines = append(lines, top)
	for i := 0; i < bodyHeight; i++ {
		line := ""
		if i < len(bodyLines) {
			line = bodyLines[i]
		}
		rightBorder := borderStyle.Render("│")
		if i < len(overlayLines) && overlayLines[i] != "" {
			rightBorder = overlayLines[i]
		}
		lines = append(lines,
			borderStyle.Render("│")+
				contentStyle.Render(line)+
				rightBorder,
		)
	}
	lines = append(lines, bottom)
	return strings.Join(lines, "\n")
}
