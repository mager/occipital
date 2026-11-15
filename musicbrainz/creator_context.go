package musicbrainz

import (
	"context"
	"errors"
	mb "github.com/mager/musicbrainz-go/musicbrainz"
	"net/http"
	"net/url"
)

// CreatorArtist retains scope, direction and attributes omitted by the legacy client.
type CreatorArtist struct {
	mb.Artist
	Relations []CreatorRelation `json:"relations"`
}
type CreatorRelation struct {
	Type       string             `json:"type"`
	TargetType string             `json:"target-type"`
	Direction  string             `json:"direction"`
	Attributes []string           `json:"attributes"`
	Begin      string             `json:"begin"`
	End        string             `json:"end"`
	Ended      bool               `json:"ended"`
	Artist     *mb.RelationArtist `json:"artist"`
	Recording  *CreatorEntity     `json:"recording"`
	Release    *CreatorEntity     `json:"release"`
	Work       *CreatorEntity     `json:"work"`
	URL        mb.URL             `json:"url"`
}
type CreatorEntity struct {
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	Disambiguation   string            `json:"disambiguation"`
	FirstReleaseDate string            `json:"first-release-date"`
	Date             string            `json:"date"`
	ArtistCredits    []mb.ArtistCredit `json:"artist-credit"`
}

func (c *MusicbrainzClient) GetCreatorArtistContext(ctx context.Context, id string) (*CreatorArtist, error) {
	var artist CreatorArtist
	err := c.getContext(ctx, "artist/"+id, url.Values{"inc": {"genres+url-rels+recording-rels+release-rels+work-rels+artist-rels+artist-credits"}}, &artist)
	return &artist, err
}
func IsNotFound(err error) bool {
	var status *recordingStatusError
	return errors.As(err, &status) && status.status == http.StatusNotFound
}
