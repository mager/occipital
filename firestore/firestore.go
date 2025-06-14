package firestore

import (
	"context"

	"cloud.google.com/go/firestore"
	"go.uber.org/zap"
)

type TracksDoc struct {
	Tracks []Track `json:"tracks" firestore:"tracks"`
}

type Track struct {
	Rank      int    `json:"rank" firestore:"rank"`
	Artist    string `json:"artist" firestore:"artist"`
	Title     string `json:"title" firestore:"title"`
	SpotifyID string `json:"spotifyID" firestore:"spotifyID"`
	Thumb     string `json:"thumb" firestore:"thumb"`
	MBID      string `json:"mbid" firestore:"mbid"`
	ISRC      string `json:"isrc" firestore:"isrc"`
}

// ProvideDB provides a firestore client
func ProvideDB(logger *zap.SugaredLogger) *firestore.Client {
	projectID := "beatbrain-dev"

	client, err := firestore.NewClient(context.TODO(), projectID)
	if err != nil {
		logger.Error("Failed to create Firestore client", zap.Error(err))
	}
	return client
}

var Options = ProvideDB
