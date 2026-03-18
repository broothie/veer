package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/broothie/cob"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"
)

// Cached git repository to avoid re-opening on every fetch.
var (
	cachedRepoMu sync.Mutex
	cachedRepo   *git.Repository
	cachedWT     *git.Worktree
)

// gitRepo implements Repo using go-git.
type gitRepo struct {
	repo     *git.Repository
	wt       *git.Worktree
	indexMap map[string]plumbing.Hash // lazily built from index entries
	headTree *object.Tree             // lazily resolved HEAD commit tree
}

// openRepo returns a gitRepo using a cached *git.Repository to avoid
// re-opening the repo on every fetch cycle. Per-fetch caches (headTree,
// indexMap) are fresh each time.
func openRepo() (*gitRepo, error) {
	cachedRepoMu.Lock()
	defer cachedRepoMu.Unlock()

	if cachedRepo == nil {
		debugf("openRepo: opening repository (first time)")
		repo, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{
			DetectDotGit:          true,
			EnableDotGitCommonDir: true,
		})
		if err != nil {
			return nil, fmt.Errorf("not a git repository: %w", err)
		}

		wt, err := repo.Worktree()
		if err != nil {
			return nil, err
		}
		debugf("openRepo: root=%s", wt.Filesystem.Root())

		// Load global and system gitignore patterns — go-git doesn't do this automatically.
		rootFS := osfs.New("/")
		if global, err := gitignore.LoadGlobalPatterns(rootFS); err == nil {
			wt.Excludes = append(wt.Excludes, global...)
		}
		if system, err := gitignore.LoadSystemPatterns(rootFS); err == nil {
			wt.Excludes = append(wt.Excludes, system...)
		}

		cachedRepo = repo
		cachedWT = wt
	} else {
		debugf("openRepo: reusing cached repository")
	}

	return &gitRepo{repo: cachedRepo, wt: cachedWT}, nil
}

func (g *gitRepo) Head() (HeadInfo, error) {
	ref, err := g.repo.Head()
	if err != nil {
		debugf("Head: failed: %v", err)
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
	} else {
		debugf("Head: commit object failed: %v", err)
	}

	debugf("Head: branch=%s sha=%s", info.Branch, info.SHA)
	return info, nil
}

func (g *gitRepo) Status() (map[string]FileChange, error) {
	debugf("Status: computing via git status --porcelain")
	root := g.wt.Filesystem.Root()
	stdout, _, _, err := cob.Output(context.Background(), "git",
		cob.SetDir(root),
		cob.AddArgs("status", "--porcelain"),
	)
	if err != nil {
		debugf("Status: failed: %v", err)
		return nil, err
	}

	changes := make(map[string]FileChange)
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 4 {
			continue
		}
		x, y := line[0], line[1] // x=staging, y=worktree
		path := line[3:]

		// Porcelain v1 renames: "R  old -> new" — use the destination path.
		if x == 'R' || x == 'C' {
			if idx := strings.LastIndex(path, " -> "); idx >= 0 {
				path = path[idx+4:]
			}
		}

		// Skip ignored files.
		if x == '!' && y == '!' {
			continue
		}

		fc := FileChange{
			Staged:          x != ' ' && x != '?',
			Unstaged:        y != ' ' || (x == '?' && y == '?'), // untracked = both '?'
			StagingDeleted:  x == 'D',
			WorktreeDeleted: y == 'D',
		}
		changes[path] = fc
	}
	debugf("Status: %d changed files", len(changes))
	return changes, nil
}

func (g *gitRepo) ensureHeadTree() (*object.Tree, error) {
	if g.headTree != nil {
		return g.headTree, nil
	}
	ref, err := g.repo.Head()
	if err != nil {
		return nil, err
	}
	commit, err := g.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	g.headTree = tree
	return tree, nil
}

func (g *gitRepo) ensureIndexMap() error {
	if g.indexMap != nil {
		return nil
	}
	idx, err := g.repo.Storer.Index()
	if err != nil {
		return err
	}
	g.indexMap = make(map[string]plumbing.Hash, len(idx.Entries))
	for _, entry := range idx.Entries {
		g.indexMap[entry.Name] = entry.Hash
	}
	return nil
}

func (g *gitRepo) HeadContent(path string) string {
	tree, err := g.ensureHeadTree()
	if err != nil {
		debugf("HeadContent(%s): ensureHeadTree failed: %v", path, err)
		return ""
	}
	f, err := tree.File(path)
	if err != nil {
		debugf("HeadContent(%s): File failed: %v", path, err)
		return ""
	}
	content, err := f.Contents()
	if err != nil {
		debugf("HeadContent(%s): Contents failed: %v", path, err)
		return ""
	}
	return content
}

