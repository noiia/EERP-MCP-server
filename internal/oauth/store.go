// Package oauth implements an in-process OAuth 2.0 authorization server.
package oauth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Client is a dynamically registered OAuth client (RFC 7591).
type Client struct {
	ID           string
	Name         string
	RedirectURIs []string
}

// AuthCode is a short-lived authorization code issued after login.
type AuthCode struct {
	ClientID            string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	ExpiresAt           time.Time
}

// Token is an opaque access token stored in memory.
type Token struct {
	ClientID  string
	ExpiresAt time.Time
}

type store struct {
	mu        sync.RWMutex
	clients   map[string]*Client
	authCodes map[string]*AuthCode
	tokens    map[string]*Token
}

func newStore() *store {
	return &store{
		clients:   make(map[string]*Client),
		authCodes: make(map[string]*AuthCode),
		tokens:    make(map[string]*Token),
	}
}

func (s *store) registerClient(name string, redirectURIs []string) *Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := &Client{ID: randomHex(16), Name: name, RedirectURIs: redirectURIs}
	s.clients[c.ID] = c
	return c
}

func (s *store) getClient(id string) (*Client, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.clients[id]
	return c, ok
}

func (s *store) storeAuthCode(code string, ac *AuthCode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authCodes[code] = ac
}

// consumeAuthCode returns the code and deletes it (single-use).
func (s *store) consumeAuthCode(code string) (*AuthCode, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ac, ok := s.authCodes[code]
	if !ok {
		return nil, false
	}
	delete(s.authCodes, code)
	return ac, true
}

func (s *store) storeToken(token string, t *Token) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = t
}

func (s *store) validateToken(token string) (*Token, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tokens[token]
	if !ok || time.Now().After(t.ExpiresAt) {
		return nil, false
	}
	return t, true
}

// cleanup removes all expired auth codes and tokens.
func (s *store) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for code, ac := range s.authCodes {
		if now.After(ac.ExpiresAt) {
			delete(s.authCodes, code)
		}
	}
	for tok, t := range s.tokens {
		if now.After(t.ExpiresAt) {
			delete(s.tokens, tok)
		}
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
