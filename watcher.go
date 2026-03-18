package main

import (
	"errors"
	"io/fs"
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

	allSkipDirs := makeSkipDirSet(extraSkipDirs)
	gitDir, err := resolveGitDir(repoRoot)
	if err != nil {
		debugf("watcher: failed to resolve git dir: %v", err)
	}

	watchGitDir(w, gitDir)
	count := watchWorktree(w, repoRoot, allSkipDirs)
	debugf("watcher: watching %d directories under %s", count, repoRoot)

	go debounceLoop(w, ch, done, repoRoot, gitDir, debounce, allSkipDirs)

	cleanup := func() {
		close(done)
		w.Close()
	}

	return ch, cleanup
}

// debounceLoop reads fsnotify events and coalesces them into single notifications.
func debounceLoop(w *fsnotify.Watcher, ch chan<- struct{}, done <-chan struct{}, repoRoot, gitDir string, debounce time.Duration, skip map[string]bool) {
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
			if ev.Has(fsnotify.Create) {
				watchNewPaths(w, ev.Name, skip)
			}
			if !isRelevantEvent(ev, repoRoot, gitDir) {
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

func makeSkipDirSet(extraSkipDirs []string) map[string]bool {
	allSkipDirs := make(map[string]bool, len(skipDirs)+len(extraSkipDirs))
	for k, v := range skipDirs {
		allSkipDirs[k] = v
	}
	for _, d := range extraSkipDirs {
		allSkipDirs[d] = true
	}
	return allSkipDirs
}

func resolveGitDir(repoRoot string) (string, error) {
	gitPath := filepath.Join(repoRoot, ".git")
	info, err := os.Lstat(gitPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return gitPath, nil
	}

	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir: "
	if !strings.HasPrefix(line, prefix) {
		return "", errors.New("unsupported .git file format")
	}
	resolved := strings.TrimSpace(line[len(prefix):])
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(repoRoot, resolved)
	}
	return filepath.Clean(resolved), nil
}

// watchGitDir watches key paths in the git dir for staging and commit changes.
func watchGitDir(w *fsnotify.Watcher, gitDir string) {
	if gitDir == "" {
		return
	}

	info, err := os.Lstat(gitDir)
	if err != nil {
		debugf("watcher: cannot stat git dir %s: %v", gitDir, err)
		return
	}
	if !info.IsDir() {
		debugf("watcher: git dir is not a directory: %s", gitDir)
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
	_ = addWatchPath(w, root, skip, &count)
	return count
}

func watchNewPaths(w *fsnotify.Watcher, path string, skip map[string]bool) {
	count := 0
	if err := addWatchPath(w, path, skip, &count); err != nil {
		debugf("watcher: failed to watch new path %s: %v", path, err)
	}
}

func addWatchPath(w *fsnotify.Watcher, path string, skip map[string]bool, count *int) error {
	return filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if skip[d.Name()] {
			return filepath.SkipDir
		}
		if count != nil && *count >= maxWatches {
			return filepath.SkipAll
		}
		if err := w.Add(path); err == nil && count != nil {
			*count = *count + 1
		}
		return nil
	})
}

// isRelevantEvent filters out noisy/irrelevant fsnotify events.
func isRelevantEvent(ev fsnotify.Event, repoRoot, gitDir string) bool {
	if gitDir != "" && (ev.Name == gitDir || strings.HasPrefix(ev.Name, gitDir+string(filepath.Separator))) {
		rel, _ := filepath.Rel(gitDir, ev.Name)
		return rel == "index" || rel == "HEAD" ||
			strings.HasPrefix(rel, "refs"+string(filepath.Separator))
	}
	if ev.Name == filepath.Join(repoRoot, ".git") {
		return true
	}
	return true
}
