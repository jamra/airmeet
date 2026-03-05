package signaling

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog/log"
	"github.com/yourname/airmeet/internal/chat"
	"github.com/yourname/airmeet/internal/room"
	"github.com/yourname/airmeet/internal/sfu"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// Handler handles WebSocket signaling
type Handler struct {
	roomManager *room.Manager
	sfu         *sfu.SFU
	chat        *chat.Service
	mu          sync.RWMutex
}

// NewHandler creates a new signaling handler
func NewHandler(roomManager *room.Manager, sfu *sfu.SFU, chat *chat.Service) *Handler {
	return &Handler{
		roomManager: roomManager,
		sfu:         sfu,
		chat:        chat,
	}
}

// ServeHTTP handles WebSocket upgrade and connection
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("WebSocket upgrade failed")
		return
	}

	// Generate peer ID
	peerID := generatePeerID()
	log.Info().Str("peerId", peerID).Msg("New WebSocket connection")

	// Handle messages
	var currentPeer *room.Peer
	var currentRoom *room.Room

	defer func() {
		conn.Close()
		if currentPeer != nil && currentRoom != nil {
			h.handlePeerLeave(currentPeer, currentRoom)
		}
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().Err(err).Str("peerId", peerID).Msg("WebSocket error")
			}
			return
		}

		msg, err := ParseMessage(data)
		if err != nil {
			log.Error().Err(err).Msg("Failed to parse message")
			h.sendError(conn, "Invalid message format")
			continue
		}

		switch m := msg.(type) {
		case *JoinMessage:
			currentPeer, currentRoom = h.handleJoin(conn, peerID, m)

		case *OfferMessage:
			if currentPeer != nil {
				h.handleOffer(currentPeer, currentRoom, m)
			}

		case *AnswerMessage:
			if currentPeer != nil {
				h.handleAnswer(currentPeer, m)
			}

		case *ICECandidateMessage:
			if currentPeer != nil {
				h.handleICECandidate(currentPeer, m)
			}

		case *ChatMessage:
			if currentPeer != nil && currentRoom != nil {
				h.chat.HandleMessage(currentPeer, currentRoom, m.Message)
			}

		case *MuteMessage:
			if currentPeer != nil && currentRoom != nil {
				h.handleMute(currentPeer, currentRoom, m)
			}
		}
	}
}

// handleJoin processes a join request
func (h *Handler) handleJoin(conn *websocket.Conn, peerID string, msg *JoinMessage) (*room.Peer, *room.Room) {
	// Create peer
	peer := room.NewPeer(peerID, msg.DisplayName, conn)

	// Get or create room
	r := h.roomManager.GetOrCreateRoom(msg.RoomID)

	// Check room capacity
	if !r.AddPeer(peer) {
		h.sendError(conn, "Room is full")
		return nil, nil
	}

	log.Info().
		Str("peerId", peerID).
		Str("roomId", msg.RoomID).
		Str("displayName", msg.DisplayName).
		Msg("Peer joined room")

	// Create peer connection
	_, err := h.sfu.CreatePeerConnection(peer, r)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create peer connection")
		r.RemovePeer(peerID)
		h.sendError(conn, "Failed to create connection")
		return nil, nil
	}

	// Get existing peers
	existingPeers := make([]PeerInfo, 0)
	for _, p := range r.GetOtherPeers(peerID) {
		existingPeers = append(existingPeers, PeerInfo{
			PeerID:      p.ID,
			DisplayName: p.DisplayName,
		})
	}

	// Send joined confirmation
	joinedMsg := JoinedMessage{
		Type:   TypeJoined,
		PeerID: peerID,
		Peers:  existingPeers,
	}
	h.sendJSON(conn, joinedMsg)

	// Notify other peers
	peerJoinedMsg := PeerJoinedMessage{
		Type:        TypePeerJoined,
		PeerID:      peerID,
		DisplayName: msg.DisplayName,
	}
	h.broadcastToOthers(r, peerID, peerJoinedMsg)

	// Send chat history
	h.chat.SendHistory(peer, r.ID)

	// Add existing tracks to new peer
	h.sfu.AddExistingTracksToNewPeer(peer, r)

	return peer, r
}

