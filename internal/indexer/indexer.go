// Package indexer orchestrates the full documentation indexing pipeline.
package indexer

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elnoia/eerp-mcp-server/internal/cache"
	"github.com/elnoia/eerp-mcp-server/internal/markdown"
	"github.com/elnoia/eerp-mcp-server/internal/models"
	"github.com/elnoia/eerp-mcp-server/internal/parser"
	"github.com/elnoia/eerp-mcp-server/internal/search"
)

// Indexer runs the MkDocs → Documents → Cache → SearchEngine pipeline.
type Indexer struct {
	mkdocsPath string
	docsRoot   string
	cache      *cache.Cache
	engine     *search.Engine
	mdParser   *markdown.Parser
	logger     *slog.Logger
}

// New creates a new Indexer. The docsRoot may be overridden by the mkdocs.yml docs_dir.
func New(mkdocsPath, docsRoot string, c *cache.Cache, e *search.Engine, logger *slog.Logger) *Indexer {
	return &Indexer{
		mkdocsPath: mkdocsPath,
		docsRoot:   docsRoot,
		cache:      c,
		engine:     e,
		mdParser:   markdown.New(),
		logger:     logger,
	}
}

// Index runs a full re-index. It is safe to call concurrently with search operations
// because cache.Update is atomic.
func (idx *Indexer) Index() error {
	start := time.Now()
	idx.logger.Info("indexing started", "mkdocs", idx.mkdocsPath)

	// 1. Parse mkdocs.yml.
	mkResult, err := parser.ParseMkDocs(idx.mkdocsPath)
	if err != nil {
		return fmt.Errorf("parse mkdocs.yml: %w", err)
	}

	// Resolve the actual docs root (mkdocs.yml may override it).
	docsRoot := idx.docsRoot
	if mkResult.DocsDir != "" {
		base := filepath.Dir(idx.mkdocsPath)
		resolved := filepath.Join(base, mkResult.DocsDir)
		if _, err := os.Stat(resolved); err == nil {
			docsRoot = resolved
		}
	}

	// 2. Discover all markdown files.
	allPaths, err := discoverMarkdown(docsRoot)
	if err != nil {
		return fmt.Errorf("discover markdown: %w", err)
	}

	// 3. Determine the ordered set of paths from navigation + any orphans.
	orderedPaths := mergePathOrder(mkResult.PathOrder, allPaths)

	// 4. Parse each file.
	docs := make([]*models.Document, 0, len(orderedPaths))
	for _, relPath := range orderedPaths {
		doc, err := idx.mdParser.ParseFile(docsRoot, relPath)
		if err != nil {
			idx.logger.Warn("skipping file", "path", relPath, "error", err)
			continue
		}

		// Assign navigation metadata.
		doc.Section = findSection(mkResult.Nav, relPath)
		doc.Breadcrumb = parser.BuildBreadcrumb(mkResult.Nav, relPath)

		docs = append(docs, doc)
	}

	// 5. Compute related pages (by shared tags + section).
	computeRelated(docs)

	// 6. Atomically update cache.
	idx.cache.Update(docs, mkResult.Nav, mkResult.Sections, mkResult.PathOrder)

	// 7. Rebuild search engine.
	idx.engine.Rebuild(idx.cache.Documents())

	idx.logger.Info("indexing complete",
		"documents", len(docs),
		"sections", len(mkResult.Sections),
		"duration", time.Since(start),
	)
	return nil
}

// discoverMarkdown returns all .md paths relative to docsRoot.
func discoverMarkdown(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			paths = append(paths, filepath.ToSlash(rel))
		}
		return nil
	})
	return paths, err
}

// mergePathOrder returns nav-ordered paths first, followed by any orphan files.
func mergePathOrder(navPaths, allPaths []string) []string {
	seen := make(map[string]bool, len(navPaths))
	out := make([]string, 0, len(allPaths))
	for _, p := range navPaths {
		seen[p] = true
		out = append(out, p)
	}
	for _, p := range allPaths {
		if !seen[p] {
			out = append(out, p)
		}
	}
	return out
}

// findSection returns the top-level nav section title for a given path.
func findSection(nav []*models.NavigationNode, path string) string {
	for _, node := range nav {
		if node.Path == path {
			return node.Title
		}
		if len(node.Children) > 0 {
			if containsPath(node.Children, path) {
				return node.Title
			}
		}
	}
	return ""
}

func containsPath(nodes []*models.NavigationNode, path string) bool {
	for _, n := range nodes {
		if n.Path == path {
			return true
		}
		if containsPath(n.Children, path) {
			return true
		}
	}
	return false
}

// computeRelated sets a simple tag/section-based related-pages annotation.
// We store related paths directly in the Keywords slice with a "related:" prefix
// so the MCP tools can surface them without adding extra fields to Document.
func computeRelated(docs []*models.Document) {
	// Build tag → paths index.
	tagDocs := make(map[string][]string)
	for _, d := range docs {
		for _, t := range d.Tags {
			tagDocs[t] = append(tagDocs[t], d.Path)
		}
	}

	for _, d := range docs {
		seen := make(map[string]bool)
		related := make([]string, 0)
		for _, t := range d.Tags {
			for _, p := range tagDocs[t] {
				if p != d.Path && !seen[p] {
					seen[p] = true
					related = append(related, p)
				}
			}
		}
		// Store as structured marker so tools can separate them.
		for _, r := range related {
			d.Keywords = append(d.Keywords, "related:"+r)
		}
	}
}
