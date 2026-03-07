package server

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog/log"
	"github.com/jamra/airmeet/internal/chat"
	"github.com/jamra/airmeet/internal/room"
	"github.com/jamra/airmeet/internal/sfu"
	"github.com/jamra/airmeet/internal/signaling"
	"github.com/jamra/airmeet/internal/turn"
	"gopkg.in/yaml.v3"
)

// Config holds server configuration
type Config struct {
	Server struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
		TLS  struct {
			Enabled bool   `yaml:"enabled"`
			Cert    string `yaml:"cert"`
			Key     string `yaml:"key"`
		} `yaml:"tls"`
	} `yaml:"server"`
	Turn struct {
		Enabled  bool   `yaml:"enabled"`
		Port     int    `yaml:"port"`
		Realm    string `yaml:"realm"`
		PublicIP string `yaml:"public_ip"`
	} `yaml:"turn"`
	Rooms struct {
		MaxParticipants     int    `yaml:"max_participants"`
		DefaultVideoQuality string `yaml:"default_video_quality"`
	} `yaml:"rooms"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Set defaults
	if config.Server.Port == 0 {
		config.Server.Port = 8080
	}
	if config.Turn.Port == 0 {
		config.Turn.Port = 3478
	}
	if config.Turn.Realm == "" {
		config.Turn.Realm = "airmeet"
	}
	if config.Rooms.MaxParticipants == 0 {
		config.Rooms.MaxParticipants = 50
	}

	return &config, nil
}

// Server represents the main server
type Server struct {
	config      *Config
	httpServer  *http.Server
	turnServer  *turn.Server
	roomManager *room.Manager
	sfuServer   *sfu.SFU
	chatService *chat.Service
	webFS       fs.FS
}

// New creates a new server
func New(config *Config, webFS fs.FS) *Server {
	return &Server{
		config: config,
		webFS:  webFS,
	}
}

// Start starts the server
func (s *Server) Start() error {
	// Initialize TURN server
	s.turnServer = turn.New(turn.Config{
		Enabled:  s.config.Turn.Enabled,
		Port:     s.config.Turn.Port,
		Realm:    s.config.Turn.Realm,
		PublicIP: s.config.Turn.PublicIP,
	})

	if err := s.turnServer.Start(); err != nil {
		log.Warn().Err(err).Msg("Failed to start TURN server")
	}

	// Initialize room manager
	s.roomManager = room.NewManager(s.config.Rooms.MaxParticipants)

	// Initialize SFU with ICE servers
	iceServers := []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	}

	// Add embedded TURN server if enabled
	if s.turnServer.IsEnabled() {
		username, password := s.turnServer.GetCredentials()
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs:       []string{s.turnServer.GetURL()},
			Username:   username,
			Credential: password,
		})
	}

	s.sfuServer = sfu.New(iceServers)

	// Initialize chat service
	s.chatService = chat.NewService(100) // Keep last 100 messages

	// Create signaling handler
	signalingHandler := signaling.NewHandler(s.roomManager, s.sfuServer, s.chatService)

	// Set up HTTP routes
	mux := http.NewServeMux()

	// WebSocket endpoint
	mux.Handle("/ws", signalingHandler)

	// API endpoints
	mux.HandleFunc("/api/rooms", s.handleListRooms)
	mux.HandleFunc("/api/room/create", s.handleCreateRoom)
	mux.HandleFunc("/api/ice-servers", s.handleICEServers)

	// Serve static files
	if s.webFS != nil {
		mux.Handle("/", http.FileServer(http.FS(s.webFS)))
	}

	// Create HTTP server
	addr := s.config.Server.Host + ":" + strconv.Itoa(s.config.Server.Port)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start HTTP server
	go func() {
		var err error
		if s.config.Server.TLS.Enabled {
			log.Info().Str("addr", addr).Msg("Starting HTTPS server")
			err = s.httpServer.ListenAndServeTLS(s.config.Server.TLS.Cert, s.config.Server.TLS.Key)
		} else {
			log.Info().Str("addr", addr).Msg("Starting HTTP server")
			err = s.httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed")
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server...")
	return s.Stop()
}

// Stop stops the server
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
	}

	if err := s.turnServer.Stop(); err != nil {
		log.Error().Err(err).Msg("TURN server shutdown error")
	}

	log.Info().Msg("Server stopped")
	return nil
}

// handleListRooms returns list of active rooms
func (s *Server) handleListRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rooms := s.roomManager.ListRooms()
	result := make([]map[string]interface{}, 0, len(rooms))

	for _, room := range rooms {
		result = append(result, map[string]interface{}{
			"id":           room.ID,
			"participants": room.PeerCount(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleCreateRoom creates a new room
func (s *Server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	room := s.roomManager.CreateRoom()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"roomId":   room.ID,
		"password": room.GetPassword(),
	})
}

// handleICEServers returns ICE server configuration
func (s *Server) handleICEServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	iceServers := []map[string]interface{}{
		{
			"urls": []string{"stun:stun.l.google.com:19302"},
		},
	}

	if s.turnServer.IsEnabled() {
		username, password := s.turnServer.GetCredentials()
		iceServers = append(iceServers, map[string]interface{}{
			"urls":       []string{s.turnServer.GetURL()},
			"username":   username,
			"credential": password,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(iceServers)
}

// EmbedFS wraps an embed.FS for use as the web filesystem
func EmbedFS(efs embed.FS, root string) (fs.FS, error) {
	return fs.Sub(efs, root)
}
