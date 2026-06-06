package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/elnoia/eerp-mcp-server/internal/models"
)

// registerTools adds all documentation tools to the MCP server.
func registerTools(s *Server) {
	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{
		Name:        "search_docs",
		Description: "Search the documentation by keyword or phrase. Returns ranked results with excerpts.",
	}, s.handleSearchDocs)

	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{
		Name:        "read_page",
		Description: "Return the full Markdown content of a documentation page by its relative path.",
	}, s.handleReadPage)

	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{
		Name:        "list_pages",
		Description: "Return the complete documentation navigation tree with titles and paths.",
	}, s.handleListPages)

	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{
		Name:        "list_sections",
		Description: "Return the top-level documentation sections as defined in mkdocs.yml.",
	}, s.handleListSections)

	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{
		Name:        "search_symbols",
		Description: "Search for API names, entity types, ORM concepts, middleware names, functions, or specific identifiers.",
	}, s.handleSearchSymbols)

	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{
		Name:        "get_full_documentation",
		Description: "Return the entire documentation concatenated in navigation order. Suitable for small doc sets.",
	}, s.handleGetFullDocumentation)
}

// ── Tool input/output types ────────────────────────────────────────────────

// navPageRef is a lightweight page reference used in list_sections output.
type navPageRef struct {
	Title string `json:"title"`
	Path  string `json:"path"`
}

// sectionItem groups a section with its child page references.
type sectionItem struct {
	Title string       `json:"title"`
	Pages []navPageRef `json:"pages"`
}

type SearchDocsInput struct {
	Query string `json:"query" jsonschema:"Search query (keywords or phrase)"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of results (default: 10)"`
}

type ReadPageInput struct {
	Path string `json:"path" jsonschema:"Relative path of the page e.g. orm/relations.md"`
}

type ListPagesInput struct{}

type ListSectionsInput struct{}

type SearchSymbolsInput struct {
	Symbol string `json:"symbol" jsonschema:"API name, entity, function, class, middleware, or ORM concept"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of results (default: 10)"`
}

type GetFullDocumentationInput struct{}

// ── Handlers ──────────────────────────────────────────────────────────────

func (s *Server) handleSearchDocs(
	ctx context.Context,
	req *sdkmcp.CallToolRequest,
	args SearchDocsInput,
) (*sdkmcp.CallToolResult, any, error) {
	start := time.Now()

	limit := args.Limit
	if limit <= 0 {
		limit = s.cfg.Search.MaxResults
	}

	results := s.engine.Search(args.Query, limit)
	s.logger.Info("search_docs",
		"query", args.Query,
		"results", len(results),
		"duration", time.Since(start),
	)

	if len(results) == 0 {
		return textResult(fmt.Sprintf("No results found for %q.", args.Query)), nil, nil
	}

	type resultItem struct {
		Title   string  `json:"title"`
		Path    string  `json:"path"`
		Score   float64 `json:"score"`
		Excerpt string  `json:"excerpt"`
		Matches []string `json:"matches,omitempty"`
	}

	items := make([]resultItem, len(results))
	for i, r := range results {
		items[i] = resultItem{
			Title:   r.Document.Title,
			Path:    r.Document.Path,
			Score:   roundScore(r.Score),
			Excerpt: r.Excerpt,
			Matches: r.Matches,
		}
	}

	return jsonResult(items)
}

func (s *Server) handleReadPage(
	ctx context.Context,
	req *sdkmcp.CallToolRequest,
	args ReadPageInput,
) (*sdkmcp.CallToolResult, any, error) {
	if args.Path == "" {
		return errorResult("path is required"), nil, nil
	}

	// Normalise path (strip leading slash, ensure .md extension).
	p := strings.TrimPrefix(args.Path, "/")
	if !strings.HasSuffix(p, ".md") {
		p += ".md"
	}

	doc, ok := s.cache.GetDocument(p)
	if !ok {
		return errorResult(fmt.Sprintf("page not found: %q", args.Path)), nil, nil
	}

	s.logger.Info("read_page", "path", p)

	type pageResponse struct {
		Title      string   `json:"title"`
		Path       string   `json:"path"`
		Breadcrumb []string `json:"breadcrumb,omitempty"`
		Section    string   `json:"section,omitempty"`
		Tags       []string `json:"tags,omitempty"`
		Content    string   `json:"content"`
	}

	resp := pageResponse{
		Title:      doc.Title,
		Path:       doc.Path,
		Breadcrumb: doc.Breadcrumb,
		Section:    doc.Section,
		Tags:       doc.Tags,
		Content:    doc.RawContent,
	}
	return jsonResult(resp)
}

