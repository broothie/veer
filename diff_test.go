package main

import (
	"testing"
)

// fakeRepo implements Repo for testing.
type fakeRepo struct {
	head     HeadInfo
	files    map[string]FileChange
	headc    map[string]string
	index    map[string]string
	worktree map[string]string
}

func (f *fakeRepo) Head() (HeadInfo, error)                { return f.head, nil }
func (f *fakeRepo) Status() (map[string]FileChange, error) { return f.files, nil }

func (f *fakeRepo) HeadContent(path string) string {
	if f.headc != nil {
		return f.headc[path]
	}
	return ""
}

func (f *fakeRepo) IndexContent(path string) string {
	if f.index != nil {
		return f.index[path]
	}
	return ""
}

func (f *fakeRepo) WorktreeContent(path string) string {
	if f.worktree != nil {
		return f.worktree[path]
	}
	return ""
}

func (f *fakeRepo) RefContent(ref, path string) string        { return "" }
func (f *fakeRepo) DiffRefPaths(ref string) ([]string, error) { return nil, nil }
func (f *fakeRepo) Log(n int) ([]CommitInfo, error)           { return nil, nil }
func (f *fakeRepo) DiffCommit(sha string) ([]FileDiff, error) { return nil, nil }

func TestFetchDiff_HeadInfo(t *testing.T) {
	repo := &fakeRepo{
		head:  HeadInfo{Branch: "main", SHA: "abc1234", Message: "init"},
		files: map[string]FileChange{},
	}

	result, err := fetchDiff(repo, config{Context: 3})
	if err != nil {
		t.Fatal(err)
	}

	if result.Branch != "main" {
		t.Errorf("branch = %q, want %q", result.Branch, "main")
	}
	if result.SHA != "abc1234" {
		t.Errorf("sha = %q, want %q", result.SHA, "abc1234")
	}
	if result.Message != "init" {
		t.Errorf("message = %q, want %q", result.Message, "init")
	}
}

func TestFetchDiff_UnstagedNewFile(t *testing.T) {
	repo := &fakeRepo{
		head:     HeadInfo{Branch: "main", SHA: "abc1234", Message: "init"},
		files:    map[string]FileChange{"hello.txt": {Unstaged: true}},
		worktree: map[string]string{"hello.txt": "hello\nworld\n"},
	}

	result, err := fetchDiff(repo, config{Context: 3})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(result.Files))
	}

	f := result.Files[0]
	if f.Path != "hello.txt" {
		t.Errorf("path = %q, want %q", f.Path, "hello.txt")
	}
	if f.Added != 2 {
		t.Errorf("added = %d, want 2", f.Added)
	}
	if f.Removed != 0 {
		t.Errorf("removed = %d, want 0", f.Removed)
	}
	if !f.Unstaged || f.Staged {
		t.Error("should be unstaged only")
	}
}

func TestFetchDiff_UnstagedModified(t *testing.T) {
	repo := &fakeRepo{
		head:     HeadInfo{Branch: "main", SHA: "abc1234", Message: "init"},
		files:    map[string]FileChange{"file.txt": {Unstaged: true}},
		index:    map[string]string{"file.txt": "line1\nline2\nline3\n"},
		worktree: map[string]string{"file.txt": "line1\nchanged\nline3\n"},
	}

	result, err := fetchDiff(repo, config{Context: 3})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(result.Files))
	}

	f := result.Files[0]
	if f.Added != 1 || f.Removed != 1 {
		t.Errorf("added=%d removed=%d, want added=1 removed=1", f.Added, f.Removed)
	}
}

func TestFetchDiff_StagedFile(t *testing.T) {
	repo := &fakeRepo{
		head:  HeadInfo{Branch: "main", SHA: "abc1234", Message: "init"},
		files: map[string]FileChange{"new.txt": {Staged: true}},
		index: map[string]string{"new.txt": "staged content\n"},
	}

	result, err := fetchDiff(repo, config{Context: 3})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(result.Files))
	}

	f := result.Files[0]
	if !f.Staged || f.Unstaged {
		t.Error("should be staged only")
	}
	if f.Added != 1 {
		t.Errorf("added = %d, want 1", f.Added)
	}
}

func TestFetchDiff_StagedAndUnstaged(t *testing.T) {
	repo := &fakeRepo{
		head:     HeadInfo{Branch: "main", SHA: "abc1234", Message: "init"},
		files:    map[string]FileChange{"file.txt": {Staged: true, Unstaged: true}},
		headc:    map[string]string{"file.txt": "original\n"},
		index:    map[string]string{"file.txt": "staged change\n"},
		worktree: map[string]string{"file.txt": "worktree change\n"},
	}

	result, err := fetchDiff(repo, config{Context: 3})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(result.Files))
	}

	f := result.Files[0]
	if !f.Staged || !f.Unstaged {
		t.Error("should be both staged and unstaged")
	}

	// Should have section headers when both staged and unstaged.
	headers := 0
	for _, l := range f.Lines {
		if l.Type == LineHeader {
			headers++
		}
	}
	if headers != 2 {
		t.Errorf("got %d section headers, want 2 (staged + unstaged)", headers)
	}

	// First header should be "staged", second "unstaged".
	headerIdx := 0
	for _, l := range f.Lines {
		if l.Type == LineHeader {
			if headerIdx == 0 && l.Content != "staged" {
				t.Errorf("first header = %q, want %q", l.Content, "staged")
			}
			if headerIdx == 1 && l.Content != "unstaged" {
				t.Errorf("second header = %q, want %q", l.Content, "unstaged")
			}
			headerIdx++
		}
	}
}

