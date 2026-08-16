package call

import (
	"context"
	"os"
	"testing"

	"github.com/rs/zerolog"

	"opencall/internal/session"
)

func TestOpenClientStore(t *testing.T) {
	r, err := session.LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	dbPath := ""
	if len(r.Sessions) > 0 {
		best := r.Sessions[0]
		if a := r.ByID(r.Active); a != nil {
			best = *a
		} else {
			for _, s := range r.Sessions {
				if s.LastUsed.After(best.LastUsed) {
					best = s
				}
			}
		}
		dbPath = session.StorePath(best.ID)
	} else if _, err := os.Stat(session.LegacyDBPath()); err == nil {
		dbPath = session.LegacyDBPath()
	}
	if dbPath == "" {
		t.Skip("session store (run `auth` first)")
	}
	logger := zerolog.Nop()
	client, err := OpenClient(context.Background(), &logger, dbPath)
	if err != nil {
		t.Fatalf("OpenClient: %v", err)
	}
	if client.Store == nil || client.Store.ID == nil {
		t.Errorf("expected a stored device to be loaded from %s", dbPath)
	}
}
