package health

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
)

// HealthHandler is an http.Handler that copies its request body
// back to the response.
type HealthHandler struct {
	log *zap.Logger
}

func (*HealthHandler) Pattern() string {
	return "/health"
}

// NewHealthHandler builds a new HealthHandler.
func NewHealthHandler(log *zap.Logger) *HealthHandler {
	return &HealthHandler{
		log: log,
	}
}

type Response struct {
	Status string `json:"status"`
}

// ServeHTTP handles an HTTP request to the /echo endpoint.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var resp Response

	h.log.Info("health check")

	resp.Status = "OK"

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
