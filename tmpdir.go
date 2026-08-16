package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

var probeSystemTemp = func() (string, error) {
	probe, err := os.CreateTemp("", "opencall-tmp-probe-*")
	if err != nil {
		return "", err
	}
	name := probe.Name()
	probe.Close()
	os.Remove(name)
	return os.TempDir(), nil
}

var fallbackTempDir = filepath.Join(".", ".tmp")

type tempDirState struct {
	mu       sync.Mutex
	resolved bool
	dir      string
	fallback bool
	err      error
}

var tempState tempDirState

func tempDir() (string, bool, error) {
	tempState.mu.Lock()
	defer tempState.mu.Unlock()
	if !tempState.resolved {
		dir, err := probeSystemTemp()
		if err == nil {
			tempState.dir = dir
		} else {
			fallback := fallbackTempDir
			if mkErr := os.MkdirAll(fallback, 0o755); mkErr != nil {
				tempState.err = fmt.Errorf("no writable temp dir: system temp: %v; fallback %s: %v", err, fallback, mkErr)
			} else {
				tempState.dir = fallback
				tempState.fallback = true
				log.Printf("WRN system temp dir unavailable (%v); using fallback %s (auto-removed when the call ends)", err, fallback)
			}
		}
		tempState.resolved = true
	}
	return tempState.dir, tempState.fallback, tempState.err
}

func cleanupTempDir() {
	tempState.mu.Lock()
	defer tempState.mu.Unlock()
	if !tempState.resolved || !tempState.fallback {
		return
	}
	os.RemoveAll(tempState.dir)
	tempState.resolved = false
	tempState.fallback = false
	tempState.dir = ""
	tempState.err = nil
}

func resetTempDirState() {
	tempState.mu.Lock()
	tempState.resolved = false
	tempState.dir = ""
	tempState.fallback = false
	tempState.err = nil
	tempState.mu.Unlock()
}
