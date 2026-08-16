# OpenCall

A WhatsApp calling CLI for the terminal. Place and receive 1:1 and group calls,
stream audio/video, apply real-time audio effects, join call links, and manage
multiple sessions — all from your console.

Built on [`whatsmeow`](https://github.com/mautrix/whatsmeow) and
[`meowcaller`](https://github.com/purpshell/meowcaller).

> **Disclaimer** — This is an independent project, not affiliated with WhatsApp
> or Meta. Using unofficial clients can get a number banned. Use a number you
> can afford to lose, and comply with WhatsApp's Terms of Service.

---

## Features

- **1:1 & group calls** — place calls to a phone number/JID, ad-hoc groups, or
  every member of a WhatsApp group.
- **Group call join** — wait for a group-call invite and auto-join it.
- **Call links** — create, preview, and join reusable call links (incl. waiting
  room).
- **Session management** — save multiple accounts (`auth`), list/switch/rename
  them interactively, or delete them.
- **Audio playback** — stream `.mp3` / `.wav` / `.opus` files during a call
  (decoded natively, no ffmpeg needed).
- **Live stdin streaming** — pipe raw 16 kHz PCM straight into a call
  (e.g. `arecord | OpenCall call <target> --stream`).
- **Audio effects** — speed, pitch, reverb, echo, bass/treble EQ, filters,
  chorus, flanger, tremolo, vibrato, bitcrusher, fade in/out, and reverse.
- **Video** — send your camera, or stream a pre-recorded `.mp4`/`.mkv`/raw
  `.h264` as your video.
- **Recording** — save peer audio (WAV), peer video (Annex-B H.264), or a single
  group participant's video.
- **Diagnostics** — dump per-stream JSONL (XMPP wire XML, keying, relay, RTP,
  media, call state) for debugging.
- **Interactive console** — answer/reject, hang up, react, toggle video,
  screenshare, hand-raise, add/ring participants, and more while a call runs.

---

## Requirements

- **Go 1.25+** (to build from source)
- **ffmpeg** *(optional)* — only needed for audio effects (`--speed`,
  `--reverb`, …) and non-Annex-B video transcoding (`--video-file foo.mp4`).
  Plain `--play` works without it.

---

## Install

### Build from source

```bash
git clone https://github.com/Lamprozx/OpenCall
cd OpenCall
go build -o OpenCall .
```

### Prebuilt binaries

| Target | File |
|---|---|
| Linux amd64 | `OpenCall-linux-amd64` |
| Android arm64 (Termux) | `OpenCall-android-arm64` |

Termux:

```bash
chmod +x OpenCall-android-arm64
./OpenCall-android-arm64 auth --pair 6281234567890
```

> The Android build uses the NDK + cgo so DNS resolves natively (no
> `proot`/`resolv.conf` tricks needed). If you only need playback without FX or
> video, you can skip installing ffmpeg entirely.

---

## Quick start

```bash
# 1) Log in (QR code by default, or pairing code)
./OpenCall auth                          # scan QR
./OpenCall auth --pair 6281234567890     # or pair a number

# 2) Receive calls (answer/reject from the console)
./OpenCall listen

# 3) Place a call
./OpenCall call 6281234567890

# 4) Stream an audio file during a call
./OpenCall call 6281234567890 --play song.mp3

# 5) Auto-answer + play audio + record the peer
./OpenCall listen --auto-answer --play greeting.wav --record in.wav
```

---

## Commands

```
usage: OpenCall <command> [flags]

commands:
  auth [--pair <phone>]                log in and save a new session (QR by
                                       default, or --pair <phone>)
  auth list                            interactive list of saved sessions
  auth switch [<name>]                 switch the active session
  deauth                               delete a session (interactive picker)
  listen [--auto-answer] [--play a,b]  receive calls
  call <target> [--video] [--play a,b] place a 1:1 call
  group [--group-id <gid>] [--video]   group call (ad-hoc or whole group)
  group join [--group-id <gid>]        auto-join a running group call
  link create [--video]                create a reusable call link
  link preview <token-or-url> [--video] inspect a call link
  link join <token-or-url> [--video]   join a call link (waiting room aware)
  version                              print version/build info
```

### Playback options

| Flag | Description |
|---|---|
| `--play a,b,c` | Stream `.mp3`/`.wav`/`.opus` files, left to right. Decoded natively. |
| `--stream` | Read raw s16le mono 16 kHz PCM from stdin (live). |
| `--volume +5s` | Adjust volume (`+5s` = 5% louder, `-3s` = 3% quieter). |
| `--loop <N>` | Repeat each file N times. |
| `--video-file F` | Stream F as your camera (raw `.h264` as-is; other containers transcoded). |
| `--record out.wav` | Save peer audio. |
| `--record-video out.h264` | Save peer video (Annex-B H.264). |
| `--record-participant <jid>[:out]` | Save one group participant's video. |

### Audio effects (require ffmpeg)

```
--speed x       tempo 0.25–4 (1 = normal)
--pitch n       semitones -12..+12
--reverb l      1–10
--echo l        1–10
--bass db       -24..24 dB
--treble db     -24..24 dB
--lowpass hz    cutoff in Hz
--highpass hz   cutoff in Hz
--chorus l      1–10
--flanger l     1–10
--tremolo hz    rate in Hz
--vibrato hz    rate in Hz
--crusher l     bitcrusher 1–10
--fade-in s     fade in seconds
--fade-out s    fade out seconds
--reverse       play in reverse
```

### Log options (can appear anywhere)

```
--log-level <level>   trace|debug|info|warn|error|fatal
--quiet               shorthand for --log-level warn
--show-noise          keep background whatsmeow noise
--diag <dir>          write per-stream call diagnostics as JSONL
```

### Interactive console (during a call)

```
answer, reject, hangup, react <emoji>, video on|off, accept-video,
orientation <0-3>, handraise on|off, screenshare on|off, add <target>,
addmany <t...>, ring <target>, approval on|off, admit <user>, deny <user>,
pause, resume, stop, meter on|off, status, loglevel <level>, help, quit
```

---

## Building for Android (Termux)

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o OpenCall-linux-arm64 .
```

or, for a native Android build with cgo DNS (needs the NDK):

```bash
GOOS=android GOARCH=arm64 CGO_ENABLED=1 \
  CC=<ndk>/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android24-clang \
  go build -o OpenCall-android-arm64 .
```

---

## License

**GPL-3.0-or-later** — see [LICENSE](LICENSE).

This is required because OpenCall links
[`go.mau.fi/libsignal`](https://github.com/mautrix/libsignal), which is
GPL-3.0. Other dependencies (`whatsmeow` MPL-2.0, `meowcaller` MIT,
`modernc.org/sqlite` BSD-3) remain compatible.
