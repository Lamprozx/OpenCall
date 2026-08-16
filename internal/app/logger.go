package app

import (
	"io"

	"github.com/purpshell/meowcaller/diag"
	"github.com/rs/zerolog"
)

// NewLogger builds the application logger: a console writer (optionally
// noise-filtered), plus a diagnostic splitter when rec is non-nil.
func NewLogger(out io.Writer, level zerolog.Level, filterNoise, interactive bool, rec *diag.Recorder) zerolog.Logger {
	var w io.Writer = out
	if filterNoise {
		w = &noiseFilter{out: out}
	}
	console := zerolog.ConsoleWriter{Out: w, TimeFormat: "15:04:05.000", NoColor: interactive}
	var lw io.Writer = console
	if rec != nil {
		ConsoleMinLevel.Store(int32(level))
		lw = zerolog.MultiLevelWriter(
			&levelGate{out: zerolog.LevelWriterAdapter{Writer: console}, min: &ConsoleMinLevel},
			newDiagSplitter(rec),
		)
		if level > zerolog.DebugLevel {
			level = zerolog.DebugLevel
		}
	}
	zerolog.SetGlobalLevel(level)
	return zerolog.New(lw).With().Timestamp().Logger()
}
