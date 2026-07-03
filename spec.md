# Business Requirements Document

## AI-Assisted Headless CMS & Static Site Builder

## 1. Executive Summary

The goal of this project is to build a lightweight, multi-tenant headless CMS that allows small businesses, freelancers, and service providers to manage website content without relying on WordPress or traditional page builders.

The system will store structured content in a backend CMS, generate static websites from that content, host the generated websites through Cloudflare Pages, and use Cloudflare caching/CDN for fast delivery.

The first use case is a simple business website: homepage, about page, services page, contact page, images, footer, phone number, social links, and basic SEO metadata.

The long-term opportunity is to package this as a SaaS or managed service where non-technical users can update their site through a simple admin interface, and later through an AI assistant that modifies website content safely.

---

## 2. Business Objective

The objective is to create a stable and practical alternative to WordPress for simple websites.

The system should allow the service provider to:

* Create and manage websites for multiple clients.
* Store each client’s content separately.
* Generate fast static websites.
* Deploy each client’s website to Cloudflare Pages.
* Connect custom domains.
* Allow clients to update content without touching code.
* Reduce manual website maintenance work.
* Build a foundation that can later support AI-assisted editing.

---

## 3. Problem Statement

Many small business websites do not need a full dynamic CMS like WordPress.

For simple websites, WordPress can introduce unnecessary complexity:

* Hosting setup.
* Plugin updates.
* Security risks.
* Database maintenance.
* Performance issues.
* Theme/plugin conflicts.
* Client confusion inside the WordPress dashboard.

At the same time, purely static websites are fast and cheap, but hard for non-technical clients to update.

This product solves the gap between both approaches:

> A static website experience for visitors, with a simple CMS experience for the owner.

---

## 4. Target Users

### 4.1 Primary Users

Small business owners who need a simple website, such as:

* Clinics
* Restaurants
* Local service providers
* Consultants
* Contractors
* Freelancers
* Agencies
* Coaches
* Small shops

### 4.2 Admin Users

The platform owner or agency operator who creates and manages websites for clients.

### 4.3 Client Users

The website owner or employee who updates basic content, such as:

* Phone number
* Business hours
* Images
* Services
* Contact information
* About section
* Page content

---

## 5. Scope

### 5.1 MVP Scope

The MVP should support:

* Multi-tenant CMS backend.
* Tenant management.
* Website content management.
* Page management.
* Section-based content editing.
* Image/file uploads.
* Static site generation.
* GitHub repository integration.
* Cloudflare Pages deployment flow.
* Cloudflare CDN/cache integration.
* Custom domain support.
* Basic user authentication.
* Role-based access control.
* Basic audit log.
* Basic SEO fields.
* Redis caching layer.
* PostgreSQL database.
* S3-compatible file storage.

### 5.2 Out of Scope for MVP

The following should not be part of the first version:

* Full drag-and-drop website builder.
* Complex visual editor.
* Plugin marketplace.
* E-commerce.
* Payments.
* Booking engine.
* Advanced analytics.
* Multi-language support.
* AI website editing.
* Real-time collaborative editing.
* Complex workflow approvals.
* Native mobile app.

These can be considered later after the core system proves useful.

---

## 6. Product Vision

The product should become a practical website operating system for simple business sites.

The long-term vision is:

> A business owner can request changes in natural language, approve them, and the system safely updates their static website.

However, the first version should not depend on AI. The first version should be stable, predictable, and useful without AI.

AI can be added later as a layer on top of the existing structured CMS.

---

## 7. Core Concept

The system will separate the website into three layers:

### 7.1 Content Layer

Stored in the headless CMS.

Examples:

* Business name
* Logo
* Hero title
* Hero description
* Services
* Images
* Contact details
* Footer links
* SEO metadata

### 7.2 Template Layer

Reusable frontend templates used to generate static websites.

Examples:

* Simple business template
* Service provider template
* Restaurant template
* Landing page template

### 7.3 Deployment Layer

