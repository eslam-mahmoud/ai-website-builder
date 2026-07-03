package models

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

func fieldsOf(typeKey string) []FieldSpec {
	for _, st := range StarterSectionTypes {
		if st.TypeKey == typeKey {
			return st.Fields
		}
	}
	return nil
}

func TestCollectMediaIDs(t *testing.T) {
	cases := []struct {
		name    string
		fields  []FieldSpec
		content string
		want    []string
	}{
		{"hero image", fieldsOf("hero"), `{"title":"x","background_image":"m1"}`, []string{"m1"}},
		{"hero empty", fieldsOf("hero"), `{"background_image":""}`, nil},
		{"list images", fieldsOf("services"),
			`{"items":[{"title":"a","image":"m1"},{"title":"b"},{"image":"m2"}]}`, []string{"m1", "m2"}},
		{"invalid json", fieldsOf("hero"), `not-json`, nil},
		{"no media fields", fieldsOf("text"), `{"body":"hello"}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CollectMediaIDs(tc.fields, json.RawMessage(tc.content))
			sort.Strings(got)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestValidateSchema(t *testing.T) {
	ok := []FieldSpec{
		{Key: "heading", Label: "Heading", Type: "heading"},
		{Key: "items", Label: "Items", Type: "list", Fields: []FieldSpec{
			{Key: "name", Label: "Name", Type: "text"},
		}},
	}
	if err := ValidateSchema(ok, LayoutHints{Variant: "cards"}); err != nil {
		t.Fatalf("valid schema rejected: %v", err)
	}

	bad := []struct {
		name   string
		fields []FieldSpec
		layout LayoutHints
		msg    string
	}{
		{"empty", nil, LayoutHints{}, "at least one field"},
		{"bad key", []FieldSpec{{Key: "Bad Key", Label: "x", Type: "text"}}, LayoutHints{}, "field key"},
		{"bad type", []FieldSpec{{Key: "a", Label: "x", Type: "wysiwyg"}}, LayoutHints{}, "unknown type"},
		{"dup key", []FieldSpec{
			{Key: "a", Label: "x", Type: "text"}, {Key: "a", Label: "y", Type: "text"},
		}, LayoutHints{}, "duplicate"},
		{"no label", []FieldSpec{{Key: "a", Label: "", Type: "text"}}, LayoutHints{}, "label"},
		{"select no options", []FieldSpec{{Key: "a", Label: "x", Type: "select"}}, LayoutHints{}, "options"},
		{"nested list", []FieldSpec{{Key: "a", Label: "x", Type: "list", Fields: []FieldSpec{
			{Key: "b", Label: "y", Type: "list", Fields: []FieldSpec{{Key: "c", Label: "z", Type: "text"}}},
		}}}, LayoutHints{}, "nested"},
		{"empty list", []FieldSpec{{Key: "a", Label: "x", Type: "list"}}, LayoutHints{}, "item fields"},
		{"bad variant", []FieldSpec{{Key: "a", Label: "x", Type: "text"}}, LayoutHints{Variant: "spiral"}, "variant"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSchema(tc.fields, tc.layout)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.msg) {
				t.Fatalf("error %q does not mention %q", err, tc.msg)
			}
		})
	}
}

func TestNormalizeContent(t *testing.T) {
	fields := []FieldSpec{
		{Key: "title", Label: "Title", Type: "heading"},
		{Key: "button", Label: "Button", Type: "button"},
		{Key: "details", Label: "Details", Type: "contact_info"},
		{Key: "items", Label: "Items", Type: "list", Fields: []FieldSpec{
			{Key: "name", Label: "Name", Type: "text"},
		}},
	}
	in := json.RawMessage(`{
		"title": "Hi", "junk": "drop me", "show_phone": true,
		"button": {"text": "Go", "link": "/x/", "onclick": "evil()"},
		"details": {"anything": 1},
		"items": [{"name": "A", "stale": 2}, {"name": "B"}]
	}`)
	out, err := NormalizeContent(fields, in)
	if err != nil {
		t.Fatalf("NormalizeContent: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["junk"]; ok {
		t.Error("unknown key not dropped")
	}
	if _, ok := got["show_phone"]; ok {
		t.Error("legacy key not dropped")
	}
	if _, ok := got["details"]; ok {
		t.Error("contact_info should store no content")
	}
	btn := got["button"].(map[string]any)
	if btn["text"] != "Go" || btn["link"] != "/x/" {
		t.Errorf("button mangled: %v", btn)
	}
	if _, ok := btn["onclick"]; ok {
		t.Error("extra button key not dropped")
	}
	items := got["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items: %v", items)
	}
	if _, ok := items[0].(map[string]any)["stale"]; ok {
		t.Error("stale item key not dropped")
	}
	if got["title"] != "Hi" {
		t.Errorf("title lost: %v", got)
	}

	if _, err := NormalizeContent(fields, json.RawMessage(`[1,2]`)); err == nil {
		t.Error("non-object content should error")
	}
	empty, err := NormalizeContent(fields, nil)
	if err != nil || string(empty) != "{}" {
		t.Errorf("nil content should normalize to {}: %s %v", empty, err)
	}
}

func TestStarterSectionTypesAreValid(t *testing.T) {
	for _, st := range StarterSectionTypes {
		if err := ValidateSchema(st.Fields, st.Layout); err != nil {
			t.Errorf("starter type %s invalid: %v", st.TypeKey, err)
		}
	}
}

func TestRoleAtLeast(t *testing.T) {
	if !RoleAtLeast(RoleTenantAdmin, RoleEditor) {
		t.Error("tenant_admin should satisfy editor")
	}
	if RoleAtLeast(RoleViewer, RoleEditor) {
		t.Error("viewer should not satisfy editor")
	}
	if RoleAtLeast("", RoleViewer) {
		t.Error("unknown role should not satisfy viewer")
	}
}
