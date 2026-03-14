package podcast

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"cloud.google.com/go/firestore"
	fsClient "github.com/mager/occipital/firestore"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

const defaultLimit = 50
const maxLimit = 200

// ShowsHandler returns podcast shows, optionally filtered by category
type ShowsHandler struct {
	log *zap.SugaredLogger
	fs  *firestore.Client
}

func NewShowsHandler(log *zap.SugaredLogger, fs *firestore.Client) *ShowsHandler {
	return &ShowsHandler{log: log, fs: fs}
}

func (h *ShowsHandler) Pattern() string {
	return "/podcasts"
}

type ShowResult struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Publisher    string   `json:"publisher"`
	Description  string   `json:"description"`
	Categories   []string `json:"categories"`
	ImageURL     string   `json:"imageURL"`
	EpisodeCount int      `json:"episodeCount"`
	Explicit     bool     `json:"explicit"`
	ExternalURL  string   `json:"externalURL"`
}

// ServeHTTP returns podcast shows, filtered by ?category=X with optional ?limit=N.
//
// @Summary      List podcast shows
// @Description  Returns podcast shows from the podcast_shows collection, filtered by category
// @Tags         Podcasts
// @Produce      json
// @Param        category  query  string  false  "Category filter"
// @Param        limit     query  int     false  "Max results (default 50, max 200)"
// @Success      200       {array}  ShowResult
// @Router       /podcasts [get]
func (h *ShowsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")

	category := r.URL.Query().Get("category")
	limit := defaultLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	var query firestore.Query
	col := h.fs.Collection("podcast_shows")

	if category != "" {
		query = col.Where("categories", "array-contains", category).Limit(limit)
	} else {
		query = col.Limit(limit)
	}

	iter := query.Documents(ctx)
	defer iter.Stop()

	var results []ShowResult

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			h.log.Errorw("firestore iteration error", "err", err)
			http.Error(w, "failed to fetch podcasts", http.StatusInternalServerError)
			return
		}

		var show fsClient.PodcastShow
		if err := doc.DataTo(&show); err != nil {
			h.log.Warnw("failed to decode podcast show", "id", doc.Ref.ID, "err", err)
			continue
		}

		results = append(results, ShowResult{
			ID:           show.ID,
			Name:         show.Name,
			Publisher:    show.Publisher,
			Description:  show.Description,
			Categories:   show.Categories,
			ImageURL:     show.ImageURL,
			EpisodeCount: show.EpisodeCount,
			Explicit:     show.Explicit,
			ExternalURL:  show.ExternalURL,
		})
	}

	if results == nil {
		results = []ShowResult{}
	}

	h.log.Infow("podcast shows fetched", "category", category, "count", len(results))
	json.NewEncoder(w).Encode(results)
}
