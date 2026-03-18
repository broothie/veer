package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var styleSidebar = lipgloss.NewStyle().PaddingRight(sidebarPad)

// treeEntry is a row in the sidebar: either a directory header or a file.
type treeEntry struct {
	name    string
	fileIdx int // -1 for directory headers
	depth   int
}

func (m model) renderSidebar(height int) string {
	fileH, commitStart := m.sidebarSplit()

	// File tree section.
	fileSection := m.renderFileTree(fileH)

	// Commit list section.
	commitH := height - fileH
	commitSection := m.renderCommitList(commitH)

	lines := make([]string, 0, height)

	// File tree lines.
	fileLines := strings.Split(fileSection, "\n")
	lines = append(lines, fileLines...)
	for len(lines) < fileH {
		lines = append(lines, "")
	}

	// Commit list lines.
	commitLines := strings.Split(commitSection, "\n")
	lines = append(lines, commitLines...)
	for len(lines) < height {
		lines = append(lines, "")
	}

	_ = commitStart
	return styleSidebar.
		Width(m.sidebarWidth + sidebarPad).
		Height(height).
		Render(strings.Join(lines[:height], "\n"))
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

	// Branch/ref header line.
	branch := m.branch
	if branch == "" && m.sha != "" {
		branch = m.sha
	}
	if branch != "" {
		label := " " + branch + " "
		lineWidth := m.sidebarWidth - lipgloss.Width(label)
		if lineWidth < 0 {
			lineWidth = 0
		}
		lines = append(lines, styleFaint.Render(label+strings.Repeat("─", lineWidth)))
		height-- // consume one row
	}

	total := len(m.commits) + 1 // +1 for working tree entry

	start := m.commitOffset
	end := min(start+height, total)

	for i := start; i < end; i++ {
		if i == 0 {
			// Working tree entry.
			prefix := "  "
			if m.commitCursor == 0 && m.focus == focusCommits {
				prefix = "> "
			}
			label := "working tree"
			if m.selectedCommit == -1 {
				label = styleActive.Render("● ") + label
			} else {
				label = styleFaint.Render("○ ") + label
			}
			line := prefix + label
			if m.commitCursor == 0 && m.focus == focusCommits {
				line = prefix + label
			}
			lines = append(lines, line)
		} else {
			ci := i - 1
			if ci >= len(m.commits) {
				continue
			}
			c := m.commits[ci]

			prefix := "  "
			if m.commitCursor == i && m.focus == focusCommits {
				prefix = "> "
			}

			sha := styleSHA.Render(c.SHA)

			// Truncate message to fit.
			usedWidth := lipgloss.Width(prefix) + lipgloss.Width(c.SHA) + 3 // 3 for "● " + space
			msgMax := m.sidebarWidth - usedWidth
			msg := c.Message
			if len(msg) > msgMax && msgMax > 3 {
				msg = msg[:msgMax-1] + "…"
			} else if msgMax <= 3 {
				msg = ""
			}

			var marker string
			if m.selectedCommit == ci {
				marker = styleActive.Render("● ")
			} else {
				marker = styleFaint.Render("○ ")
			}

			line := prefix + marker + sha + " " + msg
			if m.commitCursor == i && m.focus == focusCommits {
				// Already has > prefix, that's enough.
			}
			lines = append(lines, line)
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

	// Status indicator: S=staged, M=unstaged (modified).
	var status, coloredStatus string
	switch {
	case f.Staged && f.Unstaged:
		status = "SM"
		coloredStatus = styleStaged.Render("S") + styleSHA.Render("M")
	case f.Staged:
		status = "S "
		coloredStatus = styleStaged.Render("S") + " "
	default:
		status = "M "
		coloredStatus = styleSHA.Render("M") + " "
	}

	delta := addStr + " " + remStr + " " + status
	coloredDelta := styleAdd.Render(addStr) + " " + styleRem.Render(remStr) + " " + coloredStatus

	var marker string
	if e.fileIdx == m.cursor {
		marker = styleActive.Render("● ")
	} else {
		marker = styleFaint.Render("○ ")
	}
	markerWidth := lipgloss.Width("● ")

	// Use lipgloss.Width for accurate width calculation.
	usedWidth := lipgloss.Width(indent) + markerWidth + lipgloss.Width(delta)
	nameMaxLen := m.sidebarWidth - usedWidth - 1 // 1 for gap
	name := e.name
	if len(name) > nameMaxLen && nameMaxLen > 3 {
		name = name[:nameMaxLen-1] + "…"
	}

	gap := m.sidebarWidth - lipgloss.Width(indent) - markerWidth - lipgloss.Width(name) - lipgloss.Width(delta)
	if gap < 1 {
		gap = 1
	}
	padding := strings.Repeat(" ", gap)

	if e.fileIdx == m.cursor {
		if m.focus == focusFiles {
			return indent + marker + styleActive.Render(name) + padding + coloredDelta
		}
		return indent + marker + styleBold.Render(name) + padding + coloredDelta
	}

	return indent + marker + name + padding + coloredDelta
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
