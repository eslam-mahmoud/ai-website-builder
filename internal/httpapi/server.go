// Package httpapi exposes the CMS REST API and serves the admin dashboard,
// preview builds, and locally-stored media.
package httpapi

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/eslam/cms/internal/audit"
	"github.com/eslam/cms/internal/auth"
	"github.com/eslam/cms/internal/cache"
	"github.com/eslam/cms/internal/config"
	"github.com/eslam/cms/internal/models"
	"github.com/eslam/cms/internal/publish"
	"github.com/eslam/cms/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	pool  *pgxpool.Pool
	cache cache.Cache
	cfg   *config.Config
	auth  *auth.Manager
	audit *audit.Logger
	store storage.Storage
	pub   *publish.Publisher
	mux   *http.ServeMux
}

func New(pool *pgxpool.Pool, c cache.Cache, cfg *config.Config, am *auth.Manager,
	al *audit.Logger, st storage.Storage, pub *publish.Publisher, adminDir string) *Server {

	s := &Server{pool: pool, cache: c, cfg: cfg, auth: am, audit: al, store: st, pub: pub,
		mux: http.NewServeMux()}
	s.routes(adminDir)
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes(adminDir string) {
	m := s.mux

	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Auth
	m.HandleFunc("POST /api/auth/login", s.handleLogin)
	m.HandleFunc("POST /api/auth/refresh", s.handleRefresh)
	m.HandleFunc("POST /api/auth/logout", s.handleLogout)
	m.HandleFunc("POST /api/auth/change-password", s.requireAuth(s.handleChangePassword))
	m.HandleFunc("GET /api/me", s.requireAuth(s.handleMe))

	// Block type schemas (per tenant)
	m.HandleFunc("GET /api/tenants/{tenantID}/section-types", s.tenantRoute(models.RoleViewer, s.handleListSectionTypes))
	m.HandleFunc("POST /api/tenants/{tenantID}/section-types", s.tenantRoute(models.RoleTenantAdmin, s.handleCreateSectionType))
	m.HandleFunc("PATCH /api/tenants/{tenantID}/section-types/{typeID}", s.tenantRoute(models.RoleTenantAdmin, s.handleUpdateSectionType))
	m.HandleFunc("DELETE /api/tenants/{tenantID}/section-types/{typeID}", s.tenantRoute(models.RoleTenantAdmin, s.handleArchiveSectionType))

	// Tenants (platform admin)
	m.HandleFunc("POST /api/tenants", s.requirePlatformAdmin(s.handleCreateTenant))
	m.HandleFunc("GET /api/tenants", s.requirePlatformAdmin(s.handleListTenants))
	m.HandleFunc("PATCH /api/tenants/{tenantID}", s.requirePlatformAdmin(s.handleUpdateTenant))

	// Tenant-scoped (role names are the minimum required role)
	m.HandleFunc("GET /api/tenants/{tenantID}", s.tenantRoute(models.RoleViewer, s.handleGetTenant))
	m.HandleFunc("GET /api/tenants/{tenantID}/users", s.tenantRoute(models.RoleTenantAdmin, s.handleListTenantUsers))
	m.HandleFunc("POST /api/tenants/{tenantID}/users", s.tenantRoute(models.RoleTenantAdmin, s.handleInviteUser))
	m.HandleFunc("PATCH /api/tenants/{tenantID}/users/{userID}", s.tenantRoute(models.RoleTenantAdmin, s.handleUpdateTenantUser))
	m.HandleFunc("DELETE /api/tenants/{tenantID}/users/{userID}", s.tenantRoute(models.RoleTenantAdmin, s.handleRemoveTenantUser))

	m.HandleFunc("POST /api/tenants/{tenantID}/websites", s.tenantRoute(models.RoleTenantAdmin, s.handleCreateWebsite))
	m.HandleFunc("GET /api/tenants/{tenantID}/websites", s.tenantRoute(models.RoleViewer, s.handleListWebsites))
	m.HandleFunc("GET /api/tenants/{tenantID}/websites/{websiteID}", s.tenantRoute(models.RoleViewer, s.handleGetWebsite))
	m.HandleFunc("PATCH /api/tenants/{tenantID}/websites/{websiteID}", s.tenantRoute(models.RoleTenantAdmin, s.handleUpdateWebsite))

	m.HandleFunc("GET /api/tenants/{tenantID}/websites/{websiteID}/pages", s.tenantRoute(models.RoleViewer, s.handleListPages))
	m.HandleFunc("POST /api/tenants/{tenantID}/websites/{websiteID}/pages", s.tenantRoute(models.RoleEditor, s.handleCreatePage))
	m.HandleFunc("PUT /api/tenants/{tenantID}/websites/{websiteID}/pages/order", s.tenantRoute(models.RoleEditor, s.handleReorderPages))
	m.HandleFunc("PATCH /api/tenants/{tenantID}/pages/{pageID}", s.tenantRoute(models.RoleEditor, s.handleUpdatePage))
	m.HandleFunc("DELETE /api/tenants/{tenantID}/pages/{pageID}", s.tenantRoute(models.RoleEditor, s.handleDeletePage))

	m.HandleFunc("GET /api/tenants/{tenantID}/pages/{pageID}/sections", s.tenantRoute(models.RoleViewer, s.handleListSections))
	m.HandleFunc("POST /api/tenants/{tenantID}/pages/{pageID}/sections", s.tenantRoute(models.RoleEditor, s.handleCreateSection))
	m.HandleFunc("PUT /api/tenants/{tenantID}/pages/{pageID}/sections/order", s.tenantRoute(models.RoleEditor, s.handleReorderSections))
	m.HandleFunc("PATCH /api/tenants/{tenantID}/sections/{sectionID}", s.tenantRoute(models.RoleEditor, s.handleUpdateSection))
	m.HandleFunc("DELETE /api/tenants/{tenantID}/sections/{sectionID}", s.tenantRoute(models.RoleEditor, s.handleDeleteSection))

	m.HandleFunc("GET /api/tenants/{tenantID}/media", s.tenantRoute(models.RoleViewer, s.handleListMedia))
	m.HandleFunc("POST /api/tenants/{tenantID}/media", s.tenantRoute(models.RoleEditor, s.handleUploadMedia))
	m.HandleFunc("PATCH /api/tenants/{tenantID}/media/{mediaID}", s.tenantRoute(models.RoleEditor, s.handleUpdateMedia))
	m.HandleFunc("DELETE /api/tenants/{tenantID}/media/{mediaID}", s.tenantRoute(models.RoleEditor, s.handleDeleteMedia))

	m.HandleFunc("POST /api/tenants/{tenantID}/websites/{websiteID}/publish", s.tenantRoute(models.RoleTenantAdmin, s.handlePublish))
	m.HandleFunc("POST /api/tenants/{tenantID}/websites/{websiteID}/preview", s.tenantRoute(models.RoleEditor, s.handlePreview))
	m.HandleFunc("GET /api/tenants/{tenantID}/websites/{websiteID}/deployments", s.tenantRoute(models.RoleViewer, s.handleListDeployments))
	m.HandleFunc("GET /api/tenants/{tenantID}/deployments/{deploymentID}", s.tenantRoute(models.RoleViewer, s.handleGetDeployment))
	m.HandleFunc("POST /api/tenants/{tenantID}/deployments/{deploymentID}/rollback", s.tenantRoute(models.RoleTenantAdmin, s.handleRollback))

	m.HandleFunc("GET /api/tenants/{tenantID}/audit-logs", s.tenantRoute(models.RoleTenantAdmin, s.handleListAuditLogs))

	// Static: admin dashboard, locally-stored media, previews, local site builds.
	m.Handle("GET /admin/", http.StripPrefix("/admin/", http.FileServer(http.Dir(adminDir))))
	m.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
	})
	m.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/admin/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	mediaDir := filepath.Join(s.cfg.DataDir, "media")
	m.Handle("GET /media/", http.StripPrefix("/media/", http.FileServer(http.Dir(mediaDir))))

	sitesDir := filepath.Join(s.cfg.DataDir, "sites")
	m.Handle("GET /sites/", http.StripPrefix("/sites/", http.FileServer(http.Dir(sitesDir))))

	previewsDir := filepath.Join(s.cfg.DataDir, "previews")
	previewFS := http.StripPrefix("/preview/", http.FileServer(http.Dir(previewsDir)))
	m.HandleFunc("GET /preview/{token}/{path...}", func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		if !s.pub.PreviewValid(r.Context(), token) {
			http.Error(w, "preview expired", http.StatusNotFound)
			return
		}
		previewFS.ServeHTTP(w, r)
	})

	_ = os.MkdirAll(mediaDir, 0o755)
	_ = os.MkdirAll(sitesDir, 0o755)
	_ = os.MkdirAll(previewsDir, 0o755)
}
