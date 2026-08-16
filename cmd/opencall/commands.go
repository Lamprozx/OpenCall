package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	meowcaller "github.com/purpshell/meowcaller"
	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"

	"opencall/internal/app"
	"opencall/internal/call"
	"opencall/internal/console"
	"opencall/internal/media"
	"opencall/internal/session"
)

// ensureFFmpeg offers to install the minimal ffmpeg build when the current
// command needs it (audio effects or video transcoding) but ffmpeg is missing.
func ensureFFmpeg(play *media.PlayOptions, videoFile string) {
	if play.RequiresFFmpeg() {
		if err := app.EnsureFFmpeg("audio effects"); err != nil {
			os.Exit(1)
		}
	}
	if videoFile != "" {
		if err := app.EnsureFFmpeg("video transcoding (--video-file)"); err != nil {
			os.Exit(1)
		}
	}
}

func runAuth(ctx context.Context, cancel context.CancelFunc, args []string) {
	if len(args) > 0 {
		switch args[0] {
		case "list", "--list", "-list":
			session.AuthList()
			return
		case "switch", "--switch", "-switch":
			session.AuthSwitch(args[1:])
			return
		case "deauth", "--deauth", "-deauth":
			session.Deauth()
			return
		}
	}

	fs := flag.NewFlagSet("auth", flag.ExitOnError)
	pair := fs.String("pair", "", "log in with a pairing code for this phone number (international format, no leading 0)")
	qr := fs.Bool("qr", false, "log in by scanning a QR code (default)")
	fs.Parse(app.ReorderArgs(args))
	if *pair != "" && *qr {
		fmt.Fprintln(os.Stderr, "--pair and --qr are mutually exclusive")
		usage()
	}

	log := zerolog.Ctx(ctx)
	id := session.NewID()
	wa, _, err := call.ConnectNew(ctx, call.AuthOptions{Pair: *pair != "", Phone: *pair}, session.StorePath(id))
	if err != nil {
		os.RemoveAll(session.StoreDir(id))
		log.Fatal().Err(err).Msg("auth failed")
	}
	defer wa.Disconnect()
	if wa.Store.ID == nil {
		log.Fatal().Msg("auth completed but no device id was stored")
	}
	jid := wa.Store.ID.ToNonAD()
	phone := jid.User
	log.Info().Str("jid", jid.String()).Str("phone", phone).Msg("authenticated")

	r, err := session.LoadRegistry()
	if err != nil {
		log.Fatal().Err(err).Msg("load sessions failed")
	}
	if err := session.EnsureMigrated(r); err != nil {
		log.Fatal().Err(err).Msg("import legacy session failed")
	}
	if existing := r.ByJID(jid.String()); existing != nil {
		os.RemoveAll(session.StoreDir(id))
		if err := r.Touch(existing.ID); err != nil {
			log.Warn().Err(err).Msg("failed to update session")
		}
		log.Info().Str("session", existing.Name).Str("id", existing.ID).
			Msg("this number is already a saved session — reused")
		return
	}

	now := time.Now()
	name := session.PromptSessionName(r, fmt.Sprintf("session-%d", len(r.Sessions)+1), phone)
	if err := r.Add(session.Record{
		ID: id, Name: name, Phone: phone, JID: jid.String(),
		CreatedAt: now, LastUsed: now,
	}); err != nil {
		log.Fatal().Err(err).Msg("save session failed")
	}
	if err := r.SetActive(id); err != nil {
		log.Warn().Err(err).Msg("set active failed")
	}
	log.Info().Str("session", name).Str("id", id).Str("phone", phone).
		Msg("session saved — ready to use other commands")
}

func runListen(ctx context.Context, cancel context.CancelFunc, args []string) {
	fs := flag.NewFlagSet("listen", flag.ExitOnError)
	auto := fs.Bool("auto-answer", false, "answer every incoming call")
	play := &media.PlayOptions{}
	play.Register(fs)
	record := fs.String("record", "", "record caller audio to this .wav")
	recordVideo := fs.String("record-video", "", "record caller video to this .h264 (Annex-B)")
	recordParticipant := fs.String("record-participant", "", "save one group participant's video to <jid>[:out.h264] (Annex-B)")
	fs.Parse(app.ReorderArgs(args))
	if err := play.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage()
	}
	ensureFFmpeg(play, "")

	log := zerolog.Ctx(ctx)
	wa, client, err := call.Connect(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("connect failed")
	}
	defer wa.Disconnect()
	state := media.NewCallState()
	state.SetStdinStreaming(play.Stream)

	client.OnIncomingCall(func(c *meowcaller.Call) {
		log.Info().Str("call_id", c.ID()).Str("peer", c.Peer().String()).
			Bool("video", c.IsVideo()).Msg("incoming call")
		state.SetCall(c)
		call.WireEvents(ctx, c, state)
		state.SetMedia(media.NewMediaConfig(play, *record, *recordVideo, "", *recordParticipant))
		if !*auto {
			log.Info().Msg("type 'answer', 'reject' or 'hangup'")
			return
		}
		if err := c.Answer(); err != nil {
			log.Error().Err(err).Msg("answer failed")
			return
		}
		log.Info().Msg("answered")
		state.AttachMediaOnce(ctx, c)
	})

	go call.ConsoleLoop(ctx, cancel, state)
	log.Info().Bool("auto_answer", *auto).Msg("listening for incoming calls")
	<-ctx.Done()
}