func (s *Server) handleListPages(
	ctx context.Context,
	req *sdkmcp.CallToolRequest,
	_ ListPagesInput,
) (*sdkmcp.CallToolResult, any, error) {
	nav := s.cache.Navigation()
	s.logger.Info("list_pages", "nodes", len(nav))
	return jsonResult(nav)
}

func (s *Server) handleListSections(
	ctx context.Context,
	req *sdkmcp.CallToolRequest,
	_ ListSectionsInput,
) (*sdkmcp.CallToolResult, any, error) {
	s.logger.Info("list_sections", "sections", len(s.cache.Sections()))

	nav := s.cache.Navigation()
	result := make([]sectionItem, 0, len(nav))
	for _, node := range nav {
		item := sectionItem{Title: node.Title}
		if node.Path != "" {
			item.Pages = append(item.Pages, navPageRef{Title: node.Title, Path: node.Path})
		}
		for _, child := range node.Children {
			collectPageRefs(child, &item.Pages)
		}
		result = append(result, item)
	}

	return jsonResult(result)
}

func collectPageRefs(node *models.NavigationNode, out *[]navPageRef) {
	if node.Path != "" {
		*out = append(*out, navPageRef{Title: node.Title, Path: node.Path})
	}
	for _, c := range node.Children {
		collectPageRefs(c, out)
	}
}

func (s *Server) handleSearchSymbols(
	ctx context.Context,
	req *sdkmcp.CallToolRequest,
	args SearchSymbolsInput,
) (*sdkmcp.CallToolResult, any, error) {
	start := time.Now()

	limit := args.Limit
	if limit <= 0 {
		limit = s.cfg.Search.MaxResults
	}

	results := s.engine.SearchSymbols(args.Symbol, limit)
	s.logger.Info("search_symbols",
		"symbol", args.Symbol,
		"results", len(results),
		"duration", time.Since(start),
	)

	if len(results) == 0 {
		return textResult(fmt.Sprintf("No symbols found for %q.", args.Symbol)), nil, nil
	}

	type symbolResult struct {
		Title   string `json:"title"`
		Path    string `json:"path"`
		Section string `json:"section,omitempty"`
		Excerpt string `json:"excerpt"`
	}

	items := make([]symbolResult, len(results))
	for i, r := range results {
		items[i] = symbolResult{
			Title:   r.Document.Title,
			Path:    r.Document.Path,
			Section: r.Document.Section,
			Excerpt: r.Excerpt,
		}
	}
	return jsonResult(items)
}

func (s *Server) handleGetFullDocumentation(
	ctx context.Context,
	req *sdkmcp.CallToolRequest,
	_ GetFullDocumentationInput,
) (*sdkmcp.CallToolResult, any, error) {
	s.logger.Info("get_full_documentation")

	pathOrder := s.cache.PathOrder()
	var sb strings.Builder

	for _, path := range pathOrder {
		doc, ok := s.cache.GetDocument(path)
		if !ok {
			continue
		}
		sb.WriteString("# ")
		sb.WriteString(doc.Title)
		sb.WriteString("\n\n")
		sb.WriteString("**Path:** `")
		sb.WriteString(doc.Path)
		sb.WriteString("`\n\n")
		if len(doc.Breadcrumb) > 0 {
			sb.WriteString("**Section:** ")
			sb.WriteString(strings.Join(doc.Breadcrumb, " > "))
			sb.WriteString("\n\n")
		}
		sb.WriteString(doc.RawContent)
		sb.WriteString("\n\n---\n\n")
	}

	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: sb.String()},
		},
	}, nil, nil
}

// ── Helper utilities ───────────────────────────────────────────────────────

func textResult(text string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: text},
		},
	}
}

func errorResult(msg string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		IsError: true,
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: msg},
		},
	}
}

func jsonResult(v any) (*sdkmcp.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errorResult("failed to serialise result: " + err.Error()), nil, nil
	}
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: string(b)},
		},
	}, nil, nil
}

func roundScore(f float64) float64 {
	return float64(int(f*100)) / 100
}
