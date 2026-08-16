package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	meowcaller "github.com/purpshell/meowcaller"
	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

var bareGroupIDRe = regexp.MustCompile(`^[0-9-]+$`)

var boolFlags = map[string]bool{
	"-auto-answer": true, "--auto-answer": true,
	"-video": true, "--video": true,
	"-qr": true, "--qr": true,
	"-help": true, "--help": true,
	"-h":      true,
	"-stream": true, "--stream": true,
	"-reverse": true, "--reverse": true,
}

func reorderArgs(args []string) []string {
	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			flags = append(flags, a)
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			flags = append(flags, a)
			if !boolFlags[a] && !strings.Contains(a, "=") {
				if i+1 < len(args) {
					i++
					flags = append(flags, args[i])
				} else {
					flags = append(flags, "")
				}
			}
		} else {
			positionals = append(positionals, a)
		}
	}
	return append(flags, positionals...)
}

func runAuth(ctx context.Context, cancel context.CancelFunc, args []string) {
	if len(args) > 0 {
		switch args[0] {
		case "list", "--list", "-list":
			runAuthList()
			return
		case "switch", "--switch", "-switch":
			runAuthSwitch(args[1:])
			return
		case "deauth", "--deauth", "-deauth":
			runDeauth()
			return
		}
	}

	fs := flag.NewFlagSet("auth", flag.ExitOnError)
	pair := fs.String("pair", "", "log in with a pairing code for this phone number (international format, no leading 0)")
	qr := fs.Bool("qr", false, "log in by scanning a QR code (default)")
	fs.Parse(reorderArgs(args))
	if *pair != "" && *qr {
		fmt.Fprintln(os.Stderr, "--pair and --qr are mutually exclusive")
		usage()
	}

	log := zerolog.Ctx(ctx)
	id := newSessionID()
	wa, _, err := connectNew(ctx, authOptions{pair: *pair != "", phone: *pair}, storePath(id))
	if err != nil {
		os.RemoveAll(filepath.Join(sessionDir, id))
		log.Fatal().Err(err).Msg("auth failed")
	}
	defer wa.Disconnect()
	if wa.Store.ID == nil {
		log.Fatal().Msg("auth completed but no device id was stored")
	}
	jid := wa.Store.ID.ToNonAD()
	phone := jid.User
	log.Info().Str("jid", jid.String()).Str("phone", phone).Msg("authenticated")

	r, err := loadRegistry()
	if err != nil {
		log.Fatal().Err(err).Msg("load sessions failed")
	}
	if err := ensureMigrated(r); err != nil {
		log.Fatal().Err(err).Msg("import legacy session failed")
	}
	if existing := r.byJID(jid.String()); existing != nil {
		os.RemoveAll(filepath.Join(sessionDir, id))
		if err := r.touch(existing.ID); err != nil {
			log.Warn().Err(err).Msg("failed to update session")
		}
		log.Info().Str("session", existing.Name).Str("id", existing.ID).
			Msg("this number is already a saved session — reused")
		return
	}

	now := time.Now()
	name := promptSessionName(r, fmt.Sprintf("session-%d", len(r.Sessions)+1), phone)
	if err := r.add(sessionRecord{
		ID: id, Name: name, Phone: phone, JID: jid.String(),
		CreatedAt: now, LastUsed: now,
	}); err != nil {
		log.Fatal().Err(err).Msg("save session failed")
	}
	if err := r.setActive(id); err != nil {
		log.Warn().Err(err).Msg("set active failed")
	}
	log.Info().Str("session", name).Str("id", id).Str("phone", phone).
		Msg("session saved — ready to use other commands")
}