Responsible for generating and publishing the static site.

The deployment flow will use:

* GitHub repository per tenant or per project
* Static site generator
* Cloudflare Pages
* Cloudflare CDN/cache
* Custom domain DNS

---

## 8. High-Level Architecture

The proposed architecture is:

* Backend API: Go
* Database: PostgreSQL
* Cache: Redis or AWS ElastiCache
* File Storage: AWS S3 or S3-compatible storage
* Static Site Hosting: Cloudflare Pages
* CDN & DNS: Cloudflare
* Source Control: GitHub
* Deployment Trigger: GitHub integration or Cloudflare Pages deployment hook
* Admin UI: Web dashboard
* Static Site Generator: Go-based generator, Astro, Next.js static export, Hugo, or similar

---

## 9. System Components

### 9.1 Admin Dashboard

The admin dashboard allows platform admins and client users to manage website content.

Main features:

* Login/logout
* Tenant selection
* Website settings
* Page management
* Section editing
* Image upload
* SEO fields
* Preview content
* Trigger publishing
* View deployment status

The dashboard should be simple and form-based in the MVP.

It does not need to be a full visual builder.

---

### 9.2 Backend API

The backend API will be built using Go.

Responsibilities:

* Authentication
* Tenant management
* User management
* Role permissions
* Content CRUD operations
* Page CRUD operations
* Media upload management
* Template configuration
* Static generation orchestration
* Deployment status tracking
* Integration with GitHub
* Integration with Cloudflare
* Cache management
* Audit logging

---

### 9.3 PostgreSQL Database

PostgreSQL will be the main system of record.

It will store:

* Tenants
* Users
* Roles
* Websites
* Pages
* Sections
* Content blocks
* Media metadata
* Deployment records
* Domain settings
* Template settings
* Audit logs

PostgreSQL is preferred because the system has structured relational data and needs strong consistency.

---

### 9.4 Redis / Caching Layer

Redis or AWS ElastiCache will be used for performance and temporary data.

Possible use cases:

* Caching tenant configuration.
* Caching published content snapshots.
* Caching API responses.
* Storing short-lived preview tokens.
* Rate limiting.
* Deployment locks.
* Background job status.
* Session storage, if needed.

Redis should not be the source of truth.

PostgreSQL remains the source of truth.

---

### 9.5 File Storage

Uploaded files should be stored in S3 or an S3-compatible storage provider.

Examples:

* Logos
* Hero images
* Gallery images
* PDFs
* Documents
* Icons

The backend should store file metadata in PostgreSQL and the actual files in S3.

Media URLs can later be transformed through Cloudflare CDN or image optimization.

---

### 9.6 Static Site Generator

The static site generator will take structured content from the CMS and generate static HTML, CSS, JavaScript, and assets.

The generator should support:

* Homepage
* Dynamic pages
* Reusable sections
* SEO metadata
* Sitemap generation
* Robots.txt
* Static asset references
* Template variables
* Build-time validation

The generator can be implemented in one of two ways:

#### Option A: Go-based Generator

Pros:

* Simple backend integration.
* Fast builds.
* Single language for backend and generator.
* Easy to control.

Cons:

* More frontend/template work may need to be custom-built.

#### Option B: Existing Static Site Framework

Examples:

* Astro
* Hugo
* Next.js static export
* Eleventy

Pros:

* Faster frontend development.
* Existing template ecosystem.
* Better frontend tooling.

Cons:

* Adds another runtime/language/toolchain.
* More deployment complexity.

For the MVP, Astro or Hugo may be practical if template quality matters.
A Go-based generator may be practical if operational simplicity matters more.

---

### 9.7 GitHub Integration

GitHub will be used to store generated website code or source templates.

There are two possible models.

#### Model A: One Repository Per Client

Each client gets a separate GitHub repository.

Pros:

* Strong isolation.
* Easier rollback per client.
* Easier Cloudflare Pages connection.
* Easier custom domain management.
* Cleaner ownership if client leaves later.

