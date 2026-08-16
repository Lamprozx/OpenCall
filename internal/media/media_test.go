package media

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	meowcaller "github.com/purpshell/meowcaller"
	meowrtp "github.com/purpshell/meowcaller/rtp"
	ffmpeg "github.com/u2takey/ffmpeg-go"

	"opencall/internal/app"
)

func TestPlayOptionsGain(t *testing.T) {
	tests := []struct {
		volume string
		want   float32
		err    bool
	}{
		{"", 1, false},
		{"+5s", 1.05, false},
		{"-3s", 0.97, false},
		{"+5", 1.05, false},
		{"-3", 0.97, false},
		{"+100s", 2, false},
		{"-50s", 0.5, false},
		{"-150s", 0, false},
		{"+1000s", 10, false},
		{"abc", 0, true},
		{"+", 0, true},
		{"5x", 0, true},
	}
	for _, tt := range tests {
		p := &PlayOptions{volume: tt.volume}
		got, err := p.gain()
		if tt.err {
			if err == nil {
				t.Errorf("gain(%q): expected error, got %v", tt.volume, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("gain(%q): unexpected error %v", tt.volume, err)
			continue
		}
		if got != tt.want {
			t.Errorf("gain(%q): got %v, want %v", tt.volume, got, tt.want)
		}
	}
}

func TestPlayOptionsPlaylist(t *testing.T) {
	tests := []struct {
		files string
		want  []string
	}{
		{"", nil},
		{"  ", nil},
		{"sound1", []string{"sound1"}},
		{"sound1,sound2", []string{"sound1", "sound2"}},
		{" sound1 , sound2 ,sound3 ", []string{"sound1", "sound2", "sound3"}},
		{"a,,b,", []string{"a", "b"}},
		{"a, b ,c", []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		p := &PlayOptions{files: tt.files}
		got := p.playlist()
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("playlist(%q): got %v, want %v", tt.files, got, tt.want)
		}
	}
}

func TestPlayOptionsValidate(t *testing.T) {
	for _, loop := range []int{-1, 1, 10} {
		p := &PlayOptions{volume: "+10s", loop: loop}
		if err := p.Validate(); err != nil {
			t.Errorf("loop=%d should be valid: %v", loop, err)
		}
	}
	for _, loop := range []int{0, -2, -100} {
		p := &PlayOptions{loop: loop}
		if err := p.Validate(); err == nil {
			t.Errorf("loop=%d should be rejected", loop)
		}
	}
	if err := (&PlayOptions{volume: "oops", loop: -1}).Validate(); err == nil {
		t.Error("invalid volume accepted")
	}
	if err := (&PlayOptions{loop: -1}).Validate(); err != nil {
		t.Errorf("empty volume rejected: %v", err)
	}
}

func TestPlayOptionsStreamValidate(t *testing.T) {
	if err := (&PlayOptions{Stream: true, loop: -1}).Validate(); err != nil {
		t.Errorf("stream alone should be valid: %v", err)
	}
	if err := (&PlayOptions{Stream: true, files: "a.mp3", loop: -1}).Validate(); err == nil {
		t.Error("stream + play should be rejected")
	}
	if err := (&PlayOptions{Stream: true, loop: 3}).Validate(); err == nil {
		t.Error("stream + loop should be rejected")
	}
}

func TestMultiSink(t *testing.T) {
	var a, b int
	fa := meowcaller.SinkFunc(func([]float32) { a++ })
	fb := meowcaller.SinkFunc(func([]float32) { b++ })
	m := &multiSink{}
	idA := m.add(fa)
	m.add(fb)
	frame := []float32{0.1, 0.2}
	if err := m.WriteFrame(frame); err != nil {
		t.Fatal(err)
	}
	if a != 1 || b != 1 {
		t.Errorf("both sinks should receive the frame: a=%d b=%d", a, b)
	}
	m.remove(idA)
	if err := m.WriteFrame(frame); err != nil {
		t.Fatal(err)
	}
	if a != 1 || b != 2 {
		t.Errorf("after remove: a=%d (want 1) b=%d (want 2)", a, b)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFrame(frame); err != nil {
		t.Fatal(err)
	}
	if b != 2 {
		t.Errorf("writes after close must be dropped: b=%d", b)
	}
}

func TestMeterSink(t *testing.T) {
	m := newMeterSink()
	loud := make([]float32, 960)
	for i := range loud {
		loud[i] = 0.9
	}
	for i := 0; i < 20; i++ {
		if err := m.WriteFrame(loud); err != nil {
			t.Fatal(err)
		}
	}
	lvl, peak := m.Level(), m.Peak()
	if lvl <= 0 || lvl > 1 {
		t.Errorf("level out of range: %v", lvl)
	}
	if peak <= 0 || peak > 1 {
		t.Errorf("peak out of range: %v", peak)
	}
	if peak < lvl {
		t.Errorf("peak %v should not be below level %v", peak, lvl)
	}
	silent := make([]float32, 960)
	for i := 0; i < 200; i++ {
		if err := m.WriteFrame(silent); err != nil {
			t.Fatal(err)
		}
	}
	if m.Level() >= lvl {
		t.Errorf("level should decay after silence: %v -> %v", lvl, m.Level())
	}
}

func TestParticipantRecorder(t *testing.T) {
	dir := t.TempDir()
	spec := "6281234567890:" + filepath.Join(dir, "part.h264")
	r, err := newParticipantRecorder(spec)
	if err != nil {
		t.Fatalf("newParticipantRecorder: %v", err)
	}
	defer r.close()
	if r.path != filepath.Join(dir, "part.h264") {
		t.Errorf("path = %q", r.path)
	}
	if !r.matches("6281234567890@s.whatsapp.net") {
		t.Error("bare phone should match full JID user part")
	}
	if r.matches("6289999999999@s.whatsapp.net") {
		t.Error("different phone should not match")
	}
	if err := r.write([]byte{0x00, 0x00, 0x00, 0x01, 0x65}); err != nil {
		t.Fatal(err)
	}
	if err := r.close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(r.path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 5 {
		t.Errorf("file has %d bytes, want 5", len(data))
	}

	r2, err := newParticipantRecorder("628111@s.whatsapp.net")
	if err != nil {
		t.Fatalf("default filename: %v", err)
	}
	defer func() {
		r2.close()
		os.Remove(r2.path)
	}()
	if r2.path != "participant-628111_s.whatsapp.net.h264" {
		t.Errorf("default path = %q", r2.path)
	}
	if !r2.matches("628111@s.whatsapp.net") {
		t.Error("full JID should match itself")
	}
	if _, err := newParticipantRecorder(":out.h264"); err == nil {
		t.Error("empty participant id should be rejected")
	}
}

func TestRegisterLoopFlag(t *testing.T) {
	fs := flag.NewFlagSet("call", flag.ContinueOnError)
	p := &PlayOptions{}
	p.Register(fs)
	if err := fs.Parse(app.ReorderArgs([]string{"6281", "--loop", "3"})); err != nil {
		t.Fatalf("--loop 3 should parse: %v", err)
	}
	if p.loop != 3 {
		t.Errorf("loop = %d, want 3", p.loop)
	}

	fs2 := flag.NewFlagSet("call", flag.ContinueOnError)
	p2 := &PlayOptions{}
	p2.Register(fs2)
	if err := fs2.Parse(app.ReorderArgs([]string{"6281", "--loop"})); err == nil {
		t.Error("--loop without value should fail to parse")
	}

	fs3 := flag.NewFlagSet("call", flag.ContinueOnError)
	p3 := &PlayOptions{}
	p3.Register(fs3)
	if err := fs3.Parse(app.ReorderArgs([]string{"6281", "--loop", "abc"})); err == nil {
		t.Error("--loop abc should fail to parse")
	}
}

func TestConvertAudioToWAV(t *testing.T) {
	src := "hidup-jokowi.mp3"
	if _, err := os.Stat(src); err != nil {
		t.Skipf("test fixture %s not present: %v", src, err)
	}
	out, cleanup, err := convertAudioToWAV(src, 1, nil)
	if err != nil {
		t.Fatalf("convertAudioToWAV: %v", err)
	}
	if cleanup == nil {
		t.Fatal("expected a cleanup func")
	}
	if filepath.Ext(out) != ".wav" {
		t.Errorf("output should be a .wav, got %q", out)
	}
	srcAudio, err := openAudioFile(out)
	if err != nil {
		t.Fatalf("converted file not playable: %v", err)
	}
	frame, err := srcAudio.ReadFrame()
	if err != nil {
		t.Fatalf("read first frame: %v", err)
	}
	if len(frame) != 960 {
		t.Errorf("expected 960 samples per frame, got %d", len(frame))
	}
	srcAudio.Close()

	cleanup()
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("temp file %q should be removed by cleanup", out)
	}
}

func TestConvertPlaylistPartialFailure(t *testing.T) {
	files := []string{"/nonexistent-1.mp3", "/nonexistent-2.mp3"}
	if _, _, err := convertPlaylist(files, 1, nil); err == nil {
		t.Error("convertPlaylist should fail when first file is missing")
	}
	src := "hidup-jokowi.mp3"
	if _, err := os.Stat(src); err != nil {
		skipFixture(t, src)
	}
	_, _, err := convertPlaylist([]string{src, "/nonexistent-2.mp3"}, 1, nil)
	if err == nil {
		t.Fatal("convertPlaylist should fail on missing second file")
	}
}

func TestReorderArgsWithFlagSet(t *testing.T) {
	fs := flag.NewFlagSet("call", flag.ContinueOnError)
	play := &PlayOptions{}
	play.Register(fs)
	args := []string{"628123456789", "--play", "s1.mp3,s2.wav", "--volume", "-3s", "--loop", "4"}
	if err := fs.Parse(app.ReorderArgs(args)); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if fs.NArg() != 1 || fs.Arg(0) != "628123456789" {
		t.Errorf("positional wrong: narg=%d args=%v", fs.NArg(), fs.Args())
	}
	if got := play.playlist(); !reflect.DeepEqual(got, []string{"s1.mp3", "s2.wav"}) {
		t.Errorf("playlist = %v", got)
	}
	if play.loop != 4 {
		t.Errorf("loop = %d, want 4", play.loop)
	}
	if g, _ := play.gain(); g != 0.97 {
		t.Errorf("gain = %v, want 0.97", g)
	}
}

func TestConvertAudioToWAVLoop(t *testing.T) {
	src := "hidup-jokowi.mp3"
	if _, err := os.Stat(src); err != nil {
		skipFixture(t, src)
	}

	single, cleanup1, err := convertAudioToWAV(src, 1, nil)
	if err != nil {
		t.Fatalf("convert loop=1: %v", err)
	}
	defer cleanup1()
	repeated, cleanup2, err := convertAudioToWAV(src, 4, nil)
	if err != nil {
		t.Fatalf("convert loop=4: %v", err)
	}
	defer cleanup2()

	frames := func(path string) int {
		srcAudio, err := openAudioFile(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		defer srcAudio.Close()
		n := 0
		for {
			_, err := srcAudio.ReadFrame()
			if err != nil {
				break
			}
			n++
		}
		return n
	}
	singleFrames := frames(single)
	repeatedFrames := frames(repeated)
	if singleFrames == 0 {
		t.Fatal("loop=1 conversion produced no frames")
	}
	want := singleFrames * 4
	if repeatedFrames < want-100 || repeatedFrames > want+100 {
		t.Errorf("loop=4 frames = %d, want ~%d", repeatedFrames, want)
	}
}

func TestCallStateVideoCleanup(t *testing.T) {
	s := NewCallState()
	n := 0
	s.setVideoCleanup(func() { n++ })
	s.RunVideoCleanup()
	s.RunVideoCleanup()
	if n != 1 {
		t.Errorf("cleanup ran %d times, want 1", n)
	}
	s.Clear()
	s.RunVideoCleanup()
	if n != 1 {
		t.Errorf("cleanup ran %d times after Clear, want 1", n)
	}
}

func TestCallStateReadyFns(t *testing.T) {
	s := NewCallState()
	var order []string
	s.addReady(func() { order = append(order, "a") })
	s.addReady(func() { order = append(order, "b") })
	s.RunReady()
	s.RunReady()
	if !reflect.DeepEqual(order, []string{"a", "b"}) {
		t.Errorf("ready fns ran %v, want [a b]", order)
	}
}

func TestCallStateVideoCleanupAfterEnd(t *testing.T) {
	s := NewCallState()
	s.Clear()
	n := 0
	s.setVideoCleanup(func() { n++ })
	if n != 1 {
		t.Errorf("cleanup after end ran %d times, want 1 (immediate)", n)
	}
	s.RunVideoCleanup()
	if n != 1 {
		t.Errorf("cleanup after end ran %d times total, want 1", n)
	}
}

func TestLooksLikeAnnexB(t *testing.T) {
	raw := filepath.Join(t.TempDir(), "cam.h264")
	if err := os.WriteFile(raw, []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x1f, 0xe0, 0x01}, 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err := looksLikeAnnexB(raw)
	if err != nil {
		t.Fatalf("looksLikeAnnexB(raw): %v", err)
	}
	if !ok {
		t.Error("raw Annex-B stream not detected")
	}

	mp4 := filepath.Join(t.TempDir(), "cam.mp4")
	if err := os.WriteFile(mp4, []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err = looksLikeAnnexB(mp4)
	if err != nil {
		t.Fatalf("looksLikeAnnexB(mp4): %v", err)
	}
	if ok {
		t.Error("mp4 container misdetected as Annex-B")
	}
}

func TestConvertVideoToAnnexB(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "cam.mp4")
	err := ffmpeg.Input("color=c=blue:s=320x240:d=1", ffmpeg.KwArgs{"f": "lavfi"}).
		Output(src, ffmpeg.KwArgs{"c:v": "libx264", "preset": "ultrafast", "pix_fmt": "yuv420p"}).
		OverWriteOutput().
		Run()
	if err != nil {
		t.Fatalf("generate fixture mp4: %v", err)
	}

	out, transcoded, cleanup, err := convertVideoToAnnexB(src)
	if err != nil {
		t.Fatalf("convertVideoToAnnexB: %v", err)
	}
	if cleanup == nil {
		t.Fatal("expected a cleanup func")
	}
	if out == src {
		t.Error("mp4 should have been transcoded, not passed through")
	}
	if !transcoded {
		t.Error("mp4 should report transcoded=true")
	}
	raw, err := looksLikeAnnexB(out)
	if err != nil || !raw {
		t.Errorf("converted output is not Annex-B (ok=%v err=%v)", raw, err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("converted output is empty")
	}
	if !hasAccessUnitDelimiters(data) {
		t.Error("converted output has no AUD access-unit delimiters")
	}
	aus := splitAccessUnits(data)
	if len(aus) == 0 {
		t.Fatal("converted output produced no access units")
	}
	for i, au := range aus {
		if len(meowrtp.SplitAnnexB(au)) == 0 {
			t.Errorf("access unit %d is unparseable by meowcaller's Annex-B parser", i)
		}
	}
	if !meowrtp.AUHasIDR(aus[0]) {
		t.Error("first access unit does not contain an IDR keyframe")
	}

	cleanup()
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("temp file %q should be removed by cleanup", out)
	}
}

func TestConvertVideoToAnnexBPassthrough(t *testing.T) {
	unframed := filepath.Join(t.TempDir(), "cam.h264")
	err := ffmpeg.Input("color=c=blue:s=320x240:d=1", ffmpeg.KwArgs{"f": "lavfi"}).
		Output(unframed, ffmpeg.KwArgs{"c:v": "libx264", "preset": "ultrafast", "pix_fmt": "yuv420p", "f": "h264"}).
		OverWriteOutput().
		Run()
	if err != nil {
		t.Fatalf("generate unframed h264 fixture: %v", err)
	}
	rawData, err := os.ReadFile(unframed)
	if err != nil {
		t.Fatal(err)
	}
	if hasAccessUnitDelimiters(rawData) {
		t.Fatal("fixture unexpectedly already has AUDs")
	}
	out, transcoded, cleanup, err := convertVideoToAnnexB(unframed)
	if err != nil {
		t.Fatalf("convertVideoToAnnexB(unframed raw): %v", err)
	}
	if out == unframed {
		t.Error("unframed raw h264 should be remuxed, not passed through")
	}
	if transcoded {
		t.Error("remux should not report transcoded=true")
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !hasAccessUnitDelimiters(data) {
		t.Error("remuxed raw h264 has no AUD delimiters")
	}
	cleanup()
	if _, err := os.Stat(unframed); err != nil {
		t.Errorf("original raw file must not be removed: %v", err)
	}

	framed := filepath.Join(t.TempDir(), "framed.h264")
	framedData := []byte{
		0x00, 0x00, 0x00, 0x01, 0x09, 0x10,
		0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x1f, 0xe0, 0x01,
		0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84,
	}
	if err := os.WriteFile(framed, framedData, 0o644); err != nil {
		t.Fatal(err)
	}
	out2, transcoded2, cleanup2, err := convertVideoToAnnexB(framed)
	if err != nil {
		t.Fatalf("convertVideoToAnnexB(framed raw): %v", err)
	}
	if out2 != framed {
		t.Errorf("framed raw h264 should pass through unchanged, got %q", out2)
	}
	if transcoded2 {
		t.Error("framed passthrough should not report transcoded=true")
	}
	cleanup2()
	if _, err := os.Stat(framed); err != nil {
		t.Errorf("original framed file must not be removed: %v", err)
	}
}

func TestSplitAccessUnits(t *testing.T) {
	data := []byte{
		0x00, 0x00, 0x00, 0x01, 0x09, 0x10,
		0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x1f, 0xe0, 0x01,
		0x00, 0x00, 0x00, 0x01, 0x68, 0xce, 0x38, 0x80,
		0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84,
		0x00, 0x00, 0x00, 0x01, 0x09, 0x10,
		0x00, 0x00, 0x00, 0x01, 0x41, 0x9a, 0x08,
		0x00, 0x00, 0x00, 0x01, 0x09, 0x10,
		0x00, 0x00, 0x00, 0x01, 0x41, 0x9b, 0x08,
	}
	aus := splitAccessUnits(data)
	if len(aus) != 3 {
		t.Fatalf("splitAccessUnits produced %d units, want 3", len(aus))
	}
	for i, au := range aus {
		if !isStartCode(au, 0) {
			t.Errorf("unit %d does not start with an Annex-B start code", i)
		}
		if len(meowrtp.SplitAnnexB(au)) == 0 {
			t.Errorf("unit %d is unparseable by meowcaller's Annex-B parser", i)
		}
	}
	if !meowrtp.AUHasIDR(aus[0]) {
		t.Error("first unit should contain the IDR keyframe")
	}
	if meowrtp.AUHasIDR(aus[1]) || meowrtp.AUHasIDR(aus[2]) {
		t.Error("delta units should not contain an IDR")
	}
	noAud := []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x00, 0x00, 0x01, 0x65, 0x88}
	if aus := splitAccessUnits(noAud); len(aus) != 1 {
		t.Errorf("AUD-less stream produced %d units, want 1", len(aus))
	}
}

func TestHasAccessUnitDelimiters(t *testing.T) {
	withAUD := []byte{0x00, 0x00, 0x00, 0x01, 0x09, 0x10, 0x00, 0x00, 0x00, 0x01, 0x67, 0x42}
	if !hasAccessUnitDelimiters(withAUD) {
		t.Error("stream with an AUD should be detected")
	}
	without := []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x00, 0x00, 0x01, 0x65, 0x88}
	if hasAccessUnitDelimiters(without) {
		t.Error("stream without an AUD should not be detected")
	}
}

func skipFixture(t *testing.T, path string) {
	t.Helper()
	t.Skipf("test fixture %s not present", path)
}

func TestRepeatPlaylist(t *testing.T) {
	got := repeatPlaylist([]string{"a.mp3", "b.wav"}, 3)
	want := []string{"a.mp3", "a.mp3", "a.mp3", "b.wav", "b.wav", "b.wav"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("repeatPlaylist = %v, want %v", got, want)
	}
	if got := repeatPlaylist([]string{"a.mp3"}, 1); !reflect.DeepEqual(got, []string{"a.mp3"}) {
		t.Errorf("repeatPlaylist(n=1) = %v, want single entry", got)
	}
}
