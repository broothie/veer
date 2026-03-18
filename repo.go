package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"
)

// gitRepo implements Repo using go-git.
type gitRepo struct {
	repo *git.Repository
	wt   *git.Worktree
}

// openRepo opens the nearest git repository from the current directory.
func openRepo() (*gitRepo, error) {
	repo, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, fmt.Errorf("not a git repository")
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, err
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

	return &gitRepo{repo: repo, wt: wt}, nil
}

func (g *gitRepo) Head() (HeadInfo, error) {
	ref, err := g.repo.Head()
	if err != nil {
		return HeadInfo{}, err
	}

	info := HeadInfo{
		Branch: ref.Name().Short(),
	}

	hash := ref.Hash().String()
	if len(hash) > 7 {
		hash = hash[:7]
	}
	info.SHA = hash

	if commit, err := g.repo.CommitObject(ref.Hash()); err == nil {
		msg := strings.TrimSpace(commit.Message)
		if idx := strings.IndexByte(msg, '\n'); idx != -1 {
			msg = msg[:idx]
		}
		info.Message = msg
	}

	return info, nil
}

func (g *gitRepo) Status() (map[string]FileChange, error) {
	status, err := g.wt.Status()
	if err != nil {
		return nil, err
	}

	changes := make(map[string]FileChange)
	for path, fs := range status {
		if fs.Worktree == git.Unmodified && fs.Staging == git.Unmodified {
			continue
		}

		fc := FileChange{
			Staged:          fs.Staging != git.Unmodified && fs.Staging != git.Untracked,
			Unstaged:        fs.Worktree != git.Unmodified,
			StagingDeleted:  fs.Staging == git.Deleted,
			WorktreeDeleted: fs.Worktree == git.Deleted,
		}

		changes[path] = fc
	}
	return changes, nil
}

func (g *gitRepo) HeadContent(path string) string {
	ref, err := g.repo.Head()
	if err != nil {
		return ""
	}
	commit, err := g.repo.CommitObject(ref.Hash())
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

func (g *gitRepo) IndexContent(path string) string {
	idx, err := g.repo.Storer.Index()
	if err != nil {
		return ""
	}
	for _, entry := range idx.Entries {
		if entry.Name != path {
			continue
		}
		blob, err := g.repo.BlobObject(entry.Hash)
		if err != nil {
			return ""
		}
		r, err := blob.Reader()
		if err != nil {
			return ""
		}
		defer r.Close()
		b, err := io.ReadAll(r)
		if err != nil {
			return ""
		}
		return string(b)
	}
	return ""
}

func (g *gitRepo) WorktreeContent(path string) string {
	f, err := g.wt.Filesystem.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	return string(b)
}

func (g *gitRepo) Log(n int) ([]CommitInfo, error) {
	iter, err := g.repo.Log(&git.LogOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var commits []CommitInfo
	for i := 0; i < n; i++ {
		c, err := iter.Next()
		if err != nil {
			break
		}

		sha := c.Hash.String()
		short := sha
		if len(short) > 7 {
			short = short[:7]
		}

		msg := strings.TrimSpace(c.Message)
		if idx := strings.IndexByte(msg, '\n'); idx != -1 {
			msg = msg[:idx]
		}

		commits = append(commits, CommitInfo{
			SHA:     short,
			FullSHA: sha,
			Message: msg,
		})
	}
	return commits, nil
}

func (g *gitRepo) DiffCommit(sha string) ([]FileDiff, error) {
	hash := plumbing.NewHash(sha)
	commit, err := g.repo.CommitObject(hash)
	if err != nil {
		return nil, err
	}

	commitTree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	var parentTree *object.Tree
	if commit.NumParents() > 0 {
		if parent, err := commit.Parents().Next(); err == nil {
			parentTree, _ = parent.Tree()
		}
	}

	changes, err := object.DiffTree(parentTree, commitTree)
	if err != nil {
		return nil, err
	}

	var files []FileDiff
	for _, change := range changes {
		action, err := change.Action()
		if err != nil {
			continue
		}

		var path, oldContent, newContent string
		switch action {
		case merkletrie.Insert:
			path = change.To.Name
			newContent = treeFileContent(commitTree, path)
		case merkletrie.Delete:
			path = change.From.Name
			oldContent = treeFileContent(parentTree, path)
		case merkletrie.Modify:
			path = change.To.Name
			oldContent = treeFileContent(parentTree, path)
			newContent = treeFileContent(commitTree, path)
		}

		lines, _, added, removed, err := buildDiffLines(path, oldContent, newContent, 3)
		if err != nil || len(lines) == 0 {
			continue
		}

		files = append(files, FileDiff{
			Path:    path,
			Lines:   lines,
			Added:   added,
			Removed: removed,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func (g *gitRepo) resolveRef(ref string) (plumbing.Hash, error) {
	// Try as a branch/tag name first.
	h, err := g.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("unknown ref %q: %w", ref, err)
	}
	return *h, nil
}

func (g *gitRepo) RefContent(ref, path string) string {
	hash, err := g.resolveRef(ref)
	if err != nil {
		return ""
	}
	commit, err := g.repo.CommitObject(hash)
	if err != nil {
		return ""
	}
	tree, err := commit.Tree()
	if err != nil {
		return ""
	}
	return treeFileContent(tree, path)
}

func (g *gitRepo) DiffRefPaths(ref string) ([]string, error) {
	hash, err := g.resolveRef(ref)
	if err != nil {
		return nil, err
	}
	refCommit, err := g.repo.CommitObject(hash)
	if err != nil {
		return nil, err
	}
	refTree, err := refCommit.Tree()
	if err != nil {
		return nil, err
	}

	headRef, err := g.repo.Head()
	if err != nil {
		return nil, err
	}
	headCommit, err := g.repo.CommitObject(headRef.Hash())
	if err != nil {
		return nil, err
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return nil, err
	}

	changes, err := object.DiffTree(refTree, headTree)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, change := range changes {
		action, err := change.Action()
		if err != nil {
			continue
		}
		switch action {
		case merkletrie.Insert, merkletrie.Modify:
			paths = append(paths, change.To.Name)
		case merkletrie.Delete:
			paths = append(paths, change.From.Name)
		}
	}
	return paths, nil
}

// treeFileContent reads a file's content from a git tree, returning "" on any error.
func treeFileContent(tree *object.Tree, path string) string {
	if tree == nil {
		return ""
	}
	f, err := tree.File(path)
	if err != nil {
		return ""
	}
	content, _ := f.Contents()
	return content
}
