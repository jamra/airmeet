package room

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"sync"
)

// Manager handles room lifecycle
type Manager struct {
	rooms           map[string]*Room
	maxParticipants int
	mu              sync.RWMutex
}

// NewManager creates a new room manager
func NewManager(maxParticipants int) *Manager {
	return &Manager{
		rooms:           make(map[string]*Room),
		maxParticipants: maxParticipants,
	}
}

// CreateRoom creates a new room with an auto-generated password
func (m *Manager) CreateRoom() *Room {
	m.mu.Lock()
	defer m.mu.Unlock()

	roomID := m.generateRoomID()
	password := m.generatePassword()
	room := NewRoom(roomID, password, m.maxParticipants)
	m.rooms[roomID] = room
	return room
}

// GetOrCreateRoom gets an existing room or creates a new one
// Deprecated: Use CreateRoom for new rooms with passwords
func (m *Manager) GetOrCreateRoom(roomID string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()

	if room, exists := m.rooms[roomID]; exists {
		return room
	}

	password := m.generatePassword()
	room := NewRoom(roomID, password, m.maxParticipants)
	m.rooms[roomID] = room
	return room
}

// GetRoom gets a room by ID
func (m *Manager) GetRoom(roomID string) *Room {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rooms[roomID]
}

// RemoveRoom removes a room
func (m *Manager) RemoveRoom(roomID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rooms, roomID)
}

// RemoveRoomIfEmpty removes a room if it's empty
func (m *Manager) RemoveRoomIfEmpty(roomID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	room, exists := m.rooms[roomID]
	if !exists {
		return false
	}

	if room.IsEmpty() {
		delete(m.rooms, roomID)
		return true
	}
	return false
}

// GenerateRoomID generates a unique room ID
func (m *Manager) GenerateRoomID() string {
	return m.generateRoomID()
}

// generateRoomID generates a unique room ID (internal)
func (m *Manager) generateRoomID() string {
	bytes := make([]byte, 6)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// generatePassword generates a secure random password
func (m *Manager) generatePassword() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return base64.URLEncoding.EncodeToString(bytes)
}

// ListRooms returns all active rooms
func (m *Manager) ListRooms() []*Room {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rooms := make([]*Room, 0, len(m.rooms))
	for _, r := range m.rooms {
		rooms = append(rooms, r)
	}
	return rooms
}

// RoomCount returns the number of active rooms
func (m *Manager) RoomCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.rooms)
}
