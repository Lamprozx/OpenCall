package media

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	meowcaller "github.com/purpshell/meowcaller"
	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow/types"

	"opencall/internal/console"
)

// PlayOptions configures audio playback for a call.
type PlayOptions struct {
	files  string
	volume string
	loop   int
	Stream bool
	fx     *fxOptions
}

// MediaConfig bundles playback, recording, and video-file settings.
type MediaConfig struct {
	play              *PlayOptions
	record            string
	recordVideo       string
	videoFile         string
	recordParticipant string
}

func NewMediaConfig(play *PlayOptions, record, recordVideo, videoFile, recordParticipant string) *MediaConfig {
	return &MediaConfig{
		play:              play,
		record:            record,
		recordVideo:       recordVideo,
		videoFile:         videoFile,
		recordParticipant: recordParticipant,
	}
}

func (p *PlayOptions) playlist() []string {
	var files []string
	for _, f := range strings.Split(p.files, ",") {
		if f = strings.TrimSpace(f); f != "" {
			files = append(files, f)
		}
	}
	return files
}

func (p *PlayOptions) gain() (float32, error) {
	v := strings.TrimSpace(p.volume)
	if v == "" {
		return 1, nil
	}
	v = strings.TrimSuffix(strings.TrimSuffix(v, "s"), "S")
	pct, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid volume %q (use e.g. +5s or -3s)", p.volume)
	}
	if pct < -100 {
		pct = -100
	}
	if pct > 900 {
		pct = 900
	}
	return float32(1 + pct/100), nil
}

func (p *PlayOptions) Validate() error {
	if _, err := p.gain(); err != nil {
		return err
	}
	if p.loop != -1 && p.loop < 1 {
		return fmt.Errorf("--loop requires a positive integer value (e.g. --loop 10)")
	}
	if p.Stream {
		if p.files != "" {
			return fmt.Errorf("--stream and --play are mutually exclusive")
		}
		if p.loop != -1 {
			return fmt.Errorf("--loop cannot be used with --stream")
		}
		if console.TermUI != nil {
			return fmt.Errorf("--stream reads raw PCM from stdin, but stdin is a terminal — pipe audio in, e.g. arecord -f S16_LE -r 16000 -c 1 | OpenCall call <target> --stream")
		}
	}
	if p.fx != nil {
		if p.Stream && !p.fx.empty() {
			return fmt.Errorf("audio effects cannot be combined with --stream")
		}
		if err := p.fx.validate(); err != nil {
			return err
		}
	}
	return nil
}

// RequiresFFmpeg reports whether this playback config needs ffmpeg
// (audio effects are applied via ffmpeg).
func (p *PlayOptions) RequiresFFmpeg() bool {
	return p.fx != nil && !p.fx.empty()
}

func (p *PlayOptions) Register(fs *flag.FlagSet) {
	fs.StringVar(&p.files, "play", "", "stream .mp3/.wav/.opus files, comma-separated, played left to right")
	fs.StringVar(&p.volume, "volume", "", "volume adjustment, e.g. +5s (5% louder) or -3s (3% quieter)")
	fs.IntVar(&p.loop, "loop", -1, "repeat each file N times (required positive integer, e.g. --loop 10)")
	fs.BoolVar(&p.Stream, "stream", false, "stream raw s16le mono 16 kHz PCM from stdin instead of --play files (pipe audio in)")
	if p.fx == nil {
		p.fx = &fxOptions{}
	}
	p.fx.register(fs)
}

type playlistSource struct {
	files   []string
	idx     int
	gain    float32
	cur     meowcaller.AudioSource
	cleanup func()
}

func newPlaylistSource(files []string, gain float32, cleanup func()) (*playlistSource, error) {
	s := &playlistSource{files: files, gain: gain, cleanup: cleanup}
	if len(files) == 0 {
		return s, nil
	}
	src, err := openAudioFile(files[0])
	if err != nil {
		return nil, err
	}
	s.cur = src
	return s, nil
}

