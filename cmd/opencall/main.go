package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/purpshell/meowcaller/diag"
	"github.com/rs/zerolog"

	"opencall/internal/app"
	"opencall/internal/console"
	"opencall/internal/session"
)

func main() {
	console.TermUI = console.NewConsoleUI()
	if console.TermUI != nil {
		console.TermOut = console.TermUI
		defer console.TermUI.Disable()
		sigWinch := make(chan os.Signal, 1)
		signal.Notify(sigWinch, syscall.SIGWINCH)
		go func() {
			for range sigWinch {
				console.TermUI.Resize()
			}
		}()
	}
	log.SetOutput(console.TermOut)

	rest, levelArg, quiet, showNoise, diagDir := extractGlobalFlags(os.Args[1:])

	var rec *diag.Recorder
	if diagDir != "" {
		var derr error
		rec, derr = diag.NewRecorder(diagDir)
		if derr != nil {
			fmt.Fprintf(os.Stderr, "--diag: %v\n", derr)
			usage()
		}
		defer rec.Close()
	}
	logger := app.NewLogger(console.TermOut, resolveLogLevel(levelArg, quiet), !showNoise, console.TermUI != nil, rec)
	app.InstallLibsignalLogger(logger)
	defer app.CleanupTempDir()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx = logger.WithContext(ctx)
	if rec != nil {
		logger.Info().Str("dir", diagDir).
			Msg("--diag: writing call diagnostics (xmpp/relay/rtp/media JSONL) — logs stay at the requested verbosity")
		ctx = context.WithValue(ctx, app.DiagCtxKey{}, rec)
	}

	if len(rest) < 1 {
		prog := os.Args[0]
		if prog == "" {
			prog = "opencall"
		}
		fmt.Fprintf(os.Stderr, "Try '%s --help' for more information.\n", prog)
		return
	}
	switch rest[0] {
	case "auth":
		runAuth(ctx, cancel, rest[1:])
	case "deauth", "--deauth", "-deauth":
		session.Deauth()
	case "listen":
		runListen(ctx, cancel, rest[1:])
	case "call":
		runCall(ctx, cancel, rest[1:])
	case "group":
		runGroup(ctx, cancel, rest[1:])
	case "link":
		runLink(ctx, cancel, rest[1:])
	case "version", "--version", "-version":
		printVersion()
		return
	case "help", "-h", "--help":
		printUsage()
		return
	default:
		prog := os.Args[0]
		if prog == "" {
			prog = "opencall"
		}
		fmt.Fprintf(os.Stderr, "unknown command %q\n", rest[0])
		fmt.Fprintf(os.Stderr, "Try '%s --help' for more information.\n", prog)
		usage()
	}
}

func extractGlobalFlags(args []string) (rest []string, level string, quiet, showNoise bool, diagDir string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--log-level" || a == "-log-level":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--log-level needs a value")
				usage()
			}
			i++
			level = args[i]
		case strings.HasPrefix(a, "--log-level="):
			level = strings.TrimPrefix(a, "--log-level=")
		case strings.HasPrefix(a, "-log-level="):
			level = strings.TrimPrefix(a, "-log-level=")
		case a == "--quiet" || a == "-quiet":
			quiet = true
		case a == "--show-noise" || a == "-show-noise":
			showNoise = true
		case a == "--diag" || a == "-diag":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--diag needs a value")
				usage()
			}
			i++
			diagDir = args[i]
		case strings.HasPrefix(a, "--diag="):
			diagDir = strings.TrimPrefix(a, "--diag=")
		case strings.HasPrefix(a, "-diag="):
			diagDir = strings.TrimPrefix(a, "-diag=")
		default:
			rest = append(rest, a)
		}
	}
	return rest, level, quiet, showNoise, diagDir
}

func resolveLogLevel(flagLevel string, quiet bool) zerolog.Level {
	level := zerolog.InfoLevel
	if lvl, err := zerolog.ParseLevel(os.Getenv("MEOW_LOG_LEVEL")); err == nil && lvl != zerolog.NoLevel {
		level = lvl
	}
	if flagLevel != "" {
		lvl, err := zerolog.ParseLevel(flagLevel)
		if err != nil || lvl == zerolog.NoLevel {
			fmt.Fprintf(os.Stderr, "invalid --log-level %q (want trace|debug|info|warn|error|fatal)\n", flagLevel)
			usage()
		}
		return lvl
	}
	if quiet {
		return zerolog.WarnLevel
	}
	return level
}

