package media

import (
	"context"
	"sync"

	meowcaller "github.com/purpshell/meowcaller"
)

// CallState holds the runtime state of the active call: the call handle, the
// player, media config, and sinks.
type CallState struct {
	mu           sync.Mutex
	call         *meowcaller.Call
	player       *meowcaller.Player
	videoCleanup func()
	readyFns     []func()
	ended        bool

	media         *MediaConfig
	mediaAttached bool

	multi       *multiSink
	meter       *meterSink
	meterSinkID int

	partRec *participantRecorder

	stdinStream bool
}

func NewCallState() *CallState { return &CallState{} }

func (s *CallState) SetCall(c *meowcaller.Call) {
	s.mu.Lock()
	s.call = c
	s.mediaAttached = false
	s.mu.Unlock()
}

func (s *CallState) GetCall() *meowcaller.Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.call
}

func (s *CallState) setPlayer(p *meowcaller.Player) {
	s.mu.Lock()
	s.player = p
	s.mu.Unlock()
}

func (s *CallState) GetPlayer() *meowcaller.Player {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.player
}

func (s *CallState) setVideoCleanup(fn func()) {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		if fn != nil {
			fn()
		}
		return
	}
	s.videoCleanup = fn
	s.mu.Unlock()
}

func (s *CallState) addReady(fn func()) {
	s.mu.Lock()
	s.readyFns = append(s.readyFns, fn)
	s.mu.Unlock()
}

func (s *CallState) RunReady() {
	s.mu.Lock()
	fns := s.readyFns
	s.readyFns = nil
	s.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

func (s *CallState) RunVideoCleanup() {
	s.mu.Lock()
	fn := s.videoCleanup
	s.videoCleanup = nil
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (s *CallState) Clear() {
	s.mu.Lock()
	s.call = nil
	s.player = nil
	s.videoCleanup = nil
	s.readyFns = nil
	s.media = nil
	s.mediaAttached = false
	s.multi = nil
	s.meter = nil
	s.meterSinkID = 0
	s.partRec = nil
	s.stdinStream = false
	s.ended = true
	s.mu.Unlock()
}

func (s *CallState) SetMedia(cfg *MediaConfig) {
	s.mu.Lock()
	s.media = cfg
	s.mu.Unlock()
}

func (s *CallState) AttachMediaOnce(ctx context.Context, call *meowcaller.Call) {
	s.mu.Lock()
	if s.media == nil || s.mediaAttached {
		s.mu.Unlock()
		return
	}
	cfg := s.media
	s.mediaAttached = true
	s.mu.Unlock()
	attachMedia(ctx, call, s, cfg)
}

func (s *CallState) setMultiSink(m *multiSink) {
	s.mu.Lock()
	s.multi = m
	s.mu.Unlock()
}

func (s *CallState) EnableMeter() (level, peak func() float32, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.multi == nil {
		return nil, nil, false
	}
	if s.meter == nil {
		s.meter = newMeterSink()
		s.meterSinkID = s.multi.add(s.meter)
	}
	return s.meter.Level, s.meter.Peak, true
}

func (s *CallState) DisableMeter() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.meter != nil && s.multi != nil && s.meterSinkID > 0 {
		s.multi.remove(s.meterSinkID)
	}
	s.meter = nil
	s.meterSinkID = 0
}

func (s *CallState) setParticipantRecorder(r *participantRecorder) {
	s.mu.Lock()
	s.partRec = r
	s.mu.Unlock()
}

// ParticipantRecorderMatches reports whether the participant recorder targets
// the given participant id.
func (s *CallState) ParticipantRecorderMatches(id string) bool {
	s.mu.Lock()
	r := s.partRec
	s.mu.Unlock()
	return r != nil && r.matches(id)
}

// WriteParticipantVideo writes a participant's Annex-B access unit to the
// active participant recorder, if one is configured for them.
func (s *CallState) WriteParticipantVideo(id string, au []byte) error {
	s.mu.Lock()
	r := s.partRec
	s.mu.Unlock()
	if r == nil {
		return nil
	}
	return r.write(au)
}

func (s *CallState) CloseParticipantRecorder() error {
	s.mu.Lock()
	r := s.partRec
	s.partRec = nil
	s.mu.Unlock()
	if r != nil {
		return r.close()
	}
	return nil
}

func (s *CallState) SetStdinStreaming(b bool) {
	s.mu.Lock()
	s.stdinStream = b
	s.mu.Unlock()
}

func (s *CallState) StdinStreaming() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stdinStream
}
