package spotify

import (
	"strings"

	spot "github.com/zmb3/spotify/v2"
)

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
