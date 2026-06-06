// Package parser handles MkDocs YAML configuration parsing.
package parser

import (
	"fmt"
	"os"

	"github.com/elnoia/eerp-mcp-server/internal/models"
	"gopkg.in/yaml.v3"
)

// MkDocsConfig mirrors the fields of mkdocs.yml that the server cares about.
type MkDocsConfig struct {
	SiteName string        `yaml:"site_name"`
	DocsDir  string        `yaml:"docs_dir"`
	Nav      []interface{} `yaml:"nav"`
	Extra    struct {
		Tags map[string]string `yaml:"tags"`
	} `yaml:"extra"`
}

// ParseResult bundles everything the indexer needs from mkdocs.yml.
type ParseResult struct {
	SiteName  string
	DocsDir   string
	Nav       []*models.NavigationNode
	Sections  []string
	PathOrder []string // document paths in navigation order
}

// ParseMkDocs loads and parses a mkdocs.yml file.
func ParseMkDocs(path string) (*ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mkdocs.yml %q: %w", path, err)
	}

	var cfg MkDocsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse mkdocs.yml %q: %w", path, err)
	}

	result := &ParseResult{
		SiteName: cfg.SiteName,
		DocsDir:  cfg.DocsDir,
	}
	if result.DocsDir == "" {
		result.DocsDir = "docs"
	}

	if cfg.Nav != nil {
		result.Nav = parseNavList(cfg.Nav, nil)
		collectSections(result.Nav, &result.Sections)
		collectPaths(result.Nav, &result.PathOrder)
	}

	return result, nil
}

// parseNavList converts the raw yaml nav list into NavigationNodes.
func parseNavList(items []interface{}, breadcrumb []string) []*models.NavigationNode {
	nodes := make([]*models.NavigationNode, 0, len(items))
	for _, item := range items {
		if node := parseNavItem(item, breadcrumb); node != nil {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// parseNavItem handles a single nav entry, which can be:
//
//	string           – bare path, e.g. "index.md"
//	map[string]string – title: path
//	map[string][]    – title: [children...]
func parseNavItem(item interface{}, breadcrumb []string) *models.NavigationNode {
	switch v := item.(type) {
	case string:
		return &models.NavigationNode{Path: v, Title: titleFromPath(v)}

	case map[string]interface{}:
		for title, val := range v {
			bc := append(append([]string{}, breadcrumb...), title)
			switch child := val.(type) {
			case string:
				return &models.NavigationNode{Title: title, Path: child}
			case []interface{}:
				children := parseNavList(child, bc)
				return &models.NavigationNode{Title: title, Children: children}
			}
		}
	}
	return nil
}

// collectSections gathers top-level section titles.
func collectSections(nodes []*models.NavigationNode, out *[]string) {
	for _, n := range nodes {
		if n.Title != "" && len(n.Children) > 0 {
			*out = append(*out, n.Title)
		}
	}
}

// collectPaths walks the nav tree and appends document paths in order.
func collectPaths(nodes []*models.NavigationNode, out *[]string) {
	for _, n := range nodes {
		if n.Path != "" {
			*out = append(*out, n.Path)
		}
		if len(n.Children) > 0 {
			collectPaths(n.Children, out)
		}
	}
}

// titleFromPath converts a file path like "getting-started/install.md" → "Install".
func titleFromPath(p string) string {
	// strip leading dirs
	base := p
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			base = p[i+1:]
			break
		}
	}
	// strip extension
	for i := len(base) - 1; i >= 0; i-- {
		if base[i] == '.' {
			base = base[:i]
			break
		}
	}
	// replace dashes/underscores with spaces and title-case
	result := make([]byte, 0, len(base))
	capitalise := true
	for i := 0; i < len(base); i++ {
		c := base[i]
		if c == '-' || c == '_' {
			result = append(result, ' ')
			capitalise = true
			continue
		}
		if capitalise && c >= 'a' && c <= 'z' {
			c -= 32
			capitalise = false
		} else {
			capitalise = false
		}
		result = append(result, c)
	}
	return string(result)
}

// BuildBreadcrumb returns the breadcrumb path for a given document path.
func BuildBreadcrumb(nav []*models.NavigationNode, targetPath string) []string {
	crumbs := []string{}
	if findBreadcrumb(nav, targetPath, &crumbs) {
		return crumbs
	}
	return nil
}

func findBreadcrumb(nodes []*models.NavigationNode, target string, crumbs *[]string) bool {
	for _, n := range nodes {
		if n.Path == target {
			if n.Title != "" {
				*crumbs = append(*crumbs, n.Title)
			}
			return true
		}
		if len(n.Children) > 0 {
			prev := len(*crumbs)
			if n.Title != "" {
				*crumbs = append(*crumbs, n.Title)
			}
			if findBreadcrumb(n.Children, target, crumbs) {
				return true
			}
			*crumbs = (*crumbs)[:prev]
		}
	}
	return false
}
