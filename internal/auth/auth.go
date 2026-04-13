package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alexedwards/argon2id"

	"smoll-url/internal/config"
)

const SessionCookieName = "smoll-url-auth"

type SessionStore struct {
	mu     sync.RWMutex
	tokens map[string]int64
}

func NewSessionStore() *SessionStore {
	return &SessionStore{tokens: make(map[string]int64)}
}

func (s *SessionStore) NewToken() string {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	token := hex.EncodeToString(raw)

	s.mu.Lock()
	s.tokens[token] = time.Now().UTC().Unix()
	s.mu.Unlock()

	return token
}

func (s *SessionStore) DeleteToken(token string) {
	s.mu.Lock()
	delete(s.tokens, token)
	s.mu.Unlock()
}

func (s *SessionStore) IsValid(token string) bool {
	if token == "" {
		return false
	}

	s.mu.RLock()
	issuedAt, ok := s.tokens[token]
	s.mu.RUnlock()
	if !ok {
		return false
	}

	now := time.Now().UTC().Unix()
	return now < issuedAt+1209600
}

type APIResult struct {
	Success bool   `json:"success"`
	Error   bool   `json:"error"`
	Reason  string `json:"reason"`
}

func IsAPIAuthorized(r *http.Request, cfg config.Config) APIResult {
	keyHeader := strings.TrimSpace(r.Header.Get("X-API-Key"))

	if cfg.APIKey == "" {
		if keyHeader != "" {
			return APIResult{Success: false, Error: true, Reason: "An API key was provided, but the 'api_key' environment variable is not configured in the smoll-url instance"}
		}
		return APIResult{Success: false, Error: false, Reason: ""}
	}

	if keyHeader == "" {
		return APIResult{Success: false, Error: false, Reason: "No valid authentication was found"}
	}

	if IsKeyValid(keyHeader, cfg) {
		return APIResult{Success: true, Error: false, Reason: "Correct API key"}
	}

	return APIResult{Success: false, Error: true, Reason: "Incorrect API key"}
}

func IsKeyValid(suppliedKey string, cfg config.Config) bool {
	if cfg.APIKey == "" {
		return false
	}

	if cfg.HashAlgorithm == "Argon2" {
		match, err := argon2id.ComparePasswordAndHash(suppliedKey, cfg.APIKey)
		return err == nil && match
	}

	return cfg.APIKey == suppliedKey
}

func IsPasswordValid(suppliedPassword string, cfg config.Config) bool {
	if cfg.Password == "" {
		return false
	}

	if cfg.HashAlgorithm == "Argon2" {
		match, err := argon2id.ComparePasswordAndHash(suppliedPassword, cfg.Password)
		return err == nil && match
	}

	return cfg.Password == suppliedPassword
}
