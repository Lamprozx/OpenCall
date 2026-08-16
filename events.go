package main

import (
	"context"
	"time"

	meowcaller "github.com/purpshell/meowcaller"
	"github.com/rs/zerolog"
)

func monitorRinging(ctx context.Context, call *meowcaller.Call, target string) {
	log := zerolog.Ctx(ctx)
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				switch st := call.State(); st {
				case meowcaller.CallPhaseActive, meowcaller.CallPhaseEnded, meowcaller.CallPhaseWaitingRoom:
					return
				default:
					log.Warn().Str("target", target).Str("phase", phaseName(st)).
						Msg("still waiting for the peer to answer — if the target shows 'connecting', they haven't picked up yet")
				}
			}
		}
	}()
}

func wireEvents(ctx context.Context, call *meowcaller.Call, state *callState) {
	log := zerolog.Ctx(ctx)

	call.OnStateChange(func(p meowcaller.CallPhase) {
		log.Info().Str("phase", phaseName(p)).Msg("call state")
	})
	call.OnReady(func() {
		log.Info().Msg("media ready — streaming now")
		state.runReady()
	})
	call.OnPeerAccept(func() {
		log.Info().Msg("peer accepted the call")
	})
	call.OnEnd(func(reason string) {
		log.Info().Str("reason", reason).Msg("call ended")
		if p := state.getPlayer(); p != nil {
			p.Stop()
		}
		state.runVideoCleanup()
		if err := state.closeParticipantRecorder(); err != nil {
			log.Warn().Err(err).Msg("close participant recorder")
		}
		if termUI != nil {
			termUI.setMeter(nil, nil)
		}
		state.clear()
		cleanupTempDir()
	})
	call.OnMuteState(func(muted bool) {
		log.Info().Bool("muted", muted).Msg("peer mute state")
	})
	call.OnReaction(func(r meowcaller.CallReaction) {
		if r.Removed {
			log.Info().Str("from", r.Sender.String()).Msg("reaction removed")
		} else {
			log.Info().Str("emoji", r.Emoji).Str("from", r.Sender.String()).Msg("reaction")
		}
	})
	call.OnHandRaise(func(h meowcaller.HandRaiseState) {
		log.Info().Str("participant", h.Participant.String()).Bool("raised", h.Raised).Msg("hand raise")
	})
	call.OnScreenShare(func(s meowcaller.ScreenShareState) {
		log.Info().Str("participant", s.Participant.String()).Bool("active", s.Active).Msg("screen share")
	})
	call.OnGroupState(func(g meowcaller.GroupCallState) {
		log.Info().Uint32("txn", g.TransactionID).
			Int("participants", len(g.Participants)).
			Bool("rekey", g.RekeyRequested).Msg("group roster")
	})
	call.OnWaitingRoomState(func(w meowcaller.WaitingRoomState) {
		log.Info().Bool("enabled", w.Enabled).Bool("in_room", w.InWaitingRoom).
			Bool("admin", w.IsAdmin).Int("pending", len(w.Users)).Msg("waiting room")
	})
	call.OnVideoState(func(v meowcaller.VideoState) {
		log.Info().Bool("active", v.Active).Bool("upgrade", v.Upgrade).
			Int("orientation", v.Orientation).Msg("peer video state")
	})
	call.OnVideoKeyframeRequest(func() {
		log.Info().Msg("peer requested a video keyframe")
	})
	call.OnParticipantVideoFrame(func(f meowcaller.ParticipantVideoFrame) {
		if rec := state.participantRecorder(); rec != nil && rec.matches(f.ParticipantID) {
			if err := rec.write(f.AccessUnit); err != nil {
				log.Warn().Err(err).Str("participant", f.ParticipantID).
					Msg("write participant video failed")
			}
		}
		log.Debug().Str("participant", f.ParticipantID).
			Int("bytes", len(f.AccessUnit)).Msg("participant video frame")
	})
}
