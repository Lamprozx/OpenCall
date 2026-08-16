package main

import (
	"flag"
	"reflect"
	"strings"
	"testing"
)

func TestFxEmpty(t *testing.T) {
	if !(&fxOptions{}).empty() {
		t.Error("zero-value fxOptions should be empty")
	}
	if (&fxOptions{reverb: 3}).empty() {
		t.Error("fx with reverb should not be empty")
	}
	if (&fxOptions{reverse: true}).empty() {
		t.Error("fx with reverse should not be empty")
	}
}

func TestFxAtempoChain(t *testing.T) {
	tests := []struct {
		x    float64
		want string
	}{
		{1.5, "atempo=1.5"},
		{0.7, "atempo=0.7"},
		{3, "atempo=2,atempo=1.5"},
		{0.25, "atempo=0.5,atempo=0.5"},
		{4, "atempo=2,atempo=2"},
	}
	for _, tt := range tests {
		if got := atempoChain(tt.x); got != tt.want {
			t.Errorf("atempoChain(%v) = %q, want %q", tt.x, got, tt.want)
		}
	}
}

func TestFxPitchFilter(t *testing.T) {
	got := pitchFilter(12)
	if !strings.Contains(got, "asetrate=96000") {
		t.Errorf("octave up should use rate 96000, got %q", got)
	}
	if !strings.Contains(got, "atempo=0.5") {
		t.Errorf("octave up should compensate with atempo=0.5, got %q", got)
	}
	if !strings.Contains(got, "aresample=48000") {
		t.Errorf("pitch filter should normalize to 48 kHz first, got %q", got)
	}
}

func TestFxAf(t *testing.T) {
	f := &fxOptions{speed: 1.5, reverb: 1}
	af := f.af()
	if !strings.Contains(af, "atempo=1.5") {
		t.Errorf("af should contain atempo=1.5, got %q", af)
	}
	if !strings.Contains(af, "aecho") {
		t.Errorf("af should contain aecho for reverb, got %q", af)
	}
	if strings.Contains(af, "areverse") {
		t.Errorf("af should not contain areverse when reverse is unset, got %q", af)
	}

	if got := (&fxOptions{}).af(); got != "" {
		t.Errorf("empty fx should produce empty filter chain, got %q", got)
	}

	rev := (&fxOptions{reverse: true}).af()
	if !strings.HasSuffix(rev, "areverse") {
		t.Errorf("reverse should be the last filter, got %q", rev)
	}
}

func TestFxValidate(t *testing.T) {
	valid := []fxOptions{
		{speed: 1.5},
		{pitch: 12},
		{pitch: -12},
		{reverb: 10},
		{echo: 1},
		{bass: 6},
		{treble: -3},
		{lowpass: 3000},
		{highpass: 200},
		{chorus: 5},
		{flanger: 5},
		{tremolo: 5},
		{vibrato: 5},
		{crusher: 10},
		{fadeIn: 2},
		{fadeOut: 2},
		{reverse: true},
	}
	for _, f := range valid {
		if err := f.validate(); err != nil {
			t.Errorf("fx %+v should be valid: %v", f, err)
		}
	}

	invalid := []struct {
		f   fxOptions
		err string
	}{
		{fxOptions{speed: 5}, "--speed"},
		{fxOptions{speed: 0.1}, "--speed"},
		{fxOptions{pitch: 13}, "--pitch"},
		{fxOptions{reverb: 11}, "--reverb"},
		{fxOptions{bass: 30}, "--bass"},
		{fxOptions{lowpass: 99999}, "--lowpass"},
		{fxOptions{crusher: 11}, "--crusher"},
		{fxOptions{fadeIn: 999}, "--fade-in"},
	}
	for _, tt := range invalid {
		if err := tt.f.validate(); err == nil {
			t.Errorf("fx %+v should be rejected", tt.f)
		} else if !strings.Contains(err.Error(), tt.err) {
			t.Errorf("error %q should mention %q", err.Error(), tt.err)
		}
	}
}

func TestPlayOptionsFxValidate(t *testing.T) {
	if err := (&playOptions{stream: true, loop: -1}).validate(); err != nil {
		t.Errorf("stream without fx should be valid: %v", err)
	}
	if err := (&playOptions{stream: true, loop: -1, fx: &fxOptions{reverb: 1}}).validate(); err == nil {
		t.Error("stream + fx should be rejected")
	}
	if err := (&playOptions{loop: -1, fx: &fxOptions{speed: 1.5}}).validate(); err != nil {
		t.Errorf("play with fx should be valid: %v", err)
	}
}

func TestFxRegister(t *testing.T) {
	p := &playOptions{}
	fs := flag.NewFlagSet("call", flag.ContinueOnError)
	p.register(fs)
	if p.fx == nil {
		t.Fatal("register should initialize fx")
	}
	if err := fs.Parse(reorderArgs([]string{"6281", "--reverb", "3", "--reverse"})); err != nil {
		t.Fatalf("parse fx flags: %v", err)
	}
	if p.fx.reverb != 3 {
		t.Errorf("reverb = %d, want 3", p.fx.reverb)
	}
	if !p.fx.reverse {
		t.Error("reverse should be true")
	}
	if fs.NArg() != 1 || fs.Arg(0) != "6281" {
		t.Errorf("positional wrong: %v", fs.Args())
	}
}

func TestReorderArgsReverse(t *testing.T) {
	got := reorderArgs([]string{"6281", "--reverse"})
	want := []string{"--reverse", "6281"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reorderArgs with trailing bool --reverse = %v, want %v", got, want)
	}
}