Cons:

* More repositories to manage.
* Requires automation for repo creation and updates.

#### Model B: One Monorepo for All Clients

All client sites are stored in one repository.

Pros:

* Easier to manage at small scale.
* Shared templates and tooling.
* Fewer GitHub resources.

Cons:

* Weaker isolation.
* More complex build routing.
* One repo issue can affect many clients.
* Harder to transfer ownership.

Recommended MVP approach:

> Use one repository per client for clarity, isolation, and operational safety.

---

### 9.8 Cloudflare Pages Integration

Cloudflare Pages will host the generated static websites.

Each tenant website can be deployed to Cloudflare Pages using one of the following approaches:

#### Option A: GitHub-connected Cloudflare Pages

Cloudflare Pages watches the tenant’s GitHub repository and deploys when changes are pushed.

Pros:

* Simple and reliable.
* Native Cloudflare workflow.
* Easy rollback.
* Good for static sites.

Cons:

* Requires repo setup per tenant.

#### Option B: Direct Upload Deployment

The backend builds the static site and uploads the output directly to Cloudflare Pages.

Pros:

* No GitHub repo required for generated output.
* Faster programmatic control.

Cons:

* More Cloudflare API work.
* Less transparent for debugging.

Recommended MVP approach:

> Use GitHub-connected Cloudflare Pages first because it is simpler, more transparent, and easier to debug.

---

### 9.9 Cloudflare DNS and CDN

Cloudflare will be used for:

* DNS management.
* Custom domain routing.
* CDN caching.
* SSL certificates.
* Cache invalidation.
* Performance optimization.
* Basic security features.

For early clients, the platform can manually guide domain setup.

Later, the system can automate domain onboarding using the Cloudflare API.

---

## 10. Multi-Tenant Requirements

The system must support multiple clients safely.

Each tenant should have isolated:

* Website settings
* Pages
* Media files
* Users
* Domains
* Deployments
* Audit logs
* Template configuration

The backend must ensure that users from one tenant cannot access another tenant’s data.

Tenant isolation should be enforced at the application level and database query level.

Every major table should include `tenant_id` where applicable.

---

## 11. User Roles

### 11.1 Platform Admin

Can manage all tenants and all websites.

Permissions:

* Create tenants
* Manage users
* Manage templates
* Manage deployments
* View system logs
* Configure integrations

### 11.2 Tenant Admin

Can manage one tenant’s website.

Permissions:

* Edit website content
* Upload media
* Manage pages
* Publish site
* Manage tenant users
* Update contact information

### 11.3 Content Editor

Can update content but not system settings.

Permissions:

* Edit pages
* Edit sections
* Upload images
* Preview changes

### 11.4 Viewer

Can view content and deployment status only.

---

## 12. Functional Requirements

### 12.1 Tenant Management

The system shall allow platform admins to:

* Create a new tenant.
* Update tenant details.
* Disable a tenant.
* Assign users to a tenant.
* Configure tenant domain.
* Configure tenant template.
* View tenant deployment history.

---

### 12.2 Website Management

The system shall allow each tenant to manage:

* Website name
* Logo
* Primary color
* Contact phone
* Contact email
* WhatsApp number
* Business address
* Social links
* Footer content
* SEO defaults

---

### 12.3 Page Management

The system shall allow users to:

* Create a page.
* Edit a page.
* Delete a page.
* Reorder pages.
* Set page slug.
* Set page title.
* Set SEO title.
* Set SEO description.
* Set page visibility.
* Add page sections.

Example pages:

* Home
* About
* Services
* Contact
* Gallery
* FAQ

---

### 12.4 Section Management

Pages should be built from predefined sections.

Example section types:

* Hero section
* Text section
* Image section
* Services list
* Gallery
* Testimonials
* Contact information
* Call-to-action
* FAQ
* Footer

Each section should have a structured schema.

Example hero section fields:

* Title
* Subtitle
* Button text
* Button link
* Background image
* Alignment
* Visibility

