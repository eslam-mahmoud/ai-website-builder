package models

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// FieldSpec describes one editable field of a block type. The admin
// dashboard renders forms from these specs and the static generator
// auto-renders content from them. Keeping blocks structured (instead of
// free HTML) is what makes future AI editing safe.
type FieldSpec struct {
	Key     string      `json:"key"`
	Label   string      `json:"label"`
	Type    string      `json:"type"`
	Options []string    `json:"options,omitempty"` // for select
	Fields  []FieldSpec `json:"fields,omitempty"`  // item fields when Type == "list"
}

// LayoutHints steer the auto-renderer without user-supplied HTML.
type LayoutHints struct {
	// Variant: default | banner | cards | gallery | accordion | cta
	Variant string `json:"variant,omitempty"`
	// Background: default | alt | primary
	Background string `json:"background,omitempty"`
	// Align: left | center | right
	Align string `json:"align,omitempty"`
}

// SectionType is a tenant-owned block type definition (section_types table).
type SectionType struct {
	ID        string      `json:"id,omitempty"`
	TenantID  string      `json:"tenant_id,omitempty"`
	TypeKey   string      `json:"type_key"`
	Label     string      `json:"label"`
	Icon      string      `json:"icon"`
	Fields    []FieldSpec `json:"fields"`
	Layout    LayoutHints `json:"layout"`
	Status    string      `json:"status,omitempty"`
	CreatedAt time.Time   `json:"created_at,omitempty"`
	UpdatedAt time.Time   `json:"updated_at,omitempty"`
}

// Field types available to schema authors. "list" is only valid at the top
// level (one nesting level). "url", "select" and "bool" are stored but not
// auto-rendered; "contact_info" renders the website's contact settings and
// stores no content.
var fieldTypes = map[string]bool{
	"heading": true, "text": true, "textarea": true, "url": true,
	"image": true, "select": true, "bool": true, "button": true,
	"contact_info": true, "list": true,
}

var layoutVariants = map[string]bool{
	"": true, "default": true, "banner": true, "cards": true,
	"gallery": true, "accordion": true, "cta": true,
}

var keyRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

const maxSchemaFields = 30

// ValidateSchema checks a block type's field definitions.
func ValidateSchema(fields []FieldSpec, layout LayoutHints) error {
	if !layoutVariants[layout.Variant] {
		return fmt.Errorf("unknown layout variant %q", layout.Variant)
	}
	if len(fields) == 0 {
		return fmt.Errorf("at least one field is required")
	}
	if len(fields) > maxSchemaFields {
		return fmt.Errorf("too many fields (max %d)", maxSchemaFields)
	}
	seen := map[string]bool{}
	for _, f := range fields {
		if err := validateField(f, seen, true); err != nil {
			return err
		}
	}
	return nil
}

func validateField(f FieldSpec, seen map[string]bool, allowList bool) error {
	if !keyRe.MatchString(f.Key) || len(f.Key) > 40 {
		return fmt.Errorf("field key %q must be lowercase letters, digits and underscores", f.Key)
	}
	if seen[f.Key] {
		return fmt.Errorf("duplicate field key %q", f.Key)
	}
	seen[f.Key] = true
	if !fieldTypes[f.Type] {
		return fmt.Errorf("field %q has unknown type %q", f.Key, f.Type)
	}
	if f.Label == "" {
		return fmt.Errorf("field %q needs a label", f.Key)
	}
	switch f.Type {
	case "select":
		if len(f.Options) == 0 {
			return fmt.Errorf("select field %q needs options", f.Key)
		}
	case "list":
		if !allowList {
			return fmt.Errorf("field %q: lists cannot be nested", f.Key)
		}
		if len(f.Fields) == 0 {
			return fmt.Errorf("list field %q needs item fields", f.Key)
		}
		if len(f.Fields) > maxSchemaFields {
			return fmt.Errorf("list field %q has too many item fields", f.Key)
		}
		itemSeen := map[string]bool{}
		for _, sub := range f.Fields {
			if err := validateField(sub, itemSeen, false); err != nil {
				return err
			}
		}
	}
	return nil
}

