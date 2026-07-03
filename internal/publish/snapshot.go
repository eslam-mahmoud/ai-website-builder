package publish

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/eslam/cms/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BuildSnapshot assembles the fully-resolved content bundle for a website:
// settings, visible pages with their visible sections, the block type
// schemas those sections use, and every media file they (or the logo)
// reference.
func BuildSnapshot(ctx context.Context, pool *pgxpool.Pool, tenantID, websiteID string) (*models.Snapshot, error) {
	snap := &models.Snapshot{
		Version:      2,
		BuiltAt:      time.Now().UTC(),
		Media:        map[string]models.SnapshotMedia{},
		SectionTypes: map[string]models.SnapshotSectionType{},
	}

	// Load the tenant's active block type schemas up front; sections whose
	// type is missing are skipped at render time.
	typeRows, err := pool.Query(ctx, `
		SELECT type_key, label, fields_json, layout_json FROM section_types
		WHERE tenant_id = $1 AND status = 'active'`, tenantID)
	if err != nil {
		return nil, err
	}
	defer typeRows.Close()
	for typeRows.Next() {
		var key string
		var st models.SnapshotSectionType
		var fields, layout []byte
		if err := typeRows.Scan(&key, &st.Label, &fields, &layout); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(fields, &st.Fields); err != nil {
			return nil, fmt.Errorf("parse schema of block type %s: %w", key, err)
		}
		if err := json.Unmarshal(layout, &st.Layout); err != nil {
			return nil, fmt.Errorf("parse layout of block type %s: %w", key, err)
		}
		snap.SectionTypes[key] = st
	}
	if err := typeRows.Err(); err != nil {
		return nil, err
	}

	var settingsJSON []byte
	err = pool.QueryRow(ctx, `
		SELECT name, domain, template_id, settings_json FROM websites
		WHERE id = $1 AND tenant_id = $2`, websiteID, tenantID).
		Scan(&snap.Website.Name, &snap.Website.Domain, &snap.Website.TemplateID, &settingsJSON)
	if err != nil {
		return nil, fmt.Errorf("load website: %w", err)
	}
	if err := json.Unmarshal(settingsJSON, &snap.Website.Settings); err != nil {
		return nil, fmt.Errorf("parse website settings: %w", err)
	}

	mediaIDs := map[string]bool{}
	if id := snap.Website.Settings.LogoMediaID; id != "" {
		mediaIDs[id] = true
	}

	pageRows, err := pool.Query(ctx, `
		SELECT id, title, slug, seo_title, seo_description FROM pages
		WHERE website_id = $1 AND tenant_id = $2 AND status = 'visible'
		ORDER BY sort_order, created_at`, websiteID, tenantID)
	if err != nil {
		return nil, err
	}
	defer pageRows.Close()

	type pageRef struct {
		id   string
		page models.SnapshotPage
	}
	var refs []pageRef
	for pageRows.Next() {
		var r pageRef
		if err := pageRows.Scan(&r.id, &r.page.Title, &r.page.Slug, &r.page.SEOTitle, &r.page.SEODescription); err != nil {
			return nil, err
		}
		refs = append(refs, r)
	}
	if err := pageRows.Err(); err != nil {
		return nil, err
	}

	for i := range refs {
		rows, err := pool.Query(ctx, `
			SELECT section_type, content_json FROM sections
			WHERE page_id = $1 AND tenant_id = $2 AND status = 'visible'
			ORDER BY sort_order, created_at`, refs[i].id, tenantID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var s models.SnapshotSection
			if err := rows.Scan(&s.Type, &s.Content); err != nil {
				rows.Close()
				return nil, err
			}
			if st, ok := snap.SectionTypes[s.Type]; ok {
				for _, id := range models.CollectMediaIDs(st.Fields, s.Content) {
					mediaIDs[id] = true
				}
			}
			refs[i].page.Sections = append(refs[i].page.Sections, s)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		snap.Pages = append(snap.Pages, refs[i].page)
	}

	if len(mediaIDs) > 0 {
		ids := make([]string, 0, len(mediaIDs))
		for id := range mediaIDs {
			ids = append(ids, id)
		}
		rows, err := pool.Query(ctx, `
			SELECT id, public_url, alt_text FROM media
			WHERE tenant_id = $1 AND id = ANY($2::uuid[])`, tenantID, ids)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var m models.SnapshotMedia
			if err := rows.Scan(&id, &m.URL, &m.Alt); err != nil {
				return nil, err
			}
			snap.Media[id] = m
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return snap, nil
}
