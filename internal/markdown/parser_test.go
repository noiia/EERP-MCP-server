package markdown

import (
	"testing"
)

func TestParseBytesTitle(t *testing.T) {
	p := New()

	tests := []struct {
		name    string
		path    string
		content string
		want    string
	}{
		{
			name:    "frontmatter title",
			path:    "foo.md",
			content: "---\ntitle: My Title\n---\n\n# Ignored\n",
			want:    "My Title",
		},
		{
			name:    "h1 title",
			path:    "foo.md",
			content: "# Getting Started\n\nSome text.\n",
			want:    "Getting Started",
		},
		{
			name:    "filename fallback",
			path:    "getting-started.md",
			content: "Some text without headings.\n",
			want:    "Getting started",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := p.ParseBytes(tc.path, []byte(tc.content))
			if err != nil {
				t.Fatalf("ParseBytes error: %v", err)
			}
			if doc.Title != tc.want {
				t.Errorf("got title %q, want %q", doc.Title, tc.want)
			}
		})
	}
}

func TestParseBytesHeadings(t *testing.T) {
	p := New()
	content := `# Main Title

## Section One

### Subsection

## Section Two
`
	doc, err := p.ParseBytes("test.md", []byte(content))
	if err != nil {
		t.Fatalf("ParseBytes error: %v", err)
	}
	if len(doc.Headings) != 4 {
		t.Errorf("got %d headings, want 4", len(doc.Headings))
	}
	if doc.Headings[0].Level != 1 || doc.Headings[0].Text != "Main Title" {
		t.Errorf("unexpected first heading: %+v", doc.Headings[0])
	}
	if doc.Headings[1].Level != 2 || doc.Headings[1].Text != "Section One" {
		t.Errorf("unexpected second heading: %+v", doc.Headings[1])
	}
}

func TestParseBytesLinks(t *testing.T) {
	p := New()
	content := `See [the docs](../docs.md) and [Google](https://google.com).`
	doc, err := p.ParseBytes("test.md", []byte(content))
	if err != nil {
		t.Fatalf("ParseBytes error: %v", err)
	}
	if len(doc.Links) != 2 {
		t.Errorf("got %d links, want 2", len(doc.Links))
	}
}

func TestParseBytesTagsAndAliases(t *testing.T) {
	p := New()
	content := `---
tags:
  - orm
  - database
aliases:
  - /legacy/orm
---

# ORM Guide
`
	doc, err := p.ParseBytes("orm/guide.md", []byte(content))
	if err != nil {
		t.Fatalf("ParseBytes error: %v", err)
	}
	if len(doc.Tags) != 2 || doc.Tags[0] != "orm" {
		t.Errorf("unexpected tags: %v", doc.Tags)
	}
	if len(doc.Aliases) != 1 || doc.Aliases[0] != "/legacy/orm" {
		t.Errorf("unexpected aliases: %v", doc.Aliases)
	}
}

func TestParseBytesKeywords(t *testing.T) {
	p := New()
	content := `# ORM Relations

Define relations between entities using the ORM.
Relations can be one-to-many or many-to-many.
`
	doc, err := p.ParseBytes("orm/relations.md", []byte(content))
	if err != nil {
		t.Fatalf("ParseBytes error: %v", err)
	}
	if len(doc.Keywords) == 0 {
		t.Error("expected auto-extracted keywords, got none")
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Hello World", "hello-world"},
		{"API Reference", "api-reference"},
		{"ORM & Entities", "orm--entities"},
	}
	for _, tc := range tests {
		got := slugify(tc.in)
		if got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
