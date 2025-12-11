package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"trackr/internal/engine/analytics"
	"trackr/internal/platform/database"

	"github.com/julienschmidt/httprouter"
)

type AnalyticsHandler struct{}

func NewAnalyticsHandler() *AnalyticsHandler {
	return &AnalyticsHandler{}
}

func (h *AnalyticsHandler) GetLinkAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantCtx := r.Context().Value("tenant").(*database.TenantContext)
	params := r.Context().Value("params").(httprouter.Params)
	linkID := params.ByName("link_id")

	// Parse query params
	startDate := r.URL.Query().Get("start_date") // YYYY-MM-DD
	endDate := r.URL.Query().Get("end_date")     // YYYY-MM-DD

	if startDate == "" || endDate == "" {
		// Default to last 30 days
		now := time.Now()
		endDate = now.Format("2006-01-02")
		startDate = now.AddDate(0, 0, -30).Format("2006-01-02")
	}

	repo := analytics.NewRepository(tenantCtx.DB)
	service := analytics.NewService(repo)

	stats, err := service.GetStatsOverview(linkID, startDate, endDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *AnalyticsHandler) GetLinkClicks(w http.ResponseWriter, r *http.Request) {
	tenantCtx := r.Context().Value("tenant").(*database.TenantContext)
	params := r.Context().Value("params").(httprouter.Params)
	linkID := params.ByName("link_id")

	startStr := r.URL.Query().Get("start_ts")
	endStr := r.URL.Query().Get("end_ts")

	now := time.Now().UnixMilli()
	start := now - (24 * 60 * 60 * 1000) // 24 hours ago
	end := now

	if startStr != "" {
		if v, err := strconv.ParseInt(startStr, 10, 64); err == nil {
			start = v
		}
	}
	if endStr != "" {
		if v, err := strconv.ParseInt(endStr, 10, 64); err == nil {
			end = v
		}
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 { page = 1 }
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 { limit = 50 }
	offset := (page - 1) * limit

	repo := analytics.NewRepository(tenantCtx.DB)
	service := analytics.NewService(repo)

	clicks, err := service.GetClickHistory(linkID, start, end, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clicks)
}

func (h *AnalyticsHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	// Not implemented for this phase (Org-wide overview)
	w.WriteHeader(http.StatusNotImplemented)
}
