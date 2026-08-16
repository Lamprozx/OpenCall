package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

var (
	sessionDir = "sessions"
	legacyDB   = "wa-voip.db"
)

type sessionRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Phone     string    `json:"phone"`
	JID       string    `json:"jid"`
	CreatedAt time.Time `json:"createdAt"`
	LastUsed  time.Time `json:"lastUsed"`
}

type sessionRegistry struct {
	Active   string          `json:"active"`
	Sessions []sessionRecord `json:"sessions"`
}

func registryPath() string { return filepath.Join(sessionDir, "registry.json") }

func storePath(id string) string { return filepath.Join(sessionDir, id, "wa-voip.db") }

func newSessionID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff)
	}
	return hex.EncodeToString(b[:])
}

func loadRegistry() (*sessionRegistry, error) {
	r := &sessionRegistry{}
	data, err := os.ReadFile(registryPath())
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, r); err != nil {
		return nil, fmt.Errorf("parse %s: %w", registryPath(), err)
	}
	return r, nil
}

func (r *sessionRegistry) save() error {
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := registryPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, registryPath())
}

func (r *sessionRegistry) byID(id string) *sessionRecord {
	for i := range r.Sessions {
		if r.Sessions[i].ID == id {
			return &r.Sessions[i]
		}
	}
	return nil
}

func (r *sessionRegistry) byJID(jid string) *sessionRecord {
	for i := range r.Sessions {
		if r.Sessions[i].JID == jid {
			return &r.Sessions[i]
		}
	}
	return nil
}

func (r *sessionRegistry) nameTaken(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, s := range r.Sessions {
		if strings.ToLower(strings.TrimSpace(s.Name)) == name {
			return true
		}
	}
	return false
}

func (r *sessionRegistry) add(s sessionRecord) error {
	if r.nameTaken(s.Name) {
		return fmt.Errorf("a session named %q already exists", s.Name)
	}
	r.Sessions = append(r.Sessions, s)
	return r.save()
}

func (r *sessionRegistry) setActive(id string) error {
	if r.byID(id) == nil {
		return fmt.Errorf("no session with id %q", id)
	}
	r.Active = id
	return r.save()
}

func (r *sessionRegistry) touch(id string) error {
	s := r.byID(id)
	if s == nil {
		return fmt.Errorf("no session with id %q", id)
	}
	s.LastUsed = time.Now()
	r.Active = id
	return r.save()
}

func (r *sessionRegistry) remove(id string) error {
	idx := -1
	for i, s := range r.Sessions {
		if s.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("no session with id %q", id)
	}
	r.Sessions = append(r.Sessions[:idx], r.Sessions[idx+1:]...)
	if r.Active == id {
		r.Active = ""
	}
	if err := r.save(); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(sessionDir, id))
}

func (r *sessionRegistry) resolveActive() (*sessionRecord, error) {
	if r.Active != "" {
		if s := r.byID(r.Active); s != nil {
			return s, nil
		}
	}
	var best *sessionRecord
	for i := range r.Sessions {
		s := &r.Sessions[i]
		if best == nil || s.LastUsed.After(best.LastUsed) {
			best = s
		}
	}
	if best == nil {
		return nil, errors.New("no sessions saved — run `auth` first (QR code or --pair <phone>)")
	}
	r.Active = best.ID
	if err := r.save(); err != nil {
		return nil, err
	}
	return best, nil
}

func ensureMigrated(r *sessionRegistry) error {
	if len(r.Sessions) > 0 || r.Active != "" {
		return nil
	}
	if _, err := os.Stat(legacyDB); err != nil {
		return nil
	}
	id := newSessionID()
	dir := filepath.Join(sessionDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := []string{legacyDB, legacyDB + "-wal", legacyDB + "-shm"}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			continue
		}
		if err := copyFile(f, filepath.Join(dir, filepath.Base(f))); err != nil {
			return err
		}
	}
	phone, jid := readStoredIdentity(filepath.Join(dir, filepath.Base(legacyDB)))
	now := time.Now()
	r.Sessions = append(r.Sessions, sessionRecord{
		ID:        id,
		Name:      promptSessionName(r, "default", phone),
		Phone:     phone,
		JID:       jid,
		CreatedAt: now,
		LastUsed:  now,
	})
	r.Active = id
	if err := r.save(); err != nil {
		return err
	}
	for _, f := range files {
		_ = os.Remove(f)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func readStoredIdentity(dbPath string) (phone, jid string) {
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		return "", ""
	}
	defer db.Close()
	container := sqlstore.NewWithDB(db, "sqlite", waLog.Zerolog(zerolog.Nop()).Sub("db"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dev, err := container.GetFirstDevice(ctx)
	if err != nil || dev == nil || dev.ID == nil {
		return "", ""
	}
	j := dev.ID.ToNonAD()
	return j.User, j.String()
}

var nameSuffixPool = []string{"2", "123", "3", "12", "4", "5", "6", "7", "8", "9", "_2", "_3", "23", "1234"}

func suggestNames(base string, taken map[string]bool, n int) []string {
	if n <= 0 {
		return nil
	}
	base = strings.TrimSpace(base)
	var out []string
	for _, suf := range nameSuffixPool {
		cand := base + suf
		if !taken[strings.ToLower(cand)] {
			out = append(out, cand)
			if len(out) >= n {
				break
			}
		}
	}
	return out
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}
