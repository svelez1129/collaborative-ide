package ide

import (
	"bytes"
	"encoding/gob"
	"sync"
)

// SessionState holds the replicated state for one collaborative session.
type SessionState struct {
	Doc       []byte            // latest Yjs document state
	Proposals map[string]Proposal
	Roles     map[string]string // userID -> role
	Creator   string            // userID of session creator
}

// Proposal represents a pending suggested change.
type Proposal struct {
	ID           string
	Author       string
	Replacement  string
	OriginalText string // text being replaced (empty = insertion)
	RelStart     string // base64 Yjs relative position
	RelEnd       string // base64 Yjs relative position
	StartLine    int    // 1-based start line (snapshot at submission)
	EndLine      int    // 1-based end line (snapshot at submission)
}

// CollabServer implements the StateMachine interface for the RSM.
// All state is keyed by session invite code.
type CollabServer struct {
	mu       sync.Mutex
	sessions map[string]*SessionState // code -> state
	hub      *Hub
}

func NewCollabServer(hub *Hub) *CollabServer {
	return &CollabServer{
		sessions: make(map[string]*SessionState),
		hub:      hub,
	}
}

// getOrCreate returns the state for a session, creating it if needed.
// Must be called with cs.mu held.
func (cs *CollabServer) getOrCreate(code string) *SessionState {
	if s, ok := cs.sessions[code]; ok {
		return s
	}
	s := &SessionState{
		Doc:       []byte{},
		Proposals: make(map[string]Proposal),
		Roles:     make(map[string]string),
	}
	cs.sessions[code] = s
	return s
}

// GetDoc returns the current Yjs doc for a session (nil if none yet).
func (cs *CollabServer) GetDoc(code string) []byte {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if s, ok := cs.sessions[code]; ok {
		return s.Doc
	}
	return nil
}

// DoOp is called by the RSM every time a command is committed by Raft.
func (cs *CollabServer) DoOp(req any) any {
	switch cmd := req.(type) {

	case CRDTUpdateCmd:
		cs.mu.Lock()
		cs.getOrCreate(cmd.SessionCode).Doc = cmd.Update
		cs.mu.Unlock()

	case ProposalCmd:
		cs.mu.Lock()
		s := cs.getOrCreate(cmd.SessionCode)
		switch cmd.Action {
		case "add":
			s.Proposals[cmd.ID] = Proposal{
				ID:           cmd.ID,
				Author:       cmd.Author,
				Replacement:  cmd.Replacement,
				OriginalText: cmd.OriginalText,
				RelStart:     cmd.RelStart,
				RelEnd:       cmd.RelEnd,
				StartLine:    cmd.StartLine,
				EndLine:      cmd.EndLine,
			}
		case "accept", "reject":
			delete(s.Proposals, cmd.ID)
		}
		cs.mu.Unlock()

	case RoleChangeCmd:
		cs.mu.Lock()
		cs.getOrCreate(cmd.SessionCode).Roles[cmd.UserID] = cmd.Role
		cs.mu.Unlock()

	case SessionCmd:
		cs.mu.Lock()
		switch cmd.Action {
		case "create":
			s := cs.getOrCreate(cmd.Code)
			s.Creator = cmd.UserID
			s.Roles[cmd.UserID] = "editor"
		case "join":
			// Only set role if the user has no existing role yet.
			s := cs.getOrCreate(cmd.Code)
			if _, exists := s.Roles[cmd.UserID]; !exists {
				s.Roles[cmd.UserID] = "viewer"
			}
		}
		cs.mu.Unlock()
	}

	return nil
}

// GetSession returns the role of userID in the given session, and whether
// the session exists. Used by HTTP handlers after replication.
func (cs *CollabServer) GetSession(code, userID string) (role string, exists bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	s, ok := cs.sessions[code]
	if !ok {
		return "", false
	}
	r, ok2 := s.Roles[userID]
	if !ok2 {
		return "viewer", true
	}
	return r, true
}

// SessionExists returns true if the session code is known to this node.
func (cs *CollabServer) SessionExists(code string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	_, ok := cs.sessions[code]
	return ok
}

// GetRole returns the role of userID in a session, defaulting to "viewer".
func (cs *CollabServer) GetRole(code, userID string) string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	s, ok := cs.sessions[code]
	if !ok {
		return "viewer"
	}
	if role, ok := s.Roles[userID]; ok {
		return role
	}
	return "viewer"
}

// Snapshot serializes all session states for Raft snapshotting.
func (cs *CollabServer) Snapshot() []byte {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	var buf bytes.Buffer
	gob.NewEncoder(&buf).Encode(cs.sessions)
	return buf.Bytes()
}

// Restore deserializes a snapshot back into the server.
func (cs *CollabServer) Restore(data []byte) {
	var sessions map[string]*SessionState
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&sessions); err != nil {
		return
	}
	cs.mu.Lock()
	cs.sessions = sessions
	cs.mu.Unlock()
}
