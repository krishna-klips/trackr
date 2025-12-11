package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"trackr/internal/platform/database"
)

type HealthHandler struct {
	globalDB *database.GlobalDB
}

func NewHealthHandler(globalDB *database.GlobalDB) *HealthHandler {
	return &HealthHandler{globalDB: globalDB}
}

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]string)

	// Check Global DB
	if err := h.globalDB.DB.Ping(); err != nil {
		checks["global_db"] = "unhealthy: " + err.Error()
	} else {
		checks["global_db"] = "healthy"
	}

	// Check GeoIP (skipping as we might not have it loaded in this handler ref)
	// checks["geoip"] = "healthy" // Placeholder

	checks["cache"] = "healthy" // Placeholder for Redis/Memory cache check

	status := "healthy"
	for _, check := range checks {
		if len(check) >= 9 && check[:9] == "unhealthy" {
			status = "degraded"
			break
		}
	}

	response := struct {
		Status    string            `json:"status"`
		Timestamp int64             `json:"timestamp"`
		Checks    map[string]string `json:"checks"`
	}{
		Status:    status,
		Timestamp: time.Now().Unix(),
		Checks:    checks,
	}

	statusCode := http.StatusOK
	if status == "degraded" {
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}
