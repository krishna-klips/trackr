package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"trackr/internal/platform/auth"
	"trackr/internal/platform/database"
	"trackr/internal/platform/models"
	"trackr/internal/platform/repositories"

	"github.com/google/uuid"
	"github.com/julienschmidt/httprouter"
)

type APIKeyHandler struct {
	// API Keys are stored in GLOBAL DB (Table `api_keys` in 2.1 of PLAN.md)
	// Actually, wait. PLAN.md 2.1 lists `api_keys` under GLOBAL DATABASE.
	db *database.GlobalDB
}

func NewAPIKeyHandler(db *database.GlobalDB) *APIKeyHandler {
	return &APIKeyHandler{db: db}
}

func (h *APIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value("claims").(*auth.Claims)

	var req struct {
		Name          string   `json:"name"`
		Scopes        []string `json:"scopes"`
		ExpiresInDays int      `json:"expires_in_days"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Generate key
	rawKey := fmt.Sprintf("trk_live_%s", uuid.New().String())
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])
	keyPrefix := rawKey[:12] + "..." // e.g. trk_live_abc...

	apiKey := &models.APIKey{
		OrganizationID: claims.OrganizationID,
		UserID:         claims.UserID,
		Name:           req.Name,
		KeyHash:        keyHash,
		KeyPrefix:      keyPrefix,
		Scopes:         req.Scopes,
	}

	if req.ExpiresInDays > 0 {
		exp := time.Now().Add(time.Duration(req.ExpiresInDays) * 24 * time.Hour).Unix()
		apiKey.ExpiresAt = &exp
	}

	repo := repositories.NewAPIKeyRepository(h.db.DB)
	if err := repo.Create(apiKey); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the raw key only once
	response := struct {
		ID        string   `json:"id"`
		Key       string   `json:"key"`
		Name      string   `json:"name"`
		Scopes    []string `json:"scopes"`
		CreatedAt int64    `json:"created_at"`
	}{
		ID:        apiKey.ID,
		Key:       rawKey,
		Name:      apiKey.Name,
		Scopes:    apiKey.Scopes,
		CreatedAt: apiKey.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func (h *APIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value("claims").(*auth.Claims)

	repo := repositories.NewAPIKeyRepository(h.db.DB)
	keys, err := repo.ListByOrg(claims.OrganizationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(keys)
}

func (h *APIKeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	params := r.Context().Value("params").(httprouter.Params)
	keyID := params.ByName("key_id")

	repo := repositories.NewAPIKeyRepository(h.db.DB)
	if err := repo.Revoke(keyID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
