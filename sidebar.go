package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// treeEntry is a row in the sidebar: either a directory header or a file.
type treeEntry struct {
	name    string
	fileIdx int // -1 for directory headers
	depth   int
}

func (m model) renderSidebar(height int) string {
	if len(m.tree) == 0 {
		msg := "no changes"
		if m.err != nil {
			msg = "error"
		}
		return styleSidebar.
			Width(sidebarWidth + sidebarPad).
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

	return styleSidebar.
		Width(sidebarWidth + sidebarPad).
		Height(height).
		Render(strings.Join(lines, "\n"))
}

func (m model) renderTreeEntry(e treeEntry) string {
	indent := strings.Repeat("  ", e.depth)

	if e.fileIdx < 0 {
		return styleDir.Render(indent + e.name)
	}

	f := m.files[e.fileIdx]

	addStr := fmt.Sprintf("+%d", f.Added)
	remStr := fmt.Sprintf("-%d", f.Removed)
	delta := addStr + " " + remStr
	coloredDelta := styleAdd.Render(addStr) + " " + styleRem.Render(remStr)

	prefix := "  "
	if e.fileIdx == m.cursor {
		prefix = "> "
	}

	// Use lipgloss.Width for accurate width calculation.
	usedWidth := lipgloss.Width(indent) + lipgloss.Width(prefix) + lipgloss.Width(delta)
	nameMaxLen := sidebarWidth - usedWidth - 1 // 1 for gap
	name := e.name
	if len(name) > nameMaxLen && nameMaxLen > 3 {
		name = name[:nameMaxLen-1] + "…"
	}

	gap := sidebarWidth - lipgloss.Width(indent) - lipgloss.Width(prefix) - lipgloss.Width(name) - lipgloss.Width(delta)
	if gap < 1 {
		gap = 1
	}
	padding := strings.Repeat(" ", gap)

	if e.fileIdx == m.cursor {
		nameStr := prefix + name
		if m.sidebarFocused {
			return indent + styleActive.Render(nameStr) + padding + coloredDelta
		}
		return indent + styleBold.Render(nameStr) + padding + coloredDelta
	}

	return indent + prefix + name + padding + coloredDelta
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
