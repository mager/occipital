package podcast

import (
	"context"
	"encoding/json"
	"net/http"

	"cloud.google.com/go/firestore"
	fsClient "github.com/mager/occipital/firestore"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

// CategoriesHandler returns podcast categories with counts + preview images
type CategoriesHandler struct {
	log *zap.SugaredLogger
	fs  *firestore.Client
}

func NewCategoriesHandler(log *zap.SugaredLogger, fs *firestore.Client) *CategoriesHandler {
	return &CategoriesHandler{log: log, fs: fs}
}

func (h *CategoriesHandler) Pattern() string {
	return "/podcasts/categories"
}

type CategoryResult struct {
	Name         string `json:"name"`
	Count        int    `json:"count"`
	PreviewImage string `json:"previewImage,omitempty"`
}

// ServeHTTP returns all podcast categories with counts and a preview image.
//
// @Summary      List podcast categories
// @Description  Returns all categories derived from the podcast_shows collection
// @Tags         Podcasts
// @Produce      json
// @Success      200  {array}  CategoryResult
// @Router       /podcasts/categories [get]
func (h *CategoriesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")

	categoryMap := make(map[string]*CategoryResult)

	iter := h.fs.Collection("podcast_shows").Documents(ctx)
	defer iter.Stop()

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

		for _, cat := range show.Categories {
			if cat == "" {
				continue
			}
			entry, exists := categoryMap[cat]
			if !exists {
				entry = &CategoryResult{Name: cat}
				categoryMap[cat] = entry
			}
			entry.Count++
			// Use first image we find for this category as preview
			if entry.PreviewImage == "" && show.ImageURL != "" {
				entry.PreviewImage = show.ImageURL
			}
		}
	}

	results := make([]CategoryResult, 0, len(categoryMap))
	for _, v := range categoryMap {
		results = append(results, *v)
	}

	// Sort by count descending
	sortCategoriesByCount(results)

	h.log.Infow("podcast categories fetched", "count", len(results))
	json.NewEncoder(w).Encode(results)
}

func sortCategoriesByCount(cats []CategoryResult) {
	n := len(cats)
	for i := 0; i < n-1; i++ {
		for j := i + 1; j < n; j++ {
			if cats[j].Count > cats[i].Count {
				cats[i], cats[j] = cats[j], cats[i]
			}
		}
	}
}