This is important because structured sections make future AI editing safer.

---

### 12.5 Media Management

The system shall allow users to:

* Upload images.
* Replace images.
* Delete images.
* Add alt text.
* View media library.
* Select uploaded media inside page sections.

The system should validate:

* File type
* File size
* Image dimensions, if needed
* Tenant ownership

---

### 12.6 Publishing Flow

The system shall support a clear publishing flow:

1. User edits content.
2. User previews content.
3. User clicks publish.
4. Backend creates a content snapshot.
5. Static site generator builds the website.
6. Generated code is committed to GitHub.
7. Cloudflare Pages deploys the website.
8. Deployment status is stored.
9. User sees success or failure.

Publishing should be explicit.

The system should not publish every small edit automatically in the MVP.

---

### 12.7 Preview Flow

The system should support previewing changes before publishing.

MVP preview options:

* Preview inside admin dashboard using draft content.
* Generate temporary preview build.
* Use a preview URL if supported by deployment flow.

Simple MVP recommendation:

> Start with dashboard preview using the selected template and draft content.

---

### 12.8 Deployment History

The system shall store deployment records.

Each deployment should include:

* Tenant ID
* Website ID
* Triggered by user
* Timestamp
* Status
* Git commit hash
* Cloudflare deployment ID
* Error message, if failed
* Published content snapshot ID

Users should be able to see the latest deployment status.

---

### 12.9 Rollback

The MVP should support basic rollback by republishing a previous content snapshot.

The system should keep historical snapshots of published content.

Rollback flow:

1. Admin selects previous successful deployment.
2. System loads the content snapshot.
3. System regenerates static site.
4. System deploys the previous version again.

---

### 12.10 Audit Log

The system should keep an audit log for important actions.

Examples:

* User login
* Page created
* Page updated
* Page deleted
* Media uploaded
* Publish triggered
* Deployment failed
* Domain updated
* User invited
* User role changed

Audit logs help with support, debugging, and client trust.

---

## 13. Non-Functional Requirements

### 13.1 Performance

The public website should be static and served through Cloudflare CDN.

Expected result:

* Fast page loads.
* Low backend dependency.
* High availability for visitors.
* Minimal hosting cost per site.

The CMS backend should respond quickly for admin users.

Redis caching may be used for frequently accessed tenant configuration and content.

---

### 13.2 Reliability

The system should avoid breaking live websites during publishing.

A failed build should not affect the currently published website.

The system should store deployment status and errors clearly.

---

### 13.3 Security

Security requirements:

* Password hashing.
* Secure authentication.
* Role-based access control.
* Tenant isolation.
* Signed upload URLs, if needed.
* File validation.
* Rate limiting.
* Protection against unauthorized publishing.
* Secure API keys for GitHub, AWS, and Cloudflare.
* Secrets stored in a secure secrets manager.

---

### 13.4 Scalability

The system should initially support a small number of tenants, but the design should allow growth.

The system should scale by separating:

* CMS backend
* Database
* Cache
* File storage
* Static deployment workers
* Public website hosting

Public site traffic should mostly be handled by Cloudflare, not the CMS backend.

---

### 13.5 Maintainability

The codebase should be modular.

Suggested backend modules:

* Auth module
* Tenant module
* Content module
* Media module
* Template module
* Publishing module
* Deployment module
* Integration module
* Audit module

---

## 14. Suggested Database Entities

### 14.1 Tenants

Stores client/company information.

Fields:

* id
* name
* status
* created_at
* updated_at

---

### 14.2 Users

Stores platform and tenant users.

Fields:

* id
* name
* email
* password_hash
* status
* created_at
* updated_at

---

### 14.3 Tenant Users

Maps users to tenants.

Fields:

* id
* tenant_id
* user_id
* role
* created_at

---

### 14.4 Websites

Stores website-level settings.

Fields:

* id
* tenant_id
* name
* domain
* template_id
* status
* settings_json
* created_at
* updated_at

---

### 14.5 Pages

