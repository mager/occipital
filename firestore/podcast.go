package firestore

import "time"

// PodcastShow represents a podcast show stored in the podcast_shows collection
type PodcastShow struct {
	ID           string    `json:"id" firestore:"id"`
	Name         string    `json:"name" firestore:"name"`
	Publisher    string    `json:"publisher" firestore:"publisher"`
	Description  string    `json:"description" firestore:"description"`
	Categories   []string  `json:"categories" firestore:"categories"`
	Languages    []string  `json:"languages" firestore:"languages"`
	ImageURL     string    `json:"imageURL" firestore:"imageURL"`
	EpisodeCount int       `json:"episodeCount" firestore:"episodeCount"`
	Explicit     bool      `json:"explicit" firestore:"explicit"`
	ExternalURL  string    `json:"externalURL" firestore:"externalURL"`
	MediaType    string    `json:"mediaType" firestore:"mediaType"`
	DiscoveredIn string    `json:"discoveredIn" firestore:"discoveredIn"`
	FirstSeenAt  time.Time `json:"firstSeenAt" firestore:"firstSeenAt"`
	LastUpdated  time.Time `json:"lastUpdated" firestore:"lastUpdated"`
}
