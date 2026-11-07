package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
)

func TestServerUsesPortAndTimeouts(t *testing.T) {
	t.Setenv("PORT", "9090")
	server := NewHTTPServer(fxtest.NewLifecycle(t), nil, nil, nil, zap.NewNop().Sugar())
	if server.Addr != ":9090" {
		t.Errorf("address = %s", server.Addr)
	}
	if server.ReadHeaderTimeout <= 0 || server.ReadTimeout <= 0 || server.WriteTimeout <= 0 || server.IdleTimeout <= 0 {
		t.Error("server has unbounded I/O timeouts")
	}
}

func TestServerRequiresAPIKeyForEveryPrivatePath(t *testing.T) {
	for _, tc := range []struct {
		name, key, header string
		want              int
	}{
		{"missing header", "test-key", "", 401},
		{"wrong header", "test-key", "wrong", 401},
		{"missing configuration", "", "", 401},
		{"missing configuration with header", "", "test-key", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BEATBRAIN_API_KEY", tc.key)
			server := NewHTTPServer(fxtest.NewLifecycle(t), nil, nil, nil, zap.NewNop().Sugar())
			for _, path := range []string{"/spotify/search", "/profile", "/discover/v2", "/auth/spotify", "/auth/spotify/callback", "/spotify/play", "/np", "/unknown"} {
				r := httptest.NewRequest("GET", path, nil)
				r.Header.Set("X-Beatbrain-Key", tc.header)
				w := httptest.NewRecorder()
				server.Handler.ServeHTTP(w, r)
				if w.Code != tc.want || !json.Valid(w.Body.Bytes()) {
					t.Fatalf("%s response = %d %s", path, w.Code, w.Body.String())
				}
			}
		})
	}
}

func TestServerAcceptsAPIKey(t *testing.T) {
	t.Setenv("BEATBRAIN_API_KEY", "test-key")
	server := NewHTTPServer(fxtest.NewLifecycle(t), nil, nil, nil, zap.NewNop().Sugar())
	r := httptest.NewRequest("GET", "/spotify/search", nil)
	r.Header.Set("X-Beatbrain-Key", "test-key")
	w := httptest.NewRecorder()
	server.Handler.ServeHTTP(w, r)
	if w.Code != 405 || w.Header().Get("Allow") != "POST" {
		t.Errorf("request did not reach search handler: %d %s", w.Code, w.Body.String())
	}
}

func TestServerHealthIsPublicWithoutAPIKey(t *testing.T) {
	t.Setenv("BEATBRAIN_API_KEY", "")
	server := NewHTTPServer(fxtest.NewLifecycle(t), nil, nil, nil, zap.NewNop().Sugar())
	w := httptest.NewRecorder()
	server.Handler.ServeHTTP(w, httptest.NewRequest("GET", "/health", nil))
	if w.Code != 200 || w.Body.String() != "{\"server\":true}\n" {
		t.Errorf("health response = %d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{"/health/", "/health/../profile"} {
		w := httptest.NewRecorder()
		server.Handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 401 {
			t.Errorf("health bypass: %s = %d", path, w.Code)
		}
	}
}

func TestRetiredEndpointsAreNotRegistered(t *testing.T) {
	t.Setenv("BEATBRAIN_API_KEY", "test-key")
	server := NewHTTPServer(fxtest.NewLifecycle(t), nil, nil, nil, zap.NewNop().Sugar())
	for _, path := range []string{"/user", "/profile", "/auth/spotify", "/auth/spotify/callback", "/spotify/play", "/spotify/pause", "/spotify/devices", "/np", "/spotify/get_featured_tracks"} {
		t.Run(path, func(t *testing.T) {
			r := httptest.NewRequest("GET", path, nil)
			r.Header.Set("X-Beatbrain-Key", "test-key")
			w := httptest.NewRecorder()
			server.Handler.ServeHTTP(w, r)
			if w.Code != 404 {
				t.Errorf("retired endpoint returned %d, want 404", w.Code)
			}
		})
	}
}
