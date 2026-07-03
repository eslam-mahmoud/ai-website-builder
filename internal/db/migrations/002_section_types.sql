-- Per-tenant, user-editable block type schemas. sections.section_type keeps
-- referencing type_key, so existing section rows remain valid.
CREATE TABLE section_types (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type_key    text NOT NULL,
    label       text NOT NULL,
    icon        text NOT NULL DEFAULT '',
    fields_json jsonb NOT NULL DEFAULT '[]',
    layout_json jsonb NOT NULL DEFAULT '{}',
    status      text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, type_key)
);
CREATE INDEX section_types_tenant_idx ON section_types (tenant_id);

-- The starter library replaces the button_text/button_link field pair with a
-- single "button" field of shape {text, link}; rewrite existing content.
UPDATE sections
SET content_json = (content_json - 'button_text' - 'button_link')
    || jsonb_build_object('button', jsonb_build_object(
        'text', COALESCE(content_json->>'button_text', ''),
        'link', COALESCE(content_json->>'button_link', '')))
WHERE section_type IN ('hero', 'cta')
  AND (content_json ? 'button_text' OR content_json ? 'button_link');
