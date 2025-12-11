package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"trackr/internal/platform/auth"
	"trackr/internal/platform/database"
	"github.com/google/uuid"
)

type AuditLog struct {
	ID             string                 `json:"id"`
	OrganizationID string                 `json:"organization_id"`
	UserID         string                 `json:"user_id"`
	Action         string                 `json:"action"`
	ResourceType   string                 `json:"resource_type"`
	ResourceID     string                 `json:"resource_id"`
	Metadata       map[string]interface{} `json:"metadata"`
	IPAddress      string                 `json:"ip_address"`
	UserAgent      string                 `json:"user_agent"`
	CreatedAt      int64                  `json:"created_at"`
}

type Logger struct {
	globalDB *sql.DB
}

func NewLogger(db *sql.DB) *Logger {
	return &Logger{globalDB: db}
}

func (l *Logger) Log(ctx context.Context, action, resourceType, resourceID string, metadata map[string]interface{}) {
	// Extract info from context
	var orgID, userID string

	if claims, ok := ctx.Value("claims").(*auth.Claims); ok {
		orgID = claims.OrganizationID
		userID = claims.UserID
	}

	if tenant, ok := ctx.Value("tenant").(*database.TenantContext); ok && orgID == "" {
		orgID = tenant.OrgID
	}

	ip := "unknown"
	ua := "unknown"

	// If we have request in context (unlikely unless we put it there explicitly)
	// Usually we pass *http.Request to this function or extract from context if middleware put it there.
	// Let's assume the context *might* have request or we rely on caller to pass it.
	// The signature `LogAudit` in PLAN.md extracts from context "request".

	if req, ok := ctx.Value("request").(*http.Request); ok {
		ip = req.RemoteAddr // Simplified
		ua = req.UserAgent()
	}

	metaJSON, _ := json.Marshal(metadata)

	logEntry := &AuditLog{
		ID:             "audit_" + uuid.New().String(),
		OrganizationID: orgID,
		UserID:         userID,
		Action:         action,
		ResourceType:   resourceType,
		ResourceID:     resourceID,
		Metadata:       metadata,
		IPAddress:      ip,
		UserAgent:      ua,
		CreatedAt:      time.Now().Unix(),
	}

	go func() {
		// Insert into global DB audit_logs table
		query := `
			INSERT INTO audit_logs (id, organization_id, user_id, action, resource_type, resource_id, metadata, ip_address, user_agent, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		l.globalDB.Exec(query, logEntry.ID, logEntry.OrganizationID, logEntry.UserID, logEntry.Action, logEntry.ResourceType, logEntry.ResourceID, string(metaJSON), logEntry.IPAddress, logEntry.UserAgent, logEntry.CreatedAt)
	}()
}
