package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mager/occipital/spotify"
	"github.com/mager/occipital/util"
	spot "github.com/zmb3/spotify/v2"
	"go.uber.org/zap"
)

const (
	artistLoadTimeout     = 6 * time.Second
	artistCacheTTL        = 5 * time.Minute
	artistCacheMaxEntries = 128
	artistCacheMaxBytes   = 128 * 1024
	artistPageSize        = 10
	artistMaxOffset       = 990
)

// ArtistHandler serves a bounded artist catalog using the service credentials.
type ArtistHandler struct {
	log           *zap.SugaredLogger
	spotifyClient *spotify.SpotifyClient
	loads         chan struct{}
	mu            sync.RWMutex
	cache         map[artistCacheKey]cachedArtist
}

type artistCacheKey struct {
	id     string
	offset int
}
type cachedArtist struct {
	body    []byte
	expires time.Time
}

type ArtistDetails struct {
	SearchArtist
	Genres []string `json:"genres"`
}

type ArtistAlbum struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Image       *string `json:"image"`
	URL         string  `json:"url"`
	ReleaseDate string  `json:"release_date"`
	AlbumType   string  `json:"album_type"`
}

type ArtistResponse struct {
	Artist      ArtistDetails `json:"artist"`
	Tracks      []SearchTrack `json:"tracks"`
	Albums      []ArtistAlbum `json:"albums"`
	NextOffset  *int          `json:"next_offset"`
	Unavailable []string      `json:"unavailable"`
}

func NewArtistHandler(log *zap.SugaredLogger, spotifyClient *spotify.SpotifyClient) *ArtistHandler {
	return &ArtistHandler{log: log, spotifyClient: spotifyClient, loads: make(chan struct{}, 8), cache: make(map[artistCacheKey]cachedArtist)}
}

func (*ArtistHandler) Pattern() string { return "/artist" }

func (h *ArtistHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		util.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	query := r.URL.Query()
	id := query.Get("spotifyId")
	if len(query["spotifyId"]) != 1 || !spotifyIDPattern.MatchString(id) {
		util.WriteJSONError(w, http.StatusBadRequest, "invalid Spotify artist ID")
		return
	}
	offset := 0
	if values, present := query["offset"]; present {
		var err error
		if len(values) == 1 && values[0] != "" && strings.IndexFunc(values[0], func(r rune) bool { return r < '0' || r > '9' }) == -1 {
			offset, err = strconv.Atoi(values[0])
		} else {
			err = errors.New("invalid offset")
		}
		if err != nil || offset > artistMaxOffset {
			util.WriteJSONError(w, http.StatusBadRequest, "offset must be an integer from 0 to 990")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), artistLoadTimeout)
	defer cancel()
	if ctx.Err() != nil {
		h.writeError(w, ctx.Err())
		return
	}
	key := artistCacheKey{id: id, offset: offset}
	if body, ok := h.cached(key, time.Now()); ok {
		w.Write(body)
		return
	}
	// Keep pending provider work bounded. A waiting request retains its deadline
	// and does not start a load after the caller cancels it.
	select {
	case h.loads <- struct{}{}:
		defer func() { <-h.loads }()
	case <-ctx.Done():
		h.writeError(w, ctx.Err())
		return
	}
	if ctx.Err() != nil {
		h.writeError(w, ctx.Err())
		return
	}
	if body, ok := h.cached(key, time.Now()); ok {
		w.Write(body)
		return
	}
	resp, err := h.load(ctx, id, offset)
	if errors.Is(r.Context().Err(), context.Canceled) {
		return
	}
	if err != nil {
		h.writeError(w, err)
		return
	}
	body, err := json.Marshal(resp)
	if err != nil {
		util.WriteJSONError(w, http.StatusInternalServerError, "artist unavailable")
		return
	}
	if len(resp.Unavailable) == 0 && ctx.Err() == nil {
		h.remember(key, body, time.Now())
	}
	w.Write(body)
}

