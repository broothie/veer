package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitRepoStatus_Unmodified(t *testing.T) {
	dir := initTestRepo(t)
	repo, err := openRepoAt(dir)
	if err != nil {
		t.Fatalf("openRepo: %v", err)
	}

	changes, err := repo.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %d: %v", len(changes), changes)
	}
}

func TestGitRepoStatus_Unstaged(t *testing.T) {
	dir := initTestRepo(t)
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("modified\n"), 0644)

	repo, err := openRepoAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := repo.Status()
	if err != nil {
		t.Fatal(err)
	}

	fc, ok := changes["hello.txt"]
	if !ok {
		t.Fatal("expected hello.txt in changes")
	}
	if fc.Staged {
		t.Error("expected Staged=false")
	}
	if !fc.Unstaged {
		t.Error("expected Unstaged=true")
	}
}

func TestGitRepoStatus_Staged(t *testing.T) {
	dir := initTestRepo(t)
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("modified\n"), 0644)
	stageFileCmd(dir, "hello.txt")()

	repo, err := openRepoAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := repo.Status()
	if err != nil {
		t.Fatal(err)
	}

	fc, ok := changes["hello.txt"]
	if !ok {
		t.Fatal("expected hello.txt in changes")
	}
	if !fc.Staged {
		t.Error("expected Staged=true")
	}
	if fc.Unstaged {
		t.Error("expected Unstaged=false")
	}
}

func TestGitRepoStatus_StagedAndUnstaged(t *testing.T) {
	dir := initTestRepo(t)

	// Stage one change.
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("first\n"), 0644)
	stageFileCmd(dir, "hello.txt")()

	// Then make another change without staging.
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("second\n"), 0644)

	repo, err := openRepoAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := repo.Status()
	if err != nil {
		t.Fatal(err)
	}

	fc, ok := changes["hello.txt"]
	if !ok {
		t.Fatal("expected hello.txt in changes")
	}
	if !fc.Staged {
		t.Error("expected Staged=true")
	}
	if !fc.Unstaged {
		t.Error("expected Unstaged=true")
	}
}

func TestGitRepoStatus_Untracked(t *testing.T) {
	dir := initTestRepo(t)
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0644)

	repo, err := openRepoAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := repo.Status()
	if err != nil {
		t.Fatal(err)
	}

	fc, ok := changes["new.txt"]
	if !ok {
		t.Fatal("expected new.txt in changes")
	}
	if fc.Staged {
		t.Error("expected Staged=false for untracked")
	}
	if !fc.Unstaged {
		t.Error("expected Unstaged=true for untracked")
	}
}

func TestGitRepoStatus_IgnoresGitignored(t *testing.T) {
	dir := initTestRepo(t)

	// Write a .gitignore and an ignored file.
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.txt\n"), 0644)
	os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("ignored\n"), 0644)
	stageFileCmd(dir, ".gitignore")()

	repo, err := openRepoAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := repo.Status()
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := changes["ignored.txt"]; ok {
		t.Error("expected ignored.txt to not appear in changes")
	}
}

// openRepoAt opens a gitRepo rooted at dir, for testing.
func openRepoAt(dir string) (*gitRepo, error) {
	orig, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(dir); err != nil {
		return nil, err
	}
	defer os.Chdir(orig)
	return openRepo()
}
