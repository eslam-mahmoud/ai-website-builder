// Package publish orchestrates the publishing workflow (spec §17):
// snapshot → static build → GitHub push → Cloudflare Pages deployment →
// status tracking. A per-website lock prevents concurrent publishes, and a
// failed build never touches the previously published site.
package publish

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/eslam/cms/internal/audit"
	"github.com/eslam/cms/internal/cache"
	"github.com/eslam/cms/internal/config"
	"github.com/eslam/cms/internal/generator"
	"github.com/eslam/cms/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	lockTTL        = 15 * time.Minute
	publishTimeout = 10 * time.Minute
	previewTTL     = 30 * time.Minute
)

var ErrPublishInProgress = errors.New("a publish is already running for this website")

type Publisher struct {
	pool  *pgxpool.Pool
	cache cache.Cache
	cfg   *config.Config
	audit *audit.Logger
}

func New(pool *pgxpool.Pool, c cache.Cache, cfg *config.Config, a *audit.Logger) *Publisher {
	return &Publisher{pool: pool, cache: c, cfg: cfg, audit: a}
}

// StartPublish creates a deployment for the given snapshot and runs the
// publishing pipeline in the background. It fails fast if another publish
// holds the website's deployment lock.
func (p *Publisher) StartPublish(ctx context.Context, tenantID, websiteID, snapshotID, userID string) (*models.Deployment, error) {
	lockKey := "deploylock:" + websiteID
	if !p.cache.SetNX(ctx, lockKey, snapshotID, lockTTL) {
		return nil, ErrPublishInProgress
	}

	var d models.Deployment
	err := p.pool.QueryRow(ctx, `
		INSERT INTO deployments (tenant_id, website_id, snapshot_id, triggered_by_user_id, status)
		VALUES ($1, $2, $3, NULLIF($4, '')::uuid, 'queued')
		RETURNING id, tenant_id, website_id, snapshot_id, status, created_at`,
		tenantID, websiteID, snapshotID, userID).
		Scan(&d.ID, &d.TenantID, &d.WebsiteID, &d.SnapshotID, &d.Status, &d.CreatedAt)
	if err != nil {
		p.cache.Delete(ctx, lockKey)
		return nil, err
	}

	go func() {
		defer p.cache.Delete(context.Background(), lockKey)
		bg, cancel := context.WithTimeout(context.Background(), publishTimeout)
		defer cancel()
		if err := p.run(bg, &d); err != nil {
			p.fail(bg, &d, err)
		}
	}()
	return &d, nil
}

// run executes one deployment. Any returned error marks it failed.
func (p *Publisher) run(ctx context.Context, d *models.Deployment) error {
	defer func() {
		if r := recover(); r != nil {
			p.fail(ctx, d, fmt.Errorf("publish panicked: %v", r))
		}
	}()

	p.setStatus(ctx, d.ID, models.DeployBuilding)

	var snapJSON []byte
	if err := p.pool.QueryRow(ctx,
		`SELECT snapshot_json FROM content_snapshots WHERE id = $1 AND tenant_id = $2`,
		d.SnapshotID, d.TenantID).Scan(&snapJSON); err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}
	var snap models.Snapshot
	if err := json.Unmarshal(snapJSON, &snap); err != nil {
		return fmt.Errorf("parse snapshot: %w", err)
	}

	// Build into a temp dir first so a failed build never disturbs the
	// last good local artifact.
	buildDir, err := os.MkdirTemp("", "cms-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(buildDir)
	if err := generator.Generate(&snap, buildDir, ""); err != nil {
		return err
	}

	// Keep the latest successful build on disk, served at /sites/{id}/.
	// Regenerated with that base path so links work under the sub-path
	// (the buildDir output keeps root-relative links for real hosting).
	siteDir := filepath.Join(p.cfg.DataDir, "sites", d.WebsiteID)
	if err := generator.Generate(&snap, siteDir, "/sites/"+d.WebsiteID); err != nil {
		return err
	}

	if p.cfg.GitHubToken != "" && p.cfg.GitHubOwner != "" {
		if err := p.deployToGitHub(ctx, d, &snap, buildDir); err != nil {
			return err
		}
	} else {
		log.Printf("publish %s: GitHub not configured (set GITHUB_TOKEN and GITHUB_OWNER); site available locally at /sites/%s/", d.ID, d.WebsiteID)
	}

	_, err = p.pool.Exec(ctx, `
		UPDATE deployments SET status = 'succeeded', completed_at = now() WHERE id = $1`, d.ID)
	if err != nil {
		return err
	}
	p.audit.Record(ctx, d.TenantID, deref(d.TriggeredByUserID), "deployment.succeeded",
		"deployment", d.ID, map[string]any{"website_id": d.WebsiteID})
	return nil
}

