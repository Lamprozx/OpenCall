package app

import (
	"context"
	"net"
	"net/http"
	"time"
)

// ForceIPv4CtxKey carries a bool through context indicating that WhatsApp
// connections should use IPv4 only (workaround for broken IPv6 on some mobile
// carriers, which manifests as "software caused connection abort").
type ForceIPv4CtxKey struct{}

// IPv4OnlyHTTPClient returns an *http.Client whose transport dials only IPv4,
// regardless of what the resolver returns. Other transport settings are kept
// from http.DefaultTransport.
func IPv4OnlyHTTPClient() *http.Client {
	tr := (http.DefaultTransport.(*http.Transport)).Clone()
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		d := net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
		return d.DialContext(ctx, "tcp4", addr)
	}
	return &http.Client{Transport: tr}
}
