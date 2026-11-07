package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	spot "github.com/zmb3/spotify/v2"

	"github.com/mager/occipital/spotify"
	"github.com/mager/occipital/util"
	"go.uber.org/zap"
)

// SearchHandler searches the public track catalog without user authorization.
type SearchHandler struct {
	log           *zap.SugaredLogger
	spotifyClient *spotify.SpotifyClient
}

func (*SearchHandler) Pattern() string {
	return "/spotify/search"
}

// NewSearchHandler builds a new SearchHandler.
func NewSearchHandler(log *zap.SugaredLogger, spotifyClient *spotify.SpotifyClient) *SearchHandler {
	return &SearchHandler{
		log:           log,
		spotifyClient: spotifyClient,
	}
}

type SearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type SearchResponse struct {
	Results []SearchTrack `json:"results"`
}

type SearchTrack struct {
	Artist     string  `json:"artist"`
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Popularity int     `json:"popularity"`
	Thumb      *string `json:"thumb"`
}

// Search for tracks on Spotify
// @Summary Search Spotify for tracks
// @Description Search for tracks on Spotify using a query string.
// @Tags Spotify
// @Accept json
// @Produce json
// @Param request body SearchRequest true "Search query"
// @Success 200 {object} SearchResponse
// @Router /spotify/search [post]
func (h *SearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req SearchRequest
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		util.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&req)
	if err == nil {
		// Reject trailing values and read to EOF so the size limit covers the body.
		if trailingErr := decoder.Decode(new(any)); trailingErr != io.EOF {
			err = trailingErr
			if err == nil {
				err = errors.New("multiple JSON values")
			}
		}
	}
	if err != nil {
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			util.WriteJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			util.WriteJSONError(w, http.StatusBadRequest, "invalid JSON request")
		}
		return
	}

	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		util.WriteJSONError(w, http.StatusBadRequest, "missing search query")
		return
	}
	if utf8.RuneCountInString(req.Query) > 200 {
		util.WriteJSONError(w, http.StatusBadRequest, "search query too long")
		return
	}
	if req.Limit < 0 {
		util.WriteJSONError(w, http.StatusBadRequest, "limit must not be negative")
		return
	}
	if req.Limit == 0 {
		req.Limit = 20
	} else if req.Limit > 50 {
		req.Limit = 50
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	results, err := h.spotifyClient.Client.Search(ctx, req.Query, spot.SearchTypeTrack, spot.Limit(req.Limit))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		if errors.Is(err, context.DeadlineExceeded) {
			util.WriteJSONError(w, http.StatusGatewayTimeout, "search timed out")
			return
		}
		h.log.Errorw("Spotify search failed", "error", err)
		util.WriteJSONError(w, http.StatusBadGateway, "search unavailable")
		return
	}

	resp := SearchResponse{Results: make([]SearchTrack, 0)}

	if results.Tracks != nil {
		for _, item := range results.Tracks.Tracks {
			resp.Results = append(resp.Results, mapTrack(item))
		}

		// Sorting by Popularity (descending order)
		sort.Slice(resp.Results, func(i, j int) bool {
			return resp.Results[i].Popularity > resp.Results[j].Popularity
		})
	}

	json.NewEncoder(w).Encode(resp)
}

func mapTrack(t spot.FullTrack) SearchTrack {
	var o SearchTrack

	o.Artist = util.GetFirstArtist(t.Artists)
	o.ID = string(t.ID)
	o.Name = t.Name
	o.Popularity = int(t.Popularity)

	o.Thumb = util.GetThumb(t.Album)

	return o
}
