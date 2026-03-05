package chat

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/yourname/airmeet/internal/room"
)

// Message represents a stored chat message
type Message struct {
	FromPeerID  string `json:"fromPeerId"`
	DisplayName string `json:"displayName"`
	Content     string `json:"message"`
	Timestamp   int64  `json:"timestamp"`
}

// BroadcastMessage is the format sent to clients
type BroadcastMessage struct {
	Type       string `json:"type"`
	Message    string `json:"message"`
	FromPeerID string `json:"fromPeerId"`
	Timestamp  int64  `json:"timestamp"`
}

// History stores chat messages for a room
type History struct {
	messages []Message
	maxSize  int
	mu       sync.RWMutex
}

// NewHistory creates a new chat history
func NewHistory(maxSize int) *History {
	return &History{
		messages: make([]Message, 0),
		maxSize:  maxSize,
	}
}

// Add adds a message to the history
func (h *History) Add(msg Message) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.messages = append(h.messages, msg)

	// Trim if exceeded max size
	if len(h.messages) > h.maxSize {
		h.messages = h.messages[len(h.messages)-h.maxSize:]
	}
}

// GetAll returns all messages
func (h *History) GetAll() []Message {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]Message, len(h.messages))
	copy(result, h.messages)
	return result
}

// Service handles chat functionality
type Service struct {
	histories map[string]*History
	maxHistory int
	mu        sync.RWMutex
}

// NewService creates a new chat service
func NewService(maxHistory int) *Service {
	return &Service{
		histories:  make(map[string]*History),
		maxHistory: maxHistory,
	}
}

// getOrCreateHistory gets or creates a history for a room
func (s *Service) getOrCreateHistory(roomID string) *History {
	s.mu.Lock()
	defer s.mu.Unlock()

	if h, exists := s.histories[roomID]; exists {
		return h
	}

	h := NewHistory(s.maxHistory)
	s.histories[roomID] = h
	return h
}

// RemoveHistory removes the history for a room
func (s *Service) RemoveHistory(roomID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.histories, roomID)
}

// HandleMessage handles an incoming chat message and broadcasts it
func (s *Service) HandleMessage(peer *room.Peer, r *room.Room, content string) {
	timestamp := time.Now().UnixMilli()

	// Store in history
	msg := Message{
		FromPeerID:  peer.ID,
		DisplayName: peer.DisplayName,
		Content:     content,
		Timestamp:   timestamp,
	}
	s.getOrCreateHistory(r.ID).Add(msg)

	// Create broadcast message
	chatMsg := BroadcastMessage{
		Type:       "chat",
		Message:    content,
		FromPeerID: peer.ID,
		Timestamp:  timestamp,
	}

	data, err := json.Marshal(chatMsg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal chat message")
		return
	}

	// Broadcast to all peers in the room
	for _, p := range r.GetPeers() {
		if err := p.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Error().
				Str("peer", p.ID).
				Err(err).
				Msg("Failed to send chat message")
		}
	}

	log.Debug().
		Str("room", r.ID).
		Str("from", peer.ID).
		Str("message", content).
		Msg("Chat message broadcast")
}

// SendHistory sends chat history to a peer
func (s *Service) SendHistory(peer *room.Peer, roomID string) {
	history := s.getOrCreateHistory(roomID)
	messages := history.GetAll()

	for _, msg := range messages {
		chatMsg := BroadcastMessage{
			Type:       "chat",
			Message:    msg.Content,
			FromPeerID: msg.FromPeerID,
			Timestamp:  msg.Timestamp,
		}

		data, err := json.Marshal(chatMsg)
		if err != nil {
			continue
		}

		peer.Conn.WriteMessage(websocket.TextMessage, data)
	}
}
