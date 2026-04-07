package ide

import (
	"fmt"
	"math/rand"
	"sync"
)

// Session represents a collaborative editing session
type Session struct {
	mu      sync.Mutex
	ID      string
	Code    string            // invite code e.g. "ABC-4829"
	Roles   map[string]string // userID -> "editor"|"proposer"|"viewer"
	Creator string            // userID of session creator
}

// SessionStore manages all active sessions
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]*Session // invite code -> session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*Session),
	}
}

// NewSession creates a new session and returns it
func (ss *SessionStore) NewSession(creatorID string) *Session {
	code := generateCode()
	s := &Session{
		ID:      code,
		Code:    code,
		Roles:   map[string]string{creatorID: "editor"},
		Creator: creatorID,
	}
	ss.mu.Lock()
	ss.sessions[code] = s
	ss.mu.Unlock()
	return s
}

// Get returns a session by invite code
func (ss *SessionStore) Get(code string) (*Session, bool) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	s, ok := ss.sessions[code]
	return s, ok
}

// SetRole sets a user's role in the session
func (s *Session) SetRole(userID, role string) error {
	if role != "editor" && role != "proposer" && role != "viewer" {
		return fmt.Errorf("invalid role: %s", role)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Roles[userID] = role
	return nil
}

// GetRole returns a user's role, defaulting to "viewer"
func (s *Session) GetRole(userID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if role, ok := s.Roles[userID]; ok {
		return role
	}
	return "viewer"
}

// generateCode produces a random invite code like "ABC-4829"
func generateCode() string {
	letters := "ABCDEFGHJKLMNPQRSTUVWXYZ"
	digits := "0123456789"
	code := ""
	for i := 0; i < 3; i++ {
		code += string(letters[rand.Intn(len(letters))])
	}
	code += "-"
	for i := 0; i < 4; i++ {
		code += string(digits[rand.Intn(len(digits))])
	}
	return code
}
