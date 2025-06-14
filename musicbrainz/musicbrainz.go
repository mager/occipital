package musicbrainz

import (
	"github.com/mager/musicbrainz-go/musicbrainz"
)

type MusicbrainzClient struct {
	Client *musicbrainz.MusicbrainzClient
}

func ProvideMusicbrainz() *MusicbrainzClient {
	var c MusicbrainzClient
	c.Client = musicbrainz.NewMusicbrainzClient().
		WithUserAgent("beatbrain/occipital", "1.0.0", "https://github.com/mager/occipital")

	return &c
}

var Options = ProvideMusicbrainz
