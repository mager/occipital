package musicbrainz

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"testing"
)

func TestMusicBrainzIPv4TransportScopeAndTLS(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	sentinel := errors.New("test dial stopped")
	var network, address string
	base := original.(*http.Transport).Clone()
	base.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	base.DialContext = func(_ context.Context, n, a string) (net.Conn, error) { network, address = n, a; return nil, sentinel }
	http.DefaultTransport = base
	if err := ConfigureHTTPTransport(); err != nil {
		t.Fatal(err)
	}
	transport := http.DefaultTransport.(*http.Transport)
	if transport == base {
		t.Fatal("default transport must be cloned before modification")
	}
	if transport.TLSClientConfig.InsecureSkipVerify || transport.TLSClientConfig.ServerName != "" || transport.TLSClientConfig.MinVersion != tls.VersionTLS12 || transport.ForceAttemptHTTP2 != base.ForceAttemptHTTP2 {
		t.Fatal("TLS or HTTP/2 settings changed")
	}
	for _, tc := range []struct{ address, network string }{
		{"musicbrainz.org:443", "tcp4"},
		{"MUSICBRAINZ.ORG:443", "tcp4"},
		{"musicbrainz.org:80", "tcp"},
		{"api.spotify.com:443", "tcp"},
		{"other.musicbrainz.org:443", "tcp"},
		{"musicbrainz.org.example.com:443", "tcp"},
	} {
		_, err := transport.DialContext(context.Background(), "tcp", tc.address)
		if !errors.Is(err, sentinel) || network != tc.network || address != tc.address {
			t.Fatalf("dial %s = %s/%s, %v", tc.address, network, address, err)
		}
	}
	_, _ = base.DialContext(context.Background(), "tcp", "musicbrainz.org:443")
	if network != "tcp" {
		t.Fatal("original transport was modified")
	}
}

func TestMusicBrainzIPv4DialPreservesCancellation(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	base := original.(*http.Transport).Clone()
	base.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) { <-ctx.Done(); return nil, ctx.Err() }
	http.DefaultTransport = base
	if err := ConfigureHTTPTransport(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := http.DefaultTransport.(*http.Transport).DialContext(ctx, "tcp", "musicbrainz.org:443")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("dial lost request cancellation: %v", err)
	}
}
