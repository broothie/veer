package main

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/pmezard/go-difflib/difflib"
)

// LineType classifies a line in a diff.
type LineType int

const (
	LineContext   LineType = iota
	LineAdded
	LineRemoved
	LineSeparator
)

// DiffLine is a single rendered line in a diff, carrying line numbers.
type DiffLine struct {
	Type    LineType
	OldNum  int
	NewNum  int
	Content string
}

// DiffResult holds everything needed to render the UI.
type DiffResult struct {
	Branch  string
	SHA     string // short (7-char) hash
	Message string // first line of commit message
	Files   []FileDiff
}

// FileDiff holds the structured diff for a single file.
type FileDiff struct {
	Path    string
	Lines   []DiffLine
	Added   int
	Removed int
}

// fetchDiff opens the nearest git repository and returns unstaged diffs
// along with HEAD metadata.
func fetchDiff(args []string) (*DiffResult, error) {
	repo, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, fmt.Errorf("not a git repository")
	}

	result := &DiffResult{}

	// Populate branch / commit info from HEAD.
	if ref, err := repo.Head(); err == nil {
		result.Branch = ref.Name().Short()
		hash := ref.Hash().String()
		if len(hash) > 7 {
			hash = hash[:7]
		}
		result.SHA = hash

		if commit, err := repo.CommitObject(ref.Hash()); err == nil {
			msg := strings.TrimSpace(commit.Message)
			if idx := strings.IndexByte(msg, '\n'); idx != -1 {
				msg = msg[:idx]
			}
			result.Message = msg
		}
	}

	wt, err := repo.Worktree()
	if err != nil {
		return result, err
	}

	// Load global and system gitignore patterns — go-git doesn't do this automatically.
	// These functions expect a filesystem rooted at /, not the worktree.
	rootFS := osfs.New("/")
	if global, err := gitignore.LoadGlobalPatterns(rootFS); err == nil {
		wt.Excludes = append(wt.Excludes, global...)
	}
	if system, err := gitignore.LoadSystemPatterns(rootFS); err == nil {
		wt.Excludes = append(wt.Excludes, system...)
	}

	status, err := wt.Status()
	if err != nil {
		return result, err
	}

	pathFilters := pathFiltersFrom(args)

	var paths []string
	for path, fs := range status {
		if fs.Worktree == git.Unmodified && fs.Staging == git.Unmodified {
			continue
		}
		if !matchesFilters(path, pathFilters) {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		fs := status[path]

		old, err := indexedContent(repo, path)
		if err != nil {
			old = headContent(repo, path)
		}

		var new string
		if fs.Worktree != git.Deleted {
			if data, err := readFromFS(wt, path); err == nil {
				new = string(data)
			}
		}

		lines, added, removed, err := buildDiffLines(path, old, new)
		if err != nil || len(lines) == 0 {
			continue
		}

		result.Files = append(result.Files, FileDiff{
			Path:    path,
			Lines:   lines,
			Added:   added,
			Removed: removed,
		})
	}

	return result, nil
}

// buildDiffLines computes a unified diff and parses it into structured DiffLines.
func buildDiffLines(path, old, new string) ([]DiffLine, int, int, error) {
	ud := difflib.UnifiedDiff{
		A:        difflib.SplitLines(old),
		B:        difflib.SplitLines(new),
		FromFile: "a/" + path,
		ToFile:   "b/" + path,
		Context:  3,
	}
	text, err := difflib.GetUnifiedDiffString(ud)
	if err != nil || text == "" {
		return nil, 0, 0, err
	}
	rawLines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	return parseUnifiedDiff(rawLines)
}

// parseUnifiedDiff converts raw unified diff lines into structured DiffLines
// with line numbers and type annotations.
func parseUnifiedDiff(rawLines []string) ([]DiffLine, int, int, error) {
	var lines []DiffLine
	var oldNum, newNum int
	added, removed := 0, 0
	firstHunk := true

	for _, raw := range rawLines {
		switch {
		case strings.HasPrefix(raw, "---"), strings.HasPrefix(raw, "+++"):
			// skip file headers

		case strings.HasPrefix(raw, "@@"):
			if !firstHunk {
				lines = append(lines, DiffLine{Type: LineSeparator})
			}
			firstHunk = false
			var os, oc, ns, nc int
			fmt.Sscanf(raw, "@@ -%d,%d +%d,%d @@", &os, &oc, &ns, &nc)
			oldNum = os
			newNum = ns

		case strings.HasPrefix(raw, "+"):
			lines = append(lines, DiffLine{
				Type:    LineAdded,
				NewNum:  newNum,
				Content: raw[1:],
			})
			newNum++
			added++

		case strings.HasPrefix(raw, "-"):
			lines = append(lines, DiffLine{
				Type:    LineRemoved,
				OldNum:  oldNum,
				Content: raw[1:],
			})
			oldNum++
			removed++

		default:
			content := raw
			if len(raw) > 0 && raw[0] == ' ' {
				content = raw[1:]
			}
			lines = append(lines, DiffLine{
				Type:    LineContext,
				OldNum:  oldNum,
				NewNum:  newNum,
				Content: content,
			})
			oldNum++
			newNum++
		}
	}

	return lines, added, removed, nil
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
