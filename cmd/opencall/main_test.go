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
		wantIPv6  bool
	}{
		{[]string{"call", "6281"}, []string{"call", "6281"}, "", false, false, "", false},
		{[]string{"call", "6281", "--log-level", "debug"}, []string{"call", "6281"}, "debug", false, false, "", false},
		{[]string{"--log-level=warn", "call", "6281"}, []string{"call", "6281"}, "warn", false, false, "", false},
		{[]string{"call", "--quiet", "6281", "--show-noise"}, []string{"call", "6281"}, "", true, true, "", false},
		{[]string{"call", "-log-level", "error", "-quiet"}, []string{"call"}, "error", true, false, "", false},
		{[]string{"call", "6281", "--diag", "/tmp/d"}, []string{"call", "6281"}, "", false, false, "/tmp/d", false},
		{[]string{"--diag=/tmp/d", "call"}, []string{"call"}, "", false, false, "/tmp/d", false},
		{[]string{"auth", "--allow-ipv6", "--pair", "6281"}, []string{"auth", "--pair", "6281"}, "", false, false, "", true},
		{[]string{"-allow-ipv6", "call", "6281"}, []string{"call", "6281"}, "", false, false, "", true},
	}
	for _, c := range cases {
		rest, level, quiet, noise, diagDir, allowIPv6 := extractGlobalFlags(c.args)
		if !eqSlice(rest, c.wantRest) || level != c.wantLevel || quiet != c.wantQuiet ||
			noise != c.wantNoise || diagDir != c.wantDiag || allowIPv6 != c.wantIPv6 {
			t.Errorf("extractGlobalFlags(%v) = (%v, %q, %v, %v, %q, %v), want (%v, %q, %v, %v, %q, %v)",
				c.args, rest, level, quiet, noise, diagDir, allowIPv6,
				c.wantRest, c.wantLevel, c.wantQuiet, c.wantNoise, c.wantDiag, c.wantIPv6)
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
