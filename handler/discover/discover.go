package discover

import (
	"fmt"

	fsClient "github.com/mager/occipital/firestore"
	"github.com/mager/occipital/occipital"
)

const maxDaysToLookBack = 5

type DiscoverResponse struct {
	Tracks  []occipital.Track `json:"tracks"`
	Updated string            `json:"updated"`
}

func convertToOccipitalTrack(fsTrack fsClient.Track, thumbType string) occipital.Track {
	return occipital.Track{
		Artist:   fsTrack.Artist,
		Name:     fsTrack.Title,
		SourceID: fsTrack.SpotifyID,
		Image:    getSpotifyThumb(fsTrack.Thumb),
		ID:       fsTrack.MBID,
		ISRC:     fsTrack.ISRC,
		Rank:     fsTrack.Rank,
	}
}

func getSpotifyThumb(th string) string {
	return fmt.Sprintf("https://i.scdn.co/image/%s", th)
}
