package musicbrainz

import (
	"github.com/mager/musicbrainz-go/musicbrainz"
)

type MusicbrainzClient struct {
	Client *musicbrainz.MusicbrainzClient
}

func ProvideMusicbrainz() *MusicbrainzClient {
	var c MusicbrainzClient
	c.Client = musicbrainz.NewMusicbrainzClient()
	return &c
}

var Options = ProvideMusicbrainz
