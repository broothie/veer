package main

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alecthomas/kong"
)

var version = "dev"

type config struct {
	Interval     time.Duration    `short:"n" default:"250ms" help:"Refresh interval."`
	SidebarWidth int              `short:"w" name:"sidebar-width" default:"35" help:"Initial sidebar width."`
	Context      int              `short:"U" default:"3" help:"Number of context lines in diff."`
	Theme        string           `short:"t" default:"dracula" help:"Syntax highlighting theme (e.g. dracula, monokai, github-dark)."`
	Staged       bool             `short:"s" help:"Show only staged changes."`
	Unstaged     bool             `short:"u" help:"Show only unstaged changes."`
	Version      kong.VersionFlag `short:"v" help:"Print version."`
	Ref          string           `arg:"" optional:"" help:"Diff working tree against this ref (branch, tag, SHA)."`
	Paths        []string         `arg:"" optional:"" help:"Filter to specific paths."`
}

func main() {
	var cfg config
	kong.Parse(&cfg,
		kong.Name("veer"),
		kong.Description("A live-diffing TUI for coding with AI."),
		kong.Vars{"version": version},
		kong.UsageOnError(),
	)

	initTheme(cfg.Theme)

	p := tea.NewProgram(newModel(cfg), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