func TestFetchDiff_StagedOnly_NoHeaders(t *testing.T) {
	repo := &fakeRepo{
		head:  HeadInfo{Branch: "main", SHA: "abc1234", Message: "init"},
		files: map[string]FileChange{"file.txt": {Staged: true}},
		index: map[string]string{"file.txt": "new\n"},
	}

	result, err := fetchDiff(repo, config{Context: 3})
	if err != nil {
		t.Fatal(err)
	}

	f := result.Files[0]
	for _, l := range f.Lines {
		if l.Type == LineHeader {
			t.Error("should not have section headers when only staged")
		}
	}
}

func TestFetchDiff_DeletedFile(t *testing.T) {
	repo := &fakeRepo{
		head:     HeadInfo{Branch: "main", SHA: "abc1234", Message: "init"},
		files:    map[string]FileChange{"gone.txt": {Unstaged: true, WorktreeDeleted: true}},
		index:    map[string]string{"gone.txt": "bye\n"},
		worktree: map[string]string{"gone.txt": "should not be read"},
	}

	result, err := fetchDiff(repo, config{Context: 3})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(result.Files))
	}

	f := result.Files[0]
	if f.Added != 0 || f.Removed != 1 {
		t.Errorf("added=%d removed=%d, want added=0 removed=1", f.Added, f.Removed)
	}
}

func TestFetchDiff_PathFilter(t *testing.T) {
	repo := &fakeRepo{
		head: HeadInfo{Branch: "main", SHA: "abc1234", Message: "init"},
		files: map[string]FileChange{
			"src/a.go":  {Unstaged: true},
			"src/b.go":  {Unstaged: true},
			"docs/x.md": {Unstaged: true},
		},
		worktree: map[string]string{
			"src/a.go":  "package a\n",
			"src/b.go":  "package b\n",
			"docs/x.md": "# doc\n",
		},
	}

	result, err := fetchDiff(repo, config{Context: 3, Paths: []string{"src"}})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 2 {
		t.Fatalf("got %d files, want 2", len(result.Files))
	}
	for _, f := range result.Files {
		if f.Path != "src/a.go" && f.Path != "src/b.go" {
			t.Errorf("unexpected file %q", f.Path)
		}
	}
}

func TestFetchDiff_FilesSorted(t *testing.T) {
	repo := &fakeRepo{
		head: HeadInfo{},
		files: map[string]FileChange{
			"c.go": {Unstaged: true},
			"a.go": {Unstaged: true},
			"b.go": {Unstaged: true},
		},
		worktree: map[string]string{
			"c.go": "c\n",
			"a.go": "a\n",
			"b.go": "b\n",
		},
	}

	result, err := fetchDiff(repo, config{Context: 3})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 3 {
		t.Fatalf("got %d files, want 3", len(result.Files))
	}
	for i, want := range []string{"a.go", "b.go", "c.go"} {
		if result.Files[i].Path != want {
			t.Errorf("files[%d] = %q, want %q", i, result.Files[i].Path, want)
		}
	}
}

func TestFetchDiff_NoChanges(t *testing.T) {
	repo := &fakeRepo{
		head:  HeadInfo{Branch: "main", SHA: "abc1234", Message: "init"},
		files: map[string]FileChange{},
	}

	result, err := fetchDiff(repo, config{Context: 3})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 0 {
		t.Errorf("got %d files, want 0", len(result.Files))
	}
}

