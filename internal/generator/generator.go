// Package generator turns a content snapshot into a static website:
// HTML pages, stylesheet, sitemap.xml and robots.txt.
//
// Rendering is schema-driven: each block's fields are auto-rendered
// according to their field types, guided by the block type's layout hints
// (banner, cards, gallery, accordion, cta, default). No user-supplied HTML
// is ever executed; all values pass through html/template escaping.
package generator

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eslam/cms/internal/models"
)

//go:embed templates/*.tmpl assets/*
var assetsFS embed.FS

type navItem struct {
	Title string
	Href  string
}

type siteData struct {
	Name     string
	Settings models.WebsiteSettings
	LogoURL  string
	Nav      []navItem
	Year     int
	// BasePath prefixes internal links; empty for published sites, set for
	// previews served under /preview/{token}/.
	BasePath string
}

// blockField is one renderable value; Kind decides the markup.
type blockField struct {
	Kind     string // heading | title | text | caption | textarea | image | button | contact | items
	Text     string
	URL      string
	ImageURL string
	ImageAlt string
	Items    []blockItem
}

type blockItem struct {
	Summary string // accordion summary
	Fields  []blockField
}

// blockData is the fully-resolved view model of one section.
type blockData struct {
	Variant  string // default | banner | cards | gallery | accordion | cta
	Align    string
	BG       string // background hint: "" | alt | primary
	BGImage  string // banner background image URL
	Fields   []blockField
	HasItems bool
	Site     siteData
}

type pageData struct {
	Site     siteData
	Title    string
	SEOTitle string
	SEODesc  string
	Canon    string
	Sections []blockData
}

// Generate validates the snapshot and writes the static site to outDir.
// outDir is created if needed; existing contents are removed first so the
// output always exactly reflects the snapshot. basePath is "" for published
// sites; previews pass their mount path so internal links resolve.
func Generate(snap *models.Snapshot, outDir, basePath string) error {
	if len(snap.Pages) == 0 {
		return errors.New("build validation: website has no visible pages")
	}

	media := func(id string) (string, string) {
		if m, ok := snap.Media[id]; ok {
			return m.URL, m.Alt
		}
		return "", ""
	}
	funcs := template.FuncMap{
		"safeURL": func(s string) template.URL { return template.URL(s) },
		// safeHref admits user-entered link values with a safe scheme
		// (html/template alone would also reject legitimate tel: links).
		"safeHref": func(s string) template.URL {
			l := strings.ToLower(strings.TrimSpace(s))
			for _, p := range []string{"http://", "https://", "mailto:", "tel:", "/", "#", "./", "../"} {
				if strings.HasPrefix(l, p) {
					return template.URL(strings.TrimSpace(s))
				}
			}
			return "#"
		},
		"telURL": func(s string) template.URL {
			cleaned := strings.Map(func(r rune) rune {
				if r == '+' || (r >= '0' && r <= '9') {
					return r
				}
				return -1
			}, s)
			return template.URL("tel:" + cleaned)
		},
		"nl2br": func(s string) template.HTML {
			return template.HTML(strings.ReplaceAll(template.HTMLEscapeString(s), "\n", "<br>"))
		},
	}
	tmpl, err := template.New("").Funcs(funcs).ParseFS(assetsFS, "templates/*.tmpl")
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}

	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	logoURL, _ := media(snap.Website.Settings.LogoMediaID)
	site := siteData{
		Name:     snap.Website.Name,
		Settings: snap.Website.Settings,
		LogoURL:  logoURL,
		Year:     time.Now().Year(),
		BasePath: strings.TrimRight(basePath, "/"),
	}
	if site.Settings.PrimaryColor == "" {
		site.Settings.PrimaryColor = "#2563eb"
	}
	for _, p := range snap.Pages {
		site.Nav = append(site.Nav, navItem{Title: p.Title, Href: site.BasePath + pageHref(p.Slug)})
	}

	for _, p := range snap.Pages {
		pd := pageData{
			Site:     site,
			Title:    p.Title,
			SEOTitle: firstNonEmpty(p.SEOTitle, p.Title+" — "+site.Name, site.Settings.SEOTitle),
			SEODesc:  firstNonEmpty(p.SEODescription, site.Settings.SEODescription),
			Canon:    canonicalURL(snap.Website.Domain, p.Slug),
		}
		for _, s := range p.Sections {
			schema, ok := snap.SectionTypes[s.Type]
			if !ok {
				// Type removed since the section was created: skip rather
				// than fail the whole build.
				continue
			}
			block, err := buildBlock(schema, s.Content, site, media)
			if err != nil {
				return fmt.Errorf("build validation: page %q, %s block: %w", p.Title, s.Type, err)
			}
			pd.Sections = append(pd.Sections, block)
		}

		path := filepath.Join(outDir, pagePath(p.Slug))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		if err := tmpl.ExecuteTemplate(f, "base", pd); err != nil {
			f.Close()
			return fmt.Errorf("render page %q: %w", p.Title, err)
		}
		f.Close()
	}

	css, err := assetsFS.ReadFile("assets/styles.css")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "styles.css"), css, 0o644); err != nil {
		return err
	}
	if err := writeSitemap(snap, outDir); err != nil {
		return err
	}
	robots := "User-agent: *\nAllow: /\n"
	if snap.Website.Domain != "" {
		robots += "Sitemap: https://" + snap.Website.Domain + "/sitemap.xml\n"
	}
	return os.WriteFile(filepath.Join(outDir, "robots.txt"), []byte(robots), 0o644)
}

