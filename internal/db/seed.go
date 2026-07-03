package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/eslam/cms/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SeedSectionTypes inserts the starter block type library for a tenant.
// Existing type_keys are left untouched, so re-running is safe.
func SeedSectionTypes(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	for _, st := range models.StarterSectionTypes {
		fields, err := json.Marshal(st.Fields)
		if err != nil {
			return err
		}
		layout, err := json.Marshal(st.Layout)
		if err != nil {
			return err
		}
		_, err = pool.Exec(ctx, `
			INSERT INTO section_types (tenant_id, type_key, label, icon, fields_json, layout_json)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (tenant_id, type_key) DO NOTHING`,
			tenantID, st.TypeKey, st.Label, st.Icon, fields, layout)
		if err != nil {
			return fmt.Errorf("seed section type %s: %w", st.TypeKey, err)
		}
	}
	return nil
}

// BackfillSectionTypes seeds the starter library into any tenant that has no
// block types yet (e.g. tenants created before the section_types table).
func BackfillSectionTypes(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `
		SELECT t.id FROM tenants t
		WHERE NOT EXISTS (SELECT 1 FROM section_types st WHERE st.tenant_id = t.id)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := SeedSectionTypes(ctx, pool, id); err != nil {
			return err
		}
	}
	return nil
}