// CollectMediaIDs extracts media IDs referenced by "image" fields of the
// given schema in a section's content.
func CollectMediaIDs(fields []FieldSpec, content json.RawMessage) []string {
	var data map[string]any
	if json.Unmarshal(content, &data) != nil {
		return nil
	}
	var ids []string
	for _, f := range fields {
		switch f.Type {
		case "image":
			if s, ok := data[f.Key].(string); ok && s != "" {
				ids = append(ids, s)
			}
		case "list":
			items, _ := data[f.Key].([]any)
			for _, it := range items {
				obj, _ := it.(map[string]any)
				for _, sub := range f.Fields {
					if sub.Type != "image" {
						continue
					}
					if s, ok := obj[sub.Key].(string); ok && s != "" {
						ids = append(ids, s)
					}
				}
			}
		}
	}
	return ids
}

// StarterSectionTypes is the library seeded into every new tenant. Tenants
// can then edit these or define entirely new block types.
var StarterSectionTypes = []SectionType{
	{TypeKey: "hero", Label: "Hero", Icon: "🌄", Layout: LayoutHints{Variant: "banner", Align: "center"},
		Fields: []FieldSpec{
			{Key: "title", Label: "Title", Type: "heading"},
			{Key: "subtitle", Label: "Subtitle", Type: "textarea"},
			{Key: "button", Label: "Button", Type: "button"},
			{Key: "background_image", Label: "Background image", Type: "image"},
			{Key: "alignment", Label: "Alignment", Type: "select", Options: []string{"left", "center", "right"}},
		}},
	{TypeKey: "text", Label: "Text", Icon: "📝", Layout: LayoutHints{Variant: "default"},
		Fields: []FieldSpec{
			{Key: "heading", Label: "Heading", Type: "heading"},
			{Key: "body", Label: "Body", Type: "textarea"},
		}},
	{TypeKey: "image", Label: "Image", Icon: "🖼️", Layout: LayoutHints{Variant: "default", Align: "center"},
		Fields: []FieldSpec{
			{Key: "image", Label: "Image", Type: "image"},
			{Key: "caption", Label: "Caption", Type: "text"},
		}},
	{TypeKey: "services", Label: "Services list", Icon: "🛠️", Layout: LayoutHints{Variant: "cards"},
		Fields: []FieldSpec{
			{Key: "heading", Label: "Heading", Type: "heading"},
			{Key: "items", Label: "Services", Type: "list", Fields: []FieldSpec{
				{Key: "image", Label: "Image", Type: "image"},
				{Key: "title", Label: "Title", Type: "text"},
				{Key: "description", Label: "Description", Type: "textarea"},
			}},
		}},
	{TypeKey: "gallery", Label: "Gallery", Icon: "🎞️", Layout: LayoutHints{Variant: "gallery"},
		Fields: []FieldSpec{
			{Key: "heading", Label: "Heading", Type: "heading"},
			{Key: "items", Label: "Images", Type: "list", Fields: []FieldSpec{
				{Key: "image", Label: "Image", Type: "image"},
				{Key: "caption", Label: "Caption", Type: "text"},
			}},
		}},
	{TypeKey: "testimonials", Label: "Testimonials", Icon: "💬", Layout: LayoutHints{Variant: "cards"},
		Fields: []FieldSpec{
			{Key: "heading", Label: "Heading", Type: "heading"},
			{Key: "items", Label: "Testimonials", Type: "list", Fields: []FieldSpec{
				{Key: "quote", Label: "Quote", Type: "textarea"},
				{Key: "author", Label: "Author", Type: "text"},
				{Key: "role", Label: "Role", Type: "text"},
			}},
		}},
	{TypeKey: "contact", Label: "Contact information", Icon: "📞", Layout: LayoutHints{Variant: "default"},
		Fields: []FieldSpec{
			{Key: "heading", Label: "Heading", Type: "heading"},
			{Key: "text", Label: "Intro text", Type: "textarea"},
			{Key: "details", Label: "Contact details", Type: "contact_info"},
		}},
	{TypeKey: "cta", Label: "Call to action", Icon: "📣", Layout: LayoutHints{Variant: "cta", Align: "center"},
		Fields: []FieldSpec{
			{Key: "title", Label: "Title", Type: "heading"},
			{Key: "subtitle", Label: "Subtitle", Type: "textarea"},
			{Key: "button", Label: "Button", Type: "button"},
		}},
	{TypeKey: "faq", Label: "FAQ", Icon: "❓", Layout: LayoutHints{Variant: "accordion"},
		Fields: []FieldSpec{
			{Key: "heading", Label: "Heading", Type: "heading"},
			{Key: "items", Label: "Questions", Type: "list", Fields: []FieldSpec{
				{Key: "question", Label: "Question", Type: "text"},
				{Key: "answer", Label: "Answer", Type: "textarea"},
			}},
		}},
}
