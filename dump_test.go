package main

import (
	"strings"
	"testing"
)

func TestBuildDumpView_RendersFrame(t *testing.T) {
	repo := &fakeRepo{
		head: HeadInfo{Branch: "main", SHA: "abc1234", Message: "init"},
		files: map[string]FileChange{
			"hello.txt": {Unstaged: true},
		},
		worktree: map[string]string{
			"hello.txt": "hello\nworld\n",
		},
	}

	view, err := buildDumpView(config{
		Context:      3,
		SidebarWidth: defaultSidebarWidth,
		DumpWidth:    100,
		DumpHeight:   30,
	}, repo, "~/proj")
	if err != nil {
		t.Fatalf("buildDumpView: %v", err)
	}

	if !strings.Contains(view, "~/proj") || !strings.Contains(view, "main") {
		t.Fatal("dump view should include header metadata")
	}
	if !strings.Contains(view, "hello.txt") {
		t.Fatal("dump view should include file entries")
	}
	if got := len(strings.Split(view, "\n")); got != 30 {
		t.Fatalf("dump view line count = %d, want 30", got)
	}
}
