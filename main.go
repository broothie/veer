package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alecthomas/kong"
)

var version = "dev"
var readBuildInfo = debug.ReadBuildInfo
var runGit = func(args ...string) (string, error) {
	// #nosec G204 -- args come from fixed internal version-probing call sites.
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

type config struct {
	Interval     time.Duration    `short:"n" default:"250ms" help:"Refresh interval (fallback when file watcher is active)."`
	Debounce     time.Duration    `name:"debounce" default:"100ms" help:"File watcher debounce duration."`
	SidebarWidth int              `short:"w" name:"sidebar-width" default:"35" help:"Initial sidebar width."`
	Context      int              `short:"U" default:"3" help:"Number of context lines in diff."`
	Theme        string           `short:"t" default:"dracula" help:"Syntax highlighting theme (e.g. dracula, monokai, github-dark)."`
	DumpView     bool             `name:"dump-view" help:"Render one frame to stdout and exit."`
	DumpWidth    int              `name:"dump-width" default:"120" help:"Width to use with --dump-view."`
	DumpHeight   int              `name:"dump-height" default:"40" help:"Height to use with --dump-view."`
	NoHighlight  bool             `name:"no-syntax-highlight" help:"Disable syntax highlighting."`
	Staged       bool             `short:"s" help:"Show only staged changes."`
	Unstaged     bool             `short:"u" help:"Show only unstaged changes."`
	SkipDirs     []string         `name:"skip-dir" help:"Additional directories to skip when watching for changes."`
	Debug        bool             `short:"d" help:"Enable debug logging to ~/.veer/debug.log."`
	Version      kong.VersionFlag `short:"v" help:"Print version."`
	Ref          string           `arg:"" optional:"" help:"Diff working tree against this ref (branch, tag, SHA)."`
	Paths        []string         `arg:"" optional:"" help:"Filter to specific paths."`
}

func main() {
	var cfg config
	kong.Parse(&cfg,
		kong.Name("veer"),
		kong.Description("A live-diffing TUI for coding with AI."),
		kong.Vars{"version": currentVersion()},
		kong.UsageOnError(),
	)

	if err := initDebug(cfg.Debug); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	initTheme(cfg.Theme, !cfg.NoHighlight)

	if cfg.DumpView {
		view, err := dumpView(cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Print(view)
		return
	}

	p := tea.NewProgram(newModel(cfg), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func currentVersion() string {
	if version != "" && version != "dev" {
		return version
	}

	info, ok := readBuildInfo()
	if !ok {
		return gitVersion()
	}

	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}

	var revision string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}

	if revision == "" {
		return gitVersion()
	}
	if len(revision) > 7 {
		revision = revision[:7]
	}
	if modified {
		return revision + "-dirty"
	}
	return revision
}

func gitVersion() string {
	revision, err := runGit("rev-parse", "--short", "HEAD")
	if err != nil || revision == "" {
		return version
	}

	status, err := runGit("status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return revision
	}
	if status != "" {
		return revision + "-dirty"
	}
	return revision
}
