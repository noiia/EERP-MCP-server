// Package cache provides a thread-safe, fully in-memory document store.
package cache

import (
	"sync"

	"github.com/elnoia/eerp-mcp-server/internal/models"
)

// Cache holds all indexed documentation in memory and provides thread-safe access.
type Cache struct {
	mu         sync.RWMutex
	documents  []*models.Document
	docByPath  map[string]*models.Document
	navigation []*models.NavigationNode
	sections   []string
	pathOrder  []string
}

// New returns an empty Cache.
func New() *Cache {
	return &Cache{
		docByPath: make(map[string]*models.Document),
	}
}

// Update atomically replaces the entire index.
func (c *Cache) Update(
	docs []*models.Document,
	nav []*models.NavigationNode,
	sections []string,
	pathOrder []string,
) {
	byPath := make(map[string]*models.Document, len(docs))
	for _, d := range docs {
		byPath[d.Path] = d
	}

	c.mu.Lock()
	c.documents = docs
	c.docByPath = byPath
	c.navigation = nav
	c.sections = sections
	c.pathOrder = pathOrder
	c.mu.Unlock()
}

// Documents returns a snapshot of all indexed documents.
func (c *Cache) Documents() []*models.Document {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*models.Document, len(c.documents))
	copy(out, c.documents)
	return out
}

// GetDocument retrieves a document by its relative path.
func (c *Cache) GetDocument(path string) (*models.Document, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	d, ok := c.docByPath[path]
	return d, ok
}

// Navigation returns the MkDocs navigation tree.
func (c *Cache) Navigation() []*models.NavigationNode {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.navigation
}

// Sections returns the top-level documentation sections.
func (c *Cache) Sections() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.sections))
	copy(out, c.sections)
	return out
}

// PathOrder returns document paths sorted by MkDocs navigation order.
func (c *Cache) PathOrder() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.pathOrder))
	copy(out, c.pathOrder)
	return out
}

// Size returns the number of indexed documents.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.documents)
}
