package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPatch(t *testing.T) {
	hunk := Hunk{
		Header:   "@@ -1,3 +1,4 @@",
		RawLines: []string{"@@ -1,3 +1,4 @@", " line1", "+added", " line2", " line3"},
	}

	patch := buildPatch("foo.go", hunk)

	if !strings.HasPrefix(patch, "--- a/foo.go\n+++ b/foo.go\n") {
		t.Fatalf("patch missing file headers:\n%s", patch)
	}
	if !strings.Contains(patch, "@@ -1,3 +1,4 @@\n") {
		t.Fatalf("patch missing hunk header:\n%s", patch)
	}
	if !strings.Contains(patch, "+added\n") {
		t.Fatalf("patch missing added line:\n%s", patch)
	}
}

// initTestRepo creates a temp git repo with an initial commit containing one file.
// Returns the repo root path and a cleanup function.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "test")

	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "hello.txt")
	run("git", "commit", "-m", "initial")

	return dir
}

// gitStatus returns the short status output for a repo.
func gitStatus(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestStageFileCmd(t *testing.T) {
	dir := initTestRepo(t)

	// Modify file so it's unstaged.
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\nworld\n"), 0644)

	status := gitStatus(t, dir)
	if !strings.Contains(status, "M hello.txt") {
		t.Fatalf("expected unstaged modification, got: %s", status)
	}

	msg := stageFileCmd(dir, "hello.txt")()
	if r := msg.(stageResultMsg); r.err != nil {
		t.Fatalf("stageFileCmd: %v", r.err)
	}

	status = gitStatus(t, dir)
	if !strings.Contains(status, "M  hello.txt") {
		t.Fatalf("expected staged modification, got: %s", status)
	}
}

func TestUnstageFileCmd(t *testing.T) {
	dir := initTestRepo(t)

	// Stage a modification.
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\nworld\n"), 0644)
	stageFileCmd(dir, "hello.txt")()

	msg := unstageFileCmd(dir, "hello.txt")()
	if r := msg.(stageResultMsg); r.err != nil {
		t.Fatalf("unstageFileCmd: %v", r.err)
	}

	status := gitStatus(t, dir)
	if !strings.Contains(status, "M hello.txt") {
		t.Fatalf("expected unstaged modification after unstage, got: %s", status)
	}
}

func TestUnstageAllCmd(t *testing.T) {
	dir := initTestRepo(t)

	// Stage modifications to two files.
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("changed\n"), 0644)
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0644)
	exec.Command("git", "-C", dir, "add", "hello.txt", "new.txt").Run()

	msg := unstageAllCmd(dir)()
	if r := msg.(stageResultMsg); r.err != nil {
		t.Fatalf("unstageAllCmd: %v", r.err)
	}

	status := gitStatus(t, dir)
	if strings.Contains(status, "M  hello.txt") || strings.Contains(status, "A  new.txt") {
		t.Fatalf("expected nothing staged after unstageAll, got: %s", status)
	}
}

func TestStageAndUnstageHunkCmd(t *testing.T) {
	dir := initTestRepo(t)

	// Write a multi-line file and commit it.
	original := "line1\nline2\nline3\nline4\nline5\n"
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte(original), 0644)
	exec.Command("git", "-C", dir, "add", "hello.txt").Run()
	cmd := exec.Command("git", "-C", dir, "commit", "-m", "multi-line")
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
	cmd.Run()

	// Modify and build a diff to get a real hunk.
	modified := "line1\nline2\ninserted\nline3\nline4\nline5\n"
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte(modified), 0644)

	lines, hunks, _, _, err := buildDiffLines("hello.txt", original, modified, 3)
	if err != nil {
		t.Fatalf("buildDiffLines: %v", err)
	}
	if len(hunks) == 0 {
		t.Fatal("expected at least one hunk")
	}
	if len(lines) == 0 {
		t.Fatal("expected diff lines")
	}

	// Stage the hunk.
	msg := stageHunkCmd(dir, "hello.txt", hunks[0])()
	if r := msg.(stageResultMsg); r.err != nil {
		t.Fatalf("stageHunkCmd: %v", r.err)
	}

	status := gitStatus(t, dir)
	if !strings.Contains(status, "M") {
		t.Fatalf("expected staged change after stageHunk, got: %s", status)
	}

	// Unstage the hunk.
	msg = unstageHunkCmd(dir, "hello.txt", hunks[0])()
	if r := msg.(stageResultMsg); r.err != nil {
		t.Fatalf("unstageHunkCmd: %v", r.err)
	}

	status = gitStatus(t, dir)
	// After unstaging, the index should match HEAD again, so only worktree modification remains.
	if !strings.Contains(status, "M hello.txt") {
		t.Fatalf("expected unstaged after unstageHunk, got: %s", status)
	}
}

func TestCommitStagedCmd(t *testing.T) {
	dir := initTestRepo(t)

	// Stage a change.
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("committed\n"), 0644)
	exec.Command("git", "-C", dir, "add", "hello.txt").Run()

	cmd := commitStagedCmd(dir, "test commit message")
	msg := cmd()
	if r := msg.(commitResultMsg); r.err != nil {
		t.Fatalf("commitStagedCmd: %v", r.err)
	}

	// Verify clean status and commit message.
	status := gitStatus(t, dir)
	if status != "" {
		t.Fatalf("expected clean working tree after commit, got: %s", status)
	}

	logCmd := exec.Command("git", "-C", dir, "log", "--oneline", "-1")
	out, _ := logCmd.Output()
	if !strings.Contains(string(out), "test commit message") {
		t.Fatalf("commit message not found in log: %s", out)
	}
}
