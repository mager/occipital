package profile

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/mager/occipital/database"
	"go.uber.org/zap"
)

// ProfileHandler is an http.Handler that copies its request body
// back to the response.
type ProfileHandler struct {
	log *zap.SugaredLogger
	db  *sql.DB
}

func (*ProfileHandler) Pattern() string {
	return "/profile"
}

// NewProfileHandler builds a new ProfileHandler.
func NewProfileHandler(log *zap.SugaredLogger, db *sql.DB) *ProfileHandler {
	return &ProfileHandler{
		log: log,
		db:  db,
	}
}

type ProfileResponse struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

// GetProfile godoc
// @Summary Get profile by ID
// @Description Get profile details by user ID
// @Accept json
// @Produce json
// @Param id query string true "User ID"
// @Success 200 {object} ProfileResponse
// @Router /profile [get]
func (h *ProfileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("id")
	if userID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	query := `
        SELECT id, username
        FROM users
		WHERE id = $1
	`
	row := h.db.QueryRow(query, userID)

	var user database.User
	err := row.Scan(&user.ID, &user.Username)
	if err != nil {
		h.log.Error("Failed to fetch user", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	resp := ProfileResponse{
		ID:       user.ID,
		Username: user.Username,
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Error("Failed to encode response", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
