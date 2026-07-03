CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE tenants (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    status     text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name              text NOT NULL,
    email             text NOT NULL UNIQUE,
    password_hash     text NOT NULL,
    is_platform_admin boolean NOT NULL DEFAULT false,
    status            text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tenant_users (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       text NOT NULL CHECK (role IN ('tenant_admin', 'editor', 'viewer')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id)
);

CREATE TABLE refresh_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE websites (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name          text NOT NULL,
    domain        text NOT NULL DEFAULT '',
    template_id   text NOT NULL DEFAULT 'business',
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    settings_json jsonb NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX websites_tenant_idx ON websites (tenant_id);

CREATE TABLE pages (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    website_id      uuid NOT NULL REFERENCES websites(id) ON DELETE CASCADE,
    title           text NOT NULL,
    slug            text NOT NULL,
    status          text NOT NULL DEFAULT 'visible' CHECK (status IN ('visible', 'hidden')),
    sort_order      integer NOT NULL DEFAULT 0,
    seo_title       text NOT NULL DEFAULT '',
    seo_description text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (website_id, slug)
);
CREATE INDEX pages_tenant_idx ON pages (tenant_id);
CREATE INDEX pages_website_idx ON pages (website_id);

CREATE TABLE sections (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    page_id      uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    section_type text NOT NULL,
    sort_order   integer NOT NULL DEFAULT 0,
    content_json jsonb NOT NULL DEFAULT '{}',
    status       text NOT NULL DEFAULT 'visible' CHECK (status IN ('visible', 'hidden')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sections_tenant_idx ON sections (tenant_id);
CREATE INDEX sections_page_idx ON sections (page_id);

CREATE TABLE media (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    file_name   text NOT NULL,
    file_type   text NOT NULL,
    file_size   bigint NOT NULL,
    storage_key text NOT NULL,
    public_url  text NOT NULL,
    alt_text    text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX media_tenant_idx ON media (tenant_id);

CREATE TABLE content_snapshots (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    website_id         uuid NOT NULL REFERENCES websites(id) ON DELETE CASCADE,
    snapshot_json      jsonb NOT NULL,
    created_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX content_snapshots_website_idx ON content_snapshots (website_id);

CREATE TABLE deployments (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    website_id               uuid NOT NULL REFERENCES websites(id) ON DELETE CASCADE,
    snapshot_id              uuid NOT NULL REFERENCES content_snapshots(id),
    triggered_by_user_id     uuid REFERENCES users(id) ON DELETE SET NULL,
    status                   text NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'building', 'deploying', 'succeeded', 'failed')),
    github_repo              text NOT NULL DEFAULT '',
    git_commit_hash          text NOT NULL DEFAULT '',
    cloudflare_project_id    text NOT NULL DEFAULT '',
    cloudflare_deployment_id text NOT NULL DEFAULT '',
    error_message            text NOT NULL DEFAULT '',
    created_at               timestamptz NOT NULL DEFAULT now(),
    completed_at             timestamptz
);
CREATE INDEX deployments_website_idx ON deployments (website_id, created_at DESC);

CREATE TABLE audit_logs (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid REFERENCES tenants(id) ON DELETE CASCADE,
    user_id       uuid REFERENCES users(id) ON DELETE SET NULL,
    action        text NOT NULL,
    entity_type   text NOT NULL DEFAULT '',
    entity_id     text NOT NULL DEFAULT '',
    metadata_json jsonb NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_tenant_idx ON audit_logs (tenant_id, created_at DESC);
