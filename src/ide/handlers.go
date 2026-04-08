package ide

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/svelez1129/collaborative-ide/src/rpc"
	"github.com/svelez1129/collaborative-ide/src/rsm"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Server struct {
	hub        *Hub
	collab     *CollabServer
	rsm        *rsm.RSM // nil in single-node mode
	leaderPort int      // base HTTP port; used in redirect messages (0 = single-node)
}

func NewServer(hub *Hub, collab *CollabServer, r *rsm.RSM) *Server {
	return &Server{hub: hub, collab: collab, rsm: r}
}

// submit routes a command through Raft consensus when an RSM is configured,
// or falls back to direct DoOp in single-node mode.
// Returns false if this node is not the Raft leader — the caller should send
// a redirect to the client.
func (s *Server) submit(cmd any) bool {
	if s.rsm == nil {
		s.collab.DoOp(cmd)
		return true
	}
	err, _ := s.rsm.Submit(cmd)
	return err == rpc.OK
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/create", s.handleCreate)
	mux.HandleFunc("/join", s.handleJoin)
	mux.HandleFunc("/run", s.handleRun)
	mux.Handle("/", http.FileServer(http.Dir("src/ide/frontend/dist")))
}

// handleCreate creates a new session and returns the invite code.
// The session is replicated through Raft so all nodes know about it.
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
	code := generateCode()
	if !s.submit(SessionCmd{Action: "create", Code: code, UserID: req.UserID, Role: "editor"}) {
		http.Error(w, "not the leader", http.StatusServiceUnavailable)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{
		"code": code,
		"role": "editor",
	})
}

// handleJoin validates an invite code and returns the user's role.
// The join is replicated through Raft so the user's role is consistent.
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
	if !s.collab.SessionExists(req.Code) {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if !s.submit(SessionCmd{Action: "join", Code: req.Code, UserID: req.UserID, Role: "viewer"}) {
		http.Error(w, "not the leader", http.StatusServiceUnavailable)
		return
	}
	role := s.collab.GetRole(req.Code, req.UserID)
	json.NewEncoder(w).Encode(map[string]string{
		"code": req.Code,
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
	// proposal CRDT fields
	OriginalText string `json:"originalText"`
	RelStart     string `json:"relStart"`
	RelEnd       string `json:"relEnd"`
	StartLine    int    `json:"startLine"`
	EndLine      int    `json:"endLine"`
	// sync routing fields
	From string `json:"from"` // sender userID (sync_sv)
	To   string `json:"to"`   // target userID (sync_update)
}

// handleWS upgrades to WebSocket, then routes binary (Yjs) and text (JSON) frames.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	code := r.URL.Query().Get("code")
	if userID == "" || code == "" {
		http.Error(w, "missing user_id or code", http.StatusBadRequest)
		return
	}
	if !s.collab.SessionExists(code) {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &Client{
		ID:   userID,
		Role: s.collab.GetRole(code, userID),
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
	if doc := s.collab.GetDoc(code); len(doc) > 0 {
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
				if !s.submit(CRDTUpdateCmd{SessionCode: code, Update: msg}) {
					s.sendRedirect(conn)
					return
				}
				s.hub.BroadcastSession(WSMessage{Binary: true, Data: msg}, userID, code)
			}

		case websocket.TextMessage:
			var m incomingMsg
			if err := json.Unmarshal(msg, &m); err != nil {
				continue
			}
			switch m.Type {
			case "proposal":
				s.handleProposalMsg(client, m)
			case "role_change":
				s.handleRoleChangeMsg(client, m)
			case "run_result":
				s.hub.BroadcastSession(WSMessage{Binary: false, Data: msg}, userID, code)
			case "awareness":
				s.hub.BroadcastSession(WSMessage{Binary: false, Data: msg}, userID, code)
			case "sync_sv":
				// Relay state vector to all other clients — they'll respond with the diff.
				s.hub.BroadcastSession(WSMessage{Binary: false, Data: msg}, userID, code)
			case "sync_update":
				// Route the diff to the specific client that requested it.
				if m.To != "" {
					s.hub.SendRaw(m.To, msg)
				}
			}
		}
	}
}

func (s *Server) handleProposalMsg(client *Client, m incomingMsg) {
	switch m.Action {
	case "add":
		if client.Role != "proposer" && client.Role != "editor" {
			return
		}
		proposal := Proposal{
			ID:           m.ID,
			Author:       client.ID,
			Replacement:  m.Replacement,
			OriginalText: m.OriginalText,
			RelStart:     m.RelStart,
			RelEnd:       m.RelEnd,
			StartLine:    m.StartLine,
			EndLine:      m.EndLine,
		}
		if !s.submit(ProposalCmd{
			SessionCode:  client.Code,
			ID:           proposal.ID,
			Author:       proposal.Author,
			Replacement:  proposal.Replacement,
			OriginalText: proposal.OriginalText,
			RelStart:     proposal.RelStart,
			RelEnd:       proposal.RelEnd,
			StartLine:    proposal.StartLine,
			EndLine:      proposal.EndLine,
			Action:       "add",
		}) {
			s.sendRedirect(client.Conn)
			return
		}
		// Broadcast the new proposal to all clients in session.
		out, _ := json.Marshal(map[string]any{
			"type":   "proposal",
			"action": "add",
			"proposal": map[string]any{
				"id":           proposal.ID,
				"author":       proposal.Author,
				"replacement":  proposal.Replacement,
				"originalText": proposal.OriginalText,
				"relStart":     proposal.RelStart,
				"relEnd":       proposal.RelEnd,
				"startLine":    proposal.StartLine,
				"endLine":      proposal.EndLine,
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
		if !s.submit(ProposalCmd{SessionCode: client.Code, ID: m.Proposal.ID, Action: m.Action}) {
			s.sendRedirect(client.Conn)
			return
		}
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

func (s *Server) handleRoleChangeMsg(client *Client, m incomingMsg) {
	if client.Role != "editor" {
		return
	}
	if m.Role != "editor" && m.Role != "proposer" && m.Role != "viewer" {
		return
	}
	s.hub.UpdateClientRole(m.UserID, m.Role)
	if !s.submit(RoleChangeCmd{SessionCode: client.Code, UserID: m.UserID, Role: m.Role}) {
		s.sendRedirect(client.Conn)
		return
	}

	// Notify all clients of the role change, then send updated participant list.
	out, _ := json.Marshal(map[string]any{
		"type":   "role_change",
		"userID": m.UserID,
		"role":   m.Role,
	})
	s.hub.BroadcastSessionAll(WSMessage{Binary: false, Data: out}, client.Code)
	s.broadcastParticipants(client.Code)
}

// sendRedirect tells a client it reached a non-leader node and should retry.
// The client will reconnect to the leader's HTTP port.
func (s *Server) sendRedirect(conn *websocket.Conn) {
	out, _ := json.Marshal(map[string]any{
		"type": "redirect",
		"port": s.leaderPort,
	})
	conn.WriteMessage(websocket.TextMessage, out)
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
