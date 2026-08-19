package app

import (
	"context"
	"net"
	"net/http"
	"time"
)

// AutoFallbackHTTPClient returns an *http.Client whose transport dials IPv4
// first and automatically falls back to IPv6 when IPv4 is unavailable.
//
// Preferring IPv4 avoids the broken-IPv6 "software caused connection abort"
// seen on some mobile carriers, while the IPv6 fallback keeps IPv6-only
// networks working. No flags required.
func AutoFallbackHTTPClient() *http.Client {
	tr := (http.DefaultTransport.(*http.Transport)).Clone()
	tr.DialContext = autoFallbackDial
	return &http.Client{Transport: tr}
}

// autoFallbackDial tries IPv4 first, then IPv6, so either family being broken
// or unreachable transparently falls back to the other.
func autoFallbackDial(ctx context.Context, network, addr string) (net.Conn, error) {
	d := net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	if conn, err := d.DialContext(ctx, "tcp4", addr); err == nil {
		return conn, nil
	}
	return d.DialContext(ctx, "tcp6", addr)
}
