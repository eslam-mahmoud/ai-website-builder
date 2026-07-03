// Package audit records important system activity in the audit_logs table.
// Failures are logged but never block the action being audited.
package audit

import (
	"context"
	"encoding/json"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Logger struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Logger { return &Logger{pool: pool} }

// Record writes an audit entry. tenantID and userID may be empty.
func (l *Logger) Record(ctx context.Context, tenantID, userID, action, entityType, entityID string, metadata map[string]any) {
	meta := []byte("{}")
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			meta = b
		}
	}
	_, err := l.pool.Exec(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, entity_type, entity_id, metadata_json)
		VALUES (NULLIF($1, '')::uuid, NULLIF($2, '')::uuid, $3, $4, $5, $6)`,
		tenantID, userID, action, entityType, entityID, meta)
	if err != nil {
		log.Printf("audit: failed to record %s: %v", action, err)
	}
}
