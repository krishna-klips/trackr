package api

import (
	"context"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"trackr/internal/api/handlers"
	"trackr/internal/api/middleware"
	apiContext "trackr/internal/api/context"
	"trackr/internal/platform/auth"
	"trackr/internal/pkg/errors"
)

type Dependencies struct {
	AuthHandler       *handlers.AuthHandler
	OrgHandler        *handlers.OrgHandler
	InviteHandler     *handlers.InviteHandler
	UserHandler       *handlers.UserHandler
	LinkHandler       *handlers.LinkHandler
	AnalyticsHandler  *handlers.AnalyticsHandler
	RedirectHandler   *handlers.RedirectHandler
	AuthMiddleware    *middleware.AuthMiddleware
	TenantMiddleware  *middleware.TenantMiddleware
}

func NewRouter(deps *Dependencies) *httprouter.Router {
	router := httprouter.New()

	// Public Redirect Endpoint
	router.GET("/:short_code", wrap(deps.RedirectHandler.Handle))

	// Authentication routes
	router.POST("/api/v1/auth/signup", wrap(deps.AuthHandler.Signup))
	router.POST("/api/v1/auth/login", wrap(deps.AuthHandler.Login))
	router.POST("/api/v1/auth/refresh", wrap(deps.AuthHandler.Refresh))
	router.POST("/api/v1/auth/logout", wrap(deps.AuthHandler.Logout))

	// SAML routes
	router.GET("/api/v1/auth/saml/login", wrap(deps.AuthHandler.SAMLLogin))
	router.POST("/api/v1/auth/saml/acs", wrap(deps.AuthHandler.HandleSAMLCallback))
	router.GET("/api/v1/auth/saml/metadata/:org_slug", wrap(deps.AuthHandler.GetSAMLMetadata))

	// Middleware references
	authMid := deps.AuthMiddleware
	tenantMid := deps.TenantMiddleware

	// Organization management
	router.POST("/api/v1/organizations", wrap(deps.OrgHandler.Create))
	router.GET("/api/v1/organizations/current",
		chain(deps.OrgHandler.GetCurrent, authMid.Handle, tenantMid.Handle))
	router.PATCH("/api/v1/organizations/current",
		chain(deps.OrgHandler.Update, authMid.Handle, tenantMid.Handle, requireRole("admin", "owner")))

	// Domain verification
	router.POST("/api/v1/organizations/domains",
		chain(deps.OrgHandler.AddDomain, authMid.Handle, tenantMid.Handle, requireRole("admin", "owner")))
	router.POST("/api/v1/organizations/domains/:domain_id/verify",
		chain(deps.OrgHandler.VerifyDomain, authMid.Handle, tenantMid.Handle, requireRole("admin", "owner")))
	router.GET("/api/v1/organizations/domains",
		chain(deps.OrgHandler.ListDomains, authMid.Handle, tenantMid.Handle))

	// Invite management
	router.POST("/api/v1/invites",
		chain(deps.InviteHandler.Create, authMid.Handle, tenantMid.Handle, requireRole("admin", "owner")))
	router.GET("/api/v1/invites",
		chain(deps.InviteHandler.List, authMid.Handle, tenantMid.Handle, requireRole("admin", "owner")))
	router.GET("/api/v1/invites/:invite_id",
		chain(deps.InviteHandler.Get, authMid.Handle, tenantMid.Handle, requireRole("admin", "owner")))
	router.DELETE("/api/v1/invites/:invite_id",
		chain(deps.InviteHandler.Revoke, authMid.Handle, tenantMid.Handle, requireRole("admin", "owner")))

	// User management
	router.GET("/api/v1/users",
		chain(deps.UserHandler.List, authMid.Handle, tenantMid.Handle, requireRole("admin", "owner")))
	router.GET("/api/v1/users/:user_id",
		chain(deps.UserHandler.Get, authMid.Handle, tenantMid.Handle))
	router.PATCH("/api/v1/users/:user_id/role",
		chain(deps.UserHandler.UpdateRole, authMid.Handle, tenantMid.Handle, requireRole("owner")))
	router.DELETE("/api/v1/users/:user_id",
		chain(deps.UserHandler.Delete, authMid.Handle, tenantMid.Handle, requireRole("owner")))

	// Link management
	router.POST("/api/v1/links",
		chain(deps.LinkHandler.Create, authMid.Handle, tenantMid.Handle))
	router.GET("/api/v1/links",
		chain(deps.LinkHandler.List, authMid.Handle, tenantMid.Handle))
	router.GET("/api/v1/links/:link_id",
		chain(deps.LinkHandler.Get, authMid.Handle, tenantMid.Handle))
	router.PATCH("/api/v1/links/:link_id",
		chain(deps.LinkHandler.Update, authMid.Handle, tenantMid.Handle))
	router.DELETE("/api/v1/links/:link_id",
		chain(deps.LinkHandler.Delete, authMid.Handle, tenantMid.Handle))
	router.GET("/api/v1/links/:link_id/qr",
		chain(deps.LinkHandler.GetQRCode, authMid.Handle, tenantMid.Handle))

	// Analytics
	router.GET("/api/v1/links/:link_id/analytics",
		chain(deps.AnalyticsHandler.GetLinkAnalytics, authMid.Handle, tenantMid.Handle))
	router.GET("/api/v1/links/:link_id/clicks",
		chain(deps.AnalyticsHandler.GetLinkClicks, authMid.Handle, tenantMid.Handle))
	router.GET("/api/v1/analytics/overview",
		chain(deps.AnalyticsHandler.GetOverview, authMid.Handle, tenantMid.Handle))

	return router
}

// Helper function to chain middlewares
func chain(handler http.HandlerFunc, middlewares ...func(http.HandlerFunc) http.HandlerFunc) httprouter.Handle {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return wrap(handler)
}

// Convert http.HandlerFunc to httprouter.Handle
func wrap(handler http.HandlerFunc) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		// Inject params into context
		ctx := context.WithValue(r.Context(), apiContext.Params, ps)
		handler(w, r.WithContext(ctx))
	}
}

func requireRole(roles ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			claims := r.Context().Value(apiContext.Claims).(*auth.Claims)

			allowed := false
			for _, role := range roles {
				if claims.Role == role {
					allowed = true
					break
				}
			}

			if !allowed {
				errors.WriteError(w, http.StatusForbidden, errors.ErrCodeForbidden, "Insufficient permissions", nil)
				return
			}

			next(w, r)
		}
	}
}
