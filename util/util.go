package util

import (
	"sort"
	"strings"

	mb "github.com/mager/musicbrainz-go/musicbrainz"
	spot "github.com/zmb3/spotify/v2"
	"golang.org/x/exp/maps"
)

func GetThumb(a spot.SimpleAlbum) *string {
	var o string

	// Iterate through all images to find the one with height and width 300
	for _, img := range a.Images {
		if img.Height == 300 && img.Width == 300 {
			o = img.URL
			return &o
		}
	}

	// If no image with height and width 300 is found, return nil
	return nil
}

func GetFirstArtist(artists []spot.SimpleArtist) string {
	if len(artists) == 0 {
		return "Various Arists"
	}

	return artists[0].Name
}

func GetReleaseDate(album spot.SimpleAlbum) *string {
	return &album.ReleaseDate
}

// GetGenresForArtists returns the most common genres among the given artists, ranked by their number of occurrences
func GetGenresForArtists(artists []*spot.FullArtist) []string {
	allGenres := make(map[string]int) // Use a map to count genres directly

	for _, artist := range artists {
		if artist == nil || len(artist.Genres) == 0 {
			continue
		}
		// Split the artist's genres string into individual genres
		genres := strings.Split(artist.Genres[0], " ")
		for _, genre := range genres {
			// Count the occurrences of each genre
			allGenres[genre]++
		}
	}

	// Sort the genres by frequency (merging declaration and assignment)
	var sorted []string
	sorted = maps.Keys(allGenres)
	sort.Slice(sorted, func(i, j int) bool {
		return allGenres[sorted[i]] > allGenres[sorted[j]]
	})

	return sorted
}

func GetISRC(track *spot.FullTrack) *string {
	if isrc, ok := track.ExternalIDs["isrc"]; ok {
		return &isrc
	}

	return nil
}

// GetArtistCredits returns a formatted string of artist credits from a MusicBrainz recording
func GetArtistCredits(artistCredits []mb.ArtistCredit) string {
	if len(artistCredits) == 0 {
		return "Various Artists"
	}

	// Build the artist string by joining names with their join phrases
	var result strings.Builder
	for i, credit := range artistCredits {
		result.WriteString(credit.Name)
		if i < len(artistCredits)-1 && credit.JoinPhrase != "" {
			result.WriteString(credit.JoinPhrase)
		}
	}

	return result.String()
}

// GetArtistCreditsFromRecording returns a formatted string of artist credits from a MusicBrainz recording
func GetArtistCreditsFromRecording(artistCredits []mb.ArtistCredit) string {
	if len(artistCredits) == 0 {
		return "Various Artists"
	}

	var result strings.Builder
	for i, credit := range artistCredits {
		result.WriteString(credit.Name)
		if i < len(artistCredits)-1 {
			result.WriteString(", ")
		}
	}
	return result.String()
}