func runCall(ctx context.Context, cancel context.CancelFunc, args []string) {
	fs := flag.NewFlagSet("call", flag.ExitOnError)
	video := fs.Bool("video", false, "place a video call (H.264)")
	play := &media.PlayOptions{}
	play.Register(fs)
	record := fs.String("record", "", "record peer audio to this .wav")
	recordVideo := fs.String("record-video", "", "record peer video to this .h264")
	recordParticipant := fs.String("record-participant", "", "save one group participant's video to <jid>[:out.h264] (Annex-B)")
	videoFile := fs.String("video-file", "", "stream this video as your camera (Annex-B .h264 used as-is; .mp4/.mkv auto-transcoded to baseline H.264 in /tmp via ffmpeg)")
	fs.Parse(app.ReorderArgs(args))
	if err := play.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage()
	}
	if fs.NArg() < 1 {
		usage()
	}
	target := fs.Arg(0)
	ensureFFmpeg(play, *videoFile)

	log := zerolog.Ctx(ctx)
	wa, client, err := call.Connect(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("connect failed")
	}
	defer wa.Disconnect()

	var c *meowcaller.Call
	if *video {
		c, err = client.CallWithOptions(ctx, target, meowcaller.CallOptions{Video: true})
	} else {
		c, err = client.Call(ctx, target)
	}
	if err != nil {
		log.Fatal().Err(err).Str("target", target).Msg("place call failed")
	}

	state := media.NewCallState()
	state.SetStdinStreaming(play.Stream)
	state.SetCall(c)
	call.WireEvents(ctx, c, state)
	go call.ConsoleLoop(ctx, cancel, state)
	state.SetMedia(media.NewMediaConfig(play, *record, *recordVideo, *videoFile, *recordParticipant))
	state.AttachMediaOnce(ctx, c)
	call.MonitorRinging(ctx, c, target)

	log.Info().Str("target", target).Str("call_id", c.ID()).
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
	play := &media.PlayOptions{}
	play.Register(fs)
	record := fs.String("record", "", "record group audio to this .wav")
	recordVideo := fs.String("record-video", "", "record peer video to this .h264")
	recordParticipant := fs.String("record-participant", "", "save one group participant's video to <jid>[:out.h264] (Annex-B)")
	fs.Parse(app.ReorderArgs(args))
	if err := play.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage()
	}
	ensureFFmpeg(play, "")

	log := zerolog.Ctx(ctx)
	wa, client, err := call.Connect(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("connect failed")
	}
	defer wa.Disconnect()

	var c *meowcaller.Call
	opts := meowcaller.GroupCallOptions{Video: *video}
	switch {
	case *groupID != "":
		opts.GroupJID = *groupID
		c, err = client.GroupCallByIDWithOptions(ctx, *groupID, opts)
	case fs.NArg() >= 2:
		c, err = client.GroupCallWithOptions(ctx, fs.Args(), opts)
	default:
		fmt.Fprintln(os.Stderr, "group needs --group-id <gid> or at least 2 targets")
		usage()
	}
	if err != nil {
		log.Fatal().Err(err).Msg("place group call failed")
	}

	state := media.NewCallState()
	state.SetStdinStreaming(play.Stream)
	state.SetCall(c)
	call.WireEvents(ctx, c, state)
	go call.ConsoleLoop(ctx, cancel, state)
	state.SetMedia(media.NewMediaConfig(play, *record, *recordVideo, "", *recordParticipant))
	state.AttachMediaOnce(ctx, c)

	log.Info().Str("call_id", c.ID()).Bool("video", *video).Msg("group call placed")
	<-ctx.Done()
}

