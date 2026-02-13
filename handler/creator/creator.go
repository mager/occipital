package creator

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	mb "github.com/mager/musicbrainz-go/musicbrainz"
	"github.com/mager/occipital/musicbrainz"
	"github.com/mager/occipital/occipital"
	"go.uber.org/zap"
)

// GetCreatorHandler is an http.Handler
type GetCreatorHandler struct {
	log               *zap.SugaredLogger
	musicbrainzClient *musicbrainz.MusicbrainzClient
}

func (*GetCreatorHandler) Pattern() string {
	return "/creator"
}

// NewGetCreatorHandler builds a new GetCreatorHandler.
func NewGetCreatorHandler(
	log *zap.SugaredLogger,
	musicbrainzClient *musicbrainz.MusicbrainzClient,
) *GetCreatorHandler {
	return &GetCreatorHandler{
		log:               log,
		musicbrainzClient: musicbrainzClient,
	}
}

type GetCreatorResponse struct {
	Creator occipital.Creator `json:"creator"`
}

func (h *GetCreatorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	mbid := q.Get("mbid")

	if mbid == "" {
		http.Error(w, `{"error":"mbid is required"}`, http.StatusBadRequest)
		return
	}

	h.log.Infow("Fetching MusicBrainz artist", "mbid", mbid)

	artistResp, err := h.musicbrainzClient.Client.GetArtist(mb.GetArtistRequest{
		ID: mbid,
		Includes: []mb.Include{
			"genres",
			"url-rels",
			"recording-rels",
		},
	})
	if err != nil {
		h.log.Errorf("error fetching artist: %v", err)
		http.Error(w, `{"error":"failed to fetch artist"}`, http.StatusInternalServerError)
		return
	}

	creator := transformArtistToCreator(artistResp.Artist)

	resp := GetCreatorResponse{Creator: creator}
	json.NewEncoder(w).Encode(resp)
}

func transformArtistToCreator(artist mb.Artist) occipital.Creator {
	creator := occipital.Creator{
		ID:             artist.ID,
		Name:           artist.Name,
		Type:           artist.Type,
		Disambiguation: artist.Disambiguation,
		Country:        artist.Country,
		Genres:         extractGenres(artist),
		Links:          extractLinks(artist),
		Credits:        extractCredits(artist),
	}

	if artist.Area != nil {
		creator.Area = artist.Area.Name
	}
	if artist.BeginArea != nil {
		creator.BeginArea = artist.BeginArea.Name
	}
	if artist.LifeSpan != nil {
		creator.ActiveYears = &occipital.ActiveYears{
			Begin: artist.LifeSpan.Begin,
			End:   artist.LifeSpan.End,
			Ended: artist.LifeSpan.Ended,
		}
	}

	return creator
}

func extractGenres(artist mb.Artist) []string {
	maxGenres := 10
	genres := make([]string, 0)

	if artist.Genres == nil || len(*artist.Genres) == 0 {
		return genres
	}

	genresSlice := *artist.Genres
	sort.Slice(genresSlice, func(i, j int) bool {
		return genresSlice[i].Count > genresSlice[j].Count
	})

	for i := 0; i < maxGenres && i < len(genresSlice); i++ {
		genres = append(genres, genresSlice[i].Name)
	}

	return genres
}

func extractLinks(artist mb.Artist) []occipital.ExternalLink {
	var links []occipital.ExternalLink

	if artist.Relations == nil {
		return links
	}

	supportedDomains := map[string]string{
		"spotify":    "spotify",
		"wikipedia":  "wikipedia",
		"bandcamp":   "bandcamp",
		"soundcloud": "soundcloud",
		"discogs":    "discogs",
		"allmusic":   "allmusic",
		"youtube":    "youtube",
		"instagram":  "instagram",
		"twitter":    "twitter",
		"facebook":   "facebook",
		"wikidata":   "wikidata",
	}

	for _, rel := range *artist.Relations {
		if rel.TargetType == "url" {
			for domain, linkType := range supportedDomains {
				if strings.Contains(rel.URL.Resource, domain) {
					links = append(links, occipital.ExternalLink{
						Type: linkType,
						URL:  rel.URL.Resource,
					})
					break
				}
			}
		}
	}

	return links
}

func extractCredits(artist mb.Artist) []occipital.CreatorCredit {
	if artist.Relations == nil {
		return nil
	}

	// Group recordings by credit type
	creditMap := make(map[string][]occipital.CreatorRecording)

	for _, rel := range *artist.Relations {
		if rel.TargetType == "recording" && rel.Recording != nil {
			creditType := rel.Type
			// For instrument relations, include the attribute (e.g., "guitar", "bass")
			if rel.Type == "instrument" && len(rel.Attributes) > 0 {
				creditType = rel.Attributes[0]
			}

			creditMap[creditType] = append(creditMap[creditType], occipital.CreatorRecording{
				ID:    rel.Recording.ID,
				Title: rel.Recording.Title,
			})
		}
	}

	var credits []occipital.CreatorCredit
	for creditType, recordings := range creditMap {
		credits = append(credits, occipital.CreatorCredit{
			Type:       creditType,
			Recordings: recordings,
		})
	}

	// Sort credits by number of recordings (descending)
	sort.Slice(credits, func(i, j int) bool {
		return len(credits[i].Recordings) > len(credits[j].Recordings)
	})

	return credits
}
