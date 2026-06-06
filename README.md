# EERP Docs MCP Server

A production-ready **Model Context Protocol (MCP) server** written in Go that exposes your [MkDocs](https://www.mkdocs.org/) documentation as a first-class AI knowledge base.

It indexes your Markdown source files at startup, keeps them in memory, and serves structured search + retrieval tools to any MCP-compatible AI client (Claude Desktop, Cursor, VS Code, etc.).

---

## Features

- **Zero external dependencies** – no database, no vector store, no embeddings
- **Fully in-memory** – sub-millisecond search over thousands of pages
- **Ranked full-text search** with fuzzy matching and contextual excerpts
- **Symbol search** – finds API names, entities, middleware, ORM concepts
- **Live reload** – watches `docs/` and `mkdocs.yml`; re-indexes on change
- **Dual transport** – SSE (legacy) + Streamable HTTP (MCP 2025-11-25+) + stdio
- **Six MCP tools** – search, read, list, get full docs, list sections, symbol search
- **Docker-ready** – single binary, minimal Alpine image

---

## Project structure

```
cmd/server/          – binary entry point
internal/
  config/            – YAML config loading
  models/            – shared data types (Document, NavigationNode, SearchResult)
  parser/            – mkdocs.yml parser and breadcrumb builder
  markdown/          – Markdown parser (frontmatter, headings, links, keywords)
  cache/             – thread-safe in-memory document store
  search/            – weighted full-text search engine + Levenshtein fuzzy matching
  indexer/           – orchestrates the full mkdocs→documents→cache pipeline
  watcher/           – fsnotify-based live-reload watcher
  mcp/               – MCP SDK server + all tool handlers
configs/             – default config.yaml
Dockerfile
docker-compose.yml
```

---

## Build

**Requirements:** Go 1.24+

```bash
git clone https://github.com/elnoia/eerp-mcp-server
cd eerp-mcp-server
go mod tidy
go build -o eerp-mcp-server ./cmd/server
```

Run tests:

```bash
go test ./...
```

---

## Configuration

Copy and edit the provided template:

```bash
cp configs/config.yaml my-config.yaml
```

```yaml
server:
  transport: sse          # sse | streamable-http | stdio
  address: ":8080"
  name: eerp-docs-mcp
  version: "1.0.0"

docs:
  root: "/path/to/your/docs"
  mkdocs: "/path/to/mkdocs.yml"

search:
  max_results: 10
```

| Key | Default | Description |
|---|---|---|
| `server.transport` | `sse` | `sse` for HTTP clients, `stdio` for Claude Desktop |
| `server.address` | `:8080` | TCP listen address |
| `docs.root` | `./docs` | Markdown source directory |
| `docs.mkdocs` | `./mkdocs.yml` | MkDocs config file |
| `search.max_results` | `10` | Default search result limit |

---

## Running locally

```bash
# Point at your MkDocs project
./eerp-mcp-server \
  --config configs/config.yaml \
  # or override with flags:
  # --log-level debug

# stdio mode (for Claude Desktop)
./eerp-mcp-server --stdio
```

The server logs structured JSON to stdout. On startup it will print the number of indexed documents and the time taken.

---

## Docker usage

```bash
# Build and start, mounting your MkDocs project
MKDOCS_PROJECT_PATH=/path/to/your/mkdocs-project docker compose up -d

# View logs
docker compose logs -f

# Health check
curl http://localhost:8080/health
```

The compose file expects a `MKDOCS_PROJECT_PATH` environment variable pointing to the directory that contains both `docs/` and `mkdocs.yml`.

---

## MCP tools reference

| Tool | Description |
|---|---|
| `search_docs` | Full-text ranked search, returns title + score + excerpt |
| `read_page` | Returns the complete raw Markdown for a given path |
| `list_pages` | Returns the full navigation tree from mkdocs.yml |
| `list_sections` | Returns top-level sections with their child pages |
| `search_symbols` | Searches identifiers, API names, entity names, ORM concepts |
| `get_full_documentation` | Concatenates all pages in nav order (for small doc sets) |

### search_docs

```json
{
  "query": "orm relations",
  "limit": 5
}
```

Returns:
```json
[
  {
    "title": "ORM Relations",
    "path": "orm/relations.md",
    "score": 62.5,
    "excerpt": "…Define relations between entities using the ORM…",
    "matches": ["orm", "relations"]
  }
]
```

### read_page

```json
{ "path": "orm/relations.md" }
```

Returns the raw Markdown plus metadata (title, breadcrumb, tags, section).

### search_symbols

```json
{ "symbol": "UserEntity", "limit": 5 }
```

---

## AI client configuration

### Claude Desktop (stdio)

Edit `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "eerp-docs": {
      "command": "/absolute/path/to/eerp-mcp-server",
      "args": ["--stdio", "--config", "/absolute/path/to/configs/config.yaml"]
    }
  }
}
```

### Cursor

Open **Settings → MCP → Add server** and enter:

```json
{
  "eerp-docs": {
    "url": "http://localhost:8080/sse"
  }
}
```

Or for Streamable HTTP:
```json
{
  "eerp-docs": {
    "url": "http://localhost:8080/mcp"
  }
}
```

### VS Code (Continue extension)

In `.continuerc.json`:

```json
{
  "experimental": {
    "modelContextProtocolServers": [
      {
        "transport": {
          "type": "sse",
          "url": "http://localhost:8080/sse"
        }
      }
    ]
  }
}
```

---

## Extending the server

The architecture uses interfaces throughout. To add a new documentation provider (OpenAPI, GoDoc, Git repositories, SQL schemas):

1. Implement `indexer.Provider` (parse source → `[]*models.Document`)
2. Register the provider in `indexer.New`
3. The MCP tools work unchanged since they only read from `cache.Cache`

---

## License

MIT