func runListen(ctx context.Context, cancel context.CancelFunc, args []string) {
	fs := flag.NewFlagSet("listen", flag.ExitOnError)
	auto := fs.Bool("auto-answer", false, "answer every incoming call")
	play := &playOptions{}
	play.register(fs)
	record := fs.String("record", "", "record caller audio to this .wav")
	recordVideo := fs.String("record-video", "", "record caller video to this .h264 (Annex-B)")
	recordParticipant := fs.String("record-participant", "", "save one group participant's video to <jid>[:out.h264] (Annex-B)")
	fs.Parse(reorderArgs(args))
	if err := play.validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage()
	}

	log := zerolog.Ctx(ctx)
	wa, client, err := connect(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("connect failed")
	}
	defer wa.Disconnect()
	state := newCallState()
	state.setStdinStreaming(play.stream)

	client.OnIncomingCall(func(call *meowcaller.Call) {
		log.Info().Str("call_id", call.ID()).Str("peer", call.Peer().String()).
			Bool("video", call.IsVideo()).Msg("incoming call")
		state.setCall(call)
		wireEvents(ctx, call, state)
		state.setMedia(newMediaConfig(play, *record, *recordVideo, "", *recordParticipant))
		if !*auto {
			log.Info().Msg("type 'answer', 'reject' or 'hangup'")
			return
		}
		if err := call.Answer(); err != nil {
			log.Error().Err(err).Msg("answer failed")
			return
		}
		log.Info().Msg("answered")
		state.attachMediaOnce(ctx, call)
	})

	go consoleLoop(ctx, cancel, state)
	log.Info().Bool("auto_answer", *auto).Msg("listening for incoming calls")
	<-ctx.Done()
}

func runCall(ctx context.Context, cancel context.CancelFunc, args []string) {
	fs := flag.NewFlagSet("call", flag.ExitOnError)
	video := fs.Bool("video", false, "place a video call (H.264)")
	play := &playOptions{}
	play.register(fs)
	record := fs.String("record", "", "record peer audio to this .wav")
	recordVideo := fs.String("record-video", "", "record peer video to this .h264")
	recordParticipant := fs.String("record-participant", "", "save one group participant's video to <jid>[:out.h264] (Annex-B)")
	videoFile := fs.String("video-file", "", "stream this video as your camera (Annex-B .h264 used as-is; .mp4/.mkv auto-transcoded to baseline H.264 in /tmp via ffmpeg)")
	fs.Parse(reorderArgs(args))
	if err := play.validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage()
	}
	if fs.NArg() < 1 {
		usage()
	}
	target := fs.Arg(0)

	log := zerolog.Ctx(ctx)
	wa, client, err := connect(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("connect failed")
	}
	defer wa.Disconnect()

	var call *meowcaller.Call
	if *video {
		call, err = client.CallWithOptions(ctx, target, meowcaller.CallOptions{Video: true})
	} else {
		call, err = client.Call(ctx, target)
	}
	if err != nil {
		log.Fatal().Err(err).Str("target", target).Msg("place call failed")
	}

	state := newCallState()
	state.setStdinStreaming(play.stream)
	state.setCall(call)
	wireEvents(ctx, call, state)
	go consoleLoop(ctx, cancel, state)
	state.setMedia(newMediaConfig(play, *record, *recordVideo, *videoFile, *recordParticipant))
	state.attachMediaOnce(ctx, call)
	monitorRinging(ctx, call, target)

	log.Info().Str("target", target).Str("call_id", call.ID()).
		Bool("video", *video).Msg("call placed — media starts when the peer answers")
	<-ctx.Done()
}

func runGroup(ctx context.Context, cancel context.CancelFunc, args []string) {
	if len(args) > 0 && args[0] == "join" {
		runGroupJoin(ctx, cancel, args[1:])
		return
	}
	fs := flag.NewFlagSet("group", flag.ExitOnError)
	groupID := fs.String("group-id", "", "call every member of this WhatsApp group (bare id or @g.us JID)")
	video := fs.Bool("video", false, "group video call")
	play := &playOptions{}
	play.register(fs)
	record := fs.String("record", "", "record group audio to this .wav")
	recordVideo := fs.String("record-video", "", "record peer video to this .h264")
	recordParticipant := fs.String("record-participant", "", "save one group participant's video to <jid>[:out.h264] (Annex-B)")
	fs.Parse(reorderArgs(args))
	if err := play.validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage()
	}

	log := zerolog.Ctx(ctx)
	wa, client, err := connect(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("connect failed")
	}
	defer wa.Disconnect()

	var call *meowcaller.Call
	opts := meowcaller.GroupCallOptions{Video: *video}
	switch {
	case *groupID != "":
		opts.GroupJID = *groupID
		call, err = client.GroupCallByIDWithOptions(ctx, *groupID, opts)
	case fs.NArg() >= 2:
		call, err = client.GroupCallWithOptions(ctx, fs.Args(), opts)
	default:
		fmt.Fprintln(os.Stderr, "group needs --group-id <gid> or at least 2 targets")
		usage()
	}
	if err != nil {
		log.Fatal().Err(err).Msg("place group call failed")
	}

	state := newCallState()
	state.setStdinStreaming(play.stream)
	state.setCall(call)
	wireEvents(ctx, call, state)
	go consoleLoop(ctx, cancel, state)
	state.setMedia(newMediaConfig(play, *record, *recordVideo, "", *recordParticipant))
	state.attachMediaOnce(ctx, call)

	log.Info().Str("call_id", call.ID()).Bool("video", *video).Msg("group call placed")
	<-ctx.Done()
}

