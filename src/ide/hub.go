package ide

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

// WSMessage wraps outgoing data with its frame type.
type WSMessage struct {
	Binary bool
	Data   []byte
}

// Client represents a connected WebSocket user.
type Client struct {
	ID   string
	Role string
	Code string // session invite code
	Conn *websocket.Conn
	Send chan WSMessage
}

// Hub manages all connected clients and broadcasts messages.
type Hub struct {
	mu      sync.Mutex
	clients map[string]*Client // userID -> client
}

func NewHub() *Hub {
	return &Hub{clients: make(map[string]*Client)}
}

func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c.ID] = c
}

// Unregister removes a client only if it is still the registered client for that ID.
// This prevents a reconnecting client from being torn down by a stale cleanup.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if current, ok := h.clients[c.ID]; ok && current == c {
		close(c.Send)
		delete(h.clients, c.ID)
	}
}

// BroadcastSession sends to all clients in the same session except the sender.
func (h *Hub) BroadcastSession(msg WSMessage, senderID, code string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, c := range h.clients {
		if id == senderID || c.Code != code {
			continue
		}
		select {
		case c.Send <- msg:
		default:
		}
	}
}

// BroadcastSessionAll sends to every client in the session including the sender.
func (h *Hub) BroadcastSessionAll(msg WSMessage, code string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.clients {
		if c.Code != code {
			continue
		}
		select {
		case c.Send <- msg:
		default:
		}
	}
}

// SessionParticipants returns the current participant list for a session.
func (h *Hub) SessionParticipants(code string) []map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var list []map[string]string
	for _, c := range h.clients {
		if c.Code == code {
			list = append(list, map[string]string{"id": c.ID, "role": c.Role})
		}
	}
	return list
}

// UpdateClientRole updates the in-memory role for a connected client.
func (h *Hub) UpdateClientRole(userID, role string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c, ok := h.clients[userID]; ok {
		c.Role = role
	}
}

// SendJSON encodes v as JSON and enqueues it as a text frame for one client.
func (h *Hub) SendJSON(userID string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.sendRaw(userID, data)
}

// SendRaw enqueues already-encoded JSON bytes as a text frame for one client.
func (h *Hub) SendRaw(userID string, data []byte) {
	h.sendRaw(userID, data)
}

func (h *Hub) sendRaw(userID string, data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c, ok := h.clients[userID]; ok {
		select {
		case c.Send <- WSMessage{Binary: false, Data: data}:
		default:
		}
	}
}

// writePump drains the Send channel to the WebSocket using the correct frame type.
func (c *Client) writePump() {
	defer c.Conn.Close()
	for msg := range c.Send {
		mt := websocket.TextMessage
		if msg.Binary {
			mt = websocket.BinaryMessage
		}
		if err := c.Conn.WriteMessage(mt, msg.Data); err != nil {
			return
		}
	}
}
