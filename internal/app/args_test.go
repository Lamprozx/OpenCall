package app

import (
	"reflect"
	"testing"
)

func TestNormalizeGroupID(t *testing.T) {
	tests := []struct {
		raw  string
		want string
		err  bool
	}{
		{"1203631234567890@g.us", "1203631234567890@g.us", false},
		{"1203631234567890", "1203631234567890@g.us", false},
		{" 1203631234567890 ", "1203631234567890@g.us", false},
		{"6281234567890@s.whatsapp.net", "6281234567890@s.whatsapp.net", false},
		{"", "", true},
		{"not a jid", "", true},
	}
	for _, tt := range tests {
		got, err := NormalizeGroupID(tt.raw)
		if tt.err {
			if err == nil {
				t.Errorf("NormalizeGroupID(%q): expected error, got %q", tt.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeGroupID(%q): unexpected error %v", tt.raw, err)
			continue
		}
		if got != tt.want {
			t.Errorf("NormalizeGroupID(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestGroupJIDsEqual(t *testing.T) {
	equal := [][2]string{
		{"1203631234567890@g.us", "1203631234567890@g.us"},
		{"1203631234567890", "1203631234567890@g.us"},
		{"1203631234567890@g.us", " 1203631234567890 "},
	}
	for _, pair := range equal {
		if !GroupJIDsEqual(pair[0], pair[1]) {
			t.Errorf("GroupJIDsEqual(%q, %q) = false, want true", pair[0], pair[1])
		}
	}
	different := [][2]string{
		{"1203631234567890@g.us", "1203639999999999@g.us"},
		{"1203631234567890", ""},
		{"", ""},
	}
	for _, pair := range different {
		if GroupJIDsEqual(pair[0], pair[1]) {
			t.Errorf("GroupJIDsEqual(%q, %q) = true, want false", pair[0], pair[1])
		}
	}
}

func TestReorderArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "flags after positional with loop value",
			args: []string{"6281", "--play", "a,b", "--loop", "10"},
			want: []string{"--play", "a,b", "--loop", "10", "6281"},
		},
		{
			name: "flags before positional unchanged",
			args: []string{"--loop", "3", "6281"},
			want: []string{"--loop", "3", "6281"},
		},
		{
			name: "negative volume value kept paired",
			args: []string{"6281", "--volume", "-3s"},
			want: []string{"--volume", "-3s", "6281"},
		},
		{
			name: "bool flag without value",
			args: []string{"a", "b", "--auto-answer", "--video"},
			want: []string{"--auto-answer", "--video", "a", "b"},
		},
		{
			name: "multiple positionals preserved",
			args: []string{"t1", "t2", "--video"},
			want: []string{"--video", "t1", "t2"},
		},
		{
			name: "equals-form flag does not steal next token",
			args: []string{"6281", "--group-id=xyz", "6282"},
			want: []string{"--group-id=xyz", "6281", "6282"},
		},
		{
			name: "end-of-flags marker keeps rest positional",
			args: []string{"--auto-answer", "--", "-weird-target"},
			want: []string{"--auto-answer", "--", "-weird-target"},
		},
		{
			name: "empty",
			args: nil,
			want: nil,
		},
	}
	for _, tt := range tests {
		got := ReorderArgs(tt.args)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: ReorderArgs(%v) = %v, want %v", tt.name, tt.args, got, tt.want)
		}
	}
}

func TestReorderArgsReverse(t *testing.T) {
	got := ReorderArgs([]string{"6281", "--reverse"})
	want := []string{"--reverse", "6281"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReorderArgs with trailing bool --reverse = %v, want %v", got, want)
	}
}
