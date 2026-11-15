package creator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

type cachedDiscovery struct {
	body    json.RawMessage
	expires time.Time
}
type GetCreatorDiscoveryHandler struct {
	log     *zap.SugaredLogger
	mu      sync.Mutex
	cache   map[string]cachedDiscovery
	pending singleflight.Group
}

func NewGetCreatorDiscoveryHandler(log *zap.SugaredLogger) *GetCreatorDiscoveryHandler {
	return &GetCreatorDiscoveryHandler{log: log, cache: make(map[string]cachedDiscovery)}
}
func (*GetCreatorDiscoveryHandler) Pattern() string { return "/creator/discovery" }

func discoveryURL(q url.Values) (string, error) {
	bad := fmt.Errorf("invalid discovery query")
	ids, recordings := q["mbid"], q["recordings"]
	if len(ids) == 1 && len(recordings) == 0 && mbidPattern.MatchString(ids[0]) {
		params := url.Values{"mode": {"easy"}, "max_similar_artists": {"6"}, "max_recordings_per_artist": {"1"}, "pop_begin": {"0"}, "pop_end": {"100"}}
		return "https://api.listenbrainz.org/1/lb-radio/artist/" + strings.ToLower(ids[0]) + "?" + params.Encode(), nil
	}
	if len(recordings) != 1 || len(ids) != 0 {
		return "", bad
	}
	parts := strings.Split(strings.ToLower(recordings[0]), ",")
	if len(parts) > 24 {
		return "", bad
	}
	unique := map[string]bool{}
	for _, part := range parts {
		if !mbidPattern.MatchString(part) {
			return "", bad
		}
		unique[part] = true
	}
	parts = parts[:0]
	for id := range unique {
		parts = append(parts, id)
	}
	sort.Strings(parts)
	return "https://api.listenbrainz.org/1/metadata/recording/?" + url.Values{"recording_mbids": {strings.Join(parts, ",")}, "inc": {"artist release"}}.Encode(), nil
}

func (h *GetCreatorDiscoveryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		creatorError(w, 405, "Method not allowed.")
		return
	}
	source, err := discoveryURL(r.URL.Query())
	if err != nil {
		creatorError(w, 400, "Check the creator or recording IDs.")
		return
	}
	h.mu.Lock()
	cached, ok := h.cache[source]
	h.mu.Unlock()
	if ok && time.Now().Before(cached.expires) {
		w.Write(cached.body)
		return
	}
	// Shared work has a bounded lifetime; each waiting request can cancel on its own.
	pending := h.pending.DoChan(source, func() (any, error) {
		h.mu.Lock()
		cached, ok := h.cache[source]
		h.mu.Unlock()
		if ok && time.Now().Before(cached.expires) {
			return cached.body, nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "beatbrain/occipital (https://beatbrain.xyz)")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("discovery source status %d", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
		if err != nil || len(body) > 2<<20 {
			return nil, fmt.Errorf("discovery response incomplete")
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(body, &fields) != nil || fields == nil || fields["error"] != nil {
			return nil, fmt.Errorf("discovery response invalid")
		}
		for id, value := range fields {
			if !mbidPattern.MatchString(id) {
				return nil, fmt.Errorf("discovery response has an invalid entity")
			}
			if strings.Contains(source, "/lb-radio/artist/") {
				var rows []struct {
					ArtistID    string `json:"similar_artist_mbid"`
					Name        string `json:"similar_artist_name"`
					RecordingID string `json:"recording_mbid"`
				}
				if json.Unmarshal(value, &rows) != nil || rows == nil {
					return nil, fmt.Errorf("discovery artist data invalid")
				}
				for _, row := range rows {
					if !strings.EqualFold(row.ArtistID, id) || strings.TrimSpace(row.Name) == "" || !mbidPattern.MatchString(row.RecordingID) {
						return nil, fmt.Errorf("discovery recording data invalid")
					}
				}
			} else {
				var item map[string]json.RawMessage
				if json.Unmarshal(value, &item) != nil || item == nil || item["recording"] == nil {
					return nil, fmt.Errorf("recording metadata invalid")
				}
				var recording struct {
					Name string `json:"name"`
				}
				if json.Unmarshal(item["recording"], &recording) != nil || strings.TrimSpace(recording.Name) == "" {
					return nil, fmt.Errorf("recording identity missing")
				}
			}
		}
		h.mu.Lock()
		if len(h.cache) >= 128 {
			key := ""
			var oldest time.Time
			for k, v := range h.cache {
				if key == "" || v.expires.Before(oldest) {
					key, oldest = k, v.expires
				}
			}
			delete(h.cache, key)
		}
		h.cache[source] = cachedDiscovery{body: body, expires: time.Now().Add(time.Hour)}
		h.mu.Unlock()
		return json.RawMessage(body), nil
	})
	select {
	case <-r.Context().Done():
		return
	case result := <-pending:
		if result.Err != nil {
			h.log.Warnw("Creator discovery unavailable", "error", result.Err)
			creatorError(w, 503, "Extra music details could not be loaded. Please try again.")
			return
		}
		w.Write(result.Val.(json.RawMessage))
	}
}
