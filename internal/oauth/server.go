package oauth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/elnoia/eerp-mcp-server/internal/config"
)

// Server is a self-contained OAuth 2.0 authorization server.
// It implements dynamic client registration (RFC 7591), PKCE (RFC 7636),
// and the authorization-code grant (RFC 6749 §4.1).
type Server struct {
	cfg   *config.AuthConfig
	store *store
}

// New returns an OAuth Server, or nil if auth is disabled in cfg.
func New(cfg *config.AuthConfig) *Server {
	if !cfg.Enabled {
		return nil
	}
	return &Server{cfg: cfg, store: newStore()}
}

// Mount registers all OAuth endpoints on mux.
func (s *Server) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.handleMetadata)
	mux.HandleFunc("POST /register", s.handleRegister)
	mux.HandleFunc("GET /authorize", s.handleAuthorizeGet)
	mux.HandleFunc("POST /authorize", s.handleAuthorizePost)
	mux.HandleFunc("POST /token", s.handleToken)
}

// Middleware returns an http.Handler that enforces Bearer token auth before next.
// If srv is nil (auth disabled) it returns next unchanged.
func Middleware(srv *Server, next http.Handler) http.Handler {
	if srv == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearer(r)
		if token == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="EERP-MCP"`)
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		if _, ok := srv.store.validateToken(token); !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="EERP-MCP", error="invalid_token"`)
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// StartCleanup launches a background goroutine that prunes expired store entries
// every 5 minutes. It stops when ctx is cancelled.
func (s *Server) StartCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.store.cleanup()
			}
		}
	}()
}

// issuerBase returns the base URL to use in OAuth metadata.
// It prefers the configured Issuer, then falls back to deriving it from the request.
func (s *Server) issuerBase(r *http.Request) string {
	if s.cfg.Issuer != "" {
		return strings.TrimRight(s.cfg.Issuer, "/")
	}
	scheme := "https"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS == nil {
		scheme = "http"
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	return scheme + "://" + host
}

func extractBearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}