func runGroupJoin(ctx context.Context, cancel context.CancelFunc, args []string) {
	fs := flag.NewFlagSet("group join", flag.ExitOnError)
	groupID := fs.String("group-id", "", "only join calls from this WhatsApp group (bare id or @g.us JID); otherwise pick interactively")
	play := &playOptions{}
	play.register(fs)
	record := fs.String("record", "", "record group audio to this .wav")
	recordVideo := fs.String("record-video", "", "record peer video to this .h264")
	recordParticipant := fs.String("record-participant", "", "save one group participant's video to <jid>[:out.h264] (Annex-B)")
	fs.Parse(reorderArgs(args))
	if err := play.validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage()
	}

	log := zerolog.Ctx(ctx)
	wa, client, err := connect(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("connect failed")
	}
	defer wa.Disconnect()

	target, err := pickGroupToJoin(ctx, wa, *groupID)
	if err != nil {
		log.Fatal().Err(err).Msg("pick group to join failed")
	}
	if target == "" {
		log.Info().Msg("no group selected — quitting")
		return
	}
	log.Info().Str("group", target).Msg("will join group calls from this group")

	state := newCallState()
	state.setStdinStreaming(play.stream)

	client.OnIncomingCall(func(call *meowcaller.Call) {
		gs, ok := call.GroupState()
		if !ok {
			log.Info().Str("peer", call.Peer().String()).Msg("ignoring non-group call")
			if err := call.Reject(); err != nil {
				log.Warn().Err(err).Msg("reject non-group call failed")
			}
			return
		}
		if !groupJIDsEqual(gs.GroupJID.String(), target) {
			msg := "group call from another group — ignoring"
			if gs.GroupJID.IsEmpty() {
				msg = "group call invite carries no group identity — cannot verify, ignoring"
			}
			log.Info().Str("call_id", call.ID()).Str("group", gs.GroupJID.String()).Msg(msg)
			if err := call.Reject(); err != nil {
				log.Warn().Err(err).Msg("reject other-group call failed")
			}
			return
		}
		log.Info().Str("call_id", call.ID()).Str("group", target).
			Bool("video", call.IsVideo()).Msg("group call invite — joining")
		state.setCall(call)
		wireEvents(ctx, call, state)
		state.setMedia(newMediaConfig(play, *record, *recordVideo, "", *recordParticipant))
		if err := call.Answer(); err != nil {
			log.Error().Err(err).Msg("join group call failed")
			return
		}
		state.attachMediaOnce(ctx, call)
	})

	go consoleLoop(ctx, cancel, state)
	log.Info().Str("group", target).Msg("waiting for a group call invite in this group")
	<-ctx.Done()
}

func pickGroupToJoin(ctx context.Context, wa *whatsmeow.Client, explicit string) (string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		return normalizeGroupID(explicit)
	}
	groups, err := wa.GetJoinedGroups(ctx)
	if err != nil {
		return "", fmt.Errorf("list joined groups: %w", err)
	}
	if len(groups) == 0 {
		return "", fmt.Errorf("this session is in no groups — join a WhatsApp group first")
	}
	pt, ok := openPicker()
	if !ok {
		fmt.Fprintln(os.Stderr, "joined groups:")
		for _, g := range groups {
			fmt.Fprintf(os.Stderr, "  %-40s %s\n", orDash(g.GroupName.Name), g.JID)
		}
		return "", fmt.Errorf("interactive terminal required — pass --group-id <gid> (e.g. the JID above)")
	}
	defer pt.close()
	items := make([]string, 0, len(groups))
	for _, g := range groups {
		items = append(items, fmt.Sprintf("%-40s %s", orDash(g.GroupName.Name), g.JID))
	}
	title := fmt.Sprintf("group join — %d groups", len(groups))
	idx, key, keep := pt.pickList(title, "   name                                      group id",
		items, "↑/↓ select · Enter join · q/Ctrl-C quit")
	if !keep || key != '\r' {
		return "", nil
	}
	return normalizeGroupID(groups[idx].JID.String())
}