func printUsage() {
	fmt.Fprint(os.Stderr, `usage: OpenCall <command> [flags]

commands:
  auth [--pair <phone>]                log in and save a new session: QR code by
                                       default, or a phone pairing code with
                                       --pair <phone> (e.g. 6281234567890).
                                       After logging in the tool asks for a
                                       session name (unique, with suggestions).
  auth list                            interactive list of saved sessions
                                       (hash/id, name, phone, last used):
                                       ↑/↓ select, e = rename, q/Ctrl-C = quit
  auth switch [<name>]                 switch the active session used by the
                                       other commands. With 2 sessions it
                                       toggles directly; with 3+ an interactive
                                       ↑/↓ picker is shown; <name> switches by
                                       name non-interactively
  deauth                               delete a session. Interactive ↑/↓ picker
                                       (name + phone only); Enter asks "Are you
                                       sure...?" with the cursor on "No";
                                       q/Ctrl-C quits
  listen [--auto-answer] [--play a,b]  receive calls; without --auto-answer,
      [--volume +5s] [--loop <N>]      answer/reject from the console
      [--stream] [--record out.wav]
      [--record-video out.h264]
      [--record-participant jid[:f]]
  call <target> [--video] [--play a,b] place a 1:1 call (target: phone number,
      [--volume +5s] [--loop <N>]      phone JID or @lid JID)
      [--stream] [--record out.wav]
      [--record-video out.h264]
      [--record-participant jid[:f]]
      [--video-file cam.mp4]
  group [--group-id <gid>] [--video]   group call: to >= 2 explicit targets
      [--play a,b] [--volume +5s]      (ad-hoc), or --group-id binds every
      [--loop <N>] [--record out.wav]  member of a WhatsApp group
      [--record-video out.h264]
      [--record-participant jid[:f]]
      <target...>
  group join [--group-id <gid>]        join an ALREADY-RUNNING group call in a
      [--play a,b] [--volume +5s]      group this session belongs to: waits for
      [--loop <N>] [--record out.wav]  the group-call invite, auto-joins it and
      [--record-video out.h264]        streams the media (never answers 1:1
      [--record-participant jid[:f]]   calls). Without --group-id an
                                       interactive picker lists your joined
                                       groups (auth list style)
  link create [--video]                create a reusable call link
  link preview <token-or-url> [--video]  inspect a call link without joining
  link join <token-or-url> [--video]   join a call link (may land in the
      [--play a,b] [--volume +5s]      waiting room)
      [--loop <N>] [--record out.wav]
      [--record-video out.h264]
      [--record-participant jid[:f]]

  version                              print version, Go version, OS/arch and
                                       build info (also --version)

playback options:
  --play a,b,c     stream .mp3/.wav/.opus files, comma-separated, left to right.
                   Files are decoded natively (no ffmpeg needed) to 16 kHz mono
                   PCM. Requires ffmpeg only when audio effects are used.
  --stream         instead of --play, read raw s16le mono 16 kHz PCM from
                   stdin and stream it live (e.g. arecord -f S16_LE -r 16000
                   -c 1 | OpenCall call <target> --stream). Cannot be combined
                   with --play or --loop.
  --volume +5s     adjust volume: +5s = 5% louder, -3s = 3% quieter
  --loop <N>       repeat each file N times. N is a mandatory
                   positive integer (e.g. --loop 10); a missing or non-integer
                   value is an error
  --video-file F   stream F as your camera in a --video call. Raw Annex-B .h264
                   is used as-is; other containers (.mp4/.mkv/...) are
                   transcoded to baseline H.264 in /tmp via ffmpeg (originals
                   untouched); requires ffmpeg in PATH. Same .tmp fallback as
                   --play applies.
  --record out.wav     save peer audio to a 16 kHz mono WAV
  --record-video out.h264  save peer video as Annex-B H.264
  --record-participant <jid>[:out.h264]  save ONE group participant's video
                       (default out: participant-<jid>.h264)

audio effects (applied by ffmpeg to --play files, then streamed; requires
ffmpeg in PATH. The original file is never touched and the converted file is
removed when the call ends):
  --speed x          tempo: 1 = normal, >1 faster, <1 slower (0.25-4)
  --pitch n          pitch shift in semitones, -12..+12
  --reverb l         reverb amount 1-10
  --echo l           echo amount 1-10
  --bass db          bass gain in dB (-24..24)
  --treble db        treble gain in dB (-24..24)
  --lowpass hz       lowpass cutoff in Hz
  --highpass hz      highpass cutoff in Hz
  --chorus l         chorus amount 1-10
  --flanger l        flanger amount 1-10
  --tremolo hz       tremolo rate in Hz
  --vibrato hz       vibrato rate in Hz
  --crusher l        bitcrusher amount 1-10
  --fade-in s        fade in seconds
  --fade-out s       fade out seconds
  --reverse          play in reverse

log options (can appear anywhere):
  --log-level <level>   trace|debug|info|warn|error|fatal (default: info, or
                        the MEOW_LOG_LEVEL env var). Higher levels hide the
                        lower ones, e.g. --log-level warn hides INFO chatter.
  --quiet               shorthand for --log-level warn
  --show-noise          keep background whatsmeow noise (failed-to-decrypt
                        status/group messages, sqlite busy retries, push-name
                        saves, ...) that is filtered out by default
  --diag <dir>          write call diagnostics to <dir> as per-stream JSONL
                        (xmpp wire XML, keying, relay, RTP, media, call
                        state). The console keeps the requested verbosity

During a call the interactive console accepts: answer, reject, hangup, react
<emoji>, video on|off, accept-video, orientation <0-3>, handraise on|off,
screenshare on|off, add <target>, addmany <t...>, ring <target>,
approval on|off, admit <user>, deny <user>, pause, resume, stop, meter on|off,
status, loglevel <level>, help, quit.
`)
}

func usage() {
	printUsage()
	os.Exit(2)
}