func (s *playlistSource) ReadFrame() ([]float32, error) {
	for {
		if s.cur == nil {
			if s.idx >= len(s.files) {
				return nil, io.EOF
			}
			src, err := openAudioFile(s.files[s.idx])
			if err != nil {
				return nil, err
			}
			s.cur = src
		}
		frame, err := s.cur.ReadFrame()
		if err == io.EOF {
			_ = s.cur.Close()
			s.cur = nil
			s.idx++
			continue
		}
		if err != nil {
			return nil, err
		}
		if s.gain != 1 {
			for i := range frame {
				frame[i] *= s.gain
			}
		}
		return frame, nil
	}
}

func (s *playlistSource) Close() error {
	var err error
	if s.cur != nil {
		err = s.cur.Close()
		s.cur = nil
	}
	if s.cleanup != nil {
		s.cleanup()
		s.cleanup = nil
	}
	return err
}

func openAudioFile(path string) (meowcaller.AudioSource, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		return meowcaller.MP3File(path)
	case ".wav":
		return meowcaller.WAVFile(path)
	case ".opus":
		return meowcaller.OpusFile(path)
	default:
		return nil, fmt.Errorf("unsupported audio file %q (use .mp3/.wav/.opus)", path)
	}
}

func attachMedia(ctx context.Context, call *meowcaller.Call, state *CallState, cfg *MediaConfig) {
	log := zerolog.Ctx(ctx)
	if cfg == nil {
		return
	}
	if cfg.play != nil && cfg.play.Stream {
		state.SetStdinStreaming(true)
	}
	if cfg.recordParticipant != "" {
		rec, err := newParticipantRecorder(cfg.recordParticipant)
		if err != nil {
			log.Error().Err(err).Msg("open participant recorder")
		} else {
			state.setParticipantRecorder(rec)
			log.Info().Str("participant", rec.target).Str("file", rec.path).
				Msg("recording participant video (Annex-B h264)")
		}
	}
	multi := &multiSink{}
	call.Receive(multi)
	state.setMultiSink(multi)
	if cfg.record != "" {
		if sink, err := meowcaller.WAVRecorder(cfg.record); err == nil {
			multi.add(sink)
			log.Info().Str("file", cfg.record).Msg("recording peer audio (16 kHz mono wav)")
		} else {
			log.Error().Err(err).Msg("open wav recorder")
		}
	}
	if cfg.recordVideo != "" {
		if sink, err := meowcaller.AnnexBRecorder(cfg.recordVideo); err == nil {
			call.ReceiveVideo(sink)
			log.Info().Str("file", cfg.recordVideo).Msg("recording peer video (Annex-B h264)")
		} else {
			log.Error().Err(err).Msg("open video recorder")
		}
	}
	if cfg.play != nil && cfg.play.Stream {
		gain, err := cfg.play.gain()
		if err != nil {
			log.Error().Err(err).Msg("parse volume")
			return
		}
		state.addReady(func() {
			go func() {
				src := meowcaller.AudioSource(meowcaller.PCMStream(os.Stdin))
				if gain != 1 {
					src = &gainSource{src: src, gain: gain}
				}
				player := call.Play(src)
				state.setPlayer(player)
				log.Info().Float32("volume", gain).
					Msg("streaming raw PCM from stdin (16 kHz mono s16le)")
			}()
		})
	} else if cfg.play != nil {
		files := cfg.play.playlist()
		if len(files) > 0 {
			gain, err := cfg.play.gain()
			if err != nil {
				log.Error().Err(err).Msg("parse volume")
				return
			}
			effectiveLoop := cfg.play.loop
			if effectiveLoop < 1 {
				effectiveLoop = 1
			}
			// ffmpeg is only required for audio effects; plain playback (with or
			// without --loop) is decoded natively by meowcaller.
			useFFmpeg := cfg.play.fx != nil && !cfg.play.fx.empty()
			type audioPrep struct {
				src *playlistSource
				err error
			}
			prepCh := make(chan audioPrep, 1)
			go func() {
				if useFFmpeg {
					converted, cleanup, err := convertPlaylist(files, cfg.play.loop, cfg.play.fx)
					if err != nil {
						log.Error().Err(err).Msg("convert play files")
						prepCh <- audioPrep{err: err}
						return
					}
					src, err := newPlaylistSource(converted, gain, cleanup)
					if err != nil {
						log.Error().Err(err).Msg("open play files")
						prepCh <- audioPrep{err: err}
						return
					}
					prepCh <- audioPrep{src: src}
					return
				}
				playFiles := files
				if effectiveLoop > 1 {
					playFiles = repeatPlaylist(files, effectiveLoop)
				}
				src, err := newPlaylistSource(playFiles, gain, nil)
				if err != nil {
					log.Error().Err(err).Msg("open play files")
					prepCh <- audioPrep{err: err}
					return
				}
				prepCh <- audioPrep{src: src}
			}()
			state.addReady(func() {
				go func() {
					var prep audioPrep
					select {
					case prep = <-prepCh:
					case <-ctx.Done():
						return
					}
					if prep.err != nil {
						return
					}
					player := call.Play(prep.src)
					state.setPlayer(player)
					player.OnFinish(func() { log.Info().Msg("playback finished") })
					msg := "streaming files to peer (decoded natively)"
					if useFFmpeg {
						msg = "streaming files to peer (converted via ffmpeg)"
					}
					log.Info().Str("files", strings.Join(files, ",")).
						Float32("volume", gain).Int("loop", effectiveLoop).
						Msg(msg)
				}()
			})
		}
	}
	if cfg.videoFile != "" {
		type videoPrep struct {
			h264    string
			cleanup func()
			err     error
		}
		prepCh := make(chan videoPrep, 1)
		go func() {
			h264, transcoded, cleanup, err := convertVideoToAnnexB(cfg.videoFile)
			if err != nil {
				log.Error().Err(err).Msg("prepare video file")
				prepCh <- videoPrep{err: err}
				return
			}
			state.setVideoCleanup(cleanup)
			log.Info().Str("source", cfg.videoFile).Bool("transcoded", transcoded).
				Msg("video file prepared for camera feed")
			prepCh <- videoPrep{h264: h264, cleanup: cleanup}
		}()
		state.addReady(func() {
			go func() {
				var prep videoPrep
				select {
				case prep = <-prepCh:
				case <-ctx.Done():
					return
				}
				if prep.err != nil {
					return
				}
				defer prep.cleanup()
				sendAnnexB(ctx, call, prep.h264)
			}()
		})
	}
}

