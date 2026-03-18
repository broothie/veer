package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestResolveGitDir_Directory(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatalf("Mkdir(.git): %v", err)
	}

	got, err := resolveGitDir(root)
	if err != nil {
		t.Fatalf("resolveGitDir: %v", err)
	}
	if got != gitDir {
		t.Fatalf("resolveGitDir = %q, want %q", got, gitDir)
	}
}

func TestResolveGitDir_File(t *testing.T) {
	root := t.TempDir()
	actualGitDir := filepath.Join(root, "worktrees", "feature")
	if err := os.MkdirAll(actualGitDir, 0755); err != nil {
		t.Fatalf("MkdirAll(actualGitDir): %v", err)
	}

	gitFile := filepath.Join(root, ".git")
	content := "gitdir: worktrees/feature\n"
	if err := os.WriteFile(gitFile, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(.git): %v", err)
	}

	got, err := resolveGitDir(root)
	if err != nil {
		t.Fatalf("resolveGitDir: %v", err)
	}
	if got != actualGitDir {
		t.Fatalf("resolveGitDir = %q, want %q", got, actualGitDir)
	}
}

func TestAddWatchPath_AddsCreatedDirectory(t *testing.T) {
	root := t.TempDir()
	newDir := filepath.Join(root, "newdir")
	if err := os.Mkdir(newDir, 0755); err != nil {
		t.Fatalf("Mkdir(newDir): %v", err)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	count := 0
	if err := addWatchPath(w, newDir, makeSkipDirSet(nil), &count); err != nil {
		t.Fatalf("addWatchPath: %v", err)
	}
	if count != 1 {
		t.Fatalf("watch count = %d, want 1", count)
	}
}

func TestIsRelevantEvent_WorktreeGitDir(t *testing.T) {
	repoRoot := "/tmp/repo"
	gitDir := "/tmp/common/worktrees/feature"

	if !isRelevantEvent(fsnotify.Event{Name: filepath.Join(gitDir, "index")}, repoRoot, gitDir) {
		t.Fatal("index event in worktree git dir should be relevant")
	}
	if isRelevantEvent(fsnotify.Event{Name: filepath.Join(gitDir, "logs", "HEAD")}, repoRoot, gitDir) {
		t.Fatal("logs event in git dir should not be relevant")
	}
}
