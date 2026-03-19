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

func TestGitRepoStatus_UntrackedNestedFile(t *testing.T) {
	dir := initTestRepo(t)
	nestedDir := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "install.sh"), []byte("#!/bin/sh\n"), 0644); err != nil {
		t.Fatal(err)
	}

	repo, err := openRepoAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := repo.Status()
	if err != nil {
		t.Fatal(err)
	}

	fc, ok := changes["scripts/install.sh"]
	if !ok {
		t.Fatalf("expected scripts/install.sh in changes, got %v", changes)
	}
	if fc.Staged {
		t.Error("expected Staged=false for untracked nested file")
	}
	if !fc.Unstaged {
		t.Error("expected Unstaged=true for untracked nested file")
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

func TestParseStatusPorcelainZ_PreservesLiteralPath(t *testing.T) {
	changes, err := parseStatusPorcelainZ([]byte("?? dir with spaces/file -> name.txt\x00"))
	if err != nil {
		t.Fatalf("parseStatusPorcelainZ: %v", err)
	}

	fc, ok := changes["dir with spaces/file -> name.txt"]
	if !ok {
		t.Fatalf("expected literal path key, got %v", changes)
	}
	if fc.Staged {
		t.Fatal("untracked file should not be staged")
	}
	if !fc.Unstaged {
		t.Fatal("untracked file should be unstaged")
	}
}

func TestParseStatusPorcelainZ_RenameUsesDestinationPath(t *testing.T) {
	changes, err := parseStatusPorcelainZ([]byte("R  new name.txt\x00old -> name.txt\x00"))
	if err != nil {
		t.Fatalf("parseStatusPorcelainZ: %v", err)
	}

	if _, ok := changes["new name.txt"]; !ok {
		t.Fatalf("expected destination path key, got %v", changes)
	}
	if _, ok := changes["old -> name.txt"]; ok {
		t.Fatalf("old rename path should not be rendered, got %v", changes)
	}
}

func TestOpenRepo_ReopensWhenWorkingDirectoryChanges(t *testing.T) {
	dir1 := initTestRepo(t)
	dir2 := initTestRepo(t)

	repo1, err := openRepoAt(dir1)
	if err != nil {
		t.Fatalf("openRepoAt(dir1): %v", err)
	}
	repo2, err := openRepoAt(dir2)
	if err != nil {
		t.Fatalf("openRepoAt(dir2): %v", err)
	}

	assertSamePath(t, repo1.wt.Filesystem.Root(), dir1)
	assertSamePath(t, repo2.wt.Filesystem.Root(), dir2)
}

func assertSamePath(t *testing.T, got, want string) {
	t.Helper()

	gotEval, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", got, err)
	}
	wantEval, err := filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", want, err)
	}
	if gotEval != wantEval {
		t.Fatalf("path = %q, want %q", gotEval, wantEval)
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
