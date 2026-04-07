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
}

// Proposal represents a pending suggested change.
type Proposal struct {
	ID          string
	Author      string
	StartIndex  int
	EndIndex    int
	Replacement string
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
				ID:          cmd.ID,
				Author:      cmd.Author,
				StartIndex:  cmd.StartIndex,
				EndIndex:    cmd.EndIndex,
				Replacement: cmd.Replacement,
			}
		case "accept", "reject":
			delete(s.Proposals, cmd.ID)
		}
		cs.mu.Unlock()

	case RoleChangeCmd:
		cs.mu.Lock()
		cs.getOrCreate(cmd.SessionCode).Roles[cmd.UserID] = cmd.Role
		cs.mu.Unlock()
	}

	return nil
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
