package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/eslam/cms/internal/auth"
	"github.com/eslam/cms/internal/models"
	"github.com/jackc/pgx/v5"
)

type loginResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	User         models.User `json:"user"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(r, "login:"+clientIP(r), 10, time.Minute) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts, try again later")
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var u models.User
	err := s.pool.QueryRow(r.Context(), `
		SELECT id, name, email, password_hash, is_platform_admin, status, created_at, updated_at
		FROM users WHERE lower(email) = lower($1)`, req.Email).
		Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.IsPlatformAdmin, &u.Status,
			&u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !auth.CheckPassword(u.PasswordHash, req.Password)) {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err != nil {
		serverError(w, "login", err)
		return
	}
	if u.Status != "active" {
		writeError(w, http.StatusForbidden, "account is disabled")
		return
	}

	resp, err := s.issueTokens(r, &u)
	if err != nil {
		serverError(w, "issue tokens", err)
		return
	}
	s.audit.Record(r.Context(), "", u.ID, "user.login", "user", u.ID, nil)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) issueTokens(r *http.Request, u *models.User) (*loginResponse, error) {
	access, err := s.auth.NewAccessToken(u.ID, u.IsPlatformAdmin)
	if err != nil {
		return nil, err
	}
	refresh, hash, err := auth.NewRefreshToken()
	if err != nil {
		return nil, err
	}
	_, err = s.pool.Exec(r.Context(), `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		u.ID, hash, time.Now().Add(auth.RefreshTokenTTL))
	if err != nil {
		return nil, err
	}
	return &loginResponse{AccessToken: access, RefreshToken: refresh, User: *u}, nil
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := readJSON(r, &req); err != nil || req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token required")
		return
	}
	hash := auth.HashToken(req.RefreshToken)

	var u models.User
	var tokenID string
	err := s.pool.QueryRow(r.Context(), `
		SELECT rt.id, u.id, u.name, u.email, u.is_platform_admin, u.status, u.created_at, u.updated_at
		FROM refresh_tokens rt JOIN users u ON u.id = rt.user_id
		WHERE rt.token_hash = $1 AND rt.expires_at > now() AND rt.revoked_at IS NULL`, hash).
		Scan(&tokenID, &u.ID, &u.Name, &u.Email, &u.IsPlatformAdmin, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	if err != nil {
		serverError(w, "refresh", err)
		return
	}
	if u.Status != "active" {
		writeError(w, http.StatusForbidden, "account is disabled")
		return
	}

	// Rotate: revoke the used token and issue a new pair.
	if _, err := s.pool.Exec(r.Context(),
		`UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1`, tokenID); err != nil {
		serverError(w, "revoke refresh token", err)
		return
	}
	resp, err := s.issueTokens(r, &u)
	if err != nil {
		serverError(w, "issue tokens", err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := readJSON(r, &req); err != nil || req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token required")
		return
	}
	_, err := s.pool.Exec(r.Context(),
		`UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`,
		auth.HashToken(req.RefreshToken))
	if err != nil {
		serverError(w, "logout", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}
	var hash string
	if err := s.pool.QueryRow(r.Context(),
		`SELECT password_hash FROM users WHERE id = $1`, ai.UserID).Scan(&hash); err != nil {
		serverError(w, "load user", err)
		return
	}
	if !auth.CheckPassword(hash, req.CurrentPassword) {
		writeError(w, http.StatusForbidden, "current password is incorrect")
		return
	}
	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		serverError(w, "hash password", err)
		return
	}
	if _, err := s.pool.Exec(r.Context(),
		`UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`,
		newHash, ai.UserID); err != nil {
		serverError(w, "update password", err)
		return
	}
	// Force re-login everywhere else.
	_, _ = s.pool.Exec(r.Context(),
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, ai.UserID)
	s.audit.Record(r.Context(), "", ai.UserID, "user.password_changed", "user", ai.UserID, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleMe returns the current user and their tenant memberships — the
// dashboard uses this for tenant selection.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var u models.User
	err := s.pool.QueryRow(r.Context(), `
		SELECT id, name, email, is_platform_admin, status, created_at, updated_at
		FROM users WHERE id = $1`, ai.UserID).
		Scan(&u.ID, &u.Name, &u.Email, &u.IsPlatformAdmin, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		serverError(w, "load user", err)
		return
	}

	type membership struct {
		TenantID   string `json:"tenant_id"`
		TenantName string `json:"tenant_name"`
		Role       string `json:"role"`
	}
	memberships := []membership{}
	rows, err := s.pool.Query(r.Context(), `
		SELECT t.id, t.name, tu.role FROM tenant_users tu
		JOIN tenants t ON t.id = tu.tenant_id
		WHERE tu.user_id = $1 AND t.status = 'active' ORDER BY t.name`, ai.UserID)
	if err != nil {
		serverError(w, "load memberships", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var mb membership
		if err := rows.Scan(&mb.TenantID, &mb.TenantName, &mb.Role); err != nil {
			serverError(w, "scan membership", err)
			return
		}
		memberships = append(memberships, mb)
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u, "memberships": memberships})
}
