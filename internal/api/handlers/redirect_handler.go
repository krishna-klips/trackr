package handlers

import (
	"database/sql"
	"net/http"
	"strings"
	"sync"
	"time"

	"trackr/internal/engine/links"
	"trackr/internal/engine/redirect"
	"trackr/internal/pkg/geoip"
	"trackr/internal/pkg/parser"
	"trackr/internal/platform/database"

	"github.com/julienschmidt/httprouter"
)

type RedirectHandler struct {
	// Dependencies
	GlobalDB      *sql.DB
	TenantPool    *database.TenantDBPool
	GeoResolver   geoip.Resolver
	LinkCache     *redirect.LinkCache
	ClickLogger   *redirect.ClickLogger
	SharedDomain  string
	SystemOrgID   string // ID for the system_shared organization

	// Domain Cache
	domainCache sync.Map // map[string]cachedOrgID
}

type cachedOrgID struct {
	OrgID    string
	CachedAt time.Time
}

func NewRedirectHandler(globalDB *sql.DB, pool *database.TenantDBPool, sharedDomain string) *RedirectHandler {
	return &RedirectHandler{
		GlobalDB:     globalDB,
		TenantPool:   pool,
		GeoResolver:  geoip.NewDummyResolver(),
		LinkCache:    redirect.NewLinkCache(5 * time.Minute),
		ClickLogger:  redirect.NewClickLogger(),
		SharedDomain: sharedDomain,
		SystemOrgID:  "system_shared",
	}
}

func (h *RedirectHandler) Handle(w http.ResponseWriter, r *http.Request) {
	params := r.Context().Value("params").(httprouter.Params)
	shortCode := params.ByName("short_code")
	if shortCode == "" {
		http.NotFound(w, r)
		return
	}

	// 1. Determine Organization
	host := r.Host
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	var orgID string
	var err error

	if host == h.SharedDomain {
		orgID = h.SystemOrgID
	} else {
		orgID, err = h.resolveOrgFromDomain(host)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}

	// 2. Load Tenant DB
	// We optimize by not fetching the full Org info if we can infer or cache the DB path.
	// For now, we still fetch it but could cache this result too.
	org, err := h.getOrgByID(orgID)
	if err != nil {
		http.Error(w, "Organization unavailable", http.StatusInternalServerError)
		return
	}

	// 3. Lookup Link (Cache -> DB)
	// Check Cache First
	var link *links.Link

	// We key the cache by OrgID + ShortCode to prevent collisions across tenants if any
	// (Though shortcodes should be unique per tenant)
	cacheKey := orgID + ":" + shortCode

	if cached, found := h.LinkCache.Get(cacheKey); found {
		// Reconstruct minimal link object from cache
		link = &links.Link{
			ID:             cached.ID,
			DestinationURL: cached.DestinationURL,
			Rules:          cached.Rules,
			RedirectType:   cached.RedirectType,
			Status:         cached.Status,
			ShortCode:      shortCode,
		}
	} else {
		// Cache Miss - Load DB
		tenantDB, err := h.TenantPool.Get(orgID, org.DBFilePath)
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusInternalServerError)
			return
		}

		linkRepo := links.NewRepository(tenantDB)
		link, err = linkRepo.GetByShortCode(shortCode)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		// Set Cache
		h.LinkCache.Set(cacheKey, link)
	}

	if link.Status != "active" {
		http.Error(w, "Link is not active", http.StatusGone)
		return
	}

	// 4. Build Request Context
	ip := r.RemoteAddr
	ua := r.UserAgent()
	country, _ := h.GeoResolver.Lookup(ip)
	os, browser := parser.ParseUserAgent(ua)

	reqCtx := links.RequestContext{
		IPAddress:   ip,
		UserAgent:   ua,
		CountryCode: country,
		DeviceType:  links.ParseDeviceType(ua),
		OS:          os,
		Browser:     browser,
		Referrer:    r.Referer(),
		RequestTime: time.Now(),
	}

	// 5. Evaluate Rules
	finalURL := link.DestinationURL
	if link.Rules != nil {
		ruleURL := link.Rules.Evaluate(&reqCtx)
		if ruleURL != "" {
			finalURL = ruleURL
		}
	}

	// 6. Async Logging
	incomingQuery := r.URL.Query()
	utm := make(map[string]string)
	if v := incomingQuery.Get("utm_source"); v != "" { utm["utm_source"] = v }
	if v := incomingQuery.Get("utm_medium"); v != "" { utm["utm_medium"] = v }
	if v := incomingQuery.Get("utm_campaign"); v != "" { utm["utm_campaign"] = v }

	// Acquire DB connection for logger if we don't have it (e.g. cache hit case)
	// Note: TenantPool.Get is cheap if cached
	tenantDB, _ := h.TenantPool.Get(orgID, org.DBFilePath)
	if tenantDB != nil {
		go h.ClickLogger.LogClick(tenantDB, link.ID, link.ShortCode, finalURL, reqCtx, utm)
	}

	// 7. Redirect
	statusCode := http.StatusFound
	if link.RedirectType == "permanent" {
		statusCode = http.StatusMovedPermanently
	}

	http.Redirect(w, r, finalURL, statusCode)
}

func (h *RedirectHandler) resolveOrgFromDomain(domain string) (string, error) {
	// Check Cache
	if val, ok := h.domainCache.Load(domain); ok {
		cached := val.(cachedOrgID)
		if time.Since(cached.CachedAt) < 15*time.Minute {
			return cached.OrgID, nil
		}
		h.domainCache.Delete(domain)
	}

	// Query Global DB
	var orgID string
	query := "SELECT organization_id FROM domains WHERE domain = ? AND verified = 1"
	err := h.GlobalDB.QueryRow(query, domain).Scan(&orgID)
	if err != nil {
		return "", err
	}

	// Set Cache
	h.domainCache.Store(domain, cachedOrgID{
		OrgID:    orgID,
		CachedAt: time.Now(),
	})

	return orgID, nil
}

type OrgInfo struct {
	ID         string
	DBFilePath string
}

func (h *RedirectHandler) getOrgByID(orgID string) (*OrgInfo, error) {
	// Query Global DB
	var info OrgInfo
	info.ID = orgID
	query := "SELECT db_file_path FROM organizations WHERE id = ?"
	err := h.GlobalDB.QueryRow(query, orgID).Scan(&info.DBFilePath)
	if err != nil {
		return nil, err
	}
	return &info, nil
}