func runGroupJoin(ctx context.Context, cancel context.CancelFunc, args []string) {
	fs := flag.NewFlagSet("group join", flag.ExitOnError)
	groupID := fs.String("group-id", "", "only join calls from this WhatsApp group (bare id or @g.us JID); otherwise pick interactively")
	play := &media.PlayOptions{}
	play.Register(fs)
	record := fs.String("record", "", "record group audio to this .wav")
	recordVideo := fs.String("record-video", "", "record peer video to this .h264")
	recordParticipant := fs.String("record-participant", "", "save one group participant's video to <jid>[:out.h264] (Annex-B)")
	fs.Parse(app.ReorderArgs(args))
	if err := play.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage()
	}
	ensureFFmpeg(play, "")

	log := zerolog.Ctx(ctx)
	wa, client, err := call.Connect(ctx)
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

	state := media.NewCallState()
	state.SetStdinStreaming(play.Stream)

	client.OnIncomingCall(func(c *meowcaller.Call) {
		gs, ok := c.GroupState()
		if !ok {
			log.Info().Str("peer", c.Peer().String()).Msg("ignoring non-group call")
			if err := c.Reject(); err != nil {
				log.Warn().Err(err).Msg("reject non-group call failed")
			}
			return
		}
		if !app.GroupJIDsEqual(gs.GroupJID.String(), target) {
			msg := "group call from another group — ignoring"
			if gs.GroupJID.IsEmpty() {
				msg = "group call invite carries no group identity — cannot verify, ignoring"
			}
			log.Info().Str("call_id", c.ID()).Str("group", gs.GroupJID.String()).Msg(msg)
			if err := c.Reject(); err != nil {
				log.Warn().Err(err).Msg("reject other-group call failed")
			}
			return
		}
		log.Info().Str("call_id", c.ID()).Str("group", target).
			Bool("video", c.IsVideo()).Msg("group call invite — joining")
		state.SetCall(c)
		call.WireEvents(ctx, c, state)
		state.SetMedia(media.NewMediaConfig(play, *record, *recordVideo, "", *recordParticipant))
		if err := c.Answer(); err != nil {
			log.Error().Err(err).Msg("join group call failed")
			return
		}
		state.AttachMediaOnce(ctx, c)
	})

	go call.ConsoleLoop(ctx, cancel, state)
	log.Info().Str("group", target).Msg("waiting for a group call invite in this group")
	<-ctx.Done()
}

func pickGroupToJoin(ctx context.Context, wa *whatsmeow.Client, explicit string) (string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		return app.NormalizeGroupID(explicit)
	}
	groups, err := wa.GetJoinedGroups(ctx)
	if err != nil {
		return "", fmt.Errorf("list joined groups: %w", err)
	}
	if len(groups) == 0 {
		return "", fmt.Errorf("this session is in no groups — join a WhatsApp group first")
	}
	pt, ok := console.OpenPicker()
	if !ok {
		fmt.Fprintln(os.Stderr, "joined groups:")
		for _, g := range groups {
			fmt.Fprintf(os.Stderr, "  %-40s %s\n", session.OrDash(g.GroupName.Name), g.JID)
		}
		return "", fmt.Errorf("interactive terminal required — pass --group-id <gid> (e.g. the JID above)")
	}
	defer pt.Close()
	items := make([]string, 0, len(groups))
	for _, g := range groups {
		items = append(items, fmt.Sprintf("%-40s %s", session.OrDash(g.GroupName.Name), g.JID))
	}
	title := fmt.Sprintf("group join — %d groups", len(groups))
	idx, key, keep := pt.PickList(title, "   name                                      group id",
		items, "↑/↓ select · Enter join · q/Ctrl-C quit")
	if !keep || key != '\r' {
		return "", nil
	}
	return app.NormalizeGroupID(groups[idx].JID.String())
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
		fs.Parse(app.ReorderArgs(args[1:]))
		wa, client, err := call.Connect(ctx)
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
		fs.Parse(app.ReorderArgs(args[1:]))
		if fs.NArg() < 1 {
			usage()
		}
		wa, client, err := call.Connect(ctx)
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
		play := &media.PlayOptions{}
		play.Register(fs)
		record := fs.String("record", "", "record peer audio to this .wav")
		recordVideo := fs.String("record-video", "", "record peer video to this .h264")
		recordParticipant := fs.String("record-participant", "", "save one group participant's video to <jid>[:out.h264] (Annex-B)")
		fs.Parse(app.ReorderArgs(args[1:]))
		if err := play.Validate(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			usage()
		}
		if fs.NArg() < 1 {
			usage()
		}
		ensureFFmpeg(play, "")
		wa, client, err := call.Connect(ctx)
		if err != nil {
			log.Fatal().Err(err).Msg("connect failed")
		}
		defer wa.Disconnect()
		c, err := client.JoinCallLink(ctx, fs.Arg(0), meowcaller.CallLinkOptions{Video: *video})
		if err != nil {
			log.Fatal().Err(err).Msg("join call link failed")
		}
		state := media.NewCallState()
		state.SetStdinStreaming(play.Stream)
		state.SetCall(c)
		call.WireEvents(ctx, c, state)
		go call.ConsoleLoop(ctx, cancel, state)
		state.SetMedia(media.NewMediaConfig(play, *record, *recordVideo, "", *recordParticipant))
		state.AttachMediaOnce(ctx, c)
		if wr, ok := c.WaitingRoomState(); ok && wr.InWaitingRoom {
			log.Info().Msg("in the waiting room — wait for the host to admit you")
		} else {
			log.Info().Str("call_id", c.ID()).Msg("joined call link")
		}
		<-ctx.Done()

	default:
		usage()
	}
}
