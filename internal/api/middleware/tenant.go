package middleware

import (
	"context"
	"net/http"
	"database/sql"

	apiContext "trackr/internal/api/context"
	"trackr/internal/platform/auth"
	"trackr/internal/platform/database"
	"trackr/internal/platform/repositories"
	"trackr/internal/pkg/errors"
)

type TenantContext struct {
	OrgID   string
	OrgSlug string
	DB      *sql.DB
}

type TenantMiddleware struct {
	orgRepo *repositories.OrganizationRepository
	dbPool  *database.TenantDBPool
}

func NewTenantMiddleware(orgRepo *repositories.OrganizationRepository, dbPool *database.TenantDBPool) *TenantMiddleware {
	return &TenantMiddleware{
		orgRepo: orgRepo,
		dbPool:  dbPool,
	}
}

func (m *TenantMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(apiContext.Claims).(*auth.Claims)
		if !ok {
			errors.WriteError(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized, "No authentication claims found", nil)
			return
		}

		org, err := m.orgRepo.GetByID(claims.OrganizationID)
		if err != nil {
			errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "Failed to load organization", nil)
			return
		}
		if org == nil {
			errors.WriteError(w, http.StatusForbidden, errors.ErrCodeForbidden, "Organization not found", nil)
			return
		}

		db, err := m.dbPool.Get(org.ID, org.DBFilePath)
		if err != nil {
			errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "Failed to connect to tenant database", nil)
			return
		}

		ctx := context.WithValue(r.Context(), apiContext.Tenant, &TenantContext{
			OrgID:   org.ID,
			OrgSlug: org.Slug,
			DB:      db,
		})

		next(w, r.WithContext(ctx))
	}
}