// deployToGitHub pushes the built site to the website's repository (creating
// it on first publish) and, when Cloudflare is configured with a Pages
// project, waits for the resulting Pages deployment.
func (p *Publisher) deployToGitHub(ctx context.Context, d *models.Deployment, snap *models.Snapshot, buildDir string) error {
	p.setStatus(ctx, d.ID, models.DeployDeploying)

	repo := snap.Website.Settings.GitHubRepo
	if repo == "" {
		repo = p.cfg.GitHubOwner + "/" + repoName(snap.Website.Name, d.WebsiteID)
		if err := p.saveWebsiteSetting(ctx, d.TenantID, d.WebsiteID, "github_repo", repo); err != nil {
			return err
		}
	}
	owner, name, _ := strings.Cut(repo, "/")

	gh := newGitHubClient(p.cfg.GitHubToken)
	if err := gh.EnsureRepo(ctx, owner, name); err != nil {
		return err
	}
	commit, err := gh.Push(ctx, repo, buildDir, "Publish "+time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	if _, err := p.pool.Exec(ctx,
		`UPDATE deployments SET github_repo = $1, git_commit_hash = $2 WHERE id = $3`,
		repo, commit, d.ID); err != nil {
		return err
	}

	project := snap.Website.Settings.CloudflareProj
	if p.cfg.CloudflareAPIKey != "" && p.cfg.CloudflareAccountID != "" && project != "" {
		cf := newCloudflareClient(p.cfg.CloudflareAPIKey, p.cfg.CloudflareAccountID)
		cfID, err := cf.AwaitDeployment(ctx, project, commit, 3*time.Minute)
		if cfID != "" {
			_, _ = p.pool.Exec(ctx, `
				UPDATE deployments SET cloudflare_project_id = $1, cloudflare_deployment_id = $2
				WHERE id = $3`, project, cfID, d.ID)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// BuildPreview generates a temporary preview build of the current draft
// content and returns a token; the site is served at /preview/{token}/ until
// the token expires.
func (p *Publisher) BuildPreview(ctx context.Context, tenantID, websiteID string) (string, error) {
	snap, err := BuildSnapshot(ctx, p.pool, tenantID, websiteID)
	if err != nil {
		return "", err
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	dir := filepath.Join(p.cfg.DataDir, "previews", token)
	if err := generator.Generate(snap, dir, "/preview/"+token); err != nil {
		return "", err
	}
	p.cache.Set(ctx, "preview:"+token, websiteID, previewTTL)
	return token, nil
}

// PreviewValid reports whether a preview token is still live.
func (p *Publisher) PreviewValid(ctx context.Context, token string) bool {
	_, ok := p.cache.Get(ctx, "preview:"+token)
	return ok
}

func (p *Publisher) setStatus(ctx context.Context, id, status string) {
	if _, err := p.pool.Exec(ctx,
		`UPDATE deployments SET status = $1 WHERE id = $2`, status, id); err != nil {
		log.Printf("publish %s: update status: %v", id, err)
	}
}

func (p *Publisher) fail(ctx context.Context, d *models.Deployment, cause error) {
	log.Printf("publish %s failed: %v", d.ID, cause)
	if _, err := p.pool.Exec(ctx, `
		UPDATE deployments SET status = 'failed', error_message = $1, completed_at = now()
		WHERE id = $2`, cause.Error(), d.ID); err != nil {
		log.Printf("publish %s: record failure: %v", d.ID, err)
	}
	p.audit.Record(ctx, d.TenantID, deref(d.TriggeredByUserID), "deployment.failed",
		"deployment", d.ID, map[string]any{"error": cause.Error()})
}

func (p *Publisher) saveWebsiteSetting(ctx context.Context, tenantID, websiteID, key, value string) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE websites SET settings_json = settings_json || jsonb_build_object($1::text, $2::text),
		updated_at = now() WHERE id = $3 AND tenant_id = $4`, key, value, websiteID, tenantID)
	return err
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// repoName derives a stable, unique repository name for a website.
func repoName(websiteName, websiteID string) string {
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(websiteName), "-"), "-")
	if slug == "" {
		slug = "site"
	}
	if len(slug) > 40 {
		slug = slug[:40]
	}
	short := strings.ReplaceAll(websiteID, "-", "")
	if len(short) > 8 {
		short = short[:8]
	}
	return "cms-site-" + slug + "-" + short
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
