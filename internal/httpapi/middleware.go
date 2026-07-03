package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/eslam/cms/internal/models"
	"github.com/jackc/pgx/v5"
)

// authInfo carries the authenticated caller through a request.
type authInfo struct {
	UserID          string
	IsPlatformAdmin bool
	// TenantID and Role are set by tenantRoute.
	TenantID string
	Role     string
}

type authedHandler func(w http.ResponseWriter, r *http.Request, ai *authInfo)

// requireAuth validates the bearer token and confirms the user is active.
func (s *Server) requireAuth(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims, err := s.auth.ParseAccessToken(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		var status string
		var isAdmin bool
		err = s.pool.QueryRow(r.Context(),
			`SELECT status, is_platform_admin FROM users WHERE id = $1`, claims.UserID).
			Scan(&status, &isAdmin)
		if err != nil || status != "active" {
			writeError(w, http.StatusUnauthorized, "account unavailable")
			return
		}
		next(w, r, &authInfo{UserID: claims.UserID, IsPlatformAdmin: isAdmin})
	}
}

func (s *Server) requirePlatformAdmin(next authedHandler) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request, ai *authInfo) {
		if !ai.IsPlatformAdmin {
			writeError(w, http.StatusForbidden, "platform admin required")
			return
		}
		next(w, r, ai)
	})
}

// tenantRoute enforces tenant membership at the given minimum role. Platform
// admins have full access to every tenant. Tenant isolation: the resolved
// tenant ID is what every downstream query filters by.
func (s *Server) tenantRoute(minRole string, next authedHandler) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request, ai *authInfo) {
		tenantID := r.PathValue("tenantID")
		role, err := s.resolveTenantRole(r.Context(), ai, tenantID)
		if err != nil {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		if !models.RoleAtLeast(role, minRole) {
			writeError(w, http.StatusForbidden, "insufficient role")
			return
		}
		ai.TenantID = tenantID
		ai.Role = role
		next(w, r, ai)
	})
}

// resolveTenantRole returns the caller's effective role in the tenant.
func (s *Server) resolveTenantRole(ctx context.Context, ai *authInfo, tenantID string) (string, error) {
	var tenantStatus string
	err := s.pool.QueryRow(ctx, `SELECT status FROM tenants WHERE id = $1`, tenantID).
		Scan(&tenantStatus)
	if err != nil {
		return "", errors.New("tenant not found")
	}
	if ai.IsPlatformAdmin {
		return models.RoleTenantAdmin, nil
	}
	if tenantStatus != "active" {
		return "", errors.New("tenant is disabled")
	}
	var role string
	err = s.pool.QueryRow(ctx,
		`SELECT role FROM tenant_users WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, ai.UserID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("not a member of this tenant")
	}
	if err != nil {
		return "", errors.New("authorization check failed")
	}
	return role, nil
}

// rateLimit rejects the request when key exceeds max hits per window.
func (s *Server) rateLimit(r *http.Request, key string, max int64, window time.Duration) bool {
	n := s.cache.Incr(r.Context(), "rl:"+key, window)
	return n <= max
}
