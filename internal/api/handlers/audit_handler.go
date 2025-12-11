package handlers

import (
	"encoding/json"
	"net/http"

	"trackr/internal/platform/auth"
	"trackr/internal/platform/database"
)

type AuditHandler struct {
	globalDB *database.GlobalDB
}

func NewAuditHandler(globalDB *database.GlobalDB) *AuditHandler {
	return &AuditHandler{globalDB: globalDB}
}

func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value("claims").(*auth.Claims)

	// Fetch from global DB
	query := `SELECT id, organization_id, user_id, action, resource_type, resource_id, metadata, ip_address, user_agent, created_at FROM audit_logs WHERE organization_id = ? ORDER BY created_at DESC LIMIT 100`
	rows, err := h.globalDB.DB.Query(query, claims.OrganizationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var id, orgID, userID, action, resType, resID, metaStr, ip, ua string
		var createdAt int64
		if err := rows.Scan(&id, &orgID, &userID, &action, &resType, &resID, &metaStr, &ip, &ua, &createdAt); err != nil {
			continue
		}

		var meta map[string]interface{}
		json.Unmarshal([]byte(metaStr), &meta)

		logs = append(logs, map[string]interface{}{
			"id":            id,
			"organization_id": orgID,
			"user_id":       userID,
			"action":        action,
			"resource_type": resType,
			"resource_id":   resID,
			"metadata":      meta,
			"ip_address":    ip,
			"user_agent":    ua,
			"created_at":    createdAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}
