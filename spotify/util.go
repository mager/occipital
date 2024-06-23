package spotify

import (
	"strings"

	spot "github.com/zmb3/spotify/v2"
)

// GetFirstArtist returns the first artist
func GetFirstArtist(artists []spot.SimpleArtist) string {
	if len(artists) == 0 {
		return "Various Arists"
	}

	return artists[0].Name
}

// ConcatArtists returns a comma-separated list of artist names
func ConcatArtists(artists []spot.SimpleArtist) string {
	names := make([]string, len(artists))
	for i, a := range artists {
		names[i] = a.Name
	}
	return strings.Join(names, ", ")
}

func ExtractID(uri spot.URI) spot.ID {
	parts := strings.Split(string(uri), ":")
	return spot.ID(parts[2])
}