// handleOffer processes an SDP offer
func (h *Handler) handleOffer(peer *room.Peer, r *room.Room, msg *OfferMessage) {
	answer, err := h.sfu.HandleOffer(peer, msg.SDP)
	if err != nil {
		log.Error().Err(err).Str("peerId", peer.ID).Msg("Failed to handle offer")
		return
	}

	// Send answer back to peer
	answerMsg := AnswerMessage{
		Type: TypeAnswer,
		SDP:  answer,
	}
	h.sendJSON(peer.Conn, answerMsg)

	// Get ICE candidates and send them
	pc := peer.GetPeerConnection()
	if pc != nil {
		pc.OnICECandidate(func(c *webrtc.ICECandidate) {
			if c == nil {
				return
			}

			candidateMsg := ICECandidateMessage{
				Type:          TypeICECandidate,
				Candidate:     c.ToJSON().Candidate,
				SDPMid:        *c.ToJSON().SDPMid,
				SDPMLineIndex: *c.ToJSON().SDPMLineIndex,
			}
			h.sendJSON(peer.Conn, candidateMsg)
		})
	}
}

// handleAnswer processes an SDP answer
func (h *Handler) handleAnswer(peer *room.Peer, msg *AnswerMessage) {
	if err := h.sfu.HandleAnswer(peer, msg.SDP); err != nil {
		log.Error().Err(err).Str("peerId", peer.ID).Msg("Failed to handle answer")
	}
}

// handleICECandidate processes an ICE candidate
func (h *Handler) handleICECandidate(peer *room.Peer, msg *ICECandidateMessage) {
	if err := h.sfu.AddICECandidate(peer, msg.Candidate, msg.SDPMid, msg.SDPMLineIndex); err != nil {
		log.Error().Err(err).Str("peerId", peer.ID).Msg("Failed to add ICE candidate")
	}
}

// handleMute handles mute/unmute notifications
func (h *Handler) handleMute(peer *room.Peer, r *room.Room, msg *MuteMessage) {
	// Broadcast mute status to all other peers
	muteMsg := MuteMessage{
		Type:   TypeMute,
		Kind:   msg.Kind,
		Muted:  msg.Muted,
		PeerID: peer.ID,
	}
	h.broadcastToOthers(r, peer.ID, muteMsg)
}

// handlePeerLeave handles peer disconnection
func (h *Handler) handlePeerLeave(peer *room.Peer, r *room.Room) {
	// Close peer connection
	h.sfu.ClosePeerConnection(peer)

	// Remove from room
	r.RemovePeer(peer.ID)

	log.Info().
		Str("peerId", peer.ID).
		Str("roomId", r.ID).
		Msg("Peer left room")

	// Notify other peers
	peerLeftMsg := PeerLeftMessage{
		Type:   TypePeerLeft,
		PeerID: peer.ID,
	}
	h.broadcastToOthers(r, peer.ID, peerLeftMsg)

	// Clean up empty room
	if h.roomManager.RemoveRoomIfEmpty(r.ID) {
		h.chat.RemoveHistory(r.ID)
		log.Info().Str("roomId", r.ID).Msg("Room removed (empty)")
	}
}

// sendJSON sends a JSON message to a connection
func (h *Handler) sendJSON(conn *websocket.Conn, v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal message")
		return
	}
	conn.WriteMessage(websocket.TextMessage, data)
}

// sendError sends an error message to a connection
func (h *Handler) sendError(conn *websocket.Conn, message string) {
	h.sendJSON(conn, ErrorMessage{
		Type:    TypeError,
		Message: message,
	})
}

// broadcastToOthers sends a message to all peers except one
func (h *Handler) broadcastToOthers(r *room.Room, excludePeerID string, v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal broadcast message")
		return
	}

	for _, peer := range r.GetOtherPeers(excludePeerID) {
		if err := peer.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Error().Err(err).Str("peerId", peer.ID).Msg("Failed to send message")
		}
	}
}

// generatePeerID generates a unique peer ID
func generatePeerID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return "peer-" + hex.EncodeToString(bytes)
}
