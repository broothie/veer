package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// skipDirs are directory names to skip when setting up file watches.
// These are typically large generated/dependency directories.
var skipDirs = map[string]bool{
	".git":             true,
	"node_modules":     true,
	".next":            true,
	"__pycache__":      true,
	".cache":           true,
	".yarn":            true,
	".pnpm":            true,
	"target":           true,
	".tox":             true,
	"venv":             true,
	".venv":            true,
	"bower_components": true,
	".turbo":           true,
}

const maxWatches = 4096

type filesChangedMsg struct{}

// startWatcher creates an fsnotify file watcher for the repo root.
// Returns a channel that receives notifications when files change, and a cleanup function.
// Returns nil channel if the watcher could not be created.
func startWatcher(repoRoot string, debounce time.Duration, extraSkipDirs []string) (<-chan struct{}, func()) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		debugf("watcher: failed to create: %v", err)
		return nil, func() {}
	}

	ch := make(chan struct{}, 1)
	done := make(chan struct{})

	allSkipDirs := make(map[string]bool, len(skipDirs)+len(extraSkipDirs))
	for k, v := range skipDirs {
		allSkipDirs[k] = v
	}
	for _, d := range extraSkipDirs {
		allSkipDirs[d] = true
	}

	watchGitDir(w, repoRoot)
	count := watchWorktree(w, repoRoot, allSkipDirs)
	debugf("watcher: watching %d directories under %s", count, repoRoot)

	go debounceLoop(w, ch, done, repoRoot, debounce)

	cleanup := func() {
		close(done)
		w.Close()
	}

	return ch, cleanup
}

// debounceLoop reads fsnotify events and coalesces them into single notifications.
func debounceLoop(w *fsnotify.Watcher, ch chan<- struct{}, done <-chan struct{}, repoRoot string, debounce time.Duration) {
	var timer *time.Timer
	for {
		select {
		case <-done:
			if timer != nil {
				timer.Stop()
			}
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if !isRelevantEvent(ev, repoRoot) {
				continue
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(debounce, func() {
				select {
				case ch <- struct{}{}:
				default:
				}
			})
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			debugf("watcher: error: %v", err)
		}
	}
}

// watchGitDir watches key paths in .git for staging and commit changes.
func watchGitDir(w *fsnotify.Watcher, repoRoot string) {
	gitDir := filepath.Join(repoRoot, ".git")
	info, err := os.Lstat(gitDir)
	if err != nil {
		debugf("watcher: cannot stat .git: %v", err)
		return
	}
	if !info.IsDir() {
		// .git is a file in worktrees; skip for now.
		debugf("watcher: .git is a file (worktree), skipping .git watches")
		return
	}

	paths := []string{
		gitDir,
		filepath.Join(gitDir, "refs"),
		filepath.Join(gitDir, "refs", "heads"),
		filepath.Join(gitDir, "refs", "tags"),
	}
	for _, p := range paths {
		if err := w.Add(p); err != nil {
			debugf("watcher: failed to watch %s: %v", p, err)
		}
	}
}

// watchWorktree walks the worktree and watches directories, skipping known-large ones.
func watchWorktree(w *fsnotify.Watcher, root string, skip map[string]bool) int {
	count := 0
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if count >= maxWatches {
			return filepath.SkipAll
		}
		if skip[d.Name()] {
			return filepath.SkipDir
		}
		if err := w.Add(path); err == nil {
			count++
		}
		return nil
	})
	return count
}

// isRelevantEvent filters out noisy/irrelevant fsnotify events.
func isRelevantEvent(ev fsnotify.Event, repoRoot string) bool {
	gitDir := filepath.Join(repoRoot, ".git")
	if strings.HasPrefix(ev.Name, gitDir+string(filepath.Separator)) {
		rel, _ := filepath.Rel(gitDir, ev.Name)
		return rel == "index" || rel == "HEAD" ||
			strings.HasPrefix(rel, "refs"+string(filepath.Separator))
	}
	return true
}