func (g *gitRepo) IndexContent(path string) string {
	if err := g.ensureIndexMap(); err != nil {
		debugf("IndexContent(%s): ensureIndexMap failed: %v", path, err)
		return ""
	}
	hash, ok := g.indexMap[path]
	if !ok {
		debugf("IndexContent(%s): not found in index", path)
		return ""
	}
	blob, err := g.repo.BlobObject(hash)
	if err != nil {
		debugf("IndexContent(%s): BlobObject failed: %v", path, err)
		return ""
	}
	r, err := blob.Reader()
	if err != nil {
		debugf("IndexContent(%s): Reader failed: %v", path, err)
		return ""
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		debugf("IndexContent(%s): ReadAll failed: %v", path, err)
		return ""
	}
	return string(b)
}

func (g *gitRepo) WorktreeContent(path string) string {
	f, err := g.wt.Filesystem.Open(path)
	if err != nil {
		debugf("WorktreeContent(%s): Open failed: %v", path, err)
		return ""
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		debugf("WorktreeContent(%s): ReadAll failed: %v", path, err)
		return ""
	}
	return string(b)
}

func (g *gitRepo) Log(n int) ([]CommitInfo, error) {
	debugf("Log: fetching up to %d commits", n)
	iter, err := g.repo.Log(&git.LogOptions{})
	if err != nil {
		debugf("Log: failed: %v", err)
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
	debugf("Log: got %d commits", len(commits))
	return commits, nil
}

func (g *gitRepo) DiffCommit(sha string) ([]FileDiff, error) {
	debugf("DiffCommit: %s", sha)
	hash := plumbing.NewHash(sha)
	commit, err := g.repo.CommitObject(hash)
	if err != nil {
		debugf("DiffCommit: CommitObject failed: %v", err)
		return nil, err
	}

	commitTree, err := commit.Tree()
	if err != nil {
		debugf("DiffCommit: Tree failed: %v", err)
		return nil, err
	}

	var parentTree *object.Tree
	if commit.NumParents() > 0 {
		if parent, err := commit.Parents().Next(); err == nil {
			if pt, err := parent.Tree(); err == nil {
				parentTree = pt
			} else {
				debugf("DiffCommit: parent Tree failed: %v", err)
			}
		} else {
			debugf("DiffCommit: parent Next failed: %v", err)
		}
	}

	changes, err := object.DiffTree(parentTree, commitTree)
	if err != nil {
		debugf("DiffCommit: DiffTree failed: %v", err)
		return nil, err
	}

	var files []FileDiff
	for _, change := range changes {
		action, err := change.Action()
		if err != nil {
			debugf("DiffCommit: Action failed: %v", err)
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
		if err != nil {
			debugf("DiffCommit: buildDiffLines(%s) failed: %v", path, err)
			continue
		}
		if len(lines) == 0 {
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
	debugf("DiffCommit: %d files", len(files))
	return files, nil
}

func (g *gitRepo) resolveRef(ref string) (plumbing.Hash, error) {
	// Try as a branch/tag name first.
	h, err := g.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		debugf("resolveRef(%s): failed: %v", ref, err)
		return plumbing.ZeroHash, fmt.Errorf("unknown ref %q: %w", ref, err)
	}
	return *h, nil
}

func (g *gitRepo) RefContent(ref, path string) string {
	hash, err := g.resolveRef(ref)
	if err != nil {
		debugf("RefContent(%s, %s): resolveRef failed: %v", ref, path, err)
		return ""
	}
	commit, err := g.repo.CommitObject(hash)
	if err != nil {
		debugf("RefContent(%s, %s): CommitObject failed: %v", ref, path, err)
		return ""
	}
	tree, err := commit.Tree()
	if err != nil {
		debugf("RefContent(%s, %s): Tree failed: %v", ref, path, err)
		return ""
	}
	return treeFileContent(tree, path)
}

func (g *gitRepo) DiffRefPaths(ref string) ([]string, error) {
	debugf("DiffRefPaths: %s", ref)
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
			debugf("DiffRefPaths: Action failed: %v", err)
			continue
		}
		switch action {
		case merkletrie.Insert, merkletrie.Modify:
			paths = append(paths, change.To.Name)
		case merkletrie.Delete:
			paths = append(paths, change.From.Name)
		}
	}
	debugf("DiffRefPaths: %d paths", len(paths))
	return paths, nil
}

// treeFileContent reads a file's content from a git tree, returning "" on any error.
func treeFileContent(tree *object.Tree, path string) string {
	if tree == nil {
		return ""
	}
	f, err := tree.File(path)
	if err != nil {
		debugf("treeFileContent(%s): File failed: %v", path, err)
		return ""
	}
	content, err := f.Contents()
	if err != nil {
		debugf("treeFileContent(%s): Contents failed: %v", path, err)
		return ""
	}
	return content
}
