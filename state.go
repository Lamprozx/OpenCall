package main

import (
	"context"
	"sync"

	meowcaller "github.com/purpshell/meowcaller"
)

type callState struct {
	mu           sync.Mutex
	call         *meowcaller.Call
	player       *meowcaller.Player
	videoCleanup func()
	readyFns     []func()
	ended        bool

	media         *mediaConfig
	mediaAttached bool

	multi       *multiSink
	meter       *meterSink
	meterSinkID int

	partRec *participantRecorder

	stdinStream bool
}

func newCallState() *callState { return &callState{} }

func (s *callState) setCall(c *meowcaller.Call) {
	s.mu.Lock()
	s.call = c
	s.mediaAttached = false
	s.mu.Unlock()
}
func (s *callState) getCall() *meowcaller.Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.call
}
func (s *callState) setPlayer(p *meowcaller.Player) {
	s.mu.Lock()
	s.player = p
	s.mu.Unlock()
}
func (s *callState) getPlayer() *meowcaller.Player {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.player
}
func (s *callState) setVideoCleanup(fn func()) {
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

func (s *callState) addReady(fn func()) {
	s.mu.Lock()
	s.readyFns = append(s.readyFns, fn)
	s.mu.Unlock()
}

func (s *callState) runReady() {
	s.mu.Lock()
	fns := s.readyFns
	s.readyFns = nil
	s.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

func (s *callState) runVideoCleanup() {
	s.mu.Lock()
	fn := s.videoCleanup
	s.videoCleanup = nil
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
}
func (s *callState) clear() {
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

func (s *callState) setMedia(cfg *mediaConfig) {
	s.mu.Lock()
	s.media = cfg
	s.mu.Unlock()
}

func (s *callState) attachMediaOnce(ctx context.Context, call *meowcaller.Call) {
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

func (s *callState) setMultiSink(m *multiSink) {
	s.mu.Lock()
	s.multi = m
	s.mu.Unlock()
}

func (s *callState) enableMeter() (level, peak func() float32, ok bool) {
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

func (s *callState) disableMeter() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.meter != nil && s.multi != nil && s.meterSinkID > 0 {
		s.multi.remove(s.meterSinkID)
	}
	s.meter = nil
	s.meterSinkID = 0
}

func (s *callState) setParticipantRecorder(r *participantRecorder) {
	s.mu.Lock()
	s.partRec = r
	s.mu.Unlock()
}

func (s *callState) participantRecorder() *participantRecorder {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.partRec
}

func (s *callState) closeParticipantRecorder() error {
	s.mu.Lock()
	r := s.partRec
	s.partRec = nil
	s.mu.Unlock()
	if r != nil {
		return r.close()
	}
	return nil
}

func (s *callState) setStdinStreaming(b bool) {
	s.mu.Lock()
	s.stdinStream = b
	s.mu.Unlock()
}

func (s *callState) stdinStreaming() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stdinStream
}
