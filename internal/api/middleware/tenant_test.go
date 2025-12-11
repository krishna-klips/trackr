package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"database/sql"

	"github.com/DATA-DOG/go-sqlmock"
	apiContext "trackr/internal/api/context"
	"trackr/internal/platform/auth"
	"trackr/internal/platform/repositories"
	"trackr/internal/platform/database"
	"trackr/internal/platform/config"
)

func TestTenantMiddleware(t *testing.T) {
	// Mock Global DB
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	// Mock Org Repository
	orgRepo := repositories.NewOrganizationRepository(db)

	// Mock Tenant DB Pool
	// We need to be careful here as TenantDBPool uses real sqlite driver.
	// For unit testing middleware, we might want to mock the pool or the result of Get.
	// However, TenantDBPool is a struct, not an interface.
	// For this test, let's assume we can use a temporary directory for sqlite files if we really wanted to integration test it.
	// But let's try to mock the org retrieval first.

	// Setup TenantDBPool with a dummy config
	cfg := config.TenantDBConfig{BasePath: "/tmp", MaxConnectionsPerOrg: 1}
	pool := database.NewTenantDBPool(cfg)

	middleware := NewTenantMiddleware(orgRepo, pool)

	// Test Case: Valid Tenant
	t.Run("Valid Tenant", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/", nil)

		// Inject Claims
		claims := &auth.Claims{
			OrganizationID: "org_123",
		}
		ctx := context.WithValue(req.Context(), apiContext.Claims, claims)
		req = req.WithContext(ctx)

		// Mock DB Expectation for Org
		rows := sqlmock.NewRows([]string{"id", "slug", "name", "domain", "db_file_path", "plan_tier", "link_quota", "member_quota", "saml_enabled", "webhook_secret", "created_at", "updated_at", "deleted_at"}).
			AddRow("org_123", "test-org", "Test Org", "test.com", ":memory:", "enterprise", 1000, 10, false, "secret", 1234567890, 1234567890, nil)

		mock.ExpectQuery("SELECT (.+) FROM organizations WHERE id = ?").
			WithArgs("org_123").
			WillReturnRows(rows)

		rr := httptest.NewRecorder()
		handler := middleware.Handle(func(w http.ResponseWriter, r *http.Request) {
			tenant := r.Context().Value(apiContext.Tenant).(*TenantContext)
			if tenant.OrgID != "org_123" {
				t.Errorf("Expected OrgID org_123, got %s", tenant.OrgID)
			}
			if tenant.DB == nil {
				t.Error("Expected DB connection, got nil")
			}
			w.WriteHeader(http.StatusOK)
		})

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
		}
	})

	// Test Case: Invalid Tenant (Org not found)
	t.Run("Invalid Tenant", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/", nil)

		// Inject Claims
		claims := &auth.Claims{
			OrganizationID: "org_999",
		}
		ctx := context.WithValue(req.Context(), apiContext.Claims, claims)
		req = req.WithContext(ctx)

		// Mock DB Expectation for Org (Not Found)
		mock.ExpectQuery("SELECT (.+) FROM organizations WHERE id = ?").
			WithArgs("org_999").
			WillReturnError(sql.ErrNoRows)

		rr := httptest.NewRecorder()
		handler := middleware.Handle(func(w http.ResponseWriter, r *http.Request) {
			// Should not be called
			t.Error("Handler should not be called")
		})

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden { // Assuming 403 Forbidden for not found org as per middleware implementation
			t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusForbidden)
		}
	})
}
