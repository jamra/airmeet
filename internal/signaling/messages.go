package signaling

import "encoding/json"

// Message types
const (
	TypeJoin         = "join"
	TypeJoined       = "joined"
	TypePeerJoined   = "peer-joined"
	TypePeerLeft     = "peer-left"
	TypeOffer        = "offer"
	TypeAnswer       = "answer"
	TypeICECandidate = "ice-candidate"
	TypeChat         = "chat"
	TypeMute         = "mute"
	TypeError        = "error"
)

// BaseMessage contains the message type for routing
type BaseMessage struct {
	Type string `json:"type"`
}

// JoinMessage - client requests to join a room
type JoinMessage struct {
	Type        string `json:"type"`
	RoomID      string `json:"roomId"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

// JoinedMessage - server confirms join with peer info
type JoinedMessage struct {
	Type     string     `json:"type"`
	PeerID   string     `json:"peerId"`
	RoomID   string     `json:"roomId"`
	Password string     `json:"password,omitempty"` // only sent to host
	IsHost   bool       `json:"isHost"`
	Peers    []PeerInfo `json:"peers"`
}

// PeerInfo contains basic peer information
type PeerInfo struct {
	PeerID      string `json:"peerId"`
	DisplayName string `json:"displayName"`
}

// PeerJoinedMessage - server notifies of new peer
type PeerJoinedMessage struct {
	Type        string `json:"type"`
	PeerID      string `json:"peerId"`
	DisplayName string `json:"displayName"`
}

// PeerLeftMessage - server notifies peer left
type PeerLeftMessage struct {
	Type   string `json:"type"`
	PeerID string `json:"peerId"`
}

// OfferMessage - SDP offer
type OfferMessage struct {
	Type         string `json:"type"`
	SDP          string `json:"sdp"`
	TargetPeerID string `json:"targetPeerId,omitempty"`
	FromPeerID   string `json:"fromPeerId,omitempty"`
}

// AnswerMessage - SDP answer
type AnswerMessage struct {
	Type         string `json:"type"`
	SDP          string `json:"sdp"`
	TargetPeerID string `json:"targetPeerId,omitempty"`
	FromPeerID   string `json:"fromPeerId,omitempty"`
}

// ICECandidateMessage - ICE candidate exchange
type ICECandidateMessage struct {
	Type              string `json:"type"`
	Candidate         string `json:"candidate"`
	SDPMid            string `json:"sdpMid,omitempty"`
	SDPMLineIndex     uint16 `json:"sdpMLineIndex,omitempty"`
	UsernameFragment  string `json:"usernameFragment,omitempty"`
	TargetPeerID      string `json:"targetPeerId,omitempty"`
	FromPeerID        string `json:"fromPeerId,omitempty"`
}

// ChatMessage - chat message
type ChatMessage struct {
	Type       string `json:"type"`
	Message    string `json:"message"`
	FromPeerID string `json:"fromPeerId,omitempty"`
	Timestamp  int64  `json:"timestamp,omitempty"`
}

// MuteMessage - mute/unmute notification
type MuteMessage struct {
	Type   string `json:"type"`
	Kind   string `json:"kind"` // "audio" or "video"
	Muted  bool   `json:"muted"`
	PeerID string `json:"peerId,omitempty"`
}

// ErrorMessage - error response
type ErrorMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ParseMessage parses a raw JSON message into the appropriate type
func ParseMessage(data []byte) (interface{}, error) {
	var base BaseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, err
	}

	switch base.Type {
	case TypeJoin:
		var msg JoinMessage
		err := json.Unmarshal(data, &msg)
		return &msg, err
	case TypeOffer:
		var msg OfferMessage
		err := json.Unmarshal(data, &msg)
		return &msg, err
	case TypeAnswer:
		var msg AnswerMessage
		err := json.Unmarshal(data, &msg)
		return &msg, err
	case TypeICECandidate:
		var msg ICECandidateMessage
		err := json.Unmarshal(data, &msg)
		return &msg, err
	case TypeChat:
		var msg ChatMessage
		err := json.Unmarshal(data, &msg)
		return &msg, err
	case TypeMute:
		var msg MuteMessage
		err := json.Unmarshal(data, &msg)
		return &msg, err
	default:
		return &base, nil
	}
}
