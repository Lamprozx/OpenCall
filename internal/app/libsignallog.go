package app

import (
	"github.com/rs/zerolog"
	libsiglog "go.mau.fi/libsignal/logger"
)

type signalLog struct {
	log zerolog.Logger
}

func (s signalLog) Debug(caller, message string) {
	s.log.Debug().Str("caller", caller).Msg(message)
}
func (s signalLog) Info(caller, message string) {
	s.log.Info().Str("caller", caller).Msg(message)
}
func (s signalLog) Warning(caller, message string) {
	s.log.Warn().Str("caller", caller).Msg(message)
}
func (s signalLog) Error(caller, message string) {
	s.log.Error().Str("caller", caller).Msg(message)
}
func (s signalLog) Configure(settings string) {}

// InstallLibsignalLogger routes the Signal protocol library's logs through zerolog.
func InstallLibsignalLogger(l zerolog.Logger) {
	adapter := libsiglog.Loggable(signalLog{log: l})
	libsiglog.Setup(&adapter)
}
