package health

import (
	"encoding/json"
	"net/http"
)

// HealthHandler is an http.Handler that copies its request body
// back to the response.
type HealthHandler struct{}

func (*HealthHandler) Pattern() string {
	return "/health"
}

// NewHealthHandler builds a new HealthHandler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

type Response struct {
	Status string `json:"status"`
}

// ServeHTTP handles an HTTP request to the /echo endpoint.
func (*HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var resp Response

	resp.Status = "OK"

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
