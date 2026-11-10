package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type CatalogAlbum struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	TotalTracks int    `json:"total_tracks"`
	Copyrights  []struct {
		Text string `json:"text"`
		Type string `json:"type"`
	} `json:"copyrights"`
}

// The older SDK drops label and total_tracks. Decode the public catalog fields
// directly with the same authenticated, bounded HTTP client.
func (c *SpotifyClient) GetCatalogAlbum(ctx context.Context, id string) (*CatalogAlbum, error) {
	if c.HTTP == nil {
		return nil, fmt.Errorf("catalog HTTP client unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.spotify.com/v1/albums/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("album source status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
	if err != nil || len(b) > 1024*1024 {
		return nil, fmt.Errorf("invalid album response")
	}
	var album CatalogAlbum
	if json.Unmarshal(b, &album) != nil || album.ID != id {
		return nil, fmt.Errorf("album identity mismatch")
	}
	return &album, nil
}
