package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
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
	artistPageSize        = 24
	artistScanReleases    = 3
	artistTrackPageSize   = 50
	artistMaxOffset       = 1000000
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
	id      string
	section string
	cursor  artistCursor
	offset  int
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
	AlbumGroup  string  `json:"album_group"`
}

type ArtistTrack struct {
	SearchTrack
	AlbumID     string `json:"album_id"`
	AlbumName   string `json:"album_name"`
	ReleaseDate string `json:"release_date"`
}

type ArtistResponse struct {
	Artist ArtistDetails `json:"artist"`
	Tracks []ArtistTrack `json:"tracks"`
	Albums []ArtistAlbum `json:"albums"`
	// Keep old clients from treating a release cursor as a search offset.
	NextOffset        *int     `json:"next_offset"`
	NextCursor        *string  `json:"next_cursor"`
	NextReleaseOffset *int     `json:"next_release_offset"`
	ReleaseTotal      int      `json:"release_total"`
	ScannedReleases   int      `json:"scanned_releases"`
	Unavailable       []string `json:"unavailable"`
}

type artistCursor struct{ release, track int }

func (c artistCursor) String() string {
	return strconv.Itoa(c.release) + "." + strconv.Itoa(c.track)
}

func artistInteger(value string) (int, error) {
	if value == "" || strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) != -1 {
		return 0, errors.New("invalid catalog position")
	}
	n, err := strconv.Atoi(value)
	if err != nil || n > artistMaxOffset {
		return 0, errors.New("invalid catalog position")
	}
	return n, nil
}

func parseArtistQuery(query url.Values) (artistCacheKey, error) {
	key := artistCacheKey{id: query.Get("spotifyId"), section: "all"}
	if len(query["spotifyId"]) != 1 || !spotifyIDPattern.MatchString(key.id) {
		return key, errors.New("invalid Spotify artist ID")
	}
	if values, ok := query["section"]; ok {
		if len(values) != 1 || (values[0] != "all" && values[0] != "songs" && values[0] != "releases") {
			return key, errors.New("invalid artist section")
		}
		key.section = values[0]
	}
	if values, ok := query["offset"]; ok {
		if len(values) != 1 {
			return key, errors.New("invalid release offset")
		}
		n, err := artistInteger(values[0])
		if err != nil || strconv.Itoa(n) != values[0] {
			return key, errors.New("invalid release offset")
		}
		key.offset = n
	}
	if values, ok := query["cursor"]; ok {
		if len(values) != 1 {
			return key, errors.New("invalid song cursor")
		}
		parts := strings.Split(values[0], ".")
		if len(parts) != 2 {
			return key, errors.New("invalid song cursor")
		}
		release, err := artistInteger(parts[0])
		if err != nil {
			return key, err
		}
		track, err := artistInteger(parts[1])
		if err != nil {
			return key, err
		}
		key.cursor = artistCursor{release: release, track: track}
		if key.cursor.String() != values[0] {
			return key, errors.New("invalid song cursor")
		}
	}
	if key.section == "all" && (key.offset != 0 || key.cursor != (artistCursor{})) {
		return key, errors.New("all sections require the initial catalog position")
	}
	if key.section == "songs" && key.offset != 0 {
		return key, errors.New("songs require a cursor, not a release offset")
	}
	if key.section == "releases" && key.cursor != (artistCursor{}) {
		return key, errors.New("releases require an offset, not a song cursor")
	}
	return key, nil
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
	key, err := parseArtistQuery(r.URL.Query())
	if err != nil {
		util.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), artistLoadTimeout)
	defer cancel()
	if ctx.Err() != nil {
		h.writeError(w, ctx.Err())
		return
	}
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
	resp, err := h.load(ctx, key)
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

func (h *ArtistHandler) load(ctx context.Context, key artistCacheKey) (ArtistResponse, error) {
	artist, err := h.spotifyClient.Client.GetArtist(ctx, spot.ID(key.id))
	if err != nil {
		return ArtistResponse{}, err
	}
	if artist == nil || string(artist.ID) != key.id || strings.TrimSpace(artist.Name) == "" {
		return ArtistResponse{}, errors.New("artist identity mismatch")
	}
	resp := ArtistResponse{
		Artist: ArtistDetails{SearchArtist: mapArtist(*artist), Genres: append([]string{}, artist.Genres...)},
		Tracks: []ArtistTrack{}, Albums: []ArtistAlbum{}, Unavailable: []string{},
	}
	var releases *spot.SimpleAlbumPage
	if key.section != "songs" {
		releases, err = h.releases(ctx, key.id, key.offset, artistPageSize)
		if err != nil || releases == nil {
			resp.Unavailable = append(resp.Unavailable, "albums")
			resp.NextReleaseOffset = &key.offset
		} else {
			resp.ReleaseTotal = max(0, int(releases.Total))
			seen := make(map[string]bool)
			count := min(len(releases.Albums), artistPageSize)
			for _, album := range releases.Albums[:count] {
				if !validArtistAlbum(album) || seen[string(album.ID)] {
					continue
				}
				seen[string(album.ID)] = true
				resp.Albums = append(resp.Albums, ArtistAlbum{
					ID: string(album.ID), Name: album.Name, Image: spotifyImage(album.Images), URL: "https://open.spotify.com/album/" + string(album.ID),
					ReleaseDate: album.ReleaseDate, AlbumType: album.AlbumType, AlbumGroup: album.AlbumGroup,
				})
			}
			next := key.offset + count
			if releases.Next != "" || next < resp.ReleaseTotal {
				resp.NextReleaseOffset = &next
				if count == 0 || next > artistMaxOffset {
					resp.NextReleaseOffset = &key.offset
					resp.Unavailable = append(resp.Unavailable, "albums")
				}
			}
		}
	}
	if key.section != "releases" {
		// The initial release page already contains the first scan entries. If it
		// failed, try the smaller song source independently with the same deadline.
		h.scanSongs(ctx, key, &resp, releases)
	}
	return resp, nil
}