Stores website pages.

Fields:

* id
* tenant_id
* website_id
* title
* slug
* status
* sort_order
* seo_title
* seo_description
* created_at
* updated_at

---

### 14.6 Sections

Stores page sections.

Fields:

* id
* tenant_id
* page_id
* section_type
* sort_order
* content_json
* status
* created_at
* updated_at

---

### 14.7 Media

Stores uploaded file metadata.

Fields:

* id
* tenant_id
* file_name
* file_type
* file_size
* storage_key
* public_url
* alt_text
* created_at
* updated_at

---

### 14.8 Deployments

Stores publishing/deployment records.

Fields:

* id
* tenant_id
* website_id
* triggered_by_user_id
* status
* github_repo
* git_commit_hash
* cloudflare_project_id
* cloudflare_deployment_id
* error_message
* created_at
* completed_at

---

### 14.9 Content Snapshots

Stores the content used for each publish event.

Fields:

* id
* tenant_id
* website_id
* snapshot_json
* created_by_user_id
* created_at

---

### 14.10 Audit Logs

Stores important system activity.

Fields:

* id
* tenant_id
* user_id
* action
* entity_type
* entity_id
* metadata_json
* created_at

---

## 15. API Requirements

The backend should expose APIs for:

### 15.1 Authentication

* Login
* Logout
* Refresh token
* Reset password
* Invite user

### 15.2 Tenant Management

* Create tenant
* Update tenant
* List tenants
* Disable tenant
* Assign user to tenant

### 15.3 Website Management

* Get website settings
* Update website settings
* Configure domain
* Configure template

### 15.4 Page Management

* Create page
* Update page
* Delete page
* List pages
* Reorder pages

### 15.5 Section Management

* Add section
* Update section
* Delete section
* Reorder sections

### 15.6 Media Management

* Upload media
* List media
* Delete media
* Update alt text

### 15.7 Publishing

* Generate preview
* Publish website
* Get deployment status
* List deployments
* Rollback deployment

---

## 16. Deployment Architecture

### 16.1 CMS Backend Deployment

The backend can be deployed on:

* AWS ECS
* AWS App Runner
* EC2
* Kubernetes, later if needed

For MVP, AWS App Runner or ECS is enough.

Suggested supporting services:

* PostgreSQL on Amazon RDS
* Redis on ElastiCache
* Files on S3
* Secrets on AWS Secrets Manager
* Logs on CloudWatch

---

### 16.2 Static Website Deployment

Each client website should be deployed through:

* GitHub repository
* Cloudflare Pages project
* Custom domain
* Cloudflare DNS

Recommended tenant flow:

1. Create tenant.
2. Create GitHub repo from template.
3. Create Cloudflare Pages project.
4. Connect repo to Cloudflare Pages.
5. Configure domain.
6. Publish first version.

---

## 17. Publishing Workflow

The publishing workflow should be handled by a background worker.

Steps:

1. User clicks publish.
2. Backend validates content.
3. Backend creates content snapshot.
4. Publishing job is created.
5. Worker fetches content snapshot.
6. Worker runs static site generator.
7. Worker commits generated site/source to tenant GitHub repo.
8. GitHub push triggers Cloudflare Pages deployment.
9. Worker or webhook updates deployment status.
10. User sees final status.

There should be a deployment lock per tenant to avoid two publishes running at the same time.

Redis can be used for deployment locks.

---

## 18. AI Layer — Future Phase

AI should not be the core dependency in the MVP.

However, the content model should be designed to support AI later.

Future AI capabilities:

* “Change the phone number.”
* “Add a new service page.”
* “Rewrite the homepage in a more professional tone.”
* “Add a FAQ section.”
* “Replace the hero image.”
* “Make the contact button point to WhatsApp.”
* “Generate SEO title and description.”

AI should not directly edit raw code in the first phase.

AI should modify structured content fields and submit changes for approval.

Recommended AI safety flow:

