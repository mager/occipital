package util

import (
	spot "github.com/zmb3/spotify/v2"
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
