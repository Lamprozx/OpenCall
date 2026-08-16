package app

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

var noisePatterns = []string{
	"failed to decrypt",
	"error decrypting message",
	"no sender key",
	"received message with old counter",
	"failed to load session",
	"failed to get or create message keys",
	"failed to check if",
	"database is locked",
	"failed to save push name",
	"failed to save business name",
	"failed to delete all identities",
	"failed to delete all sessions",
	"unavailable message",
	"node handling took",
	"websocket not connected",
	"failed to close websocket",
	"got untrusted identity error",
	"missing response in item",
	"active group invite preaccepted",
	"reason=group_call_ended",
}

type noiseFilter struct {
	mu  sync.Mutex
	out io.Writer
	buf []byte
}

func (f *noiseFilter) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.buf = append(f.buf, p...)
	for {
		i := bytes.IndexByte(f.buf, '\n')
		if i < 0 {
			break
		}
		line := f.buf[:i]
		f.buf = f.buf[i+1:]
		if isNoise(line) {
			continue
		}
		if _, err := f.out.Write(append(line, '\n')); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

func isNoise(line []byte) bool {
	l := strings.ToLower(string(line))
	for _, pat := range noisePatterns {
		if strings.Contains(l, pat) {
			return true
		}
	}
	return false
}
