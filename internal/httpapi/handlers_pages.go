package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/eslam/cms/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const pageCols = `id, tenant_id, website_id, title, slug, status, sort_order,
	seo_title, seo_description, created_at, updated_at`

func scanPage(row interface{ Scan(...any) error }) (models.Page, error) {
	var p models.Page
	err := row.Scan(&p.ID, &p.TenantID, &p.WebsiteID, &p.Title, &p.Slug, &p.Status,
		&p.SortOrder, &p.SEOTitle, &p.SEODescription, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// websiteInTenant guards nested resources against cross-tenant access.
func (s *Server) websiteInTenant(r *http.Request, websiteID, tenantID string) bool {
	var ok bool
	err := s.pool.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM websites WHERE id = $1 AND tenant_id = $2)`,
		websiteID, tenantID).Scan(&ok)
	return err == nil && ok
}

func (s *Server) handleListPages(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT `+pageCols+` FROM pages WHERE website_id = $1 AND tenant_id = $2
		ORDER BY sort_order, created_at`, r.PathValue("websiteID"), ai.TenantID)
	if err != nil {
		serverError(w, "list pages", err)
		return
	}
	defer rows.Close()
	pages := []models.Page{}
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			serverError(w, "scan page", err)
			return
		}
		pages = append(pages, p)
	}
	writeJSON(w, http.StatusOK, pages)
}

