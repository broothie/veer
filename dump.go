package main

import (
	"os"

	"github.com/charmbracelet/bubbles/viewport"
)

func dumpView(cfg config) (string, error) {
	repo, err := openRepo()
	if err != nil {
		return "", err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	return buildDumpView(cfg, repo, cwd)
}

func buildDumpView(cfg config, repo Repo, cwd string) (string, error) {
	result, err := fetchDiff(repo, cfg)
	if err != nil {
		return "", err
	}

	commits, err := repo.Log(50)
	if err != nil {
		debugf("buildDumpView: Log failed: %v", err)
	}

	m := newModel(cfg)
	m.cwd = cwd
	m.width = max(1, cfg.DumpWidth)
	m.height = max(2, cfg.DumpHeight)
	m.viewport = viewport.New(m.vpWidth(), m.paneBodyHeight())
	m.branch = result.Branch
	m.sha = result.SHA
	m.message = result.Message
	m.files = result.Files
	m.tree = buildTree(m.files)
	m.commits = commits
	m.diffGen++
	m.rebuildDiffContent()

	return m.View(), nil
}
