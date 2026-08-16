package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	meowcaller "github.com/purpshell/meowcaller"
	"github.com/purpshell/meowcaller/diag"
	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	_ "modernc.org/sqlite"
)

type authOptions struct {
	pair  bool
	phone string
}

func connect(ctx context.Context) (*whatsmeow.Client, *meowcaller.Client, error) {
	r, err := loadRegistry()
	if err != nil {
		return nil, nil, err
	}
	if err := ensureMigrated(r); err != nil {
		return nil, nil, err
	}
	sess, err := r.resolveActive()
	if err != nil {
		return nil, nil, err
	}
	return connectWithAuth(ctx, authOptions{}, sess)
}

func meowOptions(ctx context.Context, log *zerolog.Logger) []meowcaller.Option {
	opts := []meowcaller.Option{meowcaller.WithLogger(*log)}
	if rec, ok := ctx.Value(diagCtxKey{}).(*diag.Recorder); ok && rec != nil {
		opts = append(opts, meowcaller.WithDiagnostics(rec))
	}
	return opts
}

func connectWithAuth(ctx context.Context, auth authOptions, sess *sessionRecord) (*whatsmeow.Client, *meowcaller.Client, error) {
	log := zerolog.Ctx(ctx)
	wa, err := openClient(ctx, log, storePath(sess.ID))
	if err != nil {
		return nil, nil, err
	}
	client := meowcaller.NewClient(wa, meowOptions(ctx, log)...)
	if err := connectClient(ctx, wa, auth); err != nil {
		wa.Disconnect()
		return nil, nil, err
	}
	if r, err := loadRegistry(); err == nil {
		if err := r.touch(sess.ID); err != nil {
			log.Warn().Err(err).Msg("failed to update session last-used")
		}
	}
	return wa, client, nil
}

func connectNew(ctx context.Context, auth authOptions, dbPath string) (*whatsmeow.Client, *meowcaller.Client, error) {
	log := zerolog.Ctx(ctx)
	wa, err := openClient(ctx, log, dbPath)
	if err != nil {
		return nil, nil, err
	}
	client := meowcaller.NewClient(wa, meowOptions(ctx, log)...)
	if err := connectClient(ctx, wa, auth); err != nil {
		wa.Disconnect()
		return nil, nil, err
	}
	return wa, client, nil
}

func openClient(ctx context.Context, log *zerolog.Logger, dbPath string) (*whatsmeow.Client, error) {
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, err
	}
	store.DeviceProps.Os = proto.String("Mac OS")
	store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()

	waLogger := *log
	db, err := sql.Open("sqlite",
		"file:"+abs+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	db.SetMaxOpenConns(1)
	container := sqlstore.NewWithDB(db, "sqlite", waLog.Zerolog(waLogger).Sub("db"))
	if err := container.Upgrade(ctx); err != nil {
		return nil, fmt.Errorf("upgrade store: %w", err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("load device: %w", err)
	}
	return whatsmeow.NewClient(device, waLog.Zerolog(waLogger).Sub("wa")), nil
}

func connectClient(ctx context.Context, client *whatsmeow.Client, auth authOptions) error {
	log := zerolog.Ctx(ctx)
	if client.Store.ID == nil {
		qr, _ := client.GetQRChannel(ctx)
		if err := client.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		paired := false
		for evt := range qr {
			switch evt.Event {
			case "code":
				if auth.pair && !paired {
					paired = true
					code, err := client.PairPhone(ctx, auth.phone, true,
						whatsmeow.PairClientChrome, "Chrome (Mac OS)")
					if err != nil {
						return fmt.Errorf("pair phone: %w", err)
					}
					log.Info().Str("code", code).
						Msg("enter this code in WhatsApp > Linked devices > Link with phone number")
					fmt.Fprintln(termOut, code)
				} else if !auth.pair {
					log.Info().Int("valid_s", int(evt.Timeout.Seconds())).
						Msg("scan in WhatsApp > Linked devices; QR code below:")
					fmt.Fprintln(termOut, evt.Code)
				}
			case "error":
				if evt.Error != nil {
					return fmt.Errorf("pairing error: %w", evt.Error)
				}
			default:
				log.Info().Str("event", evt.Event).Msg("login event")
			}
		}
	} else if err := client.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	if err := waitUntilReady(ctx, client, 60*time.Second); err != nil {
		return err
	}
	log.Info().Str("self_lid", client.Store.GetLID().String()).Msg("connected")

	if client.Store.PushName == "" {
		client.Store.PushName = "meowcaller-manager"
	}
	if err := client.SendPresence(ctx, types.PresenceAvailable); err != nil {
		log.Warn().Err(err).Msg("send presence failed; continuing")
	}
	return nil
}

func waitUntilReady(ctx context.Context, client *whatsmeow.Client, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for !(client.IsConnected() && client.IsLoggedIn()) {
		select {
		case <-ticker.C:
		case <-deadline:
			return errors.New("timed out waiting for whatsmeow connection")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
