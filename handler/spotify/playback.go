package spotify

import (
	"context"
	"encoding/json"
	"net/http"

	"cloud.google.com/go/firestore"
	"github.com/mager/occipital/config"
	spot "github.com/zmb3/spotify/v2"
	"go.uber.org/zap"
)

// --- Play Handler ---

// PlayHandler starts playback of a track on the user's active Spotify device.
type PlayHandler struct {
	log *zap.SugaredLogger
	cfg config.Config
	fs  *firestore.Client
}

func (*PlayHandler) Pattern() string {
	return "/spotify/play"
}

func NewPlayHandler(log *zap.SugaredLogger, cfg config.Config, fs *firestore.Client) *PlayHandler {
	return &PlayHandler{log: log, cfg: cfg, fs: fs}
}

type PlayRequest struct {
	UserID   string `json:"user_id"`
	TrackID  string `json:"track_id"`            // Spotify track ID (e.g. "6rqhFgbbKwnb9MLmUQDhG6")
	DeviceID string `json:"device_id,omitempty"`  // Optional: target a specific device
}

type PlayResponse struct {
	Status   string `json:"status"`
	TrackID  string `json:"track_id"`
	DeviceID string `json:"device_id,omitempty"`
}

func (h *PlayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req PlayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.UserID == "" || req.TrackID == "" {
		http.Error(w, `{"error":"user_id and track_id are required"}`, http.StatusBadRequest)
		return
	}

	client, err := getUserSpotifyClient(ctx, h.cfg, h.fs, req.UserID)
	if err != nil {
		h.log.Errorw("Failed to get user Spotify client", "error", err, "user_id", req.UserID)
		http.Error(w, `{"error":"spotify not connected for this user"}`, http.StatusUnauthorized)
		return
	}

	trackURI := spot.URI("spotify:track:" + req.TrackID)
	opts := &spot.PlayOptions{
		URIs: []spot.URI{trackURI},
	}

	if req.DeviceID != "" {
		deviceID := spot.ID(req.DeviceID)
		opts.DeviceID = &deviceID
	}

	if err := client.PlayOpt(ctx, opts); err != nil {
		h.log.Errorw("Failed to start playback", "error", err, "track_id", req.TrackID)
		http.Error(w, `{"error":"playback failed: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	h.log.Infow("Playback started", "user_id", req.UserID, "track_id", req.TrackID)

	json.NewEncoder(w).Encode(PlayResponse{
		Status:   "playing",
		TrackID:  req.TrackID,
		DeviceID: req.DeviceID,
	})
}

// --- Pause Handler ---

// PauseHandler pauses the user's current Spotify playback.
type PauseHandler struct {
	log *zap.SugaredLogger
	cfg config.Config
	fs  *firestore.Client
}

func (*PauseHandler) Pattern() string {
	return "/spotify/pause"
}

func NewPauseHandler(log *zap.SugaredLogger, cfg config.Config, fs *firestore.Client) *PauseHandler {
	return &PauseHandler{log: log, cfg: cfg, fs: fs}
}

type PauseRequest struct {
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id,omitempty"`
}

func (h *PauseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPut {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req PauseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.UserID == "" {
		http.Error(w, `{"error":"user_id is required"}`, http.StatusBadRequest)
		return
	}

	client, err := getUserSpotifyClient(ctx, h.cfg, h.fs, req.UserID)
	if err != nil {
		http.Error(w, `{"error":"spotify not connected for this user"}`, http.StatusUnauthorized)
		return
	}

	if err := client.Pause(ctx); err != nil {
		h.log.Errorw("Failed to pause playback", "error", err)
		http.Error(w, `{"error":"pause failed: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "paused"})
}

// --- Devices Handler ---

// DevicesHandler lists the user's available Spotify playback devices.
type DevicesHandler struct {
	log *zap.SugaredLogger
	cfg config.Config
	fs  *firestore.Client
}

func (*DevicesHandler) Pattern() string {
	return "/spotify/devices"
}

func NewDevicesHandler(log *zap.SugaredLogger, cfg config.Config, fs *firestore.Client) *DevicesHandler {
	return &DevicesHandler{log: log, cfg: cfg, fs: fs}
}

type DeviceInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	IsActive bool   `json:"is_active"`
	Volume   int    `json:"volume"`
}

func (h *DevicesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, `{"error":"missing user_id"}`, http.StatusBadRequest)
		return
	}

	client, err := getUserSpotifyClient(ctx, h.cfg, h.fs, userID)
	if err != nil {
		http.Error(w, `{"error":"spotify not connected for this user"}`, http.StatusUnauthorized)
		return
	}

	devices, err := client.PlayerDevices(ctx)
	if err != nil {
		h.log.Errorw("Failed to get devices", "error", err)
		http.Error(w, `{"error":"failed to get devices"}`, http.StatusInternalServerError)
		return
	}

	var result []DeviceInfo
	for _, d := range devices {
		result = append(result, DeviceInfo{
			ID:       string(d.ID),
			Name:     d.Name,
			Type:     d.Type,
			IsActive: d.Active,
			Volume:   int(d.Volume),
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"devices": result,
	})
}
