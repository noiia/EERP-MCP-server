package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ── RFC 8414 — OAuth Server Metadata ─────────────────────────────────────

type serverMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
}

func (s *Server) handleMetadata(w http.ResponseWriter, r *http.Request) {
	base := s.issuerBase(r)
	meta := serverMetadata{
		Issuer:                            base,
		AuthorizationEndpoint:             base + "/authorize",
		TokenEndpoint:                     base + "/token",
		RegistrationEndpoint:              base + "/register",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code"},
		CodeChallengeMethodsSupported:     []string{"S256"},
		TokenEndpointAuthMethodsSupported: []string{"none"},
	}
	writeJSON(w, http.StatusOK, meta)
}

// ── RFC 7591 — Dynamic Client Registration ────────────────────────────────

type registrationRequest struct {
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
}

type registrationResponse struct {
	ClientID     string   `json:"client_id"`
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		oauthErr(w, http.StatusBadRequest, "invalid_request", "malformed request body")
		return
	}
	if len(req.RedirectURIs) == 0 {
		oauthErr(w, http.StatusBadRequest, "invalid_request", "redirect_uris is required")
		return
	}
	name := req.ClientName
	if name == "" {
		name = "unnamed-client"
	}
	c := s.store.registerClient(name, req.RedirectURIs)
	writeJSON(w, http.StatusCreated, registrationResponse{
		ClientID:     c.ID,
		ClientName:   c.Name,
		RedirectURIs: c.RedirectURIs,
	})
}

// ── Authorization endpoint ────────────────────────────────────────────────

type loginData struct {
	ClientID            string
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Error               string
}

var loginTmpl = template.Must(template.New("login").Parse(loginHTML))

func (s *Server) handleAuthorizeGet(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	clientID := q.Get("client_id")
	responseType := q.Get("response_type")
	codeChallengeMethod := q.Get("code_challenge_method")

	if responseType != "code" {
		oauthErr(w, http.StatusBadRequest, "unsupported_response_type", "only 'code' is supported")
		return
	}
	if _, ok := s.store.getClient(clientID); !ok {
		oauthErr(w, http.StatusBadRequest, "invalid_client", "unknown client_id")
		return
	}
	if codeChallengeMethod != "" && codeChallengeMethod != "S256" {
		oauthErr(w, http.StatusBadRequest, "invalid_request", "only S256 PKCE is supported")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = loginTmpl.Execute(w, loginData{
		ClientID:            clientID,
		RedirectURI:         q.Get("redirect_uri"),
		State:               q.Get("state"),
		CodeChallenge:       q.Get("code_challenge"),
		CodeChallengeMethod: codeChallengeMethod,
	})
}

func (s *Server) handleAuthorizePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthErr(w, http.StatusBadRequest, "invalid_request", "bad form data")
		return
	}

	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	state := r.FormValue("state")
	codeChallenge := r.FormValue("code_challenge")
	codeChallengeMethod := r.FormValue("code_challenge_method")
	username := r.FormValue("username")
	password := r.FormValue("password")

	if _, ok := s.store.getClient(clientID); !ok {
		oauthErr(w, http.StatusBadRequest, "invalid_client", "unknown client_id")
		return
	}

	if !s.checkCredentials(username, password) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_ = loginTmpl.Execute(w, loginData{
			ClientID:            clientID,
			RedirectURI:         redirectURI,
			State:               state,
			CodeChallenge:       codeChallenge,
			CodeChallengeMethod: codeChallengeMethod,
			Error:               "Invalid username or password.",
		})
		return
	}

	code := randomHex(24)
	s.store.storeAuthCode(code, &AuthCode{
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		ExpiresAt:           time.Now().Add(5 * time.Minute),
	})

	target, err := url.Parse(redirectURI)
	if err != nil {
		oauthErr(w, http.StatusBadRequest, "invalid_request", "invalid redirect_uri")
		return
	}
	q := target.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}
	target.RawQuery = q.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

