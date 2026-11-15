package creator

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mager/occipital/musicbrainz"
	"github.com/mager/occipital/occipital"
	"github.com/mager/occipital/spotify"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

var mbidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var errIncompleteCreator = errors.New("incomplete creator data")

type cachedCreator struct {
	value   occipital.Creator
	expires time.Time
}
type GetCreatorHandler struct {
	log               *zap.SugaredLogger
	musicbrainzClient *musicbrainz.MusicbrainzClient
	mu                sync.Mutex
	cache             map[string]cachedCreator
	pending           singleflight.Group
}

func (*GetCreatorHandler) Pattern() string { return "/creator" }
func NewGetCreatorHandler(log *zap.SugaredLogger, c *musicbrainz.MusicbrainzClient, _ *spotify.SpotifyClient) *GetCreatorHandler {
	return &GetCreatorHandler{log: log, musicbrainzClient: c, cache: make(map[string]cachedCreator)}
}

type GetCreatorResponse struct {
	Creator occipital.Creator `json:"creator"`
}

func creatorError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
func (h *GetCreatorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		creatorError(w, 405, "Method not allowed.")
		return
	}
	ids := r.URL.Query()["mbid"]
	if len(ids) != 1 || !mbidPattern.MatchString(ids[0]) {
		creatorError(w, 400, "A valid creator ID is required.")
		return
	}
	id := strings.ToLower(ids[0])
	h.mu.Lock()
	cached, ok := h.cache[id]
	h.mu.Unlock()
	if ok && time.Now().Before(cached.expires) {
		json.NewEncoder(w).Encode(GetCreatorResponse{Creator: cached.value})
		return
	}
	pending := h.pending.DoChan(id, func() (any, error) {
		// A cancelled viewer does not cancel another viewer's shared request.
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		return h.loadCreator(ctx, id)
	})
	select {
	case <-r.Context().Done():
		return
	case result := <-pending:
		if result.Err != nil {
			if musicbrainz.IsNotFound(result.Err) {
				creatorError(w, 404, "This creator was not found.")
				return
			}
			if errors.Is(result.Err, errIncompleteCreator) {
				creatorError(w, 502, "The music source returned incomplete creator data.")
				return
			}
			h.log.Warnw("Creator metadata unavailable", "error", result.Err)
			creatorError(w, 503, "Creator credits could not be loaded. Please try again.")
			return
		}
		json.NewEncoder(w).Encode(GetCreatorResponse{Creator: result.Val.(occipital.Creator)})
	}
}

