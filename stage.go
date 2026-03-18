package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/broothie/cob"
	tea "github.com/charmbracelet/bubbletea"
)

type stageResultMsg struct{ err error }
type commitResultMsg struct{ err error }

func stageFileCmd(root, path string) tea.Cmd {
	return func() tea.Msg {
		_, err := cob.Run(context.Background(), "git", cob.SetDir(root), cob.AddArgs("add", "--", path))
		return stageResultMsg{err: err}
	}
}

func unstageFileCmd(root, path string) tea.Cmd {
	return func() tea.Msg {
		_, err := cob.Run(context.Background(), "git", cob.SetDir(root), cob.AddArgs("reset", "HEAD", "--", path))
		return stageResultMsg{err: err}
	}
}

func unstageAllCmd(root string) tea.Cmd {
	return func() tea.Msg {
		_, err := cob.Run(context.Background(), "git", cob.SetDir(root), cob.AddArgs("reset", "HEAD"))
		return stageResultMsg{err: err}
	}
}

func stageHunkCmd(root, path string, hunk Hunk) tea.Cmd {
	return func() tea.Msg {
		patch := buildPatch(path, hunk)
		_, err := cob.Run(context.Background(), "git",
			cob.SetDir(root),
			cob.AddArgs("apply", "--cached", "--unidiff-zero"),
			cob.SetStdin(strings.NewReader(patch)),
		)
		return stageResultMsg{err: err}
	}
}

func unstageHunkCmd(root, path string, hunk Hunk) tea.Cmd {
	return func() tea.Msg {
		patch := buildPatch(path, hunk)
		_, err := cob.Run(context.Background(), "git",
			cob.SetDir(root),
			cob.AddArgs("apply", "--cached", "--reverse", "--unidiff-zero"),
			cob.SetStdin(strings.NewReader(patch)),
		)
		return stageResultMsg{err: err}
	}
}

func commitStagedCmd(root, message string) tea.Cmd {
	return func() tea.Msg {
		_, err := cob.Run(context.Background(), "git", cob.SetDir(root), cob.AddArgs("commit", "-m", message))
		return commitResultMsg{err: err}
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
