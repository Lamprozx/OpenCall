package main

import (
	"encoding/json"
	"strings"
	"sync/atomic"

	"github.com/purpshell/meowcaller/diag"
	"github.com/rs/zerolog"
)

var consoleMinLevel atomic.Int32

type diagCtxKey struct{}

type diagSplitter struct{ rec *diag.Recorder }

func newDiagSplitter(rec *diag.Recorder) *diagSplitter { return &diagSplitter{rec: rec} }

func (d *diagSplitter) Write(p []byte) (int, error) {
	var ev map[string]any
	if err := json.Unmarshal(p, &ev); err != nil {
		return len(p), nil
	}
	stream := "log"
	if sub, ok := ev["sublogger"].(string); ok && strings.HasPrefix(sub, "wa") {
		stream = "xmpp"
	}
	d.rec.Emit(stream, ev)
	return len(p), nil
}

type levelGate struct {
	out zerolog.LevelWriter
	min *atomic.Int32
}

func (g levelGate) WriteLevel(l zerolog.Level, p []byte) (int, error) {
	if l < zerolog.Level(g.min.Load()) {
		return len(p), nil
	}
	return g.out.WriteLevel(l, p)
}

func (g levelGate) Write(p []byte) (int, error) { return g.out.Write(p) }
