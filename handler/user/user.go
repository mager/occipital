package user

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/mager/occipital/database"
	"go.uber.org/zap"
)

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

type UserResponse struct {
	ID       int     `json:"id"`
	Username *string `json:"username"`
}

// GetUser godoc
// @Summary Get user by ID
// @Description Get user details by user ID
// @Accept json
// @Produce json
// @Param id query string true "User ID"
// @Success 200 {object} UserResponse
// @Router /user [get]

// PutUser godoc
// @Summary Update user by ID
// @Description Update user details by user ID
// @Accept json
// @Produce json
// @Param id query string true "User ID"
// @Param user body database.User true "Updated user information"
// @Success 200 {object} UserResponse
// @Router /user [put]
func (h *UserHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("id")
	if userID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodGet {
		h.getUser(w, r)
	} else if r.Method == http.MethodPut {
		h.updateUser(w, r)
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *UserHandler) getUser(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("id")
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
	resp := UserResponse{
		ID:       user.ID,
		Username: user.Username,
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Error("Failed to encode response", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (h *UserHandler) updateUser(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("id")

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.log.Error("Failed to read request body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Decode the body into the user struct
	var user database.User
	err = json.Unmarshal(body, &user)
	if err != nil {
		h.log.Error("Failed to parse request body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	query := `
        UPDATE users
        SET username = $1
        WHERE id = $2
	`
	result, err := h.db.Exec(query, user.Username, userID)
	if err != nil {
		h.log.Error("Failed to execute update query", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		h.log.Error("Failed to get rows affected", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if rowsAffected == 0 {
		h.log.Warn("No user found with the given ID", zap.String("userID", userID))
		w.WriteHeader(http.StatusNotFound)
		return
	}

	userIDInt, err := strconv.Atoi(userID)
	if err != nil {
		h.log.Error("Failed to convert userID to int", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	resp := UserResponse{
		ID:       userIDInt,
		Username: user.Username,
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Error("Failed to encode response", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
