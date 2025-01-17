package spotify

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mager/occipital/spotify"
	"go.uber.org/zap"
)

type PlayerStateUpdate struct {
	TrackName  string `json:"track_name"`
	ArtistName string `json:"artist_name"`
	IsPlaying  bool   `json:"is_playing"`
	// Add other relevant fields from spotify.PlayerState
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// In production, you should validate the origin of the request
		return true
	},
}

// PlayerHandler handles WebSocket connections
func PlayerHandler(w http.ResponseWriter, r *http.Request, spotifyClient *spotify.SpotifyClient, logger *zap.SugaredLogger) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Errorw("Error upgrading connection to WebSocket", "error", err)
		return
	}
	defer conn.Close()

	logger.Info("WebSocket client connected")

	// Start a ticker to send updates periodically
	ticker := time.NewTicker(1 * time.Second) // Adjust the interval as needed
	defer ticker.Stop()

	for range ticker.C {
		playerState, err := spotifyClient.Client.PlayerState(context.Background())
		if err != nil {
			logger.Errorw("Error fetching Spotify player state", "error", err)
			return // Consider if you want to close the connection on error
		}

		if playerState.CurrentlyPlaying.Item == nil {
			// No track playing
			err = conn.WriteJSON(PlayerStateUpdate{}) // Send an empty or default state
			if err != nil {
				logger.Errorw("Error sending WebSocket message", "error", err)
				return
			}
			continue
		}

		update := PlayerStateUpdate{
			TrackName:  playerState.CurrentlyPlaying.Item.Name,
			ArtistName: playerState.CurrentlyPlaying.Item.Artists[0].Name,
			IsPlaying:  playerState.CurrentlyPlaying.Playing,
		}

		err = conn.WriteJSON(update)
		if err != nil {
			logger.Errorw("Error sending WebSocket message", "error", err)
			return // Client likely disconnected
		}
	}
}
