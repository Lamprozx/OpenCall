package call

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	meowcaller "github.com/purpshell/meowcaller"
	"github.com/rs/zerolog"

	"opencall/internal/app"
	"opencall/internal/console"
	"opencall/internal/media"
)

// ConsoleLoop runs the interactive console until the call ends or the user quits.
func ConsoleLoop(ctx context.Context, cancel context.CancelFunc, state *media.CallState) {
	log := zerolog.Ctx(ctx)
	ui := console.TermUI
	if ui != nil {
		if err := ui.Enable(ctx, cancel); err != nil {
			log.Warn().Err(err).Msg("console UI unavailable; using plain input")
			ui = nil
		} else {
			defer ui.Disable()
		}
	}
	fmt.Fprintln(console.TermOut, "console ready — type 'help' for commands")
	if ui == nil {
		if state.StdinStreaming() {
			<-ctx.Done()
			return
		}
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			if consoleExec(ctx, cancel, log, state, sc.Text()) {
				return
			}
		}
		cancel()
		return
	}
	for {
		line, ok := ui.ReadLine(ctx)
		if !ok {
			cancel()
			return
		}
		if consoleExec(ctx, cancel, log, state, line) {
			return
		}
	}
}

func consoleExec(ctx context.Context, cancel context.CancelFunc, log *zerolog.Logger, state *media.CallState, input string) bool {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return false
	}
	cmd := strings.ToLower(fields[0])
	args := fields[1:]
	call := state.GetCall()

	if cmd != "help" && cmd != "quit" && cmd != "loglevel" && call == nil {
		log.Warn().Msg("no active call")
		return false
	}
	switch cmd {
	case "answer":
		if err := call.Answer(); err != nil {
			cErr(log, "answer", err)
			return false
		}
		log.Info().Msg("answered")
		state.AttachMediaOnce(ctx, call)
	case "reject":
		cErr(log, "reject", call.Reject())
	case "hangup":
		cErr(log, "hangup", call.Hangup())
	case "react":
		if len(args) < 1 {
			log.Warn().Msg("usage: react <emoji>")
			return false
		}
		cErr(log, "react "+args[0], call.SendReaction(args[0]))
	case "video":
		if len(args) < 1 {
			log.Warn().Msg("usage: video on|off")
			return false
		}
		switch args[0] {
		case "on":
			cErr(log, "video on", call.StartVideo())
		case "off":
			cErr(log, "video off", call.StopVideo())
		default:
			log.Warn().Msg("usage: video on|off")
		}
	case "accept-video":
		cErr(log, "accept-video", call.AcceptVideo())
	case "orientation":
		if len(args) < 1 {
			log.Warn().Msg("usage: orientation <0-3>")
			return false
		}
		o, err := strconv.Atoi(args[0])
		if err != nil || o < 0 || o > 3 {
			log.Warn().Msg("orientation must be 0-3")
			return false
		}
		cErr(log, "orientation", call.SetVideoOrientation(o))
	case "handraise":
		if len(args) < 1 {
			log.Warn().Msg("usage: handraise on|off")
			return false
		}
		cErr(log, "handraise", call.SetHandRaised(args[0] == "on"))
	case "screenshare":
		if len(args) < 1 {
			log.Warn().Msg("usage: screenshare on|off")
			return false
		}
		if args[0] == "on" {
			cErr(log, "screenshare on", call.StartScreenShare(nil))
		} else {
			cErr(log, "screenshare off", call.StopScreenShare())
		}
	case "add":
		if len(args) < 1 {
			log.Warn().Msg("usage: add <target>")
			return false
		}
		cErr(log, "add "+args[0], call.AddParticipant(ctx, args[0]))
	case "addmany":
		if len(args) < 1 {
			log.Warn().Msg("usage: addmany <t1> <t2> ...")
			return false
		}
		errs := call.AddParticipants(ctx, args...)
		ok := true
		for _, e := range errs {
			if e != nil {
				ok = false
				log.Error().Err(e).Msg("add participant failed")
			}
		}
		if ok {
			log.Info().Int("count", len(args)).Msg("addmany ok")
		}
	case "ring":
		if len(args) < 1 {
			log.Warn().Msg("usage: ring <target>")
			return false
		}
		cErr(log, "ring "+args[0], call.RingParticipant(ctx, args[0]))
	case "approval":
		if len(args) < 1 {
			log.Warn().Msg("usage: approval on|off")
			return false
		}
		cErr(log, "approval", call.SetApprovalRequired(ctx, args[0] == "on"))
	case "admit":
		if len(args) < 1 {
			log.Warn().Msg("usage: admit <user>")
			return false
		}
		cErr(log, "admit "+args[0], call.AdmitParticipant(ctx, args[0]))
	case "deny":
		if len(args) < 1 {
			log.Warn().Msg("usage: deny <user>")
			return false
		}
		cErr(log, "deny "+args[0], call.DenyParticipant(ctx, args[0]))
	case "pause":
		if p := state.GetPlayer(); p != nil {
			p.Pause()
			log.Info().Msg("playback paused")
		} else {
			log.Warn().Msg("no player attached")
		}
	case "resume":
		if p := state.GetPlayer(); p != nil {
			p.Resume()
			log.Info().Msg("playback resumed")
		} else {
			log.Warn().Msg("no player attached")
		}
	case "stop":
		if p := state.GetPlayer(); p != nil {
			p.Stop()
			log.Info().Msg("playback stopped")
		} else {
			log.Warn().Msg("no player attached")
		}
	case "meter":
		if len(args) < 1 {
			log.Warn().Msg("usage: meter on|off")
			return false
		}
		switch args[0] {
		case "on":
			lvl, peak, ok := state.EnableMeter()
			if !ok {
				log.Warn().Msg("meter unavailable (no inbound audio sink — is the call answered?)")
				return false
			}
			if console.TermUI != nil {
				console.TermUI.SetMeter(lvl, peak)
			}
			log.Info().Msg("meter on — inbound audio levels shown on the console border")
		case "off":
			state.DisableMeter()
			if console.TermUI != nil {
				console.TermUI.SetMeter(nil, nil)
			}
			log.Info().Msg("meter off")
		default:
			log.Warn().Msg("usage: meter on|off")
		}
	case "status":
		printStatus(log, call)
	case "loglevel":
		if len(args) < 1 {
			log.Warn().Msg("usage: loglevel <trace|debug|info|warn|error|fatal>")
			return false
		}
		lvl, err := zerolog.ParseLevel(args[0])
		if err != nil || lvl == zerolog.NoLevel {
			log.Warn().Msg("usage: loglevel <trace|debug|info|warn|error|fatal>")
			return false
		}
		zerolog.SetGlobalLevel(lvl)
		app.ConsoleMinLevel.Store(int32(lvl))
		log.Info().Str("level", lvl.String()).Msg("log level set")
	case "help":
		printHelp()
	case "quit":
		log.Info().Msg("quitting")
		cancel()
		return true
	default:
		log.Warn().Str("cmd", cmd).Msg("unknown command — type 'help'")
	}
	return false
}

