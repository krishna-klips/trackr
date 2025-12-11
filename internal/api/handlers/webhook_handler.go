package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"trackr/internal/platform/database"
	"trackr/internal/platform/models"
	"trackr/internal/platform/repositories"

	"github.com/julienschmidt/httprouter"
)

type WebhookHandler struct{}

func NewWebhookHandler() *WebhookHandler {
	return &WebhookHandler{}
}

func (h *WebhookHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantCtx := r.Context().Value("tenant").(*database.TenantContext)

	// Webhooks are stored in the global DB or tenant DB?
	// PLAN.md section 2.2 says table `webhooks` is in TENANT DATABASE.
	// So we use tenantCtx.DB.

	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
		Secret string   `json:"secret"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	webhook := &models.Webhook{
		URL:    req.URL,
		Events: req.Events,
		Secret: req.Secret,
	}

	if webhook.Secret == "" {
		// Generate a random secret if not provided
		webhook.Secret = "whsec_" + string(time.Now().UnixNano()) // Simplified
	}

	repo := repositories.NewWebhookRepository(tenantCtx.DB)
	if err := repo.Create(webhook); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(webhook)
}

func (h *WebhookHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantCtx := r.Context().Value("tenant").(*database.TenantContext)

	repo := repositories.NewWebhookRepository(tenantCtx.DB)
	webhooks, err := repo.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(webhooks)
}

func (h *WebhookHandler) Get(w http.ResponseWriter, r *http.Request) {
	tenantCtx := r.Context().Value("tenant").(*database.TenantContext)
	params := r.Context().Value("params").(httprouter.Params)
	id := params.ByName("webhook_id")

	repo := repositories.NewWebhookRepository(tenantCtx.DB)
	webhook, err := repo.GetByID(id)
	if err != nil {
		http.Error(w, "Webhook not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(webhook)
}

func (h *WebhookHandler) Update(w http.ResponseWriter, r *http.Request) {
	tenantCtx := r.Context().Value("tenant").(*database.TenantContext)
	params := r.Context().Value("params").(httprouter.Params)
	id := params.ByName("webhook_id")

	var req models.Webhook
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	repo := repositories.NewWebhookRepository(tenantCtx.DB)
	webhook, err := repo.GetByID(id)
	if err != nil {
		http.Error(w, "Webhook not found", http.StatusNotFound)
		return
	}

	if req.URL != "" {
		webhook.URL = req.URL
	}
	if len(req.Events) > 0 {
		webhook.Events = req.Events
	}
	if req.Secret != "" {
		webhook.Secret = req.Secret
	}
	if req.Status != "" {
		webhook.Status = req.Status
	}

	if err := repo.Update(webhook); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(webhook)
}

func (h *WebhookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	tenantCtx := r.Context().Value("tenant").(*database.TenantContext)
	params := r.Context().Value("params").(httprouter.Params)
	id := params.ByName("webhook_id")

	repo := repositories.NewWebhookRepository(tenantCtx.DB)
	if err := repo.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
