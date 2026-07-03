package models

import (
	"encoding/json"
	"time"
)

// Roles within a tenant, ordered by privilege.
const (
	RoleViewer      = "viewer"
	RoleEditor      = "editor"
	RoleTenantAdmin = "tenant_admin"
)

// roleRank orders tenant roles for permission checks.
var roleRank = map[string]int{RoleViewer: 1, RoleEditor: 2, RoleTenantAdmin: 3}

// RoleAtLeast reports whether role has at least the privilege of min.
func RoleAtLeast(role, min string) bool { return roleRank[role] >= roleRank[min] }

// ValidRole reports whether role is a known tenant role.
func ValidRole(role string) bool { _, ok := roleRank[role]; return ok }

type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type User struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Email           string    `json:"email"`
	PasswordHash    string    `json:"-"`
	IsPlatformAdmin bool      `json:"is_platform_admin"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type TenantUser struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	// Joined user fields for listings.
	UserName  string `json:"user_name,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
}

// WebsiteSettings is the structured shape of websites.settings_json.
type WebsiteSettings struct {
	LogoMediaID    string            `json:"logo_media_id,omitempty"`
	PrimaryColor   string            `json:"primary_color,omitempty"`
	ContactPhone   string            `json:"contact_phone,omitempty"`
	ContactEmail   string            `json:"contact_email,omitempty"`
	WhatsappNumber string            `json:"whatsapp_number,omitempty"`
	Address        string            `json:"address,omitempty"`
	SocialLinks    map[string]string `json:"social_links,omitempty"`
	FooterText     string            `json:"footer_text,omitempty"`
	SEOTitle       string            `json:"seo_title,omitempty"`
	SEODescription string            `json:"seo_description,omitempty"`
	GitHubRepo     string            `json:"github_repo,omitempty"`     // owner/name
	CloudflareProj string            `json:"cloudflare_project,omitempty"` // Pages project name
}

type Website struct {
	ID         string          `json:"id"`
	TenantID   string          `json:"tenant_id"`
	Name       string          `json:"name"`
	Domain     string          `json:"domain"`
	TemplateID string          `json:"template_id"`
	Status     string          `json:"status"`
	Settings   json.RawMessage `json:"settings"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

type Page struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	WebsiteID      string    `json:"website_id"`
	Title          string    `json:"title"`
	Slug           string    `json:"slug"`
	Status         string    `json:"status"`
	SortOrder      int       `json:"sort_order"`
	SEOTitle       string    `json:"seo_title"`
	SEODescription string    `json:"seo_description"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Section struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	PageID      string          `json:"page_id"`
	SectionType string          `json:"section_type"`
	SortOrder   int             `json:"sort_order"`
	Content     json.RawMessage `json:"content"`
	Status      string          `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type Media struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	FileName   string    `json:"file_name"`
	FileType   string    `json:"file_type"`
	FileSize   int64     `json:"file_size"`
	StorageKey string    `json:"storage_key"`
	PublicURL  string    `json:"public_url"`
	AltText    string    `json:"alt_text"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ContentSnapshot struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenant_id"`
	WebsiteID       string          `json:"website_id"`
	SnapshotJSON    json.RawMessage `json:"snapshot_json,omitempty"`
	CreatedByUserID *string         `json:"created_by_user_id"`
	CreatedAt       time.Time       `json:"created_at"`
}

// Deployment statuses.
const (
	DeployQueued    = "queued"
	DeployBuilding  = "building"
	DeployDeploying = "deploying"
	DeploySucceeded = "succeeded"
	DeployFailed    = "failed"
)

type Deployment struct {
	ID                     string     `json:"id"`
	TenantID               string     `json:"tenant_id"`
	WebsiteID              string     `json:"website_id"`
	SnapshotID             string     `json:"snapshot_id"`
	TriggeredByUserID      *string    `json:"triggered_by_user_id"`
	Status                 string     `json:"status"`
	GitHubRepo             string     `json:"github_repo"`
	GitCommitHash          string     `json:"git_commit_hash"`
	CloudflareProjectID    string     `json:"cloudflare_project_id"`
	CloudflareDeploymentID string     `json:"cloudflare_deployment_id"`
	ErrorMessage           string     `json:"error_message"`
	CreatedAt              time.Time  `json:"created_at"`
	CompletedAt            *time.Time `json:"completed_at"`
}

type AuditLog struct {
	ID         string          `json:"id"`
	TenantID   *string         `json:"tenant_id"`
	UserID     *string         `json:"user_id"`
	Action     string          `json:"action"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	Metadata   json.RawMessage `json:"metadata"`
	CreatedAt  time.Time       `json:"created_at"`
	UserEmail  string          `json:"user_email,omitempty"`
}

// Snapshot is the fully-resolved content bundle handed to the static site
// generator and stored in content_snapshots. It embeds the block type
// schemas in use at publish time, so builds (and rollbacks) render exactly
// what the schema looked like then, and the generator needs no DB access.
type Snapshot struct {
	Website      SnapshotWebsite                `json:"website"`
	Pages        []SnapshotPage                 `json:"pages"`
	Media        map[string]SnapshotMedia       `json:"media"`
	SectionTypes map[string]SnapshotSectionType `json:"section_types"`
	Version      int                            `json:"version"`
	BuiltAt      time.Time                      `json:"built_at"`
}

// SnapshotSectionType is the schema of one block type as of publish time.
type SnapshotSectionType struct {
	Label  string      `json:"label"`
	Fields []FieldSpec `json:"fields"`
	Layout LayoutHints `json:"layout"`
}

type SnapshotWebsite struct {
	Name       string          `json:"name"`
	Domain     string          `json:"domain"`
	TemplateID string          `json:"template_id"`
	Settings   WebsiteSettings `json:"settings"`
}

type SnapshotPage struct {
	Title          string            `json:"title"`
	Slug           string            `json:"slug"`
	SEOTitle       string            `json:"seo_title"`
	SEODescription string            `json:"seo_description"`
	Sections       []SnapshotSection `json:"sections"`
}

type SnapshotSection struct {
	Type    string          `json:"type"`
	Content json.RawMessage `json:"content"`
}

type SnapshotMedia struct {
	URL string `json:"url"`
	Alt string `json:"alt"`
}