func normalizeGroupID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty group id")
	}
	if !strings.Contains(raw, "@") {
		if !bareGroupIDRe.MatchString(raw) {
			return "", fmt.Errorf("invalid group id %q (use a numeric id or a full @g.us JID)", raw)
		}
		return types.NewJID(raw, types.GroupServer).ToNonAD().String(), nil
	}
	jid, err := types.ParseJID(raw)
	if err != nil {
		return "", fmt.Errorf("invalid group id %q: %w", raw, err)
	}
	return jid.ToNonAD().String(), nil
}

func groupJIDsEqual(a, b string) bool {
	na, errA := normalizeGroupID(a)
	nb, errB := normalizeGroupID(b)
	if errA != nil || errB != nil {
		return false
	}
	return na == nb
}

func runLink(ctx context.Context, cancel context.CancelFunc, args []string) {
	if len(args) < 1 {
		usage()
	}
	log := zerolog.Ctx(ctx)

	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("link create", flag.ExitOnError)
		video := fs.Bool("video", false, "create a video call link")
		fs.Parse(reorderArgs(args[1:]))
		wa, client, err := connect(ctx)
		if err != nil {
			log.Fatal().Err(err).Msg("connect failed")
		}
		defer wa.Disconnect()
		link, err := client.CreateCallLink(ctx, meowcaller.CallLinkOptions{Video: *video})
		if err != nil {
			log.Fatal().Err(err).Msg("create call link failed")
		}
		log.Info().Str("token", link.Token).Str("url", link.URL).
			Bool("video", link.Video).Msg("call link created — share the URL")

	case "preview":
		fs := flag.NewFlagSet("link preview", flag.ExitOnError)
		video := fs.Bool("video", false, "preview as a video link")
		fs.Parse(reorderArgs(args[1:]))
		if fs.NArg() < 1 {
			usage()
		}
		wa, client, err := connect(ctx)
		if err != nil {
			log.Fatal().Err(err).Msg("connect failed")
		}
		defer wa.Disconnect()
		pv, err := client.PreviewCallLink(ctx, fs.Arg(0), meowcaller.CallLinkOptions{Video: *video})
		if err != nil {
			log.Fatal().Err(err).Msg("preview call link failed")
		}
		log.Info().Str("token", pv.Token).Bool("video", pv.Video).
			Bool("approval_required", pv.ApprovalRequired).Bool("is_admin", pv.IsAdmin).
			Str("creator", pv.Creator.String()).
			Str("creator_phone", pv.CreatorPhoneNumber.String()).Msg("call link preview")

	case "join":
		fs := flag.NewFlagSet("link join", flag.ExitOnError)
		video := fs.Bool("video", false, "join as a video call")
		play := &playOptions{}
		play.register(fs)
		record := fs.String("record", "", "record peer audio to this .wav")
		recordVideo := fs.String("record-video", "", "record peer video to this .h264")
		recordParticipant := fs.String("record-participant", "", "save one group participant's video to <jid>[:out.h264] (Annex-B)")
		fs.Parse(reorderArgs(args[1:]))
		if err := play.validate(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			usage()
		}
		if fs.NArg() < 1 {
			usage()
		}
		wa, client, err := connect(ctx)
		if err != nil {
			log.Fatal().Err(err).Msg("connect failed")
		}
		defer wa.Disconnect()
		call, err := client.JoinCallLink(ctx, fs.Arg(0), meowcaller.CallLinkOptions{Video: *video})
		if err != nil {
			log.Fatal().Err(err).Msg("join call link failed")
		}
		state := newCallState()
		state.setStdinStreaming(play.stream)
		state.setCall(call)
		wireEvents(ctx, call, state)
		go consoleLoop(ctx, cancel, state)
		state.setMedia(newMediaConfig(play, *record, *recordVideo, "", *recordParticipant))
		state.attachMediaOnce(ctx, call)
		if wr, ok := call.WaitingRoomState(); ok && wr.InWaitingRoom {
			log.Info().Msg("in the waiting room — wait for the host to admit you")
		} else {
			log.Info().Str("call_id", call.ID()).Msg("joined call link")
		}
		<-ctx.Done()

	default:
		usage()
	}
}