func sendAnnexB(ctx context.Context, call *meowcaller.Call, path string) {
	log := zerolog.Ctx(ctx)
	data, err := os.ReadFile(path)
	if err != nil {
		log.Error().Err(err).Msg("read video file")
		return
	}
	aus := splitAccessUnits(data)
	if len(aus) == 0 {
		log.Warn().Msg("no H.264 access units found in video file")
		return
	}
	log.Info().Int("access_units", len(aus)).Msg("streaming video file as camera")
	const frameDur = 33 * time.Millisecond
	next := time.Now()
	for i, au := range aus {
		if call.State() == meowcaller.CallPhaseEnded {
			log.Info().Msg("call ended — stopping video stream")
			return
		}
		if err := call.SendVideoWithDuration(au, frameDur); err != nil {
			log.Warn().Err(err).Int("au", i).Msg("send video frame failed")
			return
		}
		next = next.Add(frameDur)
		if d := time.Until(next); d > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(d):
			}
		}
	}
	log.Info().Int("frames", len(aus)).Msg("video file finished")
}

func splitAccessUnits(data []byte) [][]byte {
	var aus [][]byte
	auStart := -1
	i, n := 0, len(data)
	for i < n {
		for i < n && !isStartCode(data, i) {
			i++
		}
		if i >= n {
			break
		}
		sc := i
		scLen := startCodeLen(data, i)
		i += scLen
		if i >= n {
			break
		}
		if data[i]&0x1f == 9 && auStart >= 0 {
			aus = append(aus, data[auStart:sc])
			auStart = sc
			continue
		}
		if auStart < 0 {
			auStart = sc
		}
	}
	if auStart >= 0 && auStart < n {
		aus = append(aus, data[auStart:n])
	}
	return aus
}