func TestParseUnifiedDiff(t *testing.T) {
	raw := []string{
		"--- a/file.txt",
		"+++ b/file.txt",
		"@@ -1,3 +1,3 @@",
		" line1",
		"-old",
		"+new",
		" line3",
	}

	lines, added, removed, err := parseUnifiedDiff(raw)
	if err != nil {
		t.Fatal(err)
	}

	if added != 1 || removed != 1 {
		t.Errorf("added=%d removed=%d, want 1,1", added, removed)
	}

	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4", len(lines))
	}

	// context line
	if lines[0].Type != LineContext || lines[0].Content != "line1" {
		t.Errorf("line 0: type=%d content=%q", lines[0].Type, lines[0].Content)
	}
	if lines[0].OldNum != 1 || lines[0].NewNum != 1 {
		t.Errorf("line 0: old=%d new=%d, want 1,1", lines[0].OldNum, lines[0].NewNum)
	}

	// removed line
	if lines[1].Type != LineRemoved || lines[1].Content != "old" {
		t.Errorf("line 1: type=%d content=%q", lines[1].Type, lines[1].Content)
	}
	if lines[1].OldNum != 2 {
		t.Errorf("line 1: old=%d, want 2", lines[1].OldNum)
	}

	// added line
	if lines[2].Type != LineAdded || lines[2].Content != "new" {
		t.Errorf("line 2: type=%d content=%q", lines[2].Type, lines[2].Content)
	}
	if lines[2].NewNum != 2 {
		t.Errorf("line 2: new=%d, want 2", lines[2].NewNum)
	}

	// trailing context
	if lines[3].Type != LineContext || lines[3].Content != "line3" {
		t.Errorf("line 3: type=%d content=%q", lines[3].Type, lines[3].Content)
	}
}

func TestParseUnifiedDiff_MultipleHunks(t *testing.T) {
	raw := []string{
		"--- a/file.txt",
		"+++ b/file.txt",
		"@@ -1,2 +1,2 @@",
		" ctx",
		"+add1",
		"@@ -10,2 +10,2 @@",
		" ctx2",
		"+add2",
	}

	lines, added, _, err := parseUnifiedDiff(raw)
	if err != nil {
		t.Fatal(err)
	}

	if added != 2 {
		t.Errorf("added = %d, want 2", added)
	}

	// Should have a separator between hunks.
	hasSep := false
	for _, l := range lines {
		if l.Type == LineSeparator {
			hasSep = true
			break
		}
	}
	if !hasSep {
		t.Error("expected a separator between hunks")
	}
}

func TestBuildDiffLines(t *testing.T) {
	old := "aaa\nbbb\nccc\n"
	new := "aaa\nBBB\nccc\n"

	lines, added, removed, err := buildDiffLines("test.txt", old, new, 3)
	if err != nil {
		t.Fatal(err)
	}

	if added != 1 || removed != 1 {
		t.Errorf("added=%d removed=%d, want 1,1", added, removed)
	}

	if len(lines) == 0 {
		t.Fatal("expected lines")
	}
}

func TestBuildDiffLines_Identical(t *testing.T) {
	content := "same\n"
	lines, _, _, err := buildDiffLines("test.txt", content, content, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 0 {
		t.Errorf("got %d lines for identical content, want 0", len(lines))
	}
}

func TestMatchesFilters(t *testing.T) {
	tests := []struct {
		path    string
		filters []string
		want    bool
	}{
		{"src/main.go", nil, true},
		{"src/main.go", []string{"src"}, true},
		{"src/main.go", []string{"lib"}, false},
		{"src/main.go", []string{"src/main.go"}, true},
		{"src/main.go", []string{"lib", "src"}, true},
	}

	for _, tt := range tests {
		got := matchesFilters(tt.path, tt.filters)
		if got != tt.want {
			t.Errorf("matchesFilters(%q, %v) = %v, want %v", tt.path, tt.filters, got, tt.want)
		}
	}
}

func TestBuildTree(t *testing.T) {
	files := []FileDiff{
		{Path: "cmd/main.go"},
		{Path: "cmd/util.go"},
		{Path: "pkg/lib/a.go"},
		{Path: "root.go"},
	}

	tree := buildTree(files)

	// Expected: cmd/ main.go util.go pkg/ lib/ a.go root.go
	wantNames := []string{"cmd/", "main.go", "util.go", "pkg/", "lib/", "a.go", "root.go"}
	wantDepths := []int{0, 1, 1, 0, 1, 2, 0}
	wantFileIdx := []int{-1, 0, 1, -1, -1, 2, 3}

	if len(tree) != len(wantNames) {
		t.Fatalf("got %d entries, want %d", len(tree), len(wantNames))
	}

	for i, e := range tree {
		if e.name != wantNames[i] {
			t.Errorf("tree[%d].name = %q, want %q", i, e.name, wantNames[i])
		}
		if e.depth != wantDepths[i] {
			t.Errorf("tree[%d].depth = %d, want %d", i, e.depth, wantDepths[i])
		}
		if e.fileIdx != wantFileIdx[i] {
			t.Errorf("tree[%d].fileIdx = %d, want %d", i, e.fileIdx, wantFileIdx[i])
		}
	}
}

func TestBuildTree_FlatFiles(t *testing.T) {
	files := []FileDiff{
		{Path: "a.go"},
		{Path: "b.go"},
	}

	tree := buildTree(files)

	if len(tree) != 2 {
		t.Fatalf("got %d entries, want 2", len(tree))
	}
	if tree[0].depth != 0 || tree[1].depth != 0 {
		t.Error("flat files should have depth 0")
	}
	if tree[0].fileIdx != 0 || tree[1].fileIdx != 1 {
		t.Error("file indices should match input order")
	}
}