1. User asks for change.
2. AI converts request into structured changes.
3. System shows diff/preview.
4. User approves.
5. System saves draft.
6. User publishes.

---

## 19. MVP Success Criteria

The MVP is successful if:

* A new tenant can be created.
* A simple website can be configured.
* Pages and sections can be edited.
* Images can be uploaded.
* Static site can be generated.
* GitHub repo can be updated.
* Cloudflare Pages can deploy the site.
* Custom domain can point to the site.
* Client can update basic content without developer help.
* Failed deployments do not break the live website.
* The platform can support at least 5–10 client websites reliably.

---

## 20. Risks and Considerations

### 20.1 Building Too Much Too Early

Risk:

Trying to compete with full website builders from day one.

Mitigation:

Start with structured templates and simple content editing.

---

### 20.2 AI Editing Instability

Risk:

AI may make unexpected changes or break content.

Mitigation:

Keep AI out of the MVP. Later, AI should only modify structured CMS data with approval.

---

### 20.3 Cloudflare/GitHub Automation Complexity

Risk:

Automating repo creation, Cloudflare Pages setup, and domain setup may be complex.

Mitigation:

Start semi-manual for first clients, then automate repeated steps.

---

### 20.4 Tenant Isolation Bugs

Risk:

One client may accidentally access another client’s data.

Mitigation:

Use strong tenant scoping in all database queries and authorization checks.

---

### 20.5 Client Expectations

Risk:

Clients may expect full WordPress-like customization.

Mitigation:

Position the product clearly as a fast, simple, managed website system for small business websites.

---

## 21. Recommended MVP Roadmap

### Phase 1: Internal Tool

Goal: Build the system for yourself first.

Features:

* Create tenant
* Add website settings
* Add pages
* Add sections
* Upload media
* Generate static site manually
* Push to GitHub manually or semi-automatically
* Deploy through Cloudflare Pages

---

### Phase 2: Basic Admin CMS

Goal: Allow non-technical editing.

Features:

* Login
* Tenant dashboard
* Page editor
* Section editor
* Media library
* Publish button
* Deployment status

---

### Phase 3: Deployment Automation

Goal: Reduce manual work.

Features:

* GitHub repo creation
* GitHub commits
* Cloudflare Pages project setup
* Deployment webhooks
* Deployment history
* Rollback

---

### Phase 4: SaaS Foundation

Goal: Package as a service.

Features:

* Tenant billing status
* User invitations
* Role management
* Usage limits
* Better onboarding
* Template selection
* Domain onboarding flow

---

### Phase 5: AI Assistant

Goal: Add AI as an enhancement, not the foundation.

Features:

* AI content editing
* AI page creation
* AI SEO suggestions
* AI image alt text
* AI draft generation
* Human approval before publish

---

## 22. Recommended Technical Direction

For the first practical version:

* Backend: Go
* Database: PostgreSQL
* Cache: Redis
* File storage: S3
* Public hosting: Cloudflare Pages
* CDN/DNS: Cloudflare
* Version control: GitHub
* Tenant repo strategy: one repo per tenant
* Publishing: explicit publish button
* Generator: start with Astro or Hugo if frontend speed matters; Go generator if backend simplicity matters
* AI: defer until structured CMS and publishing flow are stable

---

## 23. Positioning

The product should not be positioned as another generic website builder.

A stronger positioning would be:

> A managed static website CMS for small businesses that want speed, simplicity, and low maintenance without WordPress complexity.

Future AI positioning:

> Update your website by asking for changes, preview them safely, and publish when ready.

---

## 24. Final Recommendation

The best practical path is to build a structured headless CMS first, not an AI website builder first.

The system should focus on:

* Stable content modeling.
* Simple admin editing.
* Static generation.
* Reliable deployment.
* Low-cost hosting.
* Tenant isolation.

Once this foundation works for real clients, AI can be added as a smart editing layer on top of the CMS.

This gives the product a better chance of becoming a reliable SaaS or managed service instead of becoming a fragile AI demo.