func (h *ArtistHandler) load(ctx context.Context, id string, offset int) (ArtistResponse, error) {
	artist, err := h.spotifyClient.Client.GetArtist(ctx, spot.ID(id))
	if err != nil {
		return ArtistResponse{}, err
	}
	if artist == nil || string(artist.ID) != id || strings.TrimSpace(artist.Name) == "" {
		return ArtistResponse{}, errors.New("artist identity mismatch")
	}
	resp := ArtistResponse{
		Artist: ArtistDetails{SearchArtist: mapArtist(*artist), Genres: append([]string{}, artist.Genres...)},
		Tracks: []SearchTrack{}, Albums: []ArtistAlbum{}, Unavailable: []string{},
	}
	var tracks *spot.SearchResult
	var albums *spot.SimpleAlbumPage
	var tracksErr, albumsErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// Quoting the catalog name narrows results. Identity checks below keep
		// namesakes and other search matches out of this artist's tracks.
		name := strings.NewReplacer(`\`, " ", `"`, " ").Replace(artist.Name)
		tracks, tracksErr = h.spotifyClient.Client.Search(ctx, `artist:"`+name+`"`, spot.SearchTypeTrack, spot.Limit(artistPageSize), spot.Offset(offset))
	}()
	go func() {
		defer wg.Done()
		albums, albumsErr = h.spotifyClient.Client.GetArtistAlbums(ctx, spot.ID(id), []spot.AlbumType{spot.AlbumTypeAlbum, spot.AlbumTypeSingle}, spot.Limit(artistPageSize))
	}()
	wg.Wait()
	if tracksErr != nil || tracks == nil || tracks.Tracks == nil {
		resp.Unavailable = append(resp.Unavailable, "tracks")
	} else {
		for i, track := range tracks.Tracks.Tracks {
			if i >= artistPageSize {
				break
			}
			for _, artist := range track.Artists {
				if string(artist.ID) == id {
					mapped := mapTrack(track)
					mapped.Thumb = spotifyImage(track.Album.Images)
					resp.Tracks = append(resp.Tracks, mapped)
					break
				}
			}
		}
		if next := offset + artistPageSize; tracks.Tracks.Next != "" && next <= artistMaxOffset {
			resp.NextOffset = &next
		}
	}
	if albumsErr != nil || albums == nil {
		resp.Unavailable = append(resp.Unavailable, "albums")
	} else {
		for _, album := range albums.Albums {
			if len(resp.Albums) >= artistPageSize {
				break
			}
			if !spotifyIDPattern.MatchString(string(album.ID)) || (album.AlbumType != "album" && album.AlbumType != "single") {
				continue
			}
			resp.Albums = append(resp.Albums, ArtistAlbum{
				ID: string(album.ID), Name: album.Name, Image: spotifyImage(album.Images),
				URL: "https://open.spotify.com/album/" + string(album.ID), ReleaseDate: album.ReleaseDate, AlbumType: album.AlbumType,
			})
		}
	}
	return resp, nil
}

func (h *ArtistHandler) writeError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		util.WriteJSONError(w, http.StatusGatewayTimeout, "artist request timed out")
		return
	}
	var sourceError spot.Error
	if errors.As(err, &sourceError) && sourceError.Status == http.StatusNotFound {
		util.WriteJSONError(w, http.StatusNotFound, "artist not found")
		return
	}
	h.log.Warn("Spotify artist lookup failed")
	util.WriteJSONError(w, http.StatusBadGateway, "artist unavailable")
}

func (h *ArtistHandler) cached(key artistCacheKey, now time.Time) ([]byte, bool) {
	h.mu.RLock()
	entry, ok := h.cache[key]
	h.mu.RUnlock()
	return entry.body, ok && now.Before(entry.expires)
}

func (h *ArtistHandler) remember(key artistCacheKey, body []byte, now time.Time) {
	if len(body) > artistCacheMaxBytes {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for key, entry := range h.cache {
		if !now.Before(entry.expires) {
			delete(h.cache, key)
		}
	}
	if len(h.cache) >= artistCacheMaxEntries {
		var oldest artistCacheKey
		var expiry time.Time
		for key, entry := range h.cache {
			if expiry.IsZero() || entry.expires.Before(expiry) {
				oldest, expiry = key, entry.expires
			}
		}
		delete(h.cache, oldest)
	}
	h.cache[key] = cachedArtist{body: body, expires: now.Add(artistCacheTTL)}
}
