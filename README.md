# CMS — AI-Ready Headless CMS & Static Site Builder

A lightweight, multi-tenant headless CMS that stores structured website
content in PostgreSQL, generates static sites with a built-in Go generator,
pushes them to one GitHub repository per website, and lets Cloudflare Pages
host them. Built to the requirements in [spec.md](spec.md).

Content is fully schema-driven: each tenant owns an editable library of
**block types** (custom fields + layout hints), seeded from a starter set.
Pages are composed in a **drag-and-drop block builder** (palette → canvas →
inspector, with live preview), and the static generator auto-renders any
block type from its schema — no per-type templates, no user HTML.

## Quick start

```bash
docker compose up -d          # PostgreSQL (port 5433) + Redis (port 6380)
go run ./cmd/server           # migrates the DB, bootstraps the admin, serves :8080
```

Open http://localhost:8080/admin/ and log in. On first run a platform admin
is created from `ADMIN_EMAIL` / `ADMIN_PASSWORD` (defaults: `admin@cms.local`
and a generated password printed to the server log).

## Configuration (.env or environment)

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | HTTP port |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Used for local media URLs |
| `DATABASE_URL` | `postgres://cms:cms@localhost:5433/cms?sslmode=disable` | PostgreSQL (source of truth) |
| `REDIS_ADDR` | _(empty → in-memory)_ | Redis for cache/locks/rate limits |
| `JWT_SECRET` | _(generated, persisted in `var/`)_ | Access-token signing key |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | `admin@cms.local` / _(generated)_ | First platform admin |
| `S3_BUCKET`, `S3_REGION`, `S3_ENDPOINT` | _(empty → local disk)_ | S3-compatible media storage |
| `AWS_ACCESS_KEY`, `AWS_SECRET_KEY` | — | S3 credentials |
| `GITHUB_TOKEN`, `GITHUB_OWNER` | _(empty → local-only publish)_ | Push generated sites to GitHub |
| `CLOUDFLARE_API_KEY`, `CLOUDFLARE_ACCOUNT_ID` | — | Poll Pages deployment status |

Integrations degrade gracefully: with nothing configured, publishing builds
the site locally and serves it at `/sites/{websiteID}/`.

## How publishing works (spec §17)

1. Tenant admin clicks **Publish** in the dashboard.
2. Backend validates content and stores an immutable **content snapshot**.
3. A background worker (guarded by a per-website deployment lock) runs the
   Go static generator: pages, sections, sitemap.xml, robots.txt.
4. If GitHub is configured, the output is committed to the website's
   repository (`cms-site-<name>-<id>`, auto-created private on first publish).
5. If the repo is connected to a Cloudflare Pages project (one-time manual
   step; set the project name in Website → Settings → Deployment), the worker
   waits for the Pages deployment and records its ID and status.
6. Deployment status/history is visible in the dashboard; any previous
   successful deployment can be **rolled back** (its snapshot is republished).

A failed build never touches the previously published site.

## Tenant onboarding flow (spec §16.2)

1. Platform admin creates the tenant and website, invites the client users.
2. Content is edited (pages, sections, media) and previewed (`Preview` builds
   a temporary site under `/preview/{token}/`, expires in 30 min).
3. First publish creates the GitHub repo.
4. In Cloudflare: create a Pages project connected to that repo (build
   command: none, output dir: `/`), then put the project name in Website
   Settings → Deployment.
5. Add the custom domain to the Pages project and point DNS at Cloudflare.

## Roles

- **Platform admin** — everything, all tenants (flag on user).
- **Tenant admin** — manage one tenant's content, users, settings, publish.
- **Editor** — edit pages/sections, upload media, preview.
- **Viewer** — read-only.

Tenant isolation is enforced in middleware (membership + role check) and in
every SQL query (`tenant_id` filter).

## Layout

```
cmd/server/          entrypoint, admin bootstrap
internal/config/     env config (.env supported)
internal/db/         pgx pool + embedded SQL migrations
internal/models/     entities + block schema validation + starter library
internal/httpapi/    REST API handlers + auth middleware
internal/auth/       JWT + bcrypt + refresh tokens
internal/cache/      Redis or in-memory (locks, rate limits, preview tokens)
internal/storage/    S3-compatible or local-disk media storage
internal/generator/  Go static site generator (embedded templates)
internal/publish/    snapshots, publish worker, GitHub push, Cloudflare polling
internal/audit/      audit log writer
web/admin/           form-based admin dashboard (vanilla JS SPA)
```
