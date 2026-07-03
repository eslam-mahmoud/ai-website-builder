package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/eslam/cms/internal/auth"
	"github.com/eslam/cms/internal/db"
	"github.com/eslam/cms/internal/models"
	"github.com/jackc/pgx/v5"
)

func (s *Server) handleCreateTenant(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	var t models.Tenant
	err := s.pool.QueryRow(r.Context(), `
		INSERT INTO tenants (name) VALUES ($1)
		RETURNING id, name, status, created_at, updated_at`, strings.TrimSpace(req.Name)).
		Scan(&t.ID, &t.Name, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		serverError(w, "create tenant", err)
		return
	}
	if err := db.SeedSectionTypes(r.Context(), s.pool, t.ID); err != nil {
		serverError(w, "seed block types", err)
		return
	}
	s.audit.Record(r.Context(), t.ID, ai.UserID, "tenant.created", "tenant", t.ID,
		map[string]any{"name": t.Name})
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleListTenants(w http.ResponseWriter, r *http.Request, _ *authInfo) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, name, status, created_at, updated_at FROM tenants ORDER BY created_at`)
	if err != nil {
		serverError(w, "list tenants", err)
		return
	}
	defer rows.Close()
	tenants := []models.Tenant{}
	for rows.Next() {
		var t models.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
			serverError(w, "scan tenant", err)
			return
		}
		tenants = append(tenants, t)
	}
	writeJSON(w, http.StatusOK, tenants)
}

func (s *Server) handleGetTenant(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var t models.Tenant
	err := s.pool.QueryRow(r.Context(), `
		SELECT id, name, status, created_at, updated_at FROM tenants WHERE id = $1`,
		ai.TenantID).Scan(&t.ID, &t.Name, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		serverError(w, "get tenant", err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleUpdateTenant(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	tenantID := r.PathValue("tenantID")
	var req struct {
		Name   *string `json:"name"`
		Status *string `json:"status"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Status != nil && *req.Status != "active" && *req.Status != "disabled" {
		writeError(w, http.StatusBadRequest, "status must be active or disabled")
		return
	}
	var t models.Tenant
	err := s.pool.QueryRow(r.Context(), `
		UPDATE tenants SET
			name = COALESCE($1, name),
			status = COALESCE($2, status),
			updated_at = now()
		WHERE id = $3
		RETURNING id, name, status, created_at, updated_at`,
		req.Name, req.Status, tenantID).
		Scan(&t.ID, &t.Name, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		serverError(w, "update tenant", err)
		return
	}
	s.audit.Record(r.Context(), t.ID, ai.UserID, "tenant.updated", "tenant", t.ID,
		map[string]any{"status": t.Status})
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleListTenantUsers(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT tu.id, tu.tenant_id, tu.user_id, tu.role, tu.created_at, u.name, u.email
		FROM tenant_users tu JOIN users u ON u.id = tu.user_id
		WHERE tu.tenant_id = $1 ORDER BY tu.created_at`, ai.TenantID)
	if err != nil {
		serverError(w, "list tenant users", err)
		return
	}
	defer rows.Close()
	users := []models.TenantUser{}
	for rows.Next() {
		var tu models.TenantUser
		if err := rows.Scan(&tu.ID, &tu.TenantID, &tu.UserID, &tu.Role, &tu.CreatedAt,
			&tu.UserName, &tu.UserEmail); err != nil {
			serverError(w, "scan tenant user", err)
			return
		}
		users = append(users, tu)
	}
	writeJSON(w, http.StatusOK, users)
}

// handleInviteUser creates the user if needed (with a one-time temporary
// password returned to the inviter) and assigns them to the tenant.
func (s *Server) handleInviteUser(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var req struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !validEmail(req.Email) {
		writeError(w, http.StatusBadRequest, "valid email is required")
		return
	}
	if !models.ValidRole(req.Role) {
		writeError(w, http.StatusBadRequest, "role must be tenant_admin, editor or viewer")
		return
	}

	var userID string
	tempPassword := ""
	err := s.pool.QueryRow(r.Context(),
		`SELECT id FROM users WHERE lower(email) = $1`, req.Email).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		if strings.TrimSpace(req.Name) == "" {
			req.Name = req.Email
		}
		tempPassword, err = auth.RandomPassword()
		if err != nil {
			serverError(w, "generate password", err)
			return
		}
		hash, err := auth.HashPassword(tempPassword)
		if err != nil {
			serverError(w, "hash password", err)
			return
		}
		err = s.pool.QueryRow(r.Context(), `
			INSERT INTO users (name, email, password_hash) VALUES ($1, $2, $3) RETURNING id`,
			strings.TrimSpace(req.Name), req.Email, hash).Scan(&userID)
		if err != nil {
			serverError(w, "create user", err)
			return
		}
	} else if err != nil {
		serverError(w, "lookup user", err)
		return
	}

	var tu models.TenantUser
	err = s.pool.QueryRow(r.Context(), `
		INSERT INTO tenant_users (tenant_id, user_id, role) VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, user_id) DO UPDATE SET role = EXCLUDED.role
		RETURNING id, tenant_id, user_id, role, created_at`,
		ai.TenantID, userID, req.Role).
		Scan(&tu.ID, &tu.TenantID, &tu.UserID, &tu.Role, &tu.CreatedAt)
	if err != nil {
		serverError(w, "assign user", err)
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "user.invited", "user", userID,
		map[string]any{"email": req.Email, "role": req.Role})

	resp := map[string]any{"membership": tu}
	if tempPassword != "" {
		resp["temporary_password"] = tempPassword
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleUpdateTenantUser(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var req struct {
		Role string `json:"role"`
	}
	if err := readJSON(r, &req); err != nil || !models.ValidRole(req.Role) {
		writeError(w, http.StatusBadRequest, "role must be tenant_admin, editor or viewer")
		return
	}
	tag, err := s.pool.Exec(r.Context(), `
		UPDATE tenant_users SET role = $1 WHERE tenant_id = $2 AND user_id = $3`,
		req.Role, ai.TenantID, r.PathValue("userID"))
	if err != nil {
		serverError(w, "update role", err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "membership not found")
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "user.role_changed", "user",
		r.PathValue("userID"), map[string]any{"role": req.Role})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleRemoveTenantUser(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	tag, err := s.pool.Exec(r.Context(),
		`DELETE FROM tenant_users WHERE tenant_id = $1 AND user_id = $2`,
		ai.TenantID, r.PathValue("userID"))
	if err != nil {
		serverError(w, "remove user", err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "membership not found")
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "user.removed", "user",
		r.PathValue("userID"), nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
