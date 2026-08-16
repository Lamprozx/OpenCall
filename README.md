# OpenCall

[![License: GPL-3.0-or-later](https://img.shields.io/badge/license-GPL--3.0--or--later-blue.svg)](LICENSE)
[![Go version](https://img.shields.io/badge/Go-1.25%2B-00ADD8.svg?logo=go&logoColor=white)](#requirements)
[![Release](https://img.shields.io/github/v/release/Lamprozx/OpenCall?label=release)](https://github.com/Lamprozx/OpenCall/releases)
![Termux Support](https://img.shields.io/badge/Termux-Support-000000?style=flat&logo=gnu-bash&logoColor=white)





A WhatsApp calling CLI for the terminal. Place and receive 1:1 and group calls,
stream audio/video, apply real-time audio effects, join call links, and manage
multiple sessions — all from your console.

Built on [`whatsmeow`](https://github.com/mautrix/whatsmeow) for
authentication/session management and
[`meowcaller`](https://github.com/purpshell/meowcaller) for the call/media layer.

> **Disclaimer** — This is an independent project, not affiliated with WhatsApp
> or Meta. Using unofficial clients can get a number banned. Use a number you
> can afford to lose, and comply with WhatsApp's Terms of Service. See
> [Troubleshooting & FAQ](#troubleshooting--faq) for mitigation tips.

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

  If you use effects or `--video-file` without ffmpeg installed, OpenCall
  detects it and offers to download + install the minimal build for you
  automatically.

  Instead of the full `ffmpeg` package (~347 MB on Termux), use the custom
  minimal build: **[ffmpeg-minimal](https://github.com/Lamprozx/ffmpeg-minimal)**
  — a ~3–4 MB static binary with only what OpenCall needs (mp3/wav/opus decode,
  libx264, and the audio filters).

Note : opencall will ask you to install `ffmpeg-minimal` automatically to your system if you don't have ffmpeg installed before 


---

## Quick Start

Pick your platform. The asset names match the release exactly.

### Termux (Android, arm64)

```bash
wget https://github.com/Lamprozx/OpenCall/releases/download/v1.0.0/OpenCall-android-arm64
chmod +x OpenCall-android-arm64
./OpenCall-android-arm64 auth --pair 628xxxxxxxxxx
```

### Linux (amd64)

```bash
wget https://github.com/Lamprozx/OpenCall/releases/download/v1.0.0/OpenCall-linux-amd64
chmod +x OpenCall-linux-amd64
sudo mv OpenCall-linux-amd64 /usr/local/bin/opencall
opencall auth --pair <phone>
```

Then:

```bash
# 1) Log in (QR code by default, or pairing code)
./OpenCall auth                          # scan QR
./OpenCall auth --pair 6281234567890     # or pair a number

# 2) Receive calls (answer/reject from the console)
./OpenCall listen

# 3) Place a call
./OpenCall call 6281234567890
```

> The Android build uses the NDK + cgo so DNS resolves natively (no
> `proot`/`resolv.conf` tricks needed). If you only need playback without FX or
> video, you can skip installing ffmpeg entirely.

---

## Build from source

```bash
git clone https://github.com/Lamprozx/OpenCall
cd OpenCall
make build            # native binary for this host -> ./OpenCall
```

Or with plain `go build`:

```bash
go build -o OpenCall ./cmd/opencall
```

Prebuilt binaries:

| Target | File |
|---|---|
| Linux amd64 | `OpenCall-linux-amd64` |
| Android arm64 (Termux) | `OpenCall-android-arm64` |

---

## Configuration

OpenCall is configured through environment variables; there is no config file.

| Variable | Purpose |
|---|---|
| `MEOW_LOG_LEVEL` | Default log level: `trace`, `debug`, `info`, `warn`, `error`, `fatal`. The `--log-level` flag overrides it. |
| `PREFIX` | On Termux, used to locate `$PREFIX/bin` when auto-installing ffmpeg. Normally set by Termux automatically. |
| `ANDROID_ROOT` | Detected automatically to identify an Android/Termux environment. You should not need to set it. |

**Session storage** — sessions live in a `sessions/` directory relative to the
current working directory:

```
sessions/
├── registry.json        # index of saved sessions (name, phone, last used)
└── <id>/
    └── wa-voip.db       # per-session WhatsApp device store (SQLite)
```

Run OpenCall from a stable directory if you want to keep the same sessions
between invocations.

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
| `--video-file F` | Stream F as your camera — implies a video call (raw `.h264` as-is; other containers transcoded). |
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

## Examples

### Stream an audio file

```bash
./OpenCall call 6281234567890 --play song.mp3
./OpenCall listen --auto-answer --play greeting.wav --record in.wav
```

### Stream live PCM from stdin

`--stream` reads raw s16le mono 16 kHz PCM from stdin — for example, a live
microphone via `arecord`:

```bash
arecord -f S16_LE -r 16000 -c 1 | ./OpenCall call 6281234567890 --stream
```

### Video call with a pre-recorded file

```bash
# --video-file implies a video call, so --video is optional
./OpenCall call 6281234567890 --video --video-file cam.mp4
```

### Group calls

```bash
# Ad-hoc group call: list 2+ targets explicitly
./OpenCall group 628111111111 628222222222 --play music.mp3

# Whole group: ring every member of a WhatsApp group
./OpenCall group --group-id <group-id-or-@g.us-jid> --video
```

> `group` needs either `--group-id <gid>` **or** at least two explicit targets.
> With `--group-id` it binds every member of that WhatsApp group; without it,
> it builds an ad-hoc group from the targets you list.

### Join a running group call

```bash
# Interactive picker of your joined groups
./OpenCall group join

# Only join calls from a specific group
./OpenCall group join --group-id <group-id-or-@g.us-jid>
```

### Call links

```bash
./OpenCall link create                          # prints a shareable URL
./OpenCall link preview https://call.whatsapp.com/...   # inspect without joining
./OpenCall link join https://call.whatsapp.com/... --play music.mp3
```

---

## Project structure

```
cmd/opencall/        entry point + CLI command wiring (package main)
internal/app/        shared kernel: logger, diag, noise filter, tmpdir, arg helpers
internal/call/       call orchestration: connect, events, interactive console
internal/console/    raw-mode terminal UI (ConsoleUI + Picker)
internal/media/      media pipeline: play options, fx, ffmpeg, call state
internal/session/    multi-session registry + auth flows
```

---

## Building for Android (Termux)

Use the Makefile — it cross-compiles with the NDK so DNS resolves natively in
Termux (no `proot`/`resolv.conf` tricks needed).

```bash
# Linux amd64 (static)
make amd64

# Android arm64 (Termux). Requires the NDK (r26d recommended).
make arm64 ANDROID_NDK_HOME=/path/to/android-ndk-r26d
```

`make` (or `make all`) builds both. The prebuilt `OpenCall-android-arm64` binary
targets **Android 7.0 (API 24) or newer** (built with the `android24` NDK
toolchain). WhatsApp's minimum Android version is lower, but the binary itself
requires API 24+.

Other targets: `make build`, `make test`, `make vet`, `make clean`.

---

## Troubleshooting & FAQ

**Q: Will my number get banned?**
Possibly — unofficial clients violate WhatsApp's Terms of Service. To lower the
risk: use a spare number you can afford to lose, warm the account up with normal
usage first, avoid mass/spam calling, add delays between calls, and don't run
multiple clients on the same number simultaneously.

**Q: DNS lookup fails on Termux (`lookup web.whatsapp.com ... connection refused`).**
Use the `OpenCall-android-arm64` build (NDK + cgo). It uses Android's native
resolver, so you don't need `resolv-conf` or `proot`.

**Q: I get `ffmpeg not found ...`.**
That's expected — ffmpeg is only required for audio effects and non-raw video.
OpenCall will offer to download and install the minimal
[ffmpeg-minimal](https://github.com/Lamprozx/ffmpeg-minimal) build
automatically. Plain `--play` works without it.

**Q: My `--video-file` doesn't show video on the other side.**
Make sure the call is actually a video call. `--video-file` implies `--video`,
but on older builds you may need to pass `--video` explicitly:
`./OpenCall call <target> --video --video-file cam.mp4`.

**Q: The ffmpeg transcode seems to hang.**
That's normal for large/high-resolution videos — it's CPU-heavy, especially on
a phone. Use a shorter or lower-resolution video, or pre-convert to raw
`.h264` (Annex-B) so it's streamed without transcoding.

---

## Contributing

Pull requests are welcome. Before opening one:

1. Run `gofmt -l .` (should print nothing), `make vet`, and `make test`.
2. Keep changes focused and follow the existing package layout above.
3. Add or update tests for behavior changes.
4. For feature ideas, open an issue first to discuss the approach.

This project is **GPL-3.0-or-later** (it links GPL-3.0 `libsignal`). By
contributing you agree to license your changes under the same terms.

---

## License

**GPL-3.0-or-later** — see [LICENSE](LICENSE).
