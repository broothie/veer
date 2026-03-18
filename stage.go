package main

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type stageResultMsg struct{ err error }
type commitResultMsg struct{ err error }

func stageFileCmd(root, path string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("git", "add", "--", path)
		cmd.Dir = root
		return stageResultMsg{err: cmd.Run()}
	}
}

func unstageFileCmd(root, path string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("git", "reset", "HEAD", "--", path)
		cmd.Dir = root
		return stageResultMsg{err: cmd.Run()}
	}
}

func unstageAllCmd(root string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("git", "reset", "HEAD")
		cmd.Dir = root
		return stageResultMsg{err: cmd.Run()}
	}
}

func stageHunkCmd(root, path string, hunk Hunk) tea.Cmd {
	return func() tea.Msg {
		patch := buildPatch(path, hunk)
		cmd := exec.Command("git", "apply", "--cached", "--unidiff-zero")
		cmd.Dir = root
		cmd.Stdin = strings.NewReader(patch)
		return stageResultMsg{err: cmd.Run()}
	}
}

func unstageHunkCmd(root, path string, hunk Hunk) tea.Cmd {
	return func() tea.Msg {
		patch := buildPatch(path, hunk)
		cmd := exec.Command("git", "apply", "--cached", "--reverse", "--unidiff-zero")
		cmd.Dir = root
		cmd.Stdin = strings.NewReader(patch)
		return stageResultMsg{err: cmd.Run()}
	}
}

func commitStagedCmd(root, message string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("git", "commit", "-m", message)
		cmd.Dir = root
		return commitResultMsg{err: cmd.Run()}
	}
}

func buildPatch(path string, hunk Hunk) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- a/%s\n", path))
	sb.WriteString(fmt.Sprintf("+++ b/%s\n", path))
	for _, line := range hunk.RawLines {
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return sb.String()
}
