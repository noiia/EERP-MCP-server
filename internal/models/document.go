// Package models defines the core data structures shared across the MCP server.
package models

// Heading represents a single Markdown heading with its nesting level.
type Heading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
	Slug  string `json:"slug"`
}

// Link represents an anchor extracted from a Markdown document.
type Link struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

// Document is the fully-parsed, indexed representation of a single Markdown page.
type Document struct {
	Title      string    `json:"title"`
	Path       string    `json:"path"`
	Headings   []Heading `json:"headings"`
	Content    string    `json:"content"`
	RawContent string    `json:"-"`
	Links      []Link    `json:"links"`
	Tags       []string  `json:"tags"`
	Keywords   []string  `json:"keywords"`
	Category   string    `json:"category"`
	Breadcrumb []string  `json:"breadcrumb"`
	Aliases    []string  `json:"aliases"`
	Section    string    `json:"section"`

	// Pre-computed lowercase fields for fast searching.
	titleLower    string
	pathLower     string
	contentLower  string
	headingsLower []string
	keywordsLower []string
	tagsLower     []string
}

// Prepare pre-computes lowercase search fields. Must be called after all fields are set.
func (d *Document) Prepare() {
	d.titleLower = toLower(d.Title)
	d.pathLower = toLower(d.Path)
	d.contentLower = toLower(d.Content)
	d.headingsLower = make([]string, len(d.Headings))
	for i, h := range d.Headings {
		d.headingsLower[i] = toLower(h.Text)
	}
	d.keywordsLower = make([]string, len(d.Keywords))
	for i, k := range d.Keywords {
		d.keywordsLower[i] = toLower(k)
	}
	d.tagsLower = make([]string, len(d.Tags))
	for i, t := range d.Tags {
		d.tagsLower[i] = toLower(t)
	}
}

func (d *Document) TitleLower() string    { return d.titleLower }
func (d *Document) PathLower() string     { return d.pathLower }
func (d *Document) ContentLower() string  { return d.contentLower }
func (d *Document) HeadingsLower() []string { return d.headingsLower }
func (d *Document) KeywordsLower() []string { return d.keywordsLower }
func (d *Document) TagsLower() []string   { return d.tagsLower }

// NavigationNode represents a node in the MkDocs navigation tree.
type NavigationNode struct {
	Title    string            `json:"title"`
	Path     string            `json:"path,omitempty"`
	Children []*NavigationNode `json:"children,omitempty"`
}

// SearchResult wraps a Document with its relevance score and a contextual excerpt.
type SearchResult struct {
	Document *Document `json:"document"`
	Score    float64   `json:"score"`
	Excerpt  string    `json:"excerpt"`
	Matches  []string  `json:"matches"`
}

// BrokenLink records an internal link that could not be resolved.
type BrokenLink struct {
	SourcePath string
	Link       Link
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		result[i] = c
	}
	return string(result)
}
