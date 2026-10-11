package api

import (
	"encoding/json"
	"net/http"
)

// Version is the build version reported by /healthz.
const Version = "0.1.0"

// HealthHandler answers the liveness probe at /healthz.
func HealthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Health{Status: "ok", Version: Version})
}
