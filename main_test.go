package main

import (
	"runtime/debug"
	"testing"
)

func TestCurrentVersion_UsesInjectedVersion(t *testing.T) {
	oldVersion := version
	oldRead := readBuildInfo
	t.Cleanup(func() {
		version = oldVersion
		readBuildInfo = oldRead
	})

	version = "v1.2.3"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		t.Fatal("readBuildInfo should not be called when version is injected")
		return nil, false
	}

	if got := currentVersion(); got != "v1.2.3" {
		t.Fatalf("currentVersion() = %q, want %q", got, "v1.2.3")
	}
}

func TestCurrentVersion_UsesBuildRevision(t *testing.T) {
	oldVersion := version
	oldRead := readBuildInfo
	t.Cleanup(func() {
		version = oldVersion
		readBuildInfo = oldRead
	})

	version = "dev"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "(devel)"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abcdef1234567890"},
			},
		}, true
	}

	if got := currentVersion(); got != "abcdef1" {
		t.Fatalf("currentVersion() = %q, want %q", got, "abcdef1")
	}
}

func TestCurrentVersion_UsesBuildModuleVersion(t *testing.T) {
	oldVersion := version
	oldRead := readBuildInfo
	t.Cleanup(func() {
		version = oldVersion
		readBuildInfo = oldRead
	})

	version = "dev"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "v1.2.3"},
		}, true
	}

	if got := currentVersion(); got != "v1.2.3" {
		t.Fatalf("currentVersion() = %q, want %q", got, "v1.2.3")
	}
}

func TestCurrentVersion_UsesDirtySuffix(t *testing.T) {
	oldVersion := version
	oldRead := readBuildInfo
	oldRunGit := runGit
	t.Cleanup(func() {
		version = oldVersion
		readBuildInfo = oldRead
		runGit = oldRunGit
	})

	version = "dev"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "(devel)"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abcdef1234567890"},
				{Key: "vcs.modified", Value: "true"},
			},
		}, true
	}

	if got := currentVersion(); got != "abcdef1-dirty" {
		t.Fatalf("currentVersion() = %q, want %q", got, "abcdef1-dirty")
	}
}

func TestCurrentVersion_FallsBackToGit(t *testing.T) {
	oldVersion := version
	oldRead := readBuildInfo
	oldRunGit := runGit
	t.Cleanup(func() {
		version = oldVersion
		readBuildInfo = oldRead
		runGit = oldRunGit
	})

	version = "dev"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true
	}
	runGit = func(args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "rev-parse" {
			return "abc1234", nil
		}
		return "", nil
	}

	if got := currentVersion(); got != "abc1234" {
		t.Fatalf("currentVersion() = %q, want %q", got, "abc1234")
	}
}

func TestCurrentVersion_FallsBackToGitWhenBuildInfoMissing(t *testing.T) {
	oldVersion := version
	oldRead := readBuildInfo
	oldRunGit := runGit
	t.Cleanup(func() {
		version = oldVersion
		readBuildInfo = oldRead
		runGit = oldRunGit
	})

	version = "dev"
	readBuildInfo = func() (*debug.BuildInfo, bool) { return nil, false }
	runGit = func(args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "rev-parse" {
			return "abc1234", nil
		}
		return "", nil
	}

	if got := currentVersion(); got != "abc1234" {
		t.Fatalf("currentVersion() = %q, want %q", got, "abc1234")
	}
}
