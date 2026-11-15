package spotify

import (
	"net/url"
	"regexp"
	"strings"

	spot "github.com/zmb3/spotify/v2"
)

var spotifyIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{22}$`)

type SearchArtist struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Image *string `json:"image"`
	URL   string  `json:"url"`
}

func mapArtist(artist spot.FullArtist) SearchArtist {
	return SearchArtist{
		ID: string(artist.ID), Name: artist.Name,
		Image: spotifyImage(artist.Images), URL: "https://open.spotify.com/artist/" + string(artist.ID),
	}
}

func spotifyImage(images []spot.Image) *string {
	for _, image := range images {
		u, err := url.Parse(image.URL)
		if err == nil && u.Scheme == "https" && u.Host == "i.scdn.co" && u.User == nil && strings.HasPrefix(u.Path, "/image/") && len(u.Path) > len("/image/") {
			return &image.URL
		}
	}
	return nil
}
