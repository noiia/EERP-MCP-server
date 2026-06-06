package parser

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleMkDocs = `
site_name: Test Docs
docs_dir: docs
nav:
  - Home: index.md
  - Getting Started:
    - Installation: getting-started/install.md
    - Configuration: getting-started/config.md
  - Reference:
    - ORM: reference/orm.md
    - API: reference/api.md
`

func TestParseMkDocs(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "mkdocs.yml")
	if err := os.WriteFile(path, []byte(sampleMkDocs), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseMkDocs(path)
	if err != nil {
		t.Fatalf("ParseMkDocs error: %v", err)
	}

	if result.SiteName != "Test Docs" {
		t.Errorf("SiteName = %q, want %q", result.SiteName, "Test Docs")
	}
	if result.DocsDir != "docs" {
		t.Errorf("DocsDir = %q, want %q", result.DocsDir, "docs")
	}
	if len(result.Nav) != 3 {
		t.Errorf("Nav len = %d, want 3", len(result.Nav))
	}
	if len(result.Sections) != 2 {
		t.Errorf("Sections = %v, want 2 items", result.Sections)
	}

	// Path order should preserve navigation order.
	if len(result.PathOrder) != 5 {
		t.Errorf("PathOrder len = %d, want 5", len(result.PathOrder))
	}
	if result.PathOrder[0] != "index.md" {
		t.Errorf("PathOrder[0] = %q, want %q", result.PathOrder[0], "index.md")
	}
}

func TestBuildBreadcrumb(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "mkdocs.yml")
	if err := os.WriteFile(path, []byte(sampleMkDocs), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseMkDocs(path)
	if err != nil {
		t.Fatalf("ParseMkDocs error: %v", err)
	}

	bc := BuildBreadcrumb(result.Nav, "getting-started/install.md")
	if len(bc) == 0 {
		t.Error("expected breadcrumb, got empty")
	}
}

func TestTitleFromPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"index.md", "Index"},
		{"getting-started.md", "Getting Started"},
		{"orm/relations.md", "Relations"},
	}
	for _, tc := range tests {
		got := titleFromPath(tc.in)
		if got != tc.want {
			t.Errorf("titleFromPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
