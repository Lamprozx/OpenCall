package main

import (
	"testing"
)

func TestExtractGlobalFlags(t *testing.T) {
	cases := []struct {
		args      []string
		wantRest  []string
		wantLevel string
		wantQuiet bool
		wantNoise bool
		wantDiag  string
	}{
		{[]string{"call", "6281"}, []string{"call", "6281"}, "", false, false, ""},
		{[]string{"call", "6281", "--log-level", "debug"}, []string{"call", "6281"}, "debug", false, false, ""},
		{[]string{"--log-level=warn", "call", "6281"}, []string{"call", "6281"}, "warn", false, false, ""},
		{[]string{"call", "--quiet", "6281", "--show-noise"}, []string{"call", "6281"}, "", true, true, ""},
		{[]string{"call", "-log-level", "error", "-quiet"}, []string{"call"}, "error", true, false, ""},
		{[]string{"call", "6281", "--diag", "/tmp/d"}, []string{"call", "6281"}, "", false, false, "/tmp/d"},
		{[]string{"--diag=/tmp/d", "call"}, []string{"call"}, "", false, false, "/tmp/d"},
	}
	for _, c := range cases {
		rest, level, quiet, noise, diagDir := extractGlobalFlags(c.args)
		if !eqSlice(rest, c.wantRest) || level != c.wantLevel || quiet != c.wantQuiet ||
			noise != c.wantNoise || diagDir != c.wantDiag {
			t.Errorf("extractGlobalFlags(%v) = (%v, %q, %v, %v, %q), want (%v, %q, %v, %v, %q)",
				c.args, rest, level, quiet, noise, diagDir, c.wantRest, c.wantLevel, c.wantQuiet, c.wantNoise, c.wantDiag)
		}
	}
}

func eqSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