func cErr(log *zerolog.Logger, action string, err error) {
	if err != nil {
		log.Error().Err(err).Str("action", action).Msg("failed")
	} else {
		log.Info().Str("action", action).Msg("ok")
	}
}

func printStatus(log *zerolog.Logger, call *meowcaller.Call) {
	if call == nil {
		log.Warn().Msg("no active call")
		return
	}
	log.Info().Str("call_id", call.ID()).Str("peer", call.Peer().String()).
		Str("phase", phaseName(call.State())).
		Bool("video", call.IsVideo()).
		Bool("sending_video", call.IsSendingVideo()).
		Bool("receiving_video", call.IsReceivingVideo()).
		Msg("call status")
	if gs, ok := call.GroupState(); ok {
		log.Info().Uint32("txn", gs.TransactionID).Bool("rekey", gs.RekeyRequested).
			Int("participants", len(gs.Participants)).Msg("group state")
		for _, p := range gs.Participants {
			log.Info().Str("jid", p.JID.String()).Str("state", p.State).
				Bool("hand_raised", p.HandRaised).Msg("participant")
		}
	}
	if wr, ok := call.WaitingRoomState(); ok {
		log.Info().Bool("enabled", wr.Enabled).Bool("in_room", wr.InWaitingRoom).
			Bool("admin", wr.IsAdmin).Int("pending", len(wr.Users)).Msg("waiting room")
		for _, u := range wr.Users {
			log.Info().Str("jid", u.JID.String()).Str("state", u.State).Msg("waiting user")
		}
	}
	for _, s := range call.ScreenShares() {
		log.Info().Str("participant", s.Participant.String()).Bool("active", s.Active).
			Uint32("id", s.ScreenShareID).Msg("screen share")
	}
}

func phaseName(p meowcaller.CallPhase) string {
	switch p {
	case meowcaller.CallPhaseIdle:
		return "idle"
	case meowcaller.CallPhaseCalling:
		return "calling"
	case meowcaller.CallPhaseRinging:
		return "ringing"
	case meowcaller.CallPhaseConnecting:
		return "connecting"
	case meowcaller.CallPhaseActive:
		return "active"
	case meowcaller.CallPhaseEnded:
		return "ended"
	case meowcaller.CallPhaseWaitingRoom:
		return "waiting-room"
	default:
		return fmt.Sprintf("phase(%d)", int(p))
	}
}

func printHelp() {
	fmt.Fprint(console.TermOut, `commands:
  answer | reject | hangup       control the active call
  react <emoji>                  send a call emoji reaction
  video on|off                   start/stop sending your video
  accept-video                   accept a mid-call audio->video upgrade
  orientation <0-3>              set your camera orientation
  handraise on|off               raise/lower your hand (group calls)
  screenshare on|off             start/stop a screen share (group calls)
  add <target> | addmany <t...>  add participants to a group call
  ring <target>                  ring an added participant
  approval on|off                toggle waiting-room approval (call links)
  admit <user> | deny <user>     approve/deny a user in the waiting room
  pause | resume | stop          control file playback
  meter on|off                   show/hide a live VU meter of inbound audio
  loglevel <level>               change log verbosity live (trace|debug|info|warn|error|fatal)
  status                         show call, group and waiting-room state
  help | quit
`)
}
