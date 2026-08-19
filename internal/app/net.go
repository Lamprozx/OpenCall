package app

import (
	"context"
	"net"
	"net/http"
	"time"
)

// AllowIPv6CtxKey carries a bool through context that opts OUT of the default
// IPv4-only dialing. By default OpenCall dials WhatsApp over IPv4 only, because
// some mobile carriers have broken IPv6 that aborts the websocket with
// "software caused connection abort". --allow-ipv6 restores dual-stack dialing
// for the rare IPv6-only network.
type AllowIPv6CtxKey struct{}

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
