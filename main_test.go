package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mager/occipital/config"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
)

func TestMalformedAuthorizationDoesNotStopServer(t *testing.T) {
	if os.Getenv("OCCIPITAL_TEST_AUTH_CHILD") == "1" {
		nextSecret = "test-secret"
		h := authNMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid token accepted") }), zap.NewNop().Sugar())
		for _, header := range []string{"Basic abc", "Bearer", "Bearer ", "Bearer a b"} {
			r := httptest.NewRequest("GET", "/user", nil)
			r.Header.Set("Authorization", header)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 401 || !json.Valid(w.Body.Bytes()) {
				t.Errorf("response = %d %s", w.Code, w.Body.String())
			}
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMalformedAuthorizationDoesNotStopServer$")
	cmd.Env = append(os.Environ(), "OCCIPITAL_TEST_AUTH_CHILD=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("malformed header stopped request handling: %v\n%s", err, out)
	}
}

func TestAuthenticationRejectsEmptySecretAndUnexpectedAlgorithm(t *testing.T) {
	previous := nextSecret
	t.Cleanup(func() { nextSecret = previous })
	for _, tc := range []struct {
		name, secret string
		method       jwt.SigningMethod
		want         int
	}{
		{"empty secret", "", jwt.SigningMethodHS256, 401},
		{"wrong algorithm", "test-secret", jwt.SigningMethodHS384, 401},
		{"valid token", "test-secret", jwt.SigningMethodHS256, 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nextSecret = tc.secret
			token, err := jwt.NewWithClaims(tc.method, jwt.MapClaims{"email": "test@example.test", "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte(tc.secret))
			if err != nil {
				t.Fatal(err)
			}
			h := authNMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), zap.NewNop().Sugar())
			r := httptest.NewRequest("GET", "/user", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Errorf("status = %d, want %d", w.Code, tc.want)
			}
			if tc.want == 401 && !json.Valid(w.Body.Bytes()) {
				t.Errorf("error is not JSON: %s", w.Body.String())
			}
		})
	}
}

func TestServerUsesPortAndTimeouts(t *testing.T) {
	t.Setenv("PORT", "9090")
	server := NewHTTPServer(fxtest.NewLifecycle(t), config.Config{}, nil, nil, nil, nil, zap.NewNop().Sugar())
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
			server := NewHTTPServer(fxtest.NewLifecycle(t), config.Config{}, nil, nil, nil, nil, zap.NewNop().Sugar())
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
	server := NewHTTPServer(fxtest.NewLifecycle(t), config.Config{}, nil, nil, nil, nil, zap.NewNop().Sugar())
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
	server := NewHTTPServer(fxtest.NewLifecycle(t), config.Config{}, nil, nil, nil, nil, zap.NewNop().Sugar())
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
