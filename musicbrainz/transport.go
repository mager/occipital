package musicbrainz

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// ConfigureHTTPTransport must run once at startup, before clients or requests.
// Cloud Run can connect to MusicBrainz over IPv6 but its TLS handshake fails.
// Scope IPv4 to this HTTPS destination; leave TLS verification and SNI unchanged.
// Both the legacy SDK and the context client use the default transport.
func ConfigureHTTPTransport() error {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return fmt.Errorf("cannot configure MusicBrainz dialing on transport %T", http.DefaultTransport)
	}
	transport := base.Clone()
	dial := transport.DialContext
	if dial == nil {
		dial = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if strings.EqualFold(address, "musicbrainz.org:443") {
			network = "tcp4"
		}
		return dial(ctx, network, address)
	}
	http.DefaultTransport = transport
	return nil
}
