package sfu

import (
	"sync"

	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog/log"
	"github.com/yourname/airmeet/internal/room"
)

// SFU handles WebRTC connections and track forwarding
type SFU struct {
	config webrtc.Configuration
	api    *webrtc.API
	mu     sync.RWMutex
}

// New creates a new SFU instance
func New(iceServers []webrtc.ICEServer) *SFU {
	// Create a MediaEngine with default codecs
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		log.Error().Err(err).Msg("Failed to register default codecs")
	}

	// Create API with MediaEngine
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))

	return &SFU{
		config: webrtc.Configuration{
			ICEServers: iceServers,
		},
		api: api,
	}
}

// CreatePeerConnection creates a new peer connection for a peer
func (s *SFU) CreatePeerConnection(peer *room.Peer, r *room.Room) (*webrtc.PeerConnection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pc, err := s.api.NewPeerConnection(s.config)
	if err != nil {
		return nil, err
	}

	// Add transceivers for receiving audio and video
	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		pc.Close()
		return nil, err
	}

	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		pc.Close()
		return nil, err
	}

	// Handle incoming tracks
	pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Info().
			Str("peer", peer.ID).
			Str("kind", remoteTrack.Kind().String()).
			Str("trackId", remoteTrack.ID()).
			Msg("Received track")

		// Create a local track to forward to other peers
		localTrack, err := webrtc.NewTrackLocalStaticRTP(
			remoteTrack.Codec().RTPCodecCapability,
			remoteTrack.ID(),
			remoteTrack.StreamID(),
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create local track")
			return
		}

		// Store the track
		peer.AddTrack(remoteTrack.ID(), localTrack)

		// Add this track to all other peers in the room
		s.addTrackToOtherPeers(localTrack, peer.ID, r)

		// Forward RTP packets from remote to local track
		go s.forwardTrack(remoteTrack, localTrack, peer.ID)
	})

	// Handle connection state changes
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Info().
			Str("peer", peer.ID).
			Str("state", state.String()).
			Msg("Connection state changed")

		if state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateClosed ||
			state == webrtc.PeerConnectionStateDisconnected {
			// Clean up tracks when peer disconnects
			for trackID := range peer.GetTracks() {
				peer.RemoveTrack(trackID)
			}
		}
	})

	// Handle ICE connection state changes
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Info().
			Str("peer", peer.ID).
			Str("state", state.String()).
			Msg("ICE connection state changed")
	})

	peer.SetPeerConnection(pc)
	return pc, nil
}

// forwardTrack forwards RTP packets from a remote track to a local track
func (s *SFU) forwardTrack(remote *webrtc.TrackRemote, local *webrtc.TrackLocalStaticRTP, peerID string) {
	buf := make([]byte, 1500)
	for {
		n, _, err := remote.Read(buf)
		if err != nil {
			log.Debug().
				Str("peer", peerID).
				Err(err).
				Msg("Track read ended")
			return
		}

		if _, err := local.Write(buf[:n]); err != nil {
			log.Debug().
				Str("peer", peerID).
				Err(err).
				Msg("Track write ended")
			return
		}
	}
}

// addTrackToOtherPeers adds a track to all other peers in the room
func (s *SFU) addTrackToOtherPeers(track *webrtc.TrackLocalStaticRTP, sourcePeerID string, r *room.Room) {
	for _, peer := range r.GetOtherPeers(sourcePeerID) {
		pc := peer.GetPeerConnection()
		if pc == nil {
			continue
		}

		sender, err := pc.AddTrack(track)
		if err != nil {
			log.Error().
				Str("peer", peer.ID).
				Err(err).
				Msg("Failed to add track to peer")
			continue
		}

		// Handle RTCP packets (for congestion control, etc.)
		go func(sender *webrtc.RTPSender) {
			buf := make([]byte, 1500)
			for {
				if _, _, err := sender.Read(buf); err != nil {
					return
				}
			}
		}(sender)

		log.Info().
			Str("targetPeer", peer.ID).
			Str("sourcePeer", sourcePeerID).
			Str("trackId", track.ID()).
			Msg("Added track to peer")
	}
}

// AddExistingTracksToNewPeer adds all existing tracks from other peers to a new peer
func (s *SFU) AddExistingTracksToNewPeer(newPeer *room.Peer, r *room.Room) {
	pc := newPeer.GetPeerConnection()
	if pc == nil {
		return
	}

	for _, peer := range r.GetOtherPeers(newPeer.ID) {
		for _, track := range peer.GetTracks() {
			sender, err := pc.AddTrack(track)
			if err != nil {
				log.Error().
					Str("peer", newPeer.ID).
					Err(err).
					Msg("Failed to add existing track")
				continue
			}

			// Handle RTCP packets
			go func(sender *webrtc.RTPSender) {
				buf := make([]byte, 1500)
				for {
					if _, _, err := sender.Read(buf); err != nil {
						return
					}
				}
			}(sender)

			log.Info().
				Str("newPeer", newPeer.ID).
				Str("sourcePeer", peer.ID).
				Str("trackId", track.ID()).
				Msg("Added existing track to new peer")
		}
	}
}

// HandleOffer processes an SDP offer and returns an answer
func (s *SFU) HandleOffer(peer *room.Peer, sdp string) (string, error) {
	pc := peer.GetPeerConnection()
	if pc == nil {
		return "", nil
	}

	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}

	if err := pc.SetRemoteDescription(offer); err != nil {
		return "", err
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return "", err
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		return "", err
	}

	return answer.SDP, nil
}

// HandleAnswer processes an SDP answer
func (s *SFU) HandleAnswer(peer *room.Peer, sdp string) error {
	pc := peer.GetPeerConnection()
	if pc == nil {
		return nil
	}

	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sdp,
	}

	return pc.SetRemoteDescription(answer)
}

// AddICECandidate adds an ICE candidate to a peer connection
func (s *SFU) AddICECandidate(peer *room.Peer, candidate string, sdpMid string, sdpMLineIndex uint16) error {
	pc := peer.GetPeerConnection()
	if pc == nil {
		return nil
	}

	iceCandidate := webrtc.ICECandidateInit{
		Candidate:     candidate,
		SDPMid:        &sdpMid,
		SDPMLineIndex: &sdpMLineIndex,
	}

	return pc.AddICECandidate(iceCandidate)
}

// ClosePeerConnection closes a peer's connection
func (s *SFU) ClosePeerConnection(peer *room.Peer) {
	pc := peer.GetPeerConnection()
	if pc != nil {
		pc.Close()
	}
}
