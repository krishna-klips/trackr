package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"trackr/internal/engine/links"
	"trackr/internal/platform/auth"
	"trackr/internal/platform/database"

	"github.com/julienschmidt/httprouter"
)

type LinkHandler struct {
	// In a real scenario, we might use a factory to get the service per tenant
	// But since the service depends on a repo which depends on a DB connection...
	// We will resolve the service inside the handler using the tenant context.
}

func NewLinkHandler() *LinkHandler {
	return &LinkHandler{}
}

func (h *LinkHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantCtx := r.Context().Value("tenant").(*database.TenantContext)
	claims := r.Context().Value("claims").(*auth.Claims)

	var req struct {
		DestinationURL   string               `json:"destination_url"`
		Title            string               `json:"title"`
		ShortCode        string               `json:"short_code"`
		RedirectType     string               `json:"redirect_type"`
		Rules            *links.RedirectRules `json:"rules"`
		DefaultUTMParams *links.UTMParams     `json:"default_utm_params"`
		ExpiresAt        *int64               `json:"expires_at"`
		Password         string               `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Password hashing should happen here if provided, but skipping for brevity/scope
	// For now assuming password hash is what we store if logic existed.

	linkReq := &links.Link{
		DestinationURL:   req.DestinationURL,
		Title:            req.Title,
		CreatedBy:        claims.UserID,
		RedirectType:     req.RedirectType,
		Rules:            req.Rules,
		DefaultUTMParams: req.DefaultUTMParams,
		ExpiresAt:        req.ExpiresAt,
	}

	repo := links.NewRepository(tenantCtx.DB)
	service := links.NewService(repo)

	link, err := service.CreateLink(linkReq, req.ShortCode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

func (h *LinkHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantCtx := r.Context().Value("tenant").(*database.TenantContext)

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 50
	}
	offset := (page - 1) * limit

	repo := links.NewRepository(tenantCtx.DB)
	service := links.NewService(repo)

	linksList, err := service.ListLinks(limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(linksList)
}

func (h *LinkHandler) Get(w http.ResponseWriter, r *http.Request) {
	tenantCtx := r.Context().Value("tenant").(*database.TenantContext)
	params := r.Context().Value("params").(httprouter.Params)
	linkID := params.ByName("link_id")

	repo := links.NewRepository(tenantCtx.DB)
	service := links.NewService(repo)

	link, err := service.GetLink(linkID)
	if err != nil {
		http.Error(w, "Link not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(link)
}

func (h *LinkHandler) Update(w http.ResponseWriter, r *http.Request) {
	tenantCtx := r.Context().Value("tenant").(*database.TenantContext)
	params := r.Context().Value("params").(httprouter.Params)
	linkID := params.ByName("link_id")

	var req links.Link
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	repo := links.NewRepository(tenantCtx.DB)
	service := links.NewService(repo)

	link, err := service.UpdateLink(linkID, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(link)
}

func (h *LinkHandler) Delete(w http.ResponseWriter, r *http.Request) {
	tenantCtx := r.Context().Value("tenant").(*database.TenantContext)
	params := r.Context().Value("params").(httprouter.Params)
	linkID := params.ByName("link_id")

	repo := links.NewRepository(tenantCtx.DB)
	service := links.NewService(repo)

	if err := service.ArchiveLink(linkID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *LinkHandler) GetQRCode(w http.ResponseWriter, r *http.Request) {
	// Placeholder for QR code generation
	w.WriteHeader(http.StatusNotImplemented)
}
