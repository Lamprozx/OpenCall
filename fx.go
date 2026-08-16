package main

import (
	"flag"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type fxOptions struct {
	speed    float64
	pitch    float64
	reverb   int
	echo     int
	bass     float64
	treble   float64
	lowpass  float64
	highpass float64
	chorus   int
	flanger  int
	tremolo  float64
	vibrato  float64
	crusher  int
	fadeIn   float64
	fadeOut  float64
	reverse  bool
}

func (f *fxOptions) empty() bool {
	return f.speed == 0 && f.pitch == 0 && f.reverb == 0 && f.echo == 0 &&
		f.bass == 0 && f.treble == 0 && f.lowpass == 0 && f.highpass == 0 &&
		f.chorus == 0 && f.flanger == 0 && f.tremolo == 0 && f.vibrato == 0 &&
		f.crusher == 0 && f.fadeIn == 0 && f.fadeOut == 0 && !f.reverse
}

func (f *fxOptions) register(fs *flag.FlagSet) {
	fs.Float64Var(&f.speed, "speed", 0, "tempo multiplier (1 = normal; >1 faster, <1 slower), e.g. --speed 1.5")
	fs.Float64Var(&f.pitch, "pitch", 0, "pitch shift in semitones (+12 = octave up, -12 = octave down), e.g. --pitch 3")
	fs.IntVar(&f.reverb, "reverb", 0, "reverb amount 1-10, e.g. --reverb 1")
	fs.IntVar(&f.echo, "echo", 0, "echo amount 1-10")
	fs.Float64Var(&f.bass, "bass", 0, "bass gain in dB, e.g. --bass 6")
	fs.Float64Var(&f.treble, "treble", 0, "treble gain in dB, e.g. --treble -3")
	fs.Float64Var(&f.lowpass, "lowpass", 0, "lowpass cutoff in Hz, e.g. --lowpass 3000")
	fs.Float64Var(&f.highpass, "highpass", 0, "highpass cutoff in Hz, e.g. --highpass 200")
	fs.IntVar(&f.chorus, "chorus", 0, "chorus amount 1-10")
	fs.IntVar(&f.flanger, "flanger", 0, "flanger amount 1-10")
	fs.Float64Var(&f.tremolo, "tremolo", 0, "tremolo rate in Hz, e.g. --tremolo 5")
	fs.Float64Var(&f.vibrato, "vibrato", 0, "vibrato rate in Hz, e.g. --vibrato 5")
	fs.IntVar(&f.crusher, "crusher", 0, "bitcrusher amount 1-10")
	fs.Float64Var(&f.fadeIn, "fade-in", 0, "fade in seconds, e.g. --fade-in 2")
	fs.Float64Var(&f.fadeOut, "fade-out", 0, "fade out seconds, e.g. --fade-out 2")
	fs.BoolVar(&f.reverse, "reverse", false, "play the audio in reverse")
}

func (f *fxOptions) validate() error {
	if f.speed != 0 && (f.speed < 0.25 || f.speed > 4) {
		return fmt.Errorf("--speed must be between 0.25 and 4 (got %v)", f.speed)
	}
	if f.pitch < -12 || f.pitch > 12 {
		return fmt.Errorf("--pitch must be between -12 and 12 semitones (got %v)", f.pitch)
	}
	if f.reverb < 0 || f.reverb > 10 {
		return fmt.Errorf("--reverb must be between 0 and 10 (got %d)", f.reverb)
	}
	if f.echo < 0 || f.echo > 10 {
		return fmt.Errorf("--echo must be between 0 and 10 (got %d)", f.echo)
	}
	if f.bass < -24 || f.bass > 24 {
		return fmt.Errorf("--bass must be between -24 and 24 dB (got %v)", f.bass)
	}
	if f.treble < -24 || f.treble > 24 {
		return fmt.Errorf("--treble must be between -24 and 24 dB (got %v)", f.treble)
	}
	if f.lowpass < 0 || f.lowpass > 20000 {
		return fmt.Errorf("--lowpass must be between 0 and 20000 Hz (got %v)", f.lowpass)
	}
	if f.highpass < 0 || f.highpass > 20000 {
		return fmt.Errorf("--highpass must be between 0 and 20000 Hz (got %v)", f.highpass)
	}
	if f.chorus < 0 || f.chorus > 10 {
		return fmt.Errorf("--chorus must be between 0 and 10 (got %d)", f.chorus)
	}
	if f.flanger < 0 || f.flanger > 10 {
		return fmt.Errorf("--flanger must be between 0 and 10 (got %d)", f.flanger)
	}
	if f.tremolo < 0 || f.tremolo > 20 {
		return fmt.Errorf("--tremolo must be between 0 and 20 Hz (got %v)", f.tremolo)
	}
	if f.vibrato < 0 || f.vibrato > 20 {
		return fmt.Errorf("--vibrato must be between 0 and 20 Hz (got %v)", f.vibrato)
	}
	if f.crusher < 0 || f.crusher > 10 {
		return fmt.Errorf("--crusher must be between 0 and 10 (got %d)", f.crusher)
	}
	if f.fadeIn < 0 || f.fadeIn > 60 {
		return fmt.Errorf("--fade-in must be between 0 and 60 seconds (got %v)", f.fadeIn)
	}
	if f.fadeOut < 0 || f.fadeOut > 60 {
		return fmt.Errorf("--fade-out must be between 0 and 60 seconds (got %v)", f.fadeOut)
	}
	return nil
}

func (f *fxOptions) af() string {
	var parts []string
	if f.speed != 0 && f.speed != 1 {
		parts = append(parts, atempoChain(f.speed))
	}
	if f.pitch != 0 {
		parts = append(parts, pitchFilter(f.pitch))
	}
	if f.reverb > 0 {
		parts = append(parts, reverbFilter(f.reverb))
	}
	if f.echo > 0 {
		parts = append(parts, echoFilter(f.echo))
	}
	if f.bass != 0 {
		parts = append(parts, "bass=g="+fnum(f.bass))
	}
	if f.treble != 0 {
		parts = append(parts, "treble=g="+fnum(f.treble))
	}
	if f.lowpass > 0 {
		parts = append(parts, "lowpass=f="+fnum(f.lowpass))
	}
	if f.highpass > 0 {
		parts = append(parts, "highpass=f="+fnum(f.highpass))
	}
	if f.chorus > 0 {
		parts = append(parts, chorusFilter(f.chorus))
	}
	if f.flanger > 0 {
		parts = append(parts, flangerFilter(f.flanger))
	}
	if f.tremolo > 0 {
		parts = append(parts, "tremolo=f="+fnum(f.tremolo)+":d=0.7")
	}
	if f.vibrato > 0 {
		parts = append(parts, "vibrato=f="+fnum(f.vibrato)+":d=0.7")
	}
	if f.crusher > 0 {
		parts = append(parts, crusherFilter(f.crusher))
	}
	if f.fadeIn > 0 {
		parts = append(parts, "afade=t=in:d="+fnum(f.fadeIn))
	}
	if f.fadeOut > 0 {
		parts = append(parts, "areverse,afade=t=in:d="+fnum(f.fadeOut)+",areverse")
	}
	if f.reverse {
		parts = append(parts, "areverse")
	}
	return strings.Join(parts, ",")
}

func atempoChain(x float64) string {
	var parts []string
	for x > 2 {
		parts = append(parts, "atempo=2")
		x /= 2
	}
	for x < 0.5 {
		parts = append(parts, "atempo=0.5")
		x /= 0.5
	}
	parts = append(parts, "atempo="+fnum(x))
	return strings.Join(parts, ",")
}

func pitchFilter(n float64) string {
	factor := math.Pow(2, n/12)
	rate := int(math.Round(48000 * factor))
	return "aresample=48000,asetrate=" + strconv.Itoa(rate) +
		",aresample=16000," + atempoChain(1/factor)
}

func reverbFilter(level int) string {
	l := float64(level) / 10.0
	return fmt.Sprintf("aecho=0.8:%.2f:60|120|180:%.2f|%.2f|%.2f",
		0.5+0.4*l, 0.30*l, 0.20*l, 0.10*l)
}

func echoFilter(level int) string {
	delay := 300 + level*150
	decay := 0.2 + 0.06*float64(level)
	return fmt.Sprintf("aecho=0.8:0.9:%d:%.2f", delay, decay)
}

func chorusFilter(level int) string {
	delay := 40 + level*5
	decay := 0.2 + 0.04*float64(level)
	return fmt.Sprintf("chorus=0.7:0.9:%d:%.2f:0.25:%.1f", delay, decay, 1.0+0.3*float64(level))
}

func flangerFilter(level int) string {
	return fmt.Sprintf("flanger=delay=10:depth=%.1f:regen=0:width=71:speed=0.5", 1.0+0.5*float64(level))
}

func crusherFilter(level int) string {
	bits := 14 - level
	return fmt.Sprintf("acrusher=level_in=1:level_out=%d:bits=%d:mode=log:aa=1", level*2, bits)
}

func fnum(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
