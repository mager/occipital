package health

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
)

type DatabaseUser struct {
	ID string `json:"id"`
}

// UserHandler is an http.Handler that copies its request body
// back to the response.
type UserHandler struct {
	log *zap.Logger
	db  *sql.DB
}

func (*UserHandler) Pattern() string {
	return "/user"
}

// NewUserHandler builds a new UserHandler.
func NewUserHandler(log *zap.Logger, db *sql.DB) *UserHandler {
	return &UserHandler{
		log: log,
		db:  db,
	}
}

type GetUserResponse struct {
	ID string `json:"id"`
}

// Get user by ID
// @Summary Get user by ID
// @Description Get user details by user ID
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} GetUserResponse
// @Router /user [get]
// @Param id query string true "User ID"
func (h *UserHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := q.Get("id")

	h.log.Info("get user", zap.String("id", id))

	// Fetch the user from the database
	row := h.db.QueryRow("SELECT id FROM users WHERE id = $1", id)

	var user DatabaseUser
	err := row.Scan(&user.ID)
	if err != nil {
		if err == sql.ErrNoRows {
			h.log.Info("User not found", zap.String("id", id))
			w.WriteHeader(http.StatusNotFound)
			return
		}
		h.log.Error("Failed to fetch user", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp := &GetUserResponse{
		ID: user.ID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
