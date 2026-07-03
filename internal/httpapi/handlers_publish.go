package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/eslam/cms/internal/models"
	"github.com/eslam/cms/internal/publish"
	"github.com/jackc/pgx/v5"
)

// handlePublish snapshots the current content and starts the publishing
// pipeline (spec §12.6). Publishing is explicit — nothing goes live until
// this endpoint is called.
func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	websiteID := r.PathValue("websiteID")
	if !s.websiteInTenant(r, websiteID, ai.TenantID) {
		writeError(w, http.StatusNotFound, "website not found")
		return
	}

	snap, err := publish.BuildSnapshot(r.Context(), s.pool, ai.TenantID, websiteID)
	if err != nil {
		serverError(w, "build snapshot", err)
		return
	}
	if len(snap.Pages) == 0 {
		writeError(w, http.StatusBadRequest, "website has no visible pages to publish")
		return
	}
	snapJSON, err := json.Marshal(snap)
	if err != nil {
		serverError(w, "encode snapshot", err)
		return
	}
	var snapshotID string
	err = s.pool.QueryRow(r.Context(), `
		INSERT INTO content_snapshots (tenant_id, website_id, snapshot_json, created_by_user_id)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		ai.TenantID, websiteID, snapJSON, ai.UserID).Scan(&snapshotID)
	if err != nil {
		serverError(w, "save snapshot", err)
		return
	}

	d, err := s.pub.StartPublish(r.Context(), ai.TenantID, websiteID, snapshotID, ai.UserID)
	if errors.Is(err, publish.ErrPublishInProgress) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		serverError(w, "start publish", err)
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "publish.triggered", "deployment", d.ID,
		map[string]any{"website_id": websiteID})
	writeJSON(w, http.StatusAccepted, d)
}

// handlePreview builds a temporary preview of the current draft content.
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	websiteID := r.PathValue("websiteID")
	if !s.websiteInTenant(r, websiteID, ai.TenantID) {
		writeError(w, http.StatusNotFound, "website not found")
		return
	}
	token, err := s.pub.BuildPreview(r.Context(), ai.TenantID, websiteID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "preview failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"url":   "/preview/" + token + "/",
		"token": token,
	})
}

const deploymentCols = `id, tenant_id, website_id, snapshot_id, triggered_by_user_id, status,
	github_repo, git_commit_hash, cloudflare_project_id, cloudflare_deployment_id,
	error_message, created_at, completed_at`

func scanDeployment(row interface{ Scan(...any) error }) (models.Deployment, error) {
	var d models.Deployment
	err := row.Scan(&d.ID, &d.TenantID, &d.WebsiteID, &d.SnapshotID, &d.TriggeredByUserID,
		&d.Status, &d.GitHubRepo, &d.GitCommitHash, &d.CloudflareProjectID,
		&d.CloudflareDeploymentID, &d.ErrorMessage, &d.CreatedAt, &d.CompletedAt)
	return d, err
}

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT `+deploymentCols+` FROM deployments
		WHERE website_id = $1 AND tenant_id = $2
		ORDER BY created_at DESC LIMIT 50`, r.PathValue("websiteID"), ai.TenantID)
	if err != nil {
		serverError(w, "list deployments", err)
		return
	}
	defer rows.Close()
	deployments := []models.Deployment{}
	for rows.Next() {
		d, err := scanDeployment(rows)
		if err != nil {
			serverError(w, "scan deployment", err)
			return
		}
		deployments = append(deployments, d)
	}
	writeJSON(w, http.StatusOK, deployments)
}

func (s *Server) handleGetDeployment(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	d, err := scanDeployment(s.pool.QueryRow(r.Context(), `
		SELECT `+deploymentCols+` FROM deployments WHERE id = $1 AND tenant_id = $2`,
		r.PathValue("deploymentID"), ai.TenantID))
	if err != nil {
		serverError(w, "get deployment", err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// handleRollback republishes the content snapshot of a previous successful
// deployment (spec §12.9).
func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var websiteID, snapshotID, status string
	err := s.pool.QueryRow(r.Context(), `
		SELECT website_id, snapshot_id, status FROM deployments
		WHERE id = $1 AND tenant_id = $2`,
		r.PathValue("deploymentID"), ai.TenantID).Scan(&websiteID, &snapshotID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "deployment not found")
		return
	}
	if err != nil {
		serverError(w, "load deployment", err)
		return
	}
	if status != models.DeploySucceeded {
		writeError(w, http.StatusBadRequest, "can only roll back to a successful deployment")
		return
	}

	d, err := s.pub.StartPublish(r.Context(), ai.TenantID, websiteID, snapshotID, ai.UserID)
	if errors.Is(err, publish.ErrPublishInProgress) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		serverError(w, "start rollback", err)
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "deployment.rollback", "deployment", d.ID,
		map[string]any{"rolled_back_to": r.PathValue("deploymentID")})
	writeJSON(w, http.StatusAccepted, d)
}

func (s *Server) handleListAuditLogs(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT a.id, a.tenant_id, a.user_id, a.action, a.entity_type, a.entity_id,
			a.metadata_json, a.created_at, COALESCE(u.email, '')
		FROM audit_logs a LEFT JOIN users u ON u.id = a.user_id
		WHERE a.tenant_id = $1 ORDER BY a.created_at DESC LIMIT 200`, ai.TenantID)
	if err != nil {
		serverError(w, "list audit logs", err)
		return
	}
	defer rows.Close()
	logs := []models.AuditLog{}
	for rows.Next() {
		var l models.AuditLog
		if err := rows.Scan(&l.ID, &l.TenantID, &l.UserID, &l.Action, &l.EntityType,
			&l.EntityID, &l.Metadata, &l.CreatedAt, &l.UserEmail); err != nil {
			serverError(w, "scan audit log", err)
			return
		}
		logs = append(logs, l)
	}
	writeJSON(w, http.StatusOK, logs)
}
