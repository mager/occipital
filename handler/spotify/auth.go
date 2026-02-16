package spotify

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/mager/occipital/config"
	spot "github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

// SpotifyToken is stored in Firestore per user.
type SpotifyToken struct {
	AccessToken  string `json:"access_token" firestore:"access_token"`
	RefreshToken string `json:"refresh_token" firestore:"refresh_token"`
	TokenType    string `json:"token_type" firestore:"token_type"`
	Expiry       int64  `json:"expiry" firestore:"expiry"`
}

var userScopes = []string{
	spotifyauth.ScopeUserReadPlaybackState,
	spotifyauth.ScopeUserModifyPlaybackState,
	spotifyauth.ScopeUserReadCurrentlyPlaying,
}

func newAuthenticator(cfg config.Config) *spotifyauth.Authenticator {
	return spotifyauth.New(
		spotifyauth.WithClientID(cfg.SpotifyID),
		spotifyauth.WithClientSecret(cfg.SpotifySecret),
		spotifyauth.WithRedirectURL(cfg.SpotifyRedirectURL),
		spotifyauth.WithScopes(userScopes...),
	)
}

// --- Auth Login Handler ---

// AuthLoginHandler redirects the user to Spotify's OAuth consent screen.
type AuthLoginHandler struct {
	log  *zap.SugaredLogger
	auth *spotifyauth.Authenticator
}

func (*AuthLoginHandler) Pattern() string {
	return "/auth/spotify"
}

func NewAuthLoginHandler(log *zap.SugaredLogger, cfg config.Config) *AuthLoginHandler {
	return &AuthLoginHandler{log: log, auth: newAuthenticator(cfg)}
}

func (h *AuthLoginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, `{"error":"missing user_id"}`, http.StatusBadRequest)
		return
	}
	url := h.auth.AuthURL(userID)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// --- Auth Callback Handler ---

// AuthCallbackHandler exchanges the OAuth code for tokens and stores them.
type AuthCallbackHandler struct {
	log  *zap.SugaredLogger
	auth *spotifyauth.Authenticator
	fs   *firestore.Client
}

func (*AuthCallbackHandler) Pattern() string {
	return "/auth/spotify/callback"
}

func NewAuthCallbackHandler(log *zap.SugaredLogger, cfg config.Config, fs *firestore.Client) *AuthCallbackHandler {
	return &AuthCallbackHandler{log: log, auth: newAuthenticator(cfg), fs: fs}
}

func (h *AuthCallbackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	userID := r.URL.Query().Get("state")
	if userID == "" {
		http.Error(w, `{"error":"missing state"}`, http.StatusBadRequest)
		return
	}

	token, err := h.auth.Token(ctx, userID, r)
	if err != nil {
		h.log.Errorw("Failed to exchange Spotify token", "error", err)
		http.Error(w, `{"error":"token exchange failed"}`, http.StatusInternalServerError)
		return
	}

	if err := storeToken(ctx, h.fs, userID, token); err != nil {
		h.log.Errorw("Failed to store token", "error", err)
		http.Error(w, `{"error":"failed to store token"}`, http.StatusInternalServerError)
		return
	}

	h.log.Infow("Spotify account connected", "user_id", userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "connected",
		"user_id": userID,
	})
}

// --- Token Helpers ---

func storeToken(ctx context.Context, fs *firestore.Client, userID string, token *oauth2.Token) error {
	st := SpotifyToken{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry.Unix(),
	}
	_, err := fs.Collection("spotify_tokens").Doc(userID).Set(ctx, st)
	return err
}

func loadToken(ctx context.Context, fs *firestore.Client, userID string) (*oauth2.Token, error) {
	doc, err := fs.Collection("spotify_tokens").Doc(userID).Get(ctx)
	if err != nil {
		return nil, err
	}
	var st SpotifyToken
	if err := doc.DataTo(&st); err != nil {
		return nil, err
	}
	return &oauth2.Token{
		AccessToken:  st.AccessToken,
		RefreshToken: st.RefreshToken,
		TokenType:    st.TokenType,
		Expiry:       time.Unix(st.Expiry, 0),
	}, nil
}

// getUserSpotifyClient creates a per-user Spotify client from stored OAuth tokens.
// The underlying oauth2 transport handles token refresh automatically.
func getUserSpotifyClient(ctx context.Context, cfg config.Config, fs *firestore.Client, userID string) (*spot.Client, error) {
	token, err := loadToken(ctx, fs, userID)
	if err != nil {
		return nil, err
	}

	auth := newAuthenticator(cfg)
	httpClient := auth.Client(ctx, token)
	return spot.New(httpClient), nil
}
