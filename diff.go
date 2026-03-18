package main

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/pmezard/go-difflib/difflib"
)

// FileDiff holds the displayable diff lines for a single file.
type FileDiff struct {
	Path  string
	Lines []string
}

// fetchDiff opens the nearest git repository and returns unstaged diffs.
// args may contain path filters (same semantics as `git diff -- <paths>`).
func fetchDiff(args []string) ([]FileDiff, error) {
	repo, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, fmt.Errorf("not a git repository")
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, err
	}

	status, err := wt.Status()
	if err != nil {
		return nil, err
	}

	// Optional path filters (args after "--", or any non-flag arg).
	pathFilters := pathFiltersFrom(args)

	// Collect paths with worktree-level changes (unstaged), sorted for stable order.
	var paths []string
	for path, fs := range status {
		if fs.Worktree == git.Unmodified || fs.Worktree == git.Untracked {
			continue
		}
		if !matchesFilters(path, pathFilters) {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var files []FileDiff
	for _, path := range paths {
		fs := status[path]

		// Old content: indexed (staged) version, falling back to HEAD.
		old, err := indexedContent(repo, path)
		if err != nil {
			old = headContent(repo, path)
		}

		// New content: current file on disk (empty for deletions).
		var new string
		if fs.Worktree != git.Deleted {
			if data, err := readFromFS(wt, path); err == nil {
				new = string(data)
			}
		}

		lines, err := unifiedDiffLines(path, old, new)
		if err != nil || len(lines) == 0 {
			continue
		}

		files = append(files, FileDiff{Path: path, Lines: lines})
	}

	return files, nil
}

// indexedContent reads a file's content from the git index (staging area).
func indexedContent(repo *git.Repository, path string) (string, error) {
	idx, err := repo.Storer.Index()
	if err != nil {
		return "", err
	}
	for _, entry := range idx.Entries {
		if entry.Name != path {
			continue
		}
		blob, err := repo.BlobObject(entry.Hash)
		if err != nil {
			return "", err
		}
		r, err := blob.Reader()
		if err != nil {
			return "", err
		}
		defer r.Close()
		b, err := io.ReadAll(r)
		return string(b), err
	}
	return "", fmt.Errorf("not in index: %s", path)
}

// headContent reads a file's content from HEAD, returning "" on any error.
func headContent(repo *git.Repository, path string) string {
	ref, err := repo.Head()
	if err != nil {
		return ""
	}
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return ""
	}
	tree, err := commit.Tree()
	if err != nil {
		return ""
	}
	f, err := tree.File(path)
	if err != nil {
		return ""
	}
	content, _ := f.Contents()
	return content
}

// readFromFS reads a file from the worktree filesystem.
func readFromFS(wt *git.Worktree, path string) ([]byte, error) {
	f, err := wt.Filesystem.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// unifiedDiffLines produces diff lines in unified format.
func unifiedDiffLines(path, old, new string) ([]string, error) {
	ud := difflib.UnifiedDiff{
		A:        difflib.SplitLines(old),
		B:        difflib.SplitLines(new),
		FromFile: "a/" + path,
		ToFile:   "b/" + path,
		Context:  3,
	}
	text, err := difflib.GetUnifiedDiffString(ud)
	if err != nil || text == "" {
		return nil, err
	}
	return strings.Split(strings.TrimRight(text, "\n"), "\n"), nil
}

// pathFiltersFrom extracts path args (non-flags, post "--") from args.
func pathFiltersFrom(args []string) []string {
	var filters []string
	pastSep := false
	for _, arg := range args {
		if arg == "--" {
			pastSep = true
			continue
		}
		if pastSep || !strings.HasPrefix(arg, "-") {
			filters = append(filters, filepath.Clean(arg))
		}
	}
	return filters
}

// matchesFilters returns true if path is under any of the filter prefixes,
// or if no filters are specified.
func matchesFilters(path string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if path == f || strings.HasPrefix(path, f+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