// ── Token endpoint ────────────────────────────────────────────────────────

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthErr(w, http.StatusBadRequest, "invalid_request", "bad form data")
		return
	}
	if r.FormValue("grant_type") != "authorization_code" {
		oauthErr(w, http.StatusBadRequest, "unsupported_grant_type", "only authorization_code is supported")
		return
	}

	code := r.FormValue("code")
	clientID := r.FormValue("client_id")
	codeVerifier := r.FormValue("code_verifier")

	ac, ok := s.store.consumeAuthCode(code)
	if !ok || time.Now().After(ac.ExpiresAt) {
		oauthErr(w, http.StatusBadRequest, "invalid_grant", "code expired or invalid")
		return
	}
	if ac.ClientID != clientID {
		oauthErr(w, http.StatusBadRequest, "invalid_client", "client_id mismatch")
		return
	}
	if ac.CodeChallenge != "" {
		if codeVerifier == "" {
			oauthErr(w, http.StatusBadRequest, "invalid_grant", "code_verifier required")
			return
		}
		if !verifyPKCE(codeVerifier, ac.CodeChallenge) {
			oauthErr(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
			return
		}
	}

	ttl := time.Duration(s.cfg.TokenTTL) * time.Second
	if ttl == 0 {
		ttl = time.Hour
	}
	accessToken := randomHex(32)
	s.store.storeToken(accessToken, &Token{
		ClientID:  clientID,
		ExpiresAt: time.Now().Add(ttl),
	})

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(ttl.Seconds()),
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────

func (s *Server) checkCredentials(username, password string) bool {
	if username != s.cfg.Username {
		return false
	}
	if s.cfg.PasswordHash != "" {
		return bcrypt.CompareHashAndPassword([]byte(s.cfg.PasswordHash), []byte(password)) == nil
	}
	return s.cfg.Password == password
}

// verifyPKCE checks that SHA256(verifier) == challenge (base64url, no padding).
func verifyPKCE(verifier, challenge string) bool {
	h := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	return computed == challenge
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func oauthErr(w http.ResponseWriter, status int, errCode, desc string) {
	writeJSON(w, status, map[string]string{
		"error":             errCode,
		"error_description": desc,
	})
}

// ── Login form ────────────────────────────────────────────────────────────

const loginHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>EERP Wiki — Sign in</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      background: #f4f5f7;
      display: flex; align-items: center; justify-content: center;
      min-height: 100vh;
    }
    .card {
      background: #fff;
      border-radius: 10px;
      box-shadow: 0 2px 16px rgba(0,0,0,.10);
      padding: 44px 40px;
      width: 360px;
    }
    h1 { font-size: 1.4rem; margin-bottom: 6px; color: #111; }
    .subtitle { color: #666; font-size: .875rem; margin-bottom: 28px; }
    .error {
      background: #fff0f0; border: 1px solid #fca5a5; border-radius: 6px;
      color: #b91c1c; font-size: .85rem; padding: 10px 12px; margin-bottom: 20px;
    }
    label { display: block; font-size: .8rem; font-weight: 600; color: #444; margin-bottom: 5px; }
    input[type=text], input[type=password] {
      width: 100%; padding: 10px 12px;
      border: 1px solid #d1d5db; border-radius: 7px;
      font-size: .95rem; margin-bottom: 18px;
      transition: border-color .15s, box-shadow .15s;
    }
    input[type=text]:focus, input[type=password]:focus {
      outline: none; border-color: #2563eb;
      box-shadow: 0 0 0 3px rgba(37,99,235,.18);
    }
    button {
      width: 100%; padding: 11px;
      background: #2563eb; color: #fff;
      border: none; border-radius: 7px;
      font-size: .95rem; font-weight: 600; cursor: pointer;
      transition: background .15s;
    }
    button:hover { background: #1d4ed8; }
  </style>
</head>
<body>
  <div class="card">
    <h1>EERP Wiki</h1>
    <p class="subtitle">Sign in to authorise MCP access</p>
    {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
    <form method="POST" action="/authorize">
      <input type="hidden" name="client_id"             value="{{.ClientID}}">
      <input type="hidden" name="redirect_uri"          value="{{.RedirectURI}}">
      <input type="hidden" name="state"                 value="{{.State}}">
      <input type="hidden" name="code_challenge"        value="{{.CodeChallenge}}">
      <input type="hidden" name="code_challenge_method" value="{{.CodeChallengeMethod}}">
      <label for="u">Username</label>
      <input id="u" type="text"     name="username" autocomplete="username"         required autofocus>
      <label for="p">Password</label>
      <input id="p" type="password" name="password" autocomplete="current-password" required>
      <button type="submit">Sign in</button>
    </form>
  </div>
</body>
</html>`
