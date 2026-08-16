package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestTempDirSystemAvailable(t *testing.T) {
	resetTempDirState()
	defer resetTempDirState()
	dir, fallback, err := tempDir()
	if err != nil {
		t.Fatalf("tempDir: %v", err)
	}
	if dir == "" {
		t.Fatal("empty temp dir")
	}
	if fallback {
		t.Fatalf("expected system temp dir, got fallback %s", dir)
	}
	cleanupTempDir()
	if _, _, err := tempDir(); err != nil {
		t.Fatalf("tempDir after cleanup: %v", err)
	}
}

func TestTempDirFallbackAndCleanup(t *testing.T) {
	resetTempDirState()
	defer resetTempDirState()
	origProbe := probeSystemTemp
	origFallback := fallbackTempDir
	defer func() { probeSystemTemp = origProbe; fallbackTempDir = origFallback }()
	probeSystemTemp = func() (string, error) { return "", errors.New("system temp unavailable") }
	fallbackTempDir = filepath.Join(t.TempDir(), ".tmp")

	dir, fallback, err := tempDir()
	if err != nil {
		t.Fatalf("tempDir: %v", err)
	}
	if !fallback {
		t.Fatal("expected fallback mode when system temp fails")
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Fatalf("fallback dir %s not created: %v", dir, statErr)
	}

	cleanupTempDir()
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatalf("fallback dir %s should be removed after cleanup (stat err=%v)", dir, statErr)
	}
	cleanupTempDir()

	dir2, _, err := tempDir()
	if err != nil {
		t.Fatalf("tempDir after cleanup: %v", err)
	}
	if dir2 != dir {
		t.Fatalf("re-resolved dir %s, want %s", dir2, dir)
	}
	if _, statErr := os.Stat(dir2); statErr != nil {
		t.Fatalf("fallback dir not recreated after cleanup: %v", statErr)
	}
}

func TestTempDirTotalFailure(t *testing.T) {
	resetTempDirState()
	defer resetTempDirState()
	origProbe := probeSystemTemp
	origFallback := fallbackTempDir
	defer func() { probeSystemTemp = origProbe; fallbackTempDir = origFallback }()
	probeSystemTemp = func() (string, error) { return "", errors.New("system temp unavailable") }
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "block"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fallbackTempDir = filepath.Join(root, "block", ".tmp")

	if _, _, err := tempDir(); err == nil {
		t.Fatal("expected error when both system temp and fallback fail")
	}
	cleanupTempDir()
}
