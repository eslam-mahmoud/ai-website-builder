package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/eslam/cms/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const maxUploadSize = 10 << 20 // 10 MiB

// allowedUploads maps permitted MIME types to their canonical extension.
var allowedUploads = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/gif":       ".gif",
	"image/webp":      ".webp",
	"image/svg+xml":   ".svg",
	"application/pdf": ".pdf",
}

const mediaCols = `id, tenant_id, file_name, file_type, file_size, storage_key,
	public_url, alt_text, created_at, updated_at`

func scanMedia(row interface{ Scan(...any) error }) (models.Media, error) {
	var m models.Media
	err := row.Scan(&m.ID, &m.TenantID, &m.FileName, &m.FileType, &m.FileSize,
		&m.StorageKey, &m.PublicURL, &m.AltText, &m.CreatedAt, &m.UpdatedAt)
	return m, err
}

func (s *Server) handleListMedia(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT `+mediaCols+` FROM media WHERE tenant_id = $1 ORDER BY created_at DESC`,
		ai.TenantID)
	if err != nil {
		serverError(w, "list media", err)
		return
	}
	defer rows.Close()
	items := []models.Media{}
	for rows.Next() {
		m, err := scanMedia(rows)
		if err != nil {
			serverError(w, "scan media", err)
			return
		}
		items = append(items, m)
	}
	writeJSON(w, http.StatusOK, items)
}

// handleUploadMedia accepts a multipart "file" field (plus optional
// "alt_text"), validates type and size, and stores it under the tenant's key
// prefix.
func (s *Server) handleUploadMedia(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize+4096)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "upload too large or malformed (max 10 MB)")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, `multipart field "file" is required`)
		return
	}
	defer file.Close()

	// Sniff the real content type rather than trusting the client header.
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		serverError(w, "read upload", err)
		return
	}
	head = head[:n]
	contentType := http.DetectContentType(head)
	if strings.HasPrefix(contentType, "text/xml") || strings.HasPrefix(contentType, "text/plain") {
		// SVG sniffs as XML/plain text; fall back to the extension.
		if strings.EqualFold(filepath.Ext(header.Filename), ".svg") {
			contentType = "image/svg+xml"
		}
	}
	contentType = strings.Split(contentType, ";")[0]
	ext, ok := allowedUploads[contentType]
	if !ok {
		writeError(w, http.StatusBadRequest,
			"unsupported file type (allowed: jpeg, png, gif, webp, svg, pdf)")
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		serverError(w, "rewind upload", err)
		return
	}

	key := fmt.Sprintf("%s/%s%s", ai.TenantID, uuid.NewString(), ext)
	publicURL, err := s.store.Put(r.Context(), key, contentType, file, header.Size)
	if err != nil {
		serverError(w, "store upload", err)
		return
	}

	m, err := scanMedia(s.pool.QueryRow(r.Context(), `
		INSERT INTO media (tenant_id, file_name, file_type, file_size, storage_key, public_url, alt_text)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+mediaCols,
		ai.TenantID, filepath.Base(header.Filename), contentType, header.Size, key,
		publicURL, r.FormValue("alt_text")))
	if err != nil {
		_ = s.store.Delete(r.Context(), key)
		serverError(w, "save media metadata", err)
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "media.uploaded", "media", m.ID,
		map[string]any{"file_name": m.FileName, "size": m.FileSize})
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) handleUpdateMedia(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var req struct {
		AltText *string `json:"alt_text"`
	}
	if err := readJSON(r, &req); err != nil || req.AltText == nil {
		writeError(w, http.StatusBadRequest, "alt_text is required")
		return
	}
	m, err := scanMedia(s.pool.QueryRow(r.Context(), `
		UPDATE media SET alt_text = $1, updated_at = now()
		WHERE id = $2 AND tenant_id = $3 RETURNING `+mediaCols,
		*req.AltText, r.PathValue("mediaID"), ai.TenantID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}
	if err != nil {
		serverError(w, "update media", err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleDeleteMedia(w http.ResponseWriter, r *http.Request, ai *authInfo) {
	var key string
	err := s.pool.QueryRow(r.Context(), `
		DELETE FROM media WHERE id = $1 AND tenant_id = $2 RETURNING storage_key`,
		r.PathValue("mediaID"), ai.TenantID).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}
	if err != nil {
		serverError(w, "delete media", err)
		return
	}
	if err := s.store.Delete(r.Context(), key); err != nil {
		// Metadata is gone; log the orphaned object but don't fail the request.
		serverError(w, "delete stored file (metadata already removed)", err)
		return
	}
	s.audit.Record(r.Context(), ai.TenantID, ai.UserID, "media.deleted", "media",
		r.PathValue("mediaID"), nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