func (s *Server) handleCreatePage(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	websiteID := r.PathValue("websiteID")
	if !s.websiteInTenant(r, websiteID, ai.TenantID) {
		writeError(w, http.StatusNotFound, "website not found")
		return
	}
	var req struct {
		Title          string `json:"title"`
		Slug           string `json:"slug"`
		SEOTitle       string `json:"seo_title"`
		SEODescription string `json:"seo_description"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if !validSlug(req.Slug) {
		writeError(w, http.StatusBadRequest, "slug must be lowercase letters, numbers and hyphens")
		return
	}
	p, err := scanPage(s.pool.QueryRow(r.Context(), `
		INSERT INTO pages (tenant_id, website_id, title, slug, seo_title, seo_description, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6,
			(SELECT COALESCE(MAX(sort_order), -1) + 1 FROM pages WHERE website_id = $2))
		RETURNING `+pageCols,
		ai.TenantID, websiteID, strings.TrimSpace(req.Title), req.Slug,
		req.SEOTitle, req.SEODescription))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "a page with this slug already exists")
			return
		}
		serverError(w, "create page", err)
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "page.created", "page", p.ID,
		map[string]any{"title": p.Title, "slug": p.Slug})
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleUpdatePage(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var req struct {
		Title          *string `json:"title"`
		Slug           *string `json:"slug"`
		Status         *string `json:"status"`
		SEOTitle       *string `json:"seo_title"`
		SEODescription *string `json:"seo_description"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Slug != nil && !validSlug(*req.Slug) {
		writeError(w, http.StatusBadRequest, "slug must be lowercase letters, numbers and hyphens")
		return
	}
	if req.Status != nil && *req.Status != "visible" && *req.Status != "hidden" {
		writeError(w, http.StatusBadRequest, "status must be visible or hidden")
		return
	}
	p, err := scanPage(s.pool.QueryRow(r.Context(), `
		UPDATE pages SET
			title = COALESCE($1, title),
			slug = COALESCE($2, slug),
			status = COALESCE($3, status),
			seo_title = COALESCE($4, seo_title),
			seo_description = COALESCE($5, seo_description),
			updated_at = now()
		WHERE id = $6 AND tenant_id = $7 RETURNING `+pageCols,
		req.Title, req.Slug, req.Status, req.SEOTitle, req.SEODescription,
		r.PathValue("pageID"), ai.TenantID))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "a page with this slug already exists")
			return
		}
		serverError(w, "update page", err)
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "page.updated", "page", p.ID, nil)
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePage(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	tag, err := s.pool.Exec(r.Context(),
		`DELETE FROM pages WHERE id = $1 AND tenant_id = $2`,
		r.PathValue("pageID"), ai.TenantID)
	if err != nil {
		serverError(w, "delete page", err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "page not found")
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "page.deleted", "page",
		r.PathValue("pageID"), nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleReorderPages(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var req struct {
		PageIDs []string `json:"page_ids"`
	}
	if err := readJSON(r, &req); err != nil || len(req.PageIDs) == 0 {
		writeError(w, http.StatusBadRequest, "page_ids is required")
		return
	}
	websiteID := r.PathValue("websiteID")
	for i, id := range req.PageIDs {
		if _, err := s.pool.Exec(r.Context(), `
			UPDATE pages SET sort_order = $1, updated_at = now()
			WHERE id = $2 AND website_id = $3 AND tenant_id = $4`,
			i, id, websiteID, ai.TenantID); err != nil {
			serverError(w, "reorder pages", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- Sections ---

const sectionCols = `id, tenant_id, page_id, section_type, sort_order, content_json,
	status, created_at, updated_at`

func scanSection(row interface{ Scan(...any) error }) (models.Section, error) {
	var sec models.Section
	err := row.Scan(&sec.ID, &sec.TenantID, &sec.PageID, &sec.SectionType, &sec.SortOrder,
		&sec.Content, &sec.Status, &sec.CreatedAt, &sec.UpdatedAt)
	return sec, err
}

func (s *Server) pageInTenant(r *http.Request, pageID, tenantID string) bool {
	var ok bool
	err := s.pool.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM pages WHERE id = $1 AND tenant_id = $2)`,
		pageID, tenantID).Scan(&ok)
	return err == nil && ok
}

func (s *Server) handleListSections(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT `+sectionCols+` FROM sections WHERE page_id = $1 AND tenant_id = $2
		ORDER BY sort_order, created_at`, r.PathValue("pageID"), ai.TenantID)
	if err != nil {
		serverError(w, "list sections", err)
		return
	}
	defer rows.Close()
	sections := []models.Section{}
	for rows.Next() {
		sec, err := scanSection(rows)
		if err != nil {
			serverError(w, "scan section", err)
			return
		}
		sections = append(sections, sec)
	}
	writeJSON(w, http.StatusOK, sections)
}

func (s *Server) handleCreateSection(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	pageID := r.PathValue("pageID")
	if !s.pageInTenant(r, pageID, ai.TenantID) {
		writeError(w, http.StatusNotFound, "page not found")
		return
	}
	var req struct {
		SectionType string          `json:"section_type"`
		Content     json.RawMessage `json:"content"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.sectionTypeExists(r, ai.TenantID, req.SectionType) {
		writeError(w, http.StatusBadRequest, "unknown block type")
		return
	}
	if len(req.Content) == 0 {
		req.Content = json.RawMessage("{}")
	}
	if !json.Valid(req.Content) {
		writeError(w, http.StatusBadRequest, "content must be valid JSON")
		return
	}
	sec, err := scanSection(s.pool.QueryRow(r.Context(), `
		INSERT INTO sections (tenant_id, page_id, section_type, content_json, sort_order)
		VALUES ($1, $2, $3, $4,
			(SELECT COALESCE(MAX(sort_order), -1) + 1 FROM sections WHERE page_id = $2))
		RETURNING `+sectionCols,
		ai.TenantID, pageID, req.SectionType, req.Content))
	if err != nil {
		serverError(w, "create section", err)
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "section.created", "section", sec.ID,
		map[string]any{"type": sec.SectionType})
	writeJSON(w, http.StatusCreated, sec)
}

func (s *Server) handleUpdateSection(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var req struct {
		Content json.RawMessage `json:"content"`
		Status  *string         `json:"status"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Status != nil && *req.Status != "visible" && *req.Status != "hidden" {
		writeError(w, http.StatusBadRequest, "status must be visible or hidden")
		return
	}
	if len(req.Content) > 0 && !json.Valid(req.Content) {
		writeError(w, http.StatusBadRequest, "content must be valid JSON")
		return
	}
	var content any
	if len(req.Content) > 0 {
		content = req.Content
	}
	sec, err := scanSection(s.pool.QueryRow(r.Context(), `
		UPDATE sections SET
			content_json = COALESCE($1, content_json),
			status = COALESCE($2, status),
			updated_at = now()
		WHERE id = $3 AND tenant_id = $4 RETURNING `+sectionCols,
		content, req.Status, r.PathValue("sectionID"), ai.TenantID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "section not found")
		return
	}
	if err != nil {
		serverError(w, "update section", err)
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "section.updated", "section", sec.ID, nil)
	writeJSON(w, http.StatusOK, sec)
}

func (s *Server) handleDeleteSection(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	tag, err := s.pool.Exec(r.Context(),
		`DELETE FROM sections WHERE id = $1 AND tenant_id = $2`,
		r.PathValue("sectionID"), ai.TenantID)
	if err != nil {
		serverError(w, "delete section", err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "section not found")
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "section.deleted", "section",
		r.PathValue("sectionID"), nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleReorderSections(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var req struct {
		SectionIDs []string `json:"section_ids"`
	}
	if err := readJSON(r, &req); err != nil || len(req.SectionIDs) == 0 {
		writeError(w, http.StatusBadRequest, "section_ids is required")
		return
	}
	pageID := r.PathValue("pageID")
	for i, id := range req.SectionIDs {
		if _, err := s.pool.Exec(r.Context(), `
			UPDATE sections SET sort_order = $1, updated_at = now()
			WHERE id = $2 AND page_id = $3 AND tenant_id = $4`,
			i, id, pageID, ai.TenantID); err != nil {
			serverError(w, "reorder sections", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
