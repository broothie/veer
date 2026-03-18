package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitDebug_Disabled(t *testing.T) {
	// Reset global state.
	debugLogger = nil
	defer func() { debugLogger = nil }()

	if err := initDebug(false); err != nil {
		t.Fatalf("initDebug(false): %v", err)
	}
	if debugLogger != nil {
		t.Fatal("expected debugLogger to be nil when disabled")
	}
}

func TestInitDebug_Enabled(t *testing.T) {
	// Use a temp dir as HOME so we don't write to the real ~/.veer.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	debugLogger = nil
	defer func() { debugLogger = nil }()

	if err := initDebug(true); err != nil {
		t.Fatalf("initDebug(true): %v", err)
	}
	if debugLogger == nil {
		t.Fatal("expected debugLogger to be set")
	}

	logPath := filepath.Join(tmp, ".veer", "debug.log")
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("expected log file at %s: %v", logPath, err)
	}

	// Verify the file has the startup message.
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(content), "debug logging started") {
		t.Fatalf("expected startup message in log, got: %s", content)
	}
}

func TestDebugf_Disabled(t *testing.T) {
	debugLogger = nil
	defer func() { debugLogger = nil }()

	// Should not panic when logger is nil.
	debugf("this should be a no-op: %d", 42)
}

func TestDebugf_Enabled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	debugLogger = nil
	defer func() { debugLogger = nil }()

	if err := initDebug(true); err != nil {
		t.Fatal(err)
	}

	debugf("test message: %s %d", "hello", 123)

	logPath := filepath.Join(tmp, ".veer", "debug.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "test message: hello 123") {
		t.Fatalf("expected test message in log, got: %s", content)
	}
}

func TestInitDebug_FilePermissions(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	debugLogger = nil
	defer func() { debugLogger = nil }()

	if err := initDebug(true); err != nil {
		t.Fatal(err)
	}

	dirPath := filepath.Join(tmp, ".veer")
	dirInfo, err := os.Stat(dirPath)
	if err != nil {
		t.Fatal(err)
	}
	dirPerm := dirInfo.Mode().Perm()
	if dirPerm&0o005 != 0 {
		t.Errorf("directory permissions too open: %o", dirPerm)
	}

	filePath := filepath.Join(dirPath, "debug.log")
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	filePerm := fileInfo.Mode().Perm()
	if filePerm&0o077 != 0 {
		t.Errorf("file permissions too open: %o", filePerm)
	}
}
