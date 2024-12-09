package firestore

import (
	"context"
	"log"

	"cloud.google.com/go/firestore"
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
}

// ProvideDB provides a firestore client
func ProvideDB() *firestore.Client {
	projectID := "beatbrain-dev"

	client, err := firestore.NewClient(context.TODO(), projectID)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	return client
}

var Options = ProvideDB
