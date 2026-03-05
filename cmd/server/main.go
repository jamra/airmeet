package main

import (
	"embed"
	"flag"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/jamra/airmeet/internal/server"
)

//go:embed all:web
var webFS embed.FS

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	// Set up logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	// Load configuration
	config, err := server.LoadConfig(*configPath)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load config, using defaults")
		config = &server.Config{}
		config.Server.Host = "0.0.0.0"
		config.Server.Port = 8080
		config.Turn.Enabled = true
		config.Turn.Port = 3478
		config.Turn.Realm = "airmeet"
		config.Rooms.MaxParticipants = 50
	}

	// Get web filesystem
	webSubFS, err := server.EmbedFS(webFS, "web")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load web files")
	}

	// Create and start server
	srv := server.New(config, webSubFS)

	log.Info().
		Str("host", config.Server.Host).
		Int("port", config.Server.Port).
		Bool("tls", config.Server.TLS.Enabled).
		Bool("turn", config.Turn.Enabled).
		Int("maxParticipants", config.Rooms.MaxParticipants).
		Msg("Airmeet starting")

	if err := srv.Start(); err != nil {
		log.Fatal().Err(err).Msg("Server error")
	}
}