// buildBlock resolves one section's content against its schema into a
// renderable view model.
func buildBlock(schema models.SnapshotSectionType, content json.RawMessage,
	site siteData, media func(string) (string, string)) (blockData, error) {

	var data map[string]any
	if len(content) > 0 {
		if err := json.Unmarshal(content, &data); err != nil {
			return blockData{}, fmt.Errorf("invalid content: %w", err)
		}
	}

	b := blockData{
		Variant: firstNonEmpty(schema.Layout.Variant, "default"),
		Align:   schema.Layout.Align,
		BG:      schema.Layout.Background,
		Site:    site,
	}
	// A per-section "alignment" select overrides the type-level hint.
	if s, ok := data["alignment"].(string); ok && s != "" {
		b.Align = s
	}

	for _, f := range schema.Fields {
		switch f.Type {
		case "heading", "text", "textarea":
			if s, ok := data[f.Key].(string); ok && s != "" {
				kind := f.Type
				if f.Type == "text" {
					kind = "text"
				}
				b.Fields = append(b.Fields, blockField{Kind: kind, Text: s})
			}
		case "image":
			id, _ := data[f.Key].(string)
			url, alt := media(id)
			if url == "" {
				continue
			}
			if b.Variant == "banner" && b.BGImage == "" {
				b.BGImage = url
				continue
			}
			b.Fields = append(b.Fields, blockField{Kind: "image", ImageURL: url, ImageAlt: alt})
		case "button":
			obj, _ := data[f.Key].(map[string]any)
			text, _ := obj["text"].(string)
			link, _ := obj["link"].(string)
			if text != "" && link != "" {
				b.Fields = append(b.Fields, blockField{Kind: "button", Text: text, URL: link})
			}
		case "contact_info":
			b.Fields = append(b.Fields, blockField{Kind: "contact"})
		case "list":
			items, _ := data[f.Key].([]any)
			var built []blockItem
			for _, raw := range items {
				obj, _ := raw.(map[string]any)
				item := buildItem(f.Fields, obj, b.Variant, media)
				if item.Summary != "" || len(item.Fields) > 0 {
					built = append(built, item)
				}
			}
			if len(built) > 0 {
				b.Fields = append(b.Fields, blockField{Kind: "items", Items: built})
				b.HasItems = true
			}
		}
		// url, select, bool: stored data, not auto-rendered.
	}
	return b, nil
}

// buildItem renders one list entry. Depending on the variant, the first
// heading/text field becomes the card title, gallery caption, or accordion
// summary.
func buildItem(fields []models.FieldSpec, data map[string]any, variant string,
	media func(string) (string, string)) blockItem {

	var item blockItem
	titled := false
	for _, f := range fields {
		switch f.Type {
		case "heading", "text":
			s, _ := data[f.Key].(string)
			if s == "" {
				continue
			}
			switch {
			case variant == "accordion" && !titled:
				item.Summary = s
				titled = true
			case variant == "gallery":
				item.Fields = append(item.Fields, blockField{Kind: "caption", Text: s})
			case !titled:
				item.Fields = append(item.Fields, blockField{Kind: "title", Text: s})
				titled = true
			default:
				item.Fields = append(item.Fields, blockField{Kind: "text", Text: s})
			}
		case "textarea":
			if s, _ := data[f.Key].(string); s != "" {
				item.Fields = append(item.Fields, blockField{Kind: "textarea", Text: s})
			}
		case "image":
			id, _ := data[f.Key].(string)
			if url, alt := media(id); url != "" {
				item.Fields = append(item.Fields, blockField{Kind: "image", ImageURL: url, ImageAlt: alt})
			}
		case "button":
			obj, _ := data[f.Key].(map[string]any)
			text, _ := obj["text"].(string)
			link, _ := obj["link"].(string)
			if text != "" && link != "" {
				item.Fields = append(item.Fields, blockField{Kind: "button", Text: text, URL: link})
			}
		}
	}
	return item
}

func writeSitemap(snap *models.Snapshot, outDir string) error {
	base := ""
	if snap.Website.Domain != "" {
		base = "https://" + snap.Website.Domain
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	slugs := make([]string, 0, len(snap.Pages))
	for _, p := range snap.Pages {
		slugs = append(slugs, p.Slug)
	}
	sort.Strings(slugs)
	for _, slug := range slugs {
		b.WriteString("  <url><loc>" + base + pageHref(slug) + "</loc></url>\n")
	}
	b.WriteString("</urlset>\n")
	return os.WriteFile(filepath.Join(outDir, "sitemap.xml"), []byte(b.String()), 0o644)
}

// pageHref returns the URL path for a slug; "home" (or empty) is the root.
func pageHref(slug string) string {
	if slug == "" || slug == "home" {
		return "/"
	}
	return "/" + slug + "/"
}

func pagePath(slug string) string {
	if slug == "" || slug == "home" {
		return "index.html"
	}
	return filepath.Join(slug, "index.html")
}

func canonicalURL(domain, slug string) string {
	if domain == "" {
		return ""
	}
	return "https://" + domain + pageHref(slug)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