func isStartCode(b []byte, i int) bool {
	if i+4 <= len(b) && b[i] == 0 && b[i+1] == 0 && b[i+2] == 0 && b[i+3] == 1 {
		return true
	}
	return i+3 <= len(b) && b[i] == 0 && b[i+1] == 0 && b[i+2] == 1
}

func startCodeLen(b []byte, i int) int {
	if i+4 <= len(b) && b[i] == 0 && b[i+1] == 0 && b[i+2] == 0 && b[i+3] == 1 {
		return 4
	}
	return 3
}

type gainSource struct {
	src  meowcaller.AudioSource
	gain float32
}

func (g *gainSource) ReadFrame() ([]float32, error) {
	frame, err := g.src.ReadFrame()
	if err != nil {
		return nil, err
	}
	if g.gain != 1 {
		for i := range frame {
			frame[i] *= g.gain
		}
	}
	return frame, nil
}

func (g *gainSource) Close() error { return g.src.Close() }

type multiSink struct {
	mu      sync.Mutex
	nextID  int
	entries []*sinkEntry
}

type sinkEntry struct {
	id   int
	sink meowcaller.AudioSink
}

func (m *multiSink) WriteFrame(frame []float32) error {
	m.mu.Lock()
	entries := m.entries
	m.mu.Unlock()
	var firstErr error
	for _, e := range entries {
		if err := e.sink.WriteFrame(frame); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *multiSink) Close() error {
	m.mu.Lock()
	entries := m.entries
	m.entries = nil
	m.mu.Unlock()
	var firstErr error
	for _, e := range entries {
		if err := e.sink.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *multiSink) add(s meowcaller.AudioSink) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	m.entries = append(m.entries, &sinkEntry{id: m.nextID, sink: s})
	return m.nextID
}

func (m *multiSink) remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.entries {
		if e.id == id {
			m.entries = append(m.entries[:i], m.entries[i+1:]...)
			break
		}
	}
}

type meterSink struct {
	mu    sync.Mutex
	level float32
	peak  float32
}

func newMeterSink() *meterSink { return &meterSink{} }

func (m *meterSink) WriteFrame(frame []float32) error {
	var peak float32
	for _, v := range frame {
		if v < 0 {
			v = -v
		}
		if v > peak {
			peak = v
		}
	}
	m.mu.Lock()
	m.level = m.level*0.8 + peak*0.2
	m.peak -= 0.02
	if peak > m.peak {
		m.peak = peak
	}
	m.mu.Unlock()
	return nil
}

func (m *meterSink) Close() error { return nil }

func (m *meterSink) Level() float32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.level
}

func (m *meterSink) Peak() float32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.peak
}

type participantRecorder struct {
	target string
	path   string
	f      *os.File
	mu     sync.Mutex
}

func newParticipantRecorder(spec string) (*participantRecorder, error) {
	jidPart, out := spec, ""
	if i := strings.IndexByte(spec, ':'); i >= 0 {
		jidPart, out = spec[:i], spec[i+1:]
	}
	jidPart = strings.TrimSpace(jidPart)
	if jidPart == "" {
		return nil, fmt.Errorf("--record-participant: empty participant id")
	}
	if out == "" {
		out = "participant-" + sanitizeFilename(jidPart) + ".h264"
	}
	f, err := os.Create(out)
	if err != nil {
		return nil, fmt.Errorf("--record-participant: create %s: %w", out, err)
	}
	return &participantRecorder{target: jidPart, path: out, f: f}, nil
}

func (r *participantRecorder) matches(id string) bool {
	if r.target == id {
		return true
	}
	if !strings.Contains(r.target, "@") {
		if j, err := types.ParseJID(id); err == nil && j.User == r.target {
			return true
		}
		return false
	}
	tj, errT := types.ParseJID(r.target)
	ij, errI := types.ParseJID(id)
	if errT == nil && errI == nil {
		return tj.User == ij.User
	}
	return false
}

func (r *participantRecorder) write(au []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	_, err := r.f.Write(au)
	return err
}

func (r *participantRecorder) close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}

func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, c := range s {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			c == '-' || c == '_' || c == '.' {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
