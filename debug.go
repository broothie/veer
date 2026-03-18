package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// debugLogger is a global logger that writes to a file when debug mode is enabled.
// When disabled, all calls are no-ops.
var debugLogger *log.Logger

func initDebug(enabled bool) error {
	if !enabled {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("debug: home dir: %w", err)
	}

	dir := filepath.Join(home, ".veer")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("debug: mkdir: %w", err)
	}

	path := filepath.Join(dir, "debug.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("debug: open log: %w", err)
	}

	debugLogger = log.New(f, "", log.Ltime|log.Lmicroseconds)
	debugLogger.Printf("debug logging started, writing to %s", path)
	return nil
}

// debugf logs a formatted message if debug mode is enabled.
func debugf(format string, args ...any) {
	if debugLogger != nil {
		debugLogger.Printf(format, args...)
	}
}
