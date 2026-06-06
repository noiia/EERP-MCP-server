// Package mcp wires together the MCP SDK server with the documentation index.
package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/elnoia/eerp-mcp-server/internal/cache"
	"github.com/elnoia/eerp-mcp-server/internal/config"
	"github.com/elnoia/eerp-mcp-server/internal/oauth"
	"github.com/elnoia/eerp-mcp-server/internal/search"
)

// Server wraps the MCP SDK server and exposes the docs tools.
type Server struct {
	cfg      *config.Config
	cache    *cache.Cache
	engine   *search.Engine
	sdk      *sdkmcp.Server
	logger   *slog.Logger
	oauthSrv *oauth.Server
}

// New creates and configures an MCP Server, registering all documentation tools.
func New(cfg *config.Config, c *cache.Cache, e *search.Engine, logger *slog.Logger) *Server {
	s := &Server{
		cfg:      cfg,
		cache:    c,
		engine:   e,
		logger:   logger,
		oauthSrv: oauth.New(&cfg.Auth),
	}

	s.sdk = sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    cfg.Server.Name,
		Version: cfg.Server.Version,
	}, nil)

	registerTools(s)
	return s
}

// SDKServer returns the underlying MCP SDK server (needed for transport setup).
func (s *Server) SDKServer() *sdkmcp.Server {
	return s.sdk
}

// RunStdio starts the server on stdio (for Claude Desktop).
func (s *Server) RunStdio(ctx context.Context) error {
	s.logger.Info("starting MCP server", "transport", "stdio")
	return s.sdk.Run(ctx, &sdkmcp.StdioTransport{})
}

// HTTPHandler returns an http.Handler that supports both SSE and Streamable HTTP.
// OAuth endpoints are mounted when auth is enabled; MCP endpoints require a valid
// Bearer token in that case.
func (s *Server) HTTPHandler() http.Handler {
	mux := http.NewServeMux()

	if s.oauthSrv != nil {
		s.oauthSrv.Mount(mux)
		s.logger.Info("OAuth 2.0 enabled", "issuer", s.cfg.Auth.Issuer)
	}

	serverFactory := func(r *http.Request) *sdkmcp.Server { return s.sdk }

	// Streamable HTTP (MCP 2025-11-25+) — protected when auth is enabled.
	mux.Handle("/mcp", oauth.Middleware(s.oauthSrv, sdkmcp.NewStreamableHTTPHandler(serverFactory, nil)))

	// Legacy SSE transport (Claude Desktop HTTP, older Cursor versions).
	mux.Handle("/sse", oauth.Middleware(s.oauthSrv, sdkmcp.NewSSEHandler(serverFactory, nil)))

	// Health probe — always public.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","documents":%d}`, s.cache.Size())
	})

	return corsMiddleware(mux)
}

// RunHTTP starts the HTTP server and blocks until ctx is cancelled.
func (s *Server) RunHTTP(ctx context.Context) error {
	addr := s.cfg.Server.Address
	s.logger.Info("starting MCP server", "transport", s.cfg.Server.Transport, "address", addr)

	if s.oauthSrv != nil {
		s.oauthSrv.StartCleanup(ctx)
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: s.HTTPHandler(),
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		return srv.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

// corsMiddleware adds permissive CORS headers for browser-based MCP clients.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