func (h *GetCreatorHandler) loadCreator(ctx context.Context, id string) (occipital.Creator, error) {
	h.mu.Lock()
	cached, ok := h.cache[id]
	h.mu.Unlock()
	if ok && time.Now().Before(cached.expires) {
		return cached.value, nil
	}
	artist, err := h.musicbrainzClient.GetCreatorArtistContext(ctx, id)
	if err != nil {
		return occipital.Creator{}, err
	}
	if !mbidPattern.MatchString(artist.ID) || strings.TrimSpace(artist.Name) == "" || artist.Relations == nil {
		return occipital.Creator{}, errIncompleteCreator
	}
	creator := transformCreator(artist)
	h.mu.Lock()
	if len(h.cache) >= 64 {
		oldestKey := ""
		var oldest time.Time
		for key, item := range h.cache {
			if oldestKey == "" || item.expires.Before(oldest) {
				oldestKey, oldest = key, item.expires
			}
		}
		delete(h.cache, oldestKey)
	}
	h.cache[id] = cachedCreator{creator, time.Now().Add(15 * time.Minute)}
	h.mu.Unlock()
	return creator, nil
}
func transformCreator(a *musicbrainz.CreatorArtist) occipital.Creator {
	c := occipital.Creator{ID: a.ID, Name: a.Name, Type: a.Type, Disambiguation: a.Disambiguation, Country: a.Country,
		Genres: []string{}, Links: []occipital.ExternalLink{}, Credits: []occipital.CreatorCredit{}, Catalog: []occipital.CreatorCatalogEntry{}, Connections: []occipital.CreatorConnection{}, SourceURL: "https://musicbrainz.org/artist/" + a.ID + "/relationships"}
	if a.Area != nil {
		c.Area = a.Area.Name
	}
	if a.BeginArea != nil {
		c.BeginArea = a.BeginArea.Name
	}
	if a.LifeSpan != nil {
		c.ActiveYears = &occipital.ActiveYears{Begin: a.LifeSpan.Begin, End: a.LifeSpan.End, Ended: a.LifeSpan.Ended}
	}
	if a.Genres != nil {
		for _, g := range *a.Genres {
			if g.Name != "" {
				c.Genres = append(c.Genres, g.Name)
			}
		}
		sort.Strings(c.Genres)
	}
	catalog := map[string]*occipital.CreatorCatalogEntry{}
	connections := map[string]bool{}
	links := map[string]bool{}
	for _, rel := range a.Relations {
		if rel.TargetType == "url" {
			u, err := url.Parse(rel.URL.Resource)
			if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && !links[u.String()] {
				links[u.String()] = true
				c.Links = append(c.Links, occipital.ExternalLink{Type: strings.TrimPrefix(u.Hostname(), "www."), URL: u.String()})
			}
			continue
		}
		if rel.TargetType == "artist" && rel.Type == "member of band" && rel.Artist != nil && mbidPattern.MatchString(rel.Artist.ID) && rel.Artist.ID != a.ID && rel.Artist.Name != "" {
			label := "Member of"
			if rel.Direction == "backward" {
				label = "Member"
			} else if rel.Direction != "forward" {
				continue
			}
			key := rel.Artist.ID + "|" + label + "|" + rel.Begin + "|" + rel.End
			if !connections[key] {
				connections[key] = true
				c.Connections = append(c.Connections, occipital.CreatorConnection{ID: rel.Artist.ID, Name: rel.Artist.Name, Relation: label, Begin: rel.Begin, End: rel.End, Ended: rel.Ended})
			}
			continue
		}
		var entity *musicbrainz.CreatorEntity
		switch rel.TargetType {
		case "recording":
			entity = rel.Recording
		case "release":
			entity = rel.Release
		case "work":
			entity = rel.Work
		}
		if entity == nil || !mbidPattern.MatchString(entity.ID) || strings.TrimSpace(entity.Title) == "" || strings.TrimSpace(rel.Type) == "" {
			continue
		}
		key := rel.TargetType + ":" + strings.ToLower(entity.ID)
		item := catalog[key]
		if item == nil {
			item = &occipital.CreatorCatalogEntry{ID: strings.ToLower(entity.ID), Kind: rel.TargetType, Title: entity.Title, Artists: []occipital.CreditArtist{}, Roles: []string{}}
			catalog[key] = item
		}
		if item.Disambiguation == "" {
			item.Disambiguation = entity.Disambiguation
		}
		if item.Date == "" {
			item.Date = entity.FirstReleaseDate
			if rel.TargetType == "release" {
				item.Date = entity.Date
			}
		}
		if len(item.Artists) == 0 {
			for _, ac := range entity.ArtistCredits {
				if ac.Name == "" {
					continue
				}
				artist := occipital.CreditArtist{Name: ac.Name}
				if mbidPattern.MatchString(ac.Artist.ID) {
					artist.ID = ac.Artist.ID
				}
				item.Artists = append(item.Artists, artist)
			}
		}
		role := rel.Type
		attrs := append([]string(nil), rel.Attributes...)
		sort.Strings(attrs)
		if len(attrs) > 0 {
			role = strings.Join(attrs, ", ") + " " + role
		}
		if !contains(item.Roles, role) {
			item.Roles = append(item.Roles, role)
		}
	}
	for _, item := range catalog {
		sort.Strings(item.Roles)
		c.Catalog = append(c.Catalog, *item)
	}
	sort.Slice(c.Catalog, func(i, j int) bool {
		a, b := c.Catalog[i], c.Catalog[j]
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.ID < b.ID
	})
	sort.Slice(c.Connections, func(i, j int) bool {
		a, b := c.Connections[i], c.Connections[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Begin < b.Begin
	})
	sort.Slice(c.Links, func(i, j int) bool { return c.Links[i].URL < c.Links[j].URL })
	groups := map[string][]occipital.CreatorRecording{}
	for _, item := range c.Catalog {
		if item.Kind != "recording" {
			continue
		}
		names := []string{}
		for _, artist := range item.Artists {
			names = append(names, artist.Name)
		}
		for _, role := range item.Roles {
			groups[role] = append(groups[role], occipital.CreatorRecording{ID: item.ID, Title: item.Title, Artist: strings.Join(names, ", ")})
		}
	}
	for role, recordings := range groups {
		c.Credits = append(c.Credits, occipital.CreatorCredit{Type: role, Recordings: recordings})
	}
	sort.Slice(c.Credits, func(i, j int) bool { return c.Credits[i].Type < c.Credits[j].Type })
	return c
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
