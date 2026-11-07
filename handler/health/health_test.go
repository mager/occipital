package health

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mager/occipital/spotify"
	"go.uber.org/zap"
)

func TestHealthReturnsOnlyServerStatus(t *testing.T) {
	h := NewHealthHandler(zap.NewNop().Sugar(), &spotify.SpotifyClient{ID: "private", Secret: "private"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/health", nil))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"server":true}` {
		t.Errorf("response = %d %s", w.Code, w.Body.String())
	}
}
