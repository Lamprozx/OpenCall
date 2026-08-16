package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempSessionPaths(t *testing.T) (cleanup func()) {
	t.Helper()
	oldDir, oldDB := sessionDir, legacyDB
	dir := t.TempDir()
	sessionDir = filepath.Join(dir, "sessions")
	legacyDB = filepath.Join(dir, "wa-voip.db")
	return func() {
		sessionDir, legacyDB = oldDir, oldDB
	}
}

func TestNewSessionID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := newSessionID()
		if len(id) != 8 {
			t.Fatalf("id %q: want 8 hex chars, got %d", id, len(id))
		}
		for _, c := range id {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Fatalf("id %q contains non-hex char %c", id, c)
			}
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestRegistryRoundtripAndUniqueness(t *testing.T) {
	cleanup := tempSessionPaths(t)
	defer cleanup()

	r := &sessionRegistry{}
	now := time.Now()
	if err := r.add(sessionRecord{ID: "abc12345", Name: "udin", Phone: "6281", JID: "6281@s.whatsapp.net", CreatedAt: now, LastUsed: now}); err != nil {
		t.Fatal(err)
	}
	if err := r.setActive("abc12345"); err != nil {
		t.Fatal(err)
	}

	r2, err := loadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Sessions) != 1 || r2.Sessions[0].Name != "udin" {
		t.Fatalf("roundtrip mismatch: %+v", r2)
	}
	if r2.Active != "abc12345" {
		t.Fatalf("active = %q, want abc12345", r2.Active)
	}
	if !r2.nameTaken("UDIN") {
		t.Fatal("nameTaken should be case-insensitive")
	}
	if r2.nameTaken("budi") {
		t.Fatal("nameTaken false positive")
	}
	if err := r2.add(sessionRecord{ID: "deadbeef", Name: "udin", Phone: "6282"}); err == nil {
		t.Fatal("adding a duplicate name should fail")
	}
}

func TestByJID(t *testing.T) {
	r := &sessionRegistry{}
	r.Sessions = append(r.Sessions, sessionRecord{ID: "a", JID: "6281@s.whatsapp.net"})
	if got := r.byJID("6281@s.whatsapp.net"); got == nil || got.ID != "a" {
		t.Fatalf("byJID miss: %+v", got)
	}
	if r.byJID("nope@s.whatsapp.net") != nil {
		t.Fatal("byJID should return nil for unknown jid")
	}
}

func TestResolveActiveFallsBack(t *testing.T) {
	cleanup := tempSessionPaths(t)
	defer cleanup()

	older := time.Now().Add(-2 * time.Hour)
	newer := time.Now()
	r := &sessionRegistry{}
	r.Sessions = []sessionRecord{
		{ID: "one", Name: "a", LastUsed: older},
		{ID: "two", Name: "b", LastUsed: newer},
	}
	r.Active = "one"
	s, err := r.resolveActive()
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "one" {
		t.Fatalf("active should win: got %s", s.ID)
	}
	r.Active = "gone"
	s, err = r.resolveActive()
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "two" {
		t.Fatalf("should fall back to most recent: got %s", s.ID)
	}
	if r.Active != "two" {
		t.Fatal("fallback should persist the new active id")
	}
}

func TestRemoveDeletesDir(t *testing.T) {
	cleanup := tempSessionPaths(t)
	defer cleanup()

	r := &sessionRegistry{}
	if err := r.add(sessionRecord{ID: "abc12345", Name: "udin", Phone: "6281"}); err != nil {
		t.Fatal(err)
	}
	if err := r.setActive("abc12345"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(sessionDir, "abc12345")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := r.remove("abc12345"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("store dir should be gone, stat err = %v", err)
	}
	r2, err := loadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Sessions) != 0 {
		t.Fatalf("registry should be empty: %+v", r2)
	}
	if r2.Active != "" {
		t.Fatalf("active should be cleared: %q", r2.Active)
	}
}

func TestSuggestNames(t *testing.T) {
	taken := map[string]bool{"udin": true, "udin2": true}
	got := suggestNames("udin", taken, 3)
	want := []string{"udin123", "udin3", "udin12"}
	if len(got) != len(want) {
		t.Fatalf("suggestNames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("suggestNames = %v, want %v", got, want)
		}
	}
	if len(suggestNames("x", nil, 0)) != 0 {
		t.Fatal("n<=0 should return nothing")
	}
}

func TestUnusedName(t *testing.T) {
	r := &sessionRegistry{}
	r.Sessions = append(r.Sessions, sessionRecord{ID: "a", Name: "default"})
	if got := unusedName(r, "default"); got != "default2" {
		t.Fatalf("unusedName = %q, want default2", got)
	}
	if got := unusedName(r, "fresh"); got != "fresh" {
		t.Fatalf("unusedName = %q, want fresh", got)
	}
	r.Sessions = append(r.Sessions, sessionRecord{ID: "b", Name: "default2"})
	if got := unusedName(r, "default"); got != "default123" {
		t.Fatalf("unusedName = %q, want default123", got)
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Now()
	cases := []struct {
		in   time.Time
		want string
	}{
		{now.Add(-5 * time.Second), "now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-2 * 24 * time.Hour), "2d ago"},
		{now.Add(-30 * 24 * time.Hour), now.Add(-30 * 24 * time.Hour).Format("2006-01-02")},
	}
	for _, c := range cases {
		if got := relativeTime(c.in); got != c.want {
			t.Errorf("relativeTime(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEnsureMigratedImportsLegacy(t *testing.T) {
	cleanup := tempSessionPaths(t)
	defer cleanup()

	if err := os.WriteFile(legacyDB, []byte("not really sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := loadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureMigrated(r); err != nil {
		t.Fatal(err)
	}
	if len(r.Sessions) != 1 {
		t.Fatalf("expected 1 imported session, got %d", len(r.Sessions))
	}
	s := r.Sessions[0]
	if s.Name != "default" {
		t.Fatalf("imported name = %q, want default", s.Name)
	}
	if r.Active != s.ID {
		t.Fatal("imported session should be active")
	}
	if _, err := os.Stat(legacyDB); !os.IsNotExist(err) {
		t.Fatal("legacy db should have been moved away")
	}
	if _, err := os.Stat(filepath.Join(sessionDir, s.ID, "wa-voip.db")); err != nil {
		t.Fatalf("moved store missing: %v", err)
	}
	if err := ensureMigrated(r); err != nil {
		t.Fatal(err)
	}
	if len(r.Sessions) != 1 {
		t.Fatalf("migration should not run twice, got %d sessions", len(r.Sessions))
	}
}