func (h *ArtistHandler) releases(ctx context.Context, id string, offset, limit int) (*spot.SimpleAlbumPage, error) {
	return h.spotifyClient.Client.GetArtistAlbums(ctx, spot.ID(id), []spot.AlbumType{spot.AlbumTypeAlbum, spot.AlbumTypeSingle, spot.AlbumTypeAppearsOn, spot.AlbumTypeCompilation}, spot.Market("US"), spot.Limit(limit), spot.Offset(offset))
}

func validArtistAlbum(album spot.SimpleAlbum) bool {
	return spotifyIDPattern.MatchString(string(album.ID)) && strings.TrimSpace(album.Name) != "" && (album.AlbumType == "album" || album.AlbumType == "single" || album.AlbumType == "compilation")
}

func (h *ArtistHandler) scanSongs(ctx context.Context, key artistCacheKey, resp *ArtistResponse, releases *spot.SimpleAlbumPage) {
	cursor := key.cursor
	var err error
	if releases == nil {
		releases, err = h.releases(ctx, key.id, cursor.release, artistScanReleases)
	}
	if err != nil || releases == nil {
		resp.songContinuation(cursor, true)
		return
	}
	resp.ReleaseTotal = max(0, int(releases.Total))
	resp.ScannedReleases = min(cursor.release, resp.ReleaseTotal)
	count := min(len(releases.Albums), artistScanReleases)
	if count == 0 {
		if releases.Next != "" || cursor.release < resp.ReleaseTotal {
			resp.songContinuation(cursor, true)
		}
		return
	}
	seen := make(map[string]bool)
	for _, album := range releases.Albums[:count] {
		if validArtistAlbum(album) {
			page, err := h.spotifyClient.Client.GetAlbumTracks(ctx, album.ID, spot.Market("US"), spot.Limit(artistTrackPageSize), spot.Offset(cursor.track))
			if err != nil || page == nil {
				resp.songContinuation(cursor, true)
				return
			}
			rawCount := min(len(page.Tracks), artistTrackPageSize)
			consumed := 0
			for _, track := range page.Tracks[:rawCount] {
				consumed++
				id := string(track.ID)
				if spotifyIDPattern.MatchString(id) && strings.TrimSpace(track.Name) != "" && !seen[id] {
					for _, credit := range track.Artists {
						if string(credit.ID) == key.id {
							seen[id] = true
							resp.Tracks = append(resp.Tracks, ArtistTrack{
								SearchTrack: SearchTrack{ID: id, Name: track.Name, Artist: util.GetFirstArtist(track.Artists), Thumb: spotifyImage(album.Images)},
								AlbumID:     string(album.ID), AlbumName: album.Name, ReleaseDate: album.ReleaseDate,
							})
							break
						}
					}
				}
				if len(resp.Tracks) == artistPageSize {
					break
				}
			}
			cursor.track += consumed
			// Consume raw provider entries, including filtered and repeated IDs. A
			// partial album must stay at its exact position in the next request.
			if consumed < rawCount || page.Next != "" || cursor.track < int(page.Total) {
				resp.songContinuation(cursor, consumed == 0 || cursor.track > artistMaxOffset)
				return
			}
		}
		cursor.release++
		cursor.track = 0
		resp.ScannedReleases = min(cursor.release, resp.ReleaseTotal)
		if len(resp.Tracks) == artistPageSize {
			break
		}
	}
	if cursor.release < resp.ReleaseTotal || cursor.release < key.cursor.release+len(releases.Albums) || releases.Next != "" {
		resp.songContinuation(cursor, false)
	}
}

func (resp *ArtistResponse) songContinuation(cursor artistCursor, unavailable bool) {
	value := cursor.String()
	resp.NextCursor = &value
	if unavailable {
		resp.Unavailable = append(resp.Unavailable, "tracks")
	}
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
