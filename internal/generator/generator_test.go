package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eslam/cms/internal/models"
)

func starterSchemas() map[string]models.SnapshotSectionType {
	m := map[string]models.SnapshotSectionType{}
	for _, st := range models.StarterSectionTypes {
		m[st.TypeKey] = models.SnapshotSectionType{Label: st.Label, Fields: st.Fields, Layout: st.Layout}
	}
	return m
}

func testSnapshot() *models.Snapshot {
	return &models.Snapshot{
		Website: models.SnapshotWebsite{
			Name:   "Acme Co",
			Domain: "acme.example.com",
			Settings: models.WebsiteSettings{
				PrimaryColor: "#112233",
				ContactPhone: "+1 555 0100",
				FooterText:   "Quality since 1999",
				SocialLinks:  map[string]string{"facebook": "https://fb.example/acme"},
			},
		},
		SectionTypes: starterSchemas(),
		Pages: []models.SnapshotPage{
			{
				Title: "Home", Slug: "home", SEOTitle: "Acme — Home", SEODescription: "Acme homepage",
				Sections: []models.SnapshotSection{
					{Type: "hero", Content: json.RawMessage(`{"title":"Welcome <b>","subtitle":"Line1\nLine2","button":{"text":"Go","link":"/about/"},"background_image":"m1"}`)},
					{Type: "contact", Content: json.RawMessage(`{"heading":"Reach us","details":true}`)},
					{Type: "faq", Content: json.RawMessage(`{"heading":"FAQ","items":[{"question":"Why?","answer":"Because."}]}`)},
				},
			},
			{
				Title: "About", Slug: "about",
				Sections: []models.SnapshotSection{
					{Type: "text", Content: json.RawMessage(`{"heading":"Us","body":"Hi"}`)},
				},
			},
		},
		Media: map[string]models.SnapshotMedia{
			"m1": {URL: "https://cdn.example.com/m1.jpg", Alt: "office"},
		},
	}
}

func TestGenerate(t *testing.T) {
	out := t.TempDir()
	if err := Generate(testSnapshot(), out, ""); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	for _, f := range []string{"index.html", "about/index.html", "styles.css", "sitemap.xml", "robots.txt"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("expected output file %s: %v", f, err)
		}
	}

	home, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	h := string(home)
	for _, want := range []string{
		"<title>Acme — Home</title>",
		`content="Acme homepage"`,
		"<h1>Welcome &lt;b&gt;</h1>",          // banner heading, escaped not injected
		"Line1<br>Line2",                      // nl2br
		"https://cdn.example.com/m1.jpg",      // banner background image resolved
		`href="/about/">Go</a>`,               // button field
		"--primary:#112233",                   // theme color
		`href="tel:&#43;15550100"`,            // contact_info renders site phone
		"<summary>Why?</summary>",             // accordion variant
		`<link rel="canonical" href="https://acme.example.com/">`,
	} {
		if !strings.Contains(h, want) {
			t.Errorf("home page missing %q", want)
		}
	}

	sitemap, _ := os.ReadFile(filepath.Join(out, "sitemap.xml"))
	if !strings.Contains(string(sitemap), "https://acme.example.com/about/") {
		t.Errorf("sitemap missing about page: %s", sitemap)
	}
}

// TestGenerateCustomType renders a block type the generator has never seen —
// the whole point of the schema-driven auto-renderer.
func TestGenerateCustomType(t *testing.T) {
	snap := testSnapshot()
	snap.SectionTypes["team"] = models.SnapshotSectionType{
		Label:  "Team",
		Layout: models.LayoutHints{Variant: "cards", Background: "alt"},
		Fields: []models.FieldSpec{
			{Key: "heading", Label: "Heading", Type: "heading"},
			{Key: "members", Label: "Members", Type: "list", Fields: []models.FieldSpec{
				{Key: "photo", Label: "Photo", Type: "image"},
				{Key: "name", Label: "Name", Type: "text"},
				{Key: "role", Label: "Role", Type: "text"},
				{Key: "bio", Label: "Bio", Type: "textarea"},
			}},
		},
	}
	snap.Pages[1].Sections = append(snap.Pages[1].Sections, models.SnapshotSection{
		Type: "team",
		Content: json.RawMessage(`{"heading":"Our team","members":[
			{"photo":"m1","name":"Jane","role":"CEO","bio":"Founder."},
			{"name":"Ali","role":"CTO"}]}`),
	})

	out := t.TempDir()
	if err := Generate(snap, out, ""); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	about, _ := os.ReadFile(filepath.Join(out, "about/index.html"))
	a := string(about)
	for _, want := range []string{
		"<h2>Our team</h2>",
		"bg-alt",                              // background hint applied
		`<div class="cards">`,                 // cards variant
		"<h3>Jane</h3>",                       // first text field = card title
		`<p class="card-meta">CEO</p>`,        // second text field = meta
		"https://cdn.example.com/m1.jpg",      // item image resolved
		"<h3>Ali</h3>",
	} {
		if !strings.Contains(a, want) {
			t.Errorf("custom type output missing %q", want)
		}
	}
}

// TestButtonLinkSchemes: tel:/mailto: pass through, javascript: is neutralized.
func TestButtonLinkSchemes(t *testing.T) {
	snap := testSnapshot()
	snap.Pages[0].Sections = []models.SnapshotSection{
		{Type: "cta", Content: json.RawMessage(`{"title":"Call","button":{"text":"Ring","link":"tel:+15550100"}}`)},
		{Type: "cta", Content: json.RawMessage(`{"title":"Evil","button":{"text":"X","link":"javascript:alert(1)"}}`)},
	}
	out := t.TempDir()
	if err := Generate(snap, out, ""); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	home, _ := os.ReadFile(filepath.Join(out, "index.html"))
	h := string(home)
	if !strings.Contains(h, `href="tel:&#43;15550100">Ring</a>`) {
		t.Errorf("tel: button link missing or mangled")
	}
	if strings.Contains(h, "javascript:") {
		t.Errorf("javascript: link not neutralized")
	}
	if !strings.Contains(h, `href="#">X</a>`) {
		t.Errorf("unsafe link should fall back to #")
	}
}

func TestGenerateSkipsUnknownType(t *testing.T) {
	snap := testSnapshot()
	snap.Pages[0].Sections = append(snap.Pages[0].Sections,
		models.SnapshotSection{Type: "ghost", Content: json.RawMessage(`{"x":1}`)})
	out := t.TempDir()
	if err := Generate(snap, out, ""); err != nil {
		t.Fatalf("unknown types should be skipped, not fail the build: %v", err)
	}
}

func TestGenerateBasePath(t *testing.T) {
	out := t.TempDir()
	if err := Generate(testSnapshot(), out, "/preview/tok123"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	home, _ := os.ReadFile(filepath.Join(out, "index.html"))
	for _, want := range []string{
		`href="/preview/tok123/styles.css"`,
		`href="/preview/tok123/about/"`,
	} {
		if !strings.Contains(string(home), want) {
			t.Errorf("preview build missing %q", want)
		}
	}
}

func TestGenerateRejectsEmpty(t *testing.T) {
	snap := &models.Snapshot{}
	if err := Generate(snap, t.TempDir(), ""); err == nil {
		t.Fatal("expected validation error for snapshot with no pages")
	}
}
