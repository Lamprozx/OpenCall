package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

func convertAudioToWAV(src string, loop int, fx *fxOptions) (string, func(), error) {
	dir, _, err := tempDir()
	if err != nil {
		return "", nil, err
	}
	tmp, err := os.CreateTemp(dir, "opencall-play-*.wav")
	if err != nil {
		return "", nil, fmt.Errorf("create temp wav: %w", err)
	}
	out := tmp.Name()
	tmp.Close()

	var inOpts ffmpeg.KwArgs
	if loop > 1 {
		inOpts = ffmpeg.KwArgs{"stream_loop": strconv.Itoa(loop - 1)}
	}
	outKw := ffmpeg.KwArgs{"ar": 16000, "ac": 1, "f": "wav"}
	if fx != nil {
		if af := fx.af(); af != "" {
			outKw["af"] = af
		}
	}
	var stderr bytes.Buffer
	err = ffmpeg.Input(src, inOpts).
		Output(out, outKw).
		OverWriteOutput().
		WithErrorOutput(&stderr).
		Run()
	if err != nil {
		os.Remove(out)
		return "", nil, fmt.Errorf("ffmpeg convert %q: %w: %s", src, err, stderr.String())
	}
	return out, func() { os.Remove(out) }, nil
}

func looksLikeAnnexB(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	buf := make([]byte, 16)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return false, err
	}
	buf = buf[:n]
	for _, off := range []int{0, 1} {
		if isStartCode(buf, off) {
			after := off + startCodeLen(buf, off)
			if after < len(buf) && buf[after] < 0x80 {
				return true, nil
			}
		}
	}
	return false, nil
}

func hasAccessUnitDelimiters(data []byte) bool {
	i := 0
	for i < len(data) {
		for i < len(data) && !isStartCode(data, i) {
			i++
		}
		if i >= len(data) {
			break
		}
		i += startCodeLen(data, i)
		if i < len(data) && data[i]&0x1f == 9 {
			return true
		}
	}
	return false
}

func convertVideoToAnnexB(src string) (string, bool, func(), error) {
	raw, err := looksLikeAnnexB(src)
	if err != nil {
		return "", false, nil, fmt.Errorf("read video file %q: %w", src, err)
	}
	if raw {
		if data, err := os.ReadFile(src); err == nil && hasAccessUnitDelimiters(data) {
			return src, false, func() {}, nil
		}
	}

	dir, _, err := tempDir()
	if err != nil {
		return "", false, nil, err
	}
	tmp, err := os.CreateTemp(dir, "opencall-video-*.h264")
	if err != nil {
		return "", false, nil, fmt.Errorf("create temp h264: %w", err)
	}
	out := tmp.Name()
	tmp.Close()

	args := ffmpeg.KwArgs{
		"bsf:v": "h264_metadata=aud=insert",
		"f":     "h264",
	}
	if raw {
		args["c:v"] = "copy"
	} else {
		args["c:v"] = "libx264"
		args["profile:v"] = "baseline"
		args["preset"] = "veryfast"
		args["pix_fmt"] = "yuv420p"
		args["r"] = "30"
		args["g"] = "60"
		args["an"] = ""
	}
	var in *ffmpeg.Stream
	if raw {
		in = ffmpeg.Input(src, ffmpeg.KwArgs{"f": "h264"})
	} else {
		in = ffmpeg.Input(src)
	}
	var stderr bytes.Buffer
	err = in.
		Output(out, args).
		OverWriteOutput().
		WithErrorOutput(&stderr).
		Run()
	if err != nil {
		os.Remove(out)
		return "", false, nil, fmt.Errorf("ffmpeg convert video %q: %w: %s", src, err, stderr.String())
	}
	return out, !raw, func() { os.Remove(out) }, nil
}

func convertPlaylist(files []string, loop int, fx *fxOptions) ([]string, func(), error) {
	converted := make([]string, 0, len(files))
	for _, f := range files {
		out, _, err := convertAudioToWAV(f, loop, fx)
		if err != nil {
			for _, c := range converted {
				os.Remove(c)
			}
			return nil, nil, err
		}
		converted = append(converted, out)
	}
	cleanup := func() {
		for _, c := range converted {
			os.Remove(c)
		}
	}
	return converted, cleanup, nil
}

// repeatPlaylist returns a playlist where each file is repeated n times in place,
// matching the ffmpeg stream_loop behavior (--loop N plays each file N times).
func repeatPlaylist(files []string, n int) []string {
	out := make([]string, 0, len(files)*n)
	for _, f := range files {
		for i := 0; i < n; i++ {
			out = append(out, f)
		}
	}
	return out
}
