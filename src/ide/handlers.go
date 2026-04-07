package ide

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Server struct {
	hub      *Hub
	sessions *SessionStore
	collab   *CollabServer
}

func NewServer(hub *Hub, sessions *SessionStore, collab *CollabServer) *Server {
	return &Server{hub: hub, sessions: sessions, collab: collab}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/create", s.handleCreate)
	mux.HandleFunc("/join", s.handleJoin)
	mux.HandleFunc("/run", s.handleRun)
	mux.Handle("/", http.FileServer(http.Dir("src/ide/frontend/dist")))
}

// handleCreate creates a new session and returns the invite code.
func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		http.Error(w, "missing user_id", http.StatusBadRequest)
		return
	}
	session := s.sessions.NewSession(req.UserID)
	json.NewEncoder(w).Encode(map[string]string{
		"code": session.Code,
		"role": "editor",
	})
}

// handleJoin validates an invite code and returns the user's role.
func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Code   string `json:"code"`
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	session, ok := s.sessions.Get(req.Code)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	role := session.GetRole(req.UserID)
	json.NewEncoder(w).Encode(map[string]string{
		"code": session.Code,
		"role": role,
	})
}

// incomingMsg is the shape of every JSON text frame from the client.
type incomingMsg struct {
	Type        string `json:"type"`
	// proposal fields (action: "add")
	Action      string `json:"action"`
	ID          string `json:"id"`
	Replacement string `json:"replacement"`
	StartIndex  int    `json:"startIndex"`
	EndIndex    int    `json:"endIndex"`
	// proposal fields (action: "accept" | "reject") — nested proposal object
	Proposal *struct {
		ID          string `json:"id"`
		Author      string `json:"author"`
		Replacement string `json:"replacement"`
		StartIndex  int    `json:"startIndex"`
		EndIndex    int    `json:"endIndex"`
	} `json:"proposal"`
	// role_change fields
	UserID string `json:"userID"`
	Role   string `json:"role"`
}

// handleWS upgrades to WebSocket, then routes binary (Yjs) and text (JSON) frames.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	code := r.URL.Query().Get("code")
	if userID == "" || code == "" {
		http.Error(w, "missing user_id or code", http.StatusBadRequest)
		return
	}
	session, ok := s.sessions.Get(code)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &Client{
		ID:   userID,
		Role: session.GetRole(userID),
		Code: code,
		Conn: conn,
		Send: make(chan WSMessage, 64),
	}
	s.hub.Register(client)
	defer func() {
		s.hub.Unregister(client)
		s.broadcastParticipants(code)
	}()

	// Send current doc state to the new client.
	s.collab.mu.Lock()
	doc := s.collab.state.Doc
	s.collab.mu.Unlock()
	if len(doc) > 0 {
		conn.WriteMessage(websocket.BinaryMessage, doc)
	}

	// Send the current participant list to the new client, then notify everyone.
	s.broadcastParticipants(code)

	go client.writePump()

	for {
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		switch mt {
		case websocket.BinaryMessage:
			// Raw Yjs update — only editors can push CRDT changes.
			if client.Role == "editor" {
				s.collab.DoOp(CRDTUpdateCmd{Update: msg})
				s.hub.BroadcastSession(WSMessage{Binary: true, Data: msg}, userID, code)
			}

		case websocket.TextMessage:
			var m incomingMsg
			if err := json.Unmarshal(msg, &m); err != nil {
				continue
			}
			switch m.Type {
			case "proposal":
				s.handleProposalMsg(client, session, m)
			case "role_change":
				s.handleRoleChangeMsg(client, session, m)
			case "run_result":
				// Relay run results to all other clients in the session as-is.
				s.hub.BroadcastSession(WSMessage{Binary: false, Data: msg}, userID, code)
			}
		}
	}
}

func (s *Server) handleProposalMsg(client *Client, session *Session, m incomingMsg) {
	switch m.Action {
	case "add":
		if client.Role != "proposer" && client.Role != "editor" {
			return
		}
		proposal := Proposal{
			ID:          m.ID,
			Author:      client.ID,
			StartIndex:  m.StartIndex,
			EndIndex:    m.EndIndex,
			Replacement: m.Replacement,
		}
		s.collab.DoOp(ProposalCmd{
			ID:          proposal.ID,
			Author:      proposal.Author,
			StartIndex:  proposal.StartIndex,
			EndIndex:    proposal.EndIndex,
			Replacement: proposal.Replacement,
			Action:      "add",
		})
		// Broadcast the new proposal to all clients in session.
		out, _ := json.Marshal(map[string]any{
			"type":   "proposal",
			"action": "add",
			"proposal": map[string]any{
				"id":          proposal.ID,
				"author":      proposal.Author,
				"startIndex":  proposal.StartIndex,
				"endIndex":    proposal.EndIndex,
				"replacement": proposal.Replacement,
			},
		})
		s.hub.BroadcastSessionAll(WSMessage{Binary: false, Data: out}, client.Code)

	case "accept", "reject":
		if client.Role != "editor" {
			return
		}
		if m.Proposal == nil {
			return
		}
		s.collab.DoOp(ProposalCmd{ID: m.Proposal.ID, Action: m.Action})
		out, _ := json.Marshal(map[string]any{
			"type":   "proposal",
			"action": m.Action,
			"proposal": map[string]any{
				"id": m.Proposal.ID,
			},
		})
		s.hub.BroadcastSessionAll(WSMessage{Binary: false, Data: out}, client.Code)
	}
}

func (s *Server) handleRoleChangeMsg(client *Client, session *Session, m incomingMsg) {
	if client.Role != "editor" {
		return
	}
	if err := session.SetRole(m.UserID, m.Role); err != nil {
		return
	}
	s.hub.UpdateClientRole(m.UserID, m.Role)
	s.collab.DoOp(RoleChangeCmd{UserID: m.UserID, Role: m.Role})

	// Notify all clients of the role change, then send updated participant list.
	out, _ := json.Marshal(map[string]any{
		"type":   "role_change",
		"userID": m.UserID,
		"role":   m.Role,
	})
	s.hub.BroadcastSessionAll(WSMessage{Binary: false, Data: out}, client.Code)
	s.broadcastParticipants(client.Code)
}

// broadcastParticipants sends the current participant list to all clients in the session.
func (s *Server) broadcastParticipants(code string) {
	list := s.hub.SessionParticipants(code)
	out, _ := json.Marshal(map[string]any{
		"type": "participants",
		"list": list,
	})
	s.hub.BroadcastSessionAll(WSMessage{Binary: false, Data: out}, code)
}

// handleRun proxies to the Go Playground API to avoid CORS issues.
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	form := url.Values{}
	form.Set("version", "2")
	form.Set("body", string(body))
	resp, err := http.Post(
		"https://go.dev/_/compile?backend=",
		"application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		http.Error(w, "playground unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}
