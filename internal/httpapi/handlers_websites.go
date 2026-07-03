package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/eslam/cms/internal/models"
)

const websiteCols = `id, tenant_id, name, domain, template_id, status, settings_json, created_at, updated_at`

func scanWebsite(row interface{ Scan(...any) error }) (models.Website, error) {
	var wsite models.Website
	err := row.Scan(&wsite.ID, &wsite.TenantID, &wsite.Name, &wsite.Domain, &wsite.TemplateID,
		&wsite.Status, &wsite.Settings, &wsite.CreatedAt, &wsite.UpdatedAt)
	return wsite, err
}

func (s *Server) handleCreateWebsite(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var req struct {
		Name       string `json:"name"`
		Domain     string `json:"domain"`
		TemplateID string `json:"template_id"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.TemplateID == "" {
		req.TemplateID = "business"
	}
	wsite, err := scanWebsite(s.pool.QueryRow(r.Context(), `
		INSERT INTO websites (tenant_id, name, domain, template_id)
		VALUES ($1, $2, $3, $4) RETURNING `+websiteCols,
		ai.TenantID, strings.TrimSpace(req.Name), strings.TrimSpace(req.Domain), req.TemplateID))
	if err != nil {
		serverError(w, "create website", err)
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "website.created", "website", wsite.ID,
		map[string]any{"name": wsite.Name})
	writeJSON(w, http.StatusCreated, wsite)
}

func (s *Server) handleListWebsites(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	rows, err := s.pool.Query(r.Context(),
		`SELECT `+websiteCols+` FROM websites WHERE tenant_id = $1 ORDER BY created_at`, ai.TenantID)
	if err != nil {
		serverError(w, "list websites", err)
		return
	}
	defer rows.Close()
	sites := []models.Website{}
	for rows.Next() {
		wsite, err := scanWebsite(rows)
		if err != nil {
			serverError(w, "scan website", err)
			return
		}
		sites = append(sites, wsite)
	}
	writeJSON(w, http.StatusOK, sites)
}

func (s *Server) handleGetWebsite(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	wsite, err := scanWebsite(s.pool.QueryRow(r.Context(),
		`SELECT `+websiteCols+` FROM websites WHERE id = $1 AND tenant_id = $2`,
		r.PathValue("websiteID"), ai.TenantID))
	if err != nil {
		serverError(w, "get website", err)
		return
	}
	writeJSON(w, http.StatusOK, wsite)
}

// handleUpdateWebsite updates website fields and/or settings. Settings are
// validated against the structured WebsiteSettings shape.
func (s *Server) handleUpdateWebsite(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var req struct {
		Name       *string                 `json:"name"`
		Domain     *string                 `json:"domain"`
		TemplateID *string                 `json:"template_id"`
		Status     *string                 `json:"status"`
		Settings   *models.WebsiteSettings `json:"settings"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Status != nil && *req.Status != "active" && *req.Status != "disabled" {
		writeError(w, http.StatusBadRequest, "status must be active or disabled")
		return
	}
	var settingsJSON []byte
	if req.Settings != nil {
		b, err := json.Marshal(req.Settings)
		if err != nil {
			serverError(w, "encode settings", err)
			return
		}
		settingsJSON = b
	}
	wsite, err := scanWebsite(s.pool.QueryRow(r.Context(), `
		UPDATE websites SET
			name = COALESCE($1, name),
			domain = COALESCE($2, domain),
			template_id = COALESCE($3, template_id),
			status = COALESCE($4, status),
			settings_json = COALESCE($5, settings_json),
			updated_at = now()
		WHERE id = $6 AND tenant_id = $7 RETURNING `+websiteCols,
		req.Name, req.Domain, req.TemplateID, req.Status, settingsJSON,
		r.PathValue("websiteID"), ai.TenantID))
	if err != nil {
		serverError(w, "update website", err)
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "website.updated", "website", wsite.ID, nil)
	writeJSON(w, http.StatusOK, wsite)
}
