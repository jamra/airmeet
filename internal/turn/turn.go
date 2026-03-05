package turn

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"strconv"

	"github.com/pion/turn/v4"
	"github.com/rs/zerolog/log"
)

// Config holds TURN server configuration
type Config struct {
	Enabled  bool
	Port     int
	Realm    string
	PublicIP string
}

// Server represents the embedded TURN server
type Server struct {
	server   *turn.Server
	config   Config
	username string
	password string
}

// New creates a new TURN server
func New(config Config) *Server {
	// Generate random credentials
	usernameBytes := make([]byte, 8)
	passwordBytes := make([]byte, 16)
	rand.Read(usernameBytes)
	rand.Read(passwordBytes)

	return &Server{
		config:   config,
		username: hex.EncodeToString(usernameBytes),
		password: hex.EncodeToString(passwordBytes),
	}
}

// Start starts the TURN server
func (s *Server) Start() error {
	if !s.config.Enabled {
		log.Info().Msg("TURN server disabled")
		return nil
	}

	// Parse public IP
	publicIP := s.config.PublicIP
	if publicIP == "" {
		// Try to detect public IP
		publicIP = "127.0.0.1"
		log.Warn().Msg("No public IP configured for TURN, using localhost")
	}

	// Create UDP listener
	udpListener, err := net.ListenPacket("udp4", "0.0.0.0:"+strconv.Itoa(s.config.Port))
	if err != nil {
		return err
	}

	// Create TURN server
	s.server, err = turn.NewServer(turn.ServerConfig{
		Realm: s.config.Realm,
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			if username == s.username {
				return turn.GenerateAuthKey(username, realm, s.password), true
			}
			return nil, false
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP(publicIP),
					Address:      "0.0.0.0",
				},
			},
		},
	})
	if err != nil {
		udpListener.Close()
		return err
	}

	log.Info().
		Int("port", s.config.Port).
		Str("realm", s.config.Realm).
		Msg("TURN server started")

	return nil
}

// Stop stops the TURN server
func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// GetCredentials returns the TURN credentials
func (s *Server) GetCredentials() (username, password string) {
	return s.username, s.password
}

// GetURL returns the TURN server URL
func (s *Server) GetURL() string {
	return "turn:localhost:" + strconv.Itoa(s.config.Port)
}

// IsEnabled returns whether the TURN server is enabled
func (s *Server) IsEnabled() bool {
	return s.config.Enabled
}
