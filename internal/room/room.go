package room

import (
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

// Peer represents a participant in a room
type Peer struct {
	ID          string
	DisplayName string
	Conn        *websocket.Conn
	PC          *webrtc.PeerConnection
	Tracks      map[string]*webrtc.TrackLocalStaticRTP
	mu          sync.RWMutex
}

// NewPeer creates a new peer
func NewPeer(id, displayName string, conn *websocket.Conn) *Peer {
	return &Peer{
		ID:          id,
		DisplayName: displayName,
		Conn:        conn,
		Tracks:      make(map[string]*webrtc.TrackLocalStaticRTP),
	}
}

// SetPeerConnection sets the WebRTC peer connection
func (p *Peer) SetPeerConnection(pc *webrtc.PeerConnection) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.PC = pc
}

// GetPeerConnection gets the WebRTC peer connection
func (p *Peer) GetPeerConnection() *webrtc.PeerConnection {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.PC
}

// AddTrack adds a track to the peer
func (p *Peer) AddTrack(trackID string, track *webrtc.TrackLocalStaticRTP) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Tracks[trackID] = track
}

// RemoveTrack removes a track from the peer
func (p *Peer) RemoveTrack(trackID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.Tracks, trackID)
}

// GetTracks returns all tracks for the peer
func (p *Peer) GetTracks() map[string]*webrtc.TrackLocalStaticRTP {
	p.mu.RLock()
	defer p.mu.RUnlock()
	tracks := make(map[string]*webrtc.TrackLocalStaticRTP)
	for k, v := range p.Tracks {
		tracks[k] = v
	}
	return tracks
}

// Room represents a video conference room
type Room struct {
	ID              string
	Password        string
	HostPeerID      string
	Peers           map[string]*Peer
	MaxParticipants int
	mu              sync.RWMutex
}

// NewRoom creates a new room
func NewRoom(id, password string, maxParticipants int) *Room {
	return &Room{
		ID:              id,
		Password:        password,
		Peers:           make(map[string]*Peer),
		MaxParticipants: maxParticipants,
	}
}

// AddPeer adds a peer to the room
func (r *Room) AddPeer(peer *Peer) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.Peers) >= r.MaxParticipants {
		return false
	}

	r.Peers[peer.ID] = peer
	return true
}

// RemovePeer removes a peer from the room
func (r *Room) RemovePeer(peerID string) *Peer {
	r.mu.Lock()
	defer r.mu.Unlock()

	peer, exists := r.Peers[peerID]
	if !exists {
		return nil
	}

	delete(r.Peers, peerID)
	return peer
}

// GetPeer gets a peer by ID
func (r *Room) GetPeer(peerID string) *Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Peers[peerID]
}

// GetPeers returns all peers in the room
func (r *Room) GetPeers() []*Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	peers := make([]*Peer, 0, len(r.Peers))
	for _, p := range r.Peers {
		peers = append(peers, p)
	}
	return peers
}

// GetOtherPeers returns all peers except the specified one
func (r *Room) GetOtherPeers(excludePeerID string) []*Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	peers := make([]*Peer, 0, len(r.Peers)-1)
	for _, p := range r.Peers {
		if p.ID != excludePeerID {
			peers = append(peers, p)
		}
	}
	return peers
}

// IsEmpty returns true if the room has no peers
func (r *Room) IsEmpty() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Peers) == 0
}

// PeerCount returns the number of peers in the room
func (r *Room) PeerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Peers)
}

// ValidatePassword checks if the provided password matches
func (r *Room) ValidatePassword(password string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Password == password
}

// SetHost sets the host peer ID
func (r *Room) SetHost(peerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.HostPeerID == "" {
		r.HostPeerID = peerID
	}
}

// IsHost checks if the peer is the host
func (r *Room) IsHost(peerID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.HostPeerID == peerID
}

// GetPassword returns the room password
func (r *Room) GetPassword() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Password
}
