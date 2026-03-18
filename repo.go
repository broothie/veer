package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
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
