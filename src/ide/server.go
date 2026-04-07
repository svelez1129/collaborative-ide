package ide

import (
	"bytes"
	"encoding/gob"
	"sync"
)

// ServerState is everything we need to snapshot
type ServerState struct {
	Doc       []byte            // latest Yjs document state
	Proposals map[string]Proposal
	Roles     map[string]string // userID -> role
}

// Proposal represents a pending suggested change
type Proposal struct {
	ID          string
	Author      string
	StartIndex  int
	EndIndex    int
	Replacement string
}

// CollabServer implements the StateMachine interface for the RSM
type CollabServer struct {
	mu    sync.Mutex
	state ServerState
	hub   *Hub
}

func NewCollabServer(hub *Hub) *CollabServer {
	return &CollabServer{
		state: ServerState{
			Doc:       []byte{},
			Proposals: make(map[string]Proposal),
			Roles:     make(map[string]string),
		},
		hub: hub,
	}
}

// DoOp is called by the RSM every time a command is committed by Raft
func (cs *CollabServer) DoOp(req any) any {
	switch cmd := req.(type) {

	case CRDTUpdateCmd:
		cs.mu.Lock()
		cs.state.Doc = cmd.Update
		cs.mu.Unlock()

	case ProposalCmd:
		cs.mu.Lock()
		switch cmd.Action {
		case "add":
			cs.state.Proposals[cmd.ID] = Proposal{
				ID:          cmd.ID,
				Author:      cmd.Author,
				StartIndex:  cmd.StartIndex,
				EndIndex:    cmd.EndIndex,
				Replacement: cmd.Replacement,
			}
		case "accept", "reject":
			delete(cs.state.Proposals, cmd.ID)
		}
		cs.mu.Unlock()

	case RoleChangeCmd:
		cs.mu.Lock()
		cs.state.Roles[cmd.UserID] = cmd.Role
		cs.mu.Unlock()
	}

	return nil
}

// Snapshot serializes the current state for Raft snapshotting
func (cs *CollabServer) Snapshot() []byte {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	enc.Encode(cs.state)
	return buf.Bytes()
}

// Restore deserializes a snapshot back into the server state
func (cs *CollabServer) Restore(data []byte) {
	var state ServerState
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&state); err != nil {
		return
	}
	cs.mu.Lock()
	cs.state = state
	cs.mu.Unlock()
}
