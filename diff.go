package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// LineType classifies a line in a diff.
type LineType int

const (
	LineContext   LineType = iota
	LineAdded
	LineRemoved
	LineSeparator
	LineHeader // section header (e.g., "staged", "unstaged")
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
	Path     string
	Lines    []DiffLine
	Added    int
	Removed  int
	Staged   bool
	Unstaged bool
}

// HeadInfo holds metadata about the current HEAD.
type HeadInfo struct {
	Branch  string
	SHA     string
	Message string
}

// FileChange describes the state of a changed file.
type FileChange struct {
	Staged          bool
	Unstaged        bool
	StagingDeleted  bool
	WorktreeDeleted bool
}

// Repo abstracts git repository operations needed for diffing.
type Repo interface {
	Head() (HeadInfo, error)
	Status() (map[string]FileChange, error)
	HeadContent(path string) string
	IndexContent(path string) string
	WorktreeContent(path string) string
}

// fetchDiff queries the repo and returns diffs along with HEAD metadata.
func fetchDiff(repo Repo, args []string) (*DiffResult, error) {
	result := &DiffResult{}

	if head, err := repo.Head(); err == nil {
		result.Branch = head.Branch
		result.SHA = head.SHA
		result.Message = head.Message
	}

	status, err := repo.Status()
	if err != nil {
		return result, err
	}

	pathFilters := pathFiltersFrom(args)

	var paths []string
	for path := range status {
		if !matchesFilters(path, pathFilters) {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		fc := status[path]
		fd := FileDiff{
			Path:     path,
			Staged:   fc.Staged,
			Unstaged: fc.Unstaged,
		}

		headContent := repo.HeadContent(path)
		indexContent := repo.IndexContent(path)

		var worktreeContent string
		if !fc.WorktreeDeleted {
			worktreeContent = repo.WorktreeContent(path)
		}

		hasBoth := fc.Staged && fc.Unstaged

		// Staged diff: HEAD vs index.
		if fc.Staged {
			var stagedOld, stagedNew string
			stagedOld = headContent
			if !fc.StagingDeleted {
				stagedNew = indexContent
			}

			lines, added, removed, err := buildDiffLines(path, stagedOld, stagedNew)
			if err == nil && len(lines) > 0 {
				if hasBoth {
					fd.Lines = append(fd.Lines, DiffLine{Type: LineHeader, Content: "staged"})
				}
				fd.Lines = append(fd.Lines, lines...)
				fd.Added += added
				fd.Removed += removed
			}
		}

		// Unstaged diff: index vs worktree.
		if fc.Unstaged {
			var unstagedOld string
			if indexContent != "" {
				unstagedOld = indexContent
			} else {
				unstagedOld = headContent
			}

			lines, added, removed, err := buildDiffLines(path, unstagedOld, worktreeContent)
			if err == nil && len(lines) > 0 {
				if hasBoth {
					fd.Lines = append(fd.Lines, DiffLine{Type: LineHeader, Content: "unstaged"})
				}
				fd.Lines = append(fd.Lines, lines...)
				fd.Added += added
				fd.Removed += removed
			}
		}

		if len(fd.Lines) == 0 {
			continue
		}

		result.Files = append(result.Files, fd)
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
