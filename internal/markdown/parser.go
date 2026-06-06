// Package markdown provides a Markdown document parser that extracts structured metadata.
package markdown

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/elnoia/eerp-mcp-server/internal/models"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

var (
	reFrontmatter = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n`)
	reLink        = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reCodeBlock   = regexp.MustCompile("(?s)```[^`]*```|`[^`]+`")
)

// frontmatter mirrors the YAML keys supported in MkDocs page metadata.
type frontmatter struct {
	Title    string   `yaml:"title"`
	Tags     []string `yaml:"tags"`
	Aliases  []string `yaml:"aliases"`
	Keywords []string `yaml:"keywords"`
}

// Parser converts raw Markdown bytes into a structured Document.
type Parser struct {
	md goldmark.Markdown
}

// New returns a Parser ready to use.
func New() *Parser {
	return &Parser{
		md: goldmark.New(),
	}
}

// ParseFile reads a file from disk and delegates to ParseBytes.
func (p *Parser) ParseFile(docsRoot, relPath string) (*models.Document, error) {
	absPath := filepath.Join(docsRoot, relPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", absPath, err)
	}
	return p.ParseBytes(relPath, data)
}

// ParseBytes parses raw Markdown content for the given relative path.
func (p *Parser) ParseBytes(relPath string, data []byte) (*models.Document, error) {
	doc := &models.Document{
		Path:       relPath,
		RawContent: string(data),
	}

	// 1. Extract and strip YAML frontmatter.
	fm, body := extractFrontmatter(data)
	doc.Tags = fm.Tags
	doc.Aliases = fm.Aliases
	doc.Keywords = fm.Keywords
	if fm.Title != "" {
		doc.Title = fm.Title
	}

	// 2. Parse the Markdown AST.
	reader := text.NewReader(body)
	mdRoot := p.md.Parser().Parse(reader)
	doc.Headings = extractHeadings(mdRoot, body)

	// 3. Title fallback: first H1 → filename.
	if doc.Title == "" && len(doc.Headings) > 0 && doc.Headings[0].Level == 1 {
		doc.Title = doc.Headings[0].Text
	}
	if doc.Title == "" {
		doc.Title = titleFromPath(relPath)
	}

	// 4. Extract links.
	doc.Links = extractLinks(body)

	// 5. Plain-text content (strip code blocks and markdown syntax for search).
	doc.Content = extractPlainText(body)

	// 6. Auto-extract keywords from content if none declared.
	if len(doc.Keywords) == 0 {
		doc.Keywords = extractKeywords(doc.Content, doc.Title, doc.Headings)
	}

	doc.Prepare()
	return doc, nil
}

// extractFrontmatter separates YAML frontmatter from body.
func extractFrontmatter(data []byte) (frontmatter, []byte) {
	var fm frontmatter
	m := reFrontmatter.FindSubmatch(data)
	if m == nil {
		return fm, data
	}
	_ = yaml.Unmarshal(m[1], &fm)
	return fm, data[len(m[0]):]
}

// extractHeadings walks the AST collecting ATX/Setext headings.
func extractHeadings(root ast.Node, src []byte) []models.Heading {
	var headings []models.Heading
	ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		var buf bytes.Buffer
		for child := h.FirstChild(); child != nil; child = child.NextSibling() {
			buf.Write(child.Text(src))
		}
		text := strings.TrimSpace(buf.String())
		headings = append(headings, models.Heading{
			Level: h.Level,
			Text:  text,
			Slug:  slugify(text),
		})
		return ast.WalkContinue, nil
	})
	return headings
}

// extractLinks finds all Markdown inline links in the raw body.
func extractLinks(body []byte) []models.Link {
	matches := reLink.FindAllSubmatch(body, -1)
	links := make([]models.Link, 0, len(matches))
	for _, m := range matches {
		links = append(links, models.Link{
			Text: string(m[1]),
			URL:  string(m[2]),
		})
	}
	return links
}

// extractPlainText produces a searchable plain-text version of the content.
// It strips code blocks, Markdown syntax characters, and excess whitespace.
func extractPlainText(body []byte) string {
	// Remove fenced and inline code blocks first.
	stripped := reCodeBlock.ReplaceAll(body, []byte(" "))

	var sb strings.Builder
	lines := bytes.Split(stripped, []byte("\n"))
	for _, line := range lines {
		l := string(bytes.TrimSpace(line))
		if l == "" {
			continue
		}
		// Remove ATX heading markers.
		l = strings.TrimLeft(l, "# ")
		// Remove Markdown emphasis and strong markers.
		l = strings.NewReplacer("**", "", "__", "", "*", "", "_", "").Replace(l)
		// Remove blockquote markers.
		if strings.HasPrefix(l, ">") {
			l = strings.TrimPrefix(l, ">")
			l = strings.TrimSpace(l)
		}
		if l != "" {
			sb.WriteString(l)
			sb.WriteByte(' ')
		}
	}
	return strings.TrimSpace(sb.String())
}

// stopWords are excluded from auto-keyword extraction.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "by": true, "from": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "been": true, "being": true, "have": true, "has": true,
	"had": true, "do": true, "does": true, "did": true, "will": true, "would": true,
	"could": true, "should": true, "may": true, "might": true, "can": true, "this": true,
	"that": true, "these": true, "those": true, "it": true, "its": true, "not": true,
	"you": true, "your": true, "we": true, "our": true, "as": true, "if": true,
	"then": true, "than": true, "so": true, "also": true, "more": true, "how": true,
	"when": true, "what": true, "which": true, "who": true, "where": true,
}

// extractKeywords returns the most frequent meaningful words from the content.
func extractKeywords(content, title string, headings []models.Heading) []string {
	freq := make(map[string]int)

	countWords := func(s string, weight int) {
		for _, word := range strings.FieldsFunc(s, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		}) {
			w := strings.ToLower(word)
			if len(w) < 3 || stopWords[w] {
				continue
			}
			freq[w] += weight
		}
	}

	countWords(title, 5)
	for _, h := range headings {
		countWords(h.Text, 3)
	}
	countWords(content, 1)

	// Sort by frequency descending and take top 20.
	type kv struct {
		key string
		val int
	}
	pairs := make([]kv, 0, len(freq))
	for k, v := range freq {
		pairs = append(pairs, kv{k, v})
	}
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[j].val > pairs[i].val {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}

	const maxKeywords = 20
	limit := len(pairs)
	if limit > maxKeywords {
		limit = maxKeywords
	}
	keywords := make([]string, limit)
	for i := 0; i < limit; i++ {
		keywords[i] = pairs[i].key
	}
	return keywords
}

// slugify converts heading text to a URL-safe slug.
func slugify(s string) string {
	s = strings.ToLower(s)
	var buf bytes.Buffer
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			buf.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			buf.WriteByte('-')
		}
	}
	return strings.Trim(buf.String(), "-")
}

// titleFromPath converts a relative file path into a human-readable title.
func titleFromPath(p string) string {
	base := filepath.Base(p)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]
	name = strings.NewReplacer("-", " ", "_", " ").Replace(name)
	if len(name) == 0 {
		return p
	}
	runes := []rune(name)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
