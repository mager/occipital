package health

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
)

// UserHandler is an http.Handler that copies its request body
// back to the response.
type UserHandler struct {
	log *zap.Logger
}

func (*UserHandler) Pattern() string {
	return "/user"
}

// NewUserHandler builds a new UserHandler.
func NewUserHandler(log *zap.Logger) *UserHandler {
	return &UserHandler{
		log: log,
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

	// Use the ID to fetch user data (replace this with actual logic)
	resp := GetUserResponse{
		ID: id,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
