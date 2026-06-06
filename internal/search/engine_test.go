package search

import (
	"testing"

	"github.com/elnoia/eerp-mcp-server/internal/models"
)

func makeDoc(title, path, content string, tags []string) *models.Document {
	d := &models.Document{
		Title:   title,
		Path:    path,
		Content: content,
		Tags:    tags,
	}
	d.Prepare()
	return d
}

func TestSearchBasic(t *testing.T) {
	docs := []*models.Document{
		makeDoc("ORM Relations", "orm/relations.md", "Define relations between entities.", []string{"orm"}),
		makeDoc("Installation", "install.md", "Install the application.", nil),
		makeDoc("Configuration", "config.md", "Configure the database connection.", nil),
	}
	e := New(docs)

	results := e.Search("orm relations", 10)
	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}
	if results[0].Document.Path != "orm/relations.md" {
		t.Errorf("top result should be ORM Relations, got %q", results[0].Document.Path)
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	docs := []*models.Document{
		makeDoc("Database Setup", "setup.md", "Setup your PostgreSQL database.", nil),
	}
	e := New(docs)

	for _, q := range []string{"DATABASE", "Database", "database", "SETUP"} {
		results := e.Search(q, 10)
		if len(results) == 0 {
			t.Errorf("expected results for query %q", q)
		}
	}
}

func TestSearchRanking(t *testing.T) {
	docs := []*models.Document{
		makeDoc("Migration Guide", "migrations.md", "Run migrations with the CLI.", nil),
		makeDoc("ORM Overview", "orm.md", "The ORM supports migrations, models, and queries.", nil),
		makeDoc("Configuration", "config.md", "Configure logging and the database.", nil),
	}
	e := New(docs)

	results := e.Search("migrations", 10)
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	// Title match should rank higher than body mention.
	if results[0].Document.Path != "migrations.md" {
		t.Errorf("title match should rank first, got %q", results[0].Document.Path)
	}
}

func TestSearchNoResults(t *testing.T) {
	docs := []*models.Document{
		makeDoc("Home", "index.md", "Welcome to the documentation.", nil),
	}
	e := New(docs)
	results := e.Search("xyzzyfrob", 10)
	if len(results) != 0 {
		t.Errorf("expected no results for gibberish, got %d", len(results))
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	docs := []*models.Document{makeDoc("Home", "index.md", "Content.", nil)}
	e := New(docs)
	if results := e.Search("", 10); len(results) != 0 {
		t.Errorf("empty query should return no results")
	}
}

func TestSearchSymbols(t *testing.T) {
	docs := []*models.Document{
		makeDoc("UserEntity", "entities/user.md", "The User entity represents a system user.", []string{"entity"}),
		makeDoc("Migration", "migrations.md", "Run database migrations.", nil),
	}
	e := New(docs)

	results := e.SearchSymbols("UserEntity", 10)
	if len(results) == 0 {
		t.Fatal("expected results for UserEntity symbol search")
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"kitten", "sitting", 3},
		{"relations", "relation", 1},
		{"hello", "hello", 0},
	}
	for _, tc := range tests {
		got := levenshtein(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestBuildExcerpt(t *testing.T) {
	content := "The quick brown fox jumps over the lazy dog. The fox is very quick."
	excerpt := buildExcerpt(content, []string{"fox"}, 40)
	if excerpt == "" {
		t.Error("expected non-empty excerpt")
	}
}
