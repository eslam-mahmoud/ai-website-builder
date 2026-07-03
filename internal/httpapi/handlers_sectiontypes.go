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

const sectionTypeCols = `id, tenant_id, type_key, label, icon, fields_json, layout_json,
	status, created_at, updated_at`

func scanSectionType(row interface{ Scan(...any) error }) (models.SectionType, error) {
	var st models.SectionType
	var fields, layout []byte
	err := row.Scan(&st.ID, &st.TenantID, &st.TypeKey, &st.Label, &st.Icon,
		&fields, &layout, &st.Status, &st.CreatedAt, &st.UpdatedAt)
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(fields, &st.Fields); err != nil {
		return st, err
	}
	if err := json.Unmarshal(layout, &st.Layout); err != nil {
		return st, err
	}
	return st, nil
}

func (s *Server) handleListSectionTypes(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT `+sectionTypeCols+` FROM section_types
		WHERE tenant_id = $1 AND status = 'active' ORDER BY created_at`, ai.TenantID)
	if err != nil {
		serverError(w, "list section types", err)
		return
	}
	defer rows.Close()
	types := []models.SectionType{}
	for rows.Next() {
		st, err := scanSectionType(rows)
		if err != nil {
			serverError(w, "scan section type", err)
			return
		}
		types = append(types, st)
	}
	writeJSON(w, http.StatusOK, types)
}

type sectionTypeRequest struct {
	TypeKey string              `json:"type_key"`
	Label   string              `json:"label"`
	Icon    string              `json:"icon"`
	Fields  []models.FieldSpec  `json:"fields"`
	Layout  *models.LayoutHints `json:"layout"`
}

func (s *Server) handleCreateSectionType(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var req sectionTypeRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if !validSlug(strings.ReplaceAll(req.TypeKey, "_", "-")) {
		writeError(w, http.StatusBadRequest, "type_key must be lowercase letters, digits, hyphens or underscores")
		return
	}
	if strings.TrimSpace(req.Label) == "" {
		writeError(w, http.StatusBadRequest, "label is required")
		return
	}
	layout := models.LayoutHints{}
	if req.Layout != nil {
		layout = *req.Layout
	}
	if err := models.ValidateSchema(req.Fields, layout); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	fieldsJSON, _ := json.Marshal(req.Fields)
	layoutJSON, _ := json.Marshal(layout)

	st, err := scanSectionType(s.pool.QueryRow(r.Context(), `
		INSERT INTO section_types (tenant_id, type_key, label, icon, fields_json, layout_json)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+sectionTypeCols,
		ai.TenantID, req.TypeKey, strings.TrimSpace(req.Label), req.Icon, fieldsJSON, layoutJSON))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "a block type with this key already exists")
			return
		}
		serverError(w, "create section type", err)
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "section_type.created",
		"section_type", st.ID, map[string]any{"type_key": st.TypeKey})
	writeJSON(w, http.StatusCreated, st)
}

func (s *Server) handleUpdateSectionType(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var req sectionTypeRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Label) == "" {
		writeError(w, http.StatusBadRequest, "label is required")
		return
	}
	layout := models.LayoutHints{}
	if req.Layout != nil {
		layout = *req.Layout
	}
	if err := models.ValidateSchema(req.Fields, layout); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	fieldsJSON, _ := json.Marshal(req.Fields)
	layoutJSON, _ := json.Marshal(layout)

	// type_key is immutable — sections reference it.
	st, err := scanSectionType(s.pool.QueryRow(r.Context(), `
		UPDATE section_types SET label = $1, icon = $2, fields_json = $3, layout_json = $4,
			updated_at = now()
		WHERE id = $5 AND tenant_id = $6 AND status = 'active'
		RETURNING `+sectionTypeCols,
		strings.TrimSpace(req.Label), req.Icon, fieldsJSON, layoutJSON,
		r.PathValue("typeID"), ai.TenantID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "block type not found")
		return
	}
	if err != nil {
		serverError(w, "update section type", err)
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "section_type.updated",
		"section_type", st.ID, map[string]any{"type_key": st.TypeKey})
	writeJSON(w, http.StatusOK, st)
}

// handleArchiveSectionType archives a block type. Refused while sections
// still use it, so pages never reference a missing schema.
func (s *Server) handleArchiveSectionType(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var typeKey string
	err := s.pool.QueryRow(r.Context(), `
		SELECT type_key FROM section_types WHERE id = $1 AND tenant_id = $2 AND status = 'active'`,
		r.PathValue("typeID"), ai.TenantID).Scan(&typeKey)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "block type not found")
		return
	}
	if err != nil {
		serverError(w, "load section type", err)
		return
	}
	var inUse int
	if err := s.pool.QueryRow(r.Context(), `
		SELECT count(*) FROM sections WHERE tenant_id = $1 AND section_type = $2`,
		ai.TenantID, typeKey).Scan(&inUse); err != nil {
		serverError(w, "check usage", err)
		return
	}
	if inUse > 0 {
		writeError(w, http.StatusConflict,
			"this block type is used by existing sections; remove those blocks first")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `
		UPDATE section_types SET status = 'archived', updated_at = now()
		WHERE id = $1 AND tenant_id = $2`, r.PathValue("typeID"), ai.TenantID); err != nil {
		serverError(w, "archive section type", err)
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "section_type.archived",
		"section_type", r.PathValue("typeID"), map[string]any{"type_key": typeKey})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// sectionTypeExists checks that a type_key is an active block type of the tenant.
func (s *Server) sectionTypeExists(r *http.Request, tenantID, typeKey string) bool {
	var ok bool
	err := s.pool.QueryRow(r.Context(), `
		SELECT EXISTS (SELECT 1 FROM section_types
		WHERE tenant_id = $1 AND type_key = $2 AND status = 'active')`,
		tenantID, typeKey).Scan(&ok)
	return err == nil && ok
}
