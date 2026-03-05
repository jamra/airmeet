# Airmeet

A self-hosted, web-based video conferencing platform built with Go. Supports 10-50 participants per room with video, audio, screen sharing, and chat.

## Features

- **Multi-party video calls** - SFU architecture using [Pion WebRTC](https://github.com/pion/webrtc) scales to 50+ participants
- **Screen sharing** - Share your screen with one click
- **Real-time chat** - Text chat with message history
- **Embedded TURN server** - NAT traversal out of the box
- **Zero dependencies** - Single binary with embedded web UI
- **Self-hosted** - Your data stays on your server

## Quick Start

```bash
# Clone and build
git clone https://github.com/jamra/airmeet.git
cd airmeet
go build -o airmeet ./cmd/server

# Run
./airmeet
```

Open http://localhost:8080 in your browser. Enter your name, optionally enter a room ID (or leave blank to create a new room), and click "Join Meeting".

## Configuration

Create a `config.yaml` file:

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  tls:
    enabled: false
    cert: "/path/to/cert.pem"
    key: "/path/to/key.pem"

turn:
  enabled: true
  port: 3478
  realm: "airmeet"
  public_ip: ""  # Set this to your server's public IP for production

rooms:
  max_participants: 50
```

### Command Line Options

```bash
./airmeet -config /path/to/config.yaml  # Custom config file
./airmeet -debug                         # Enable debug logging
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Web Clients                              │
│                    (WebRTC + WebSocket)                          │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Go Server                                   │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │   Signaling     │  │      SFU        │  │      Chat       │  │
│  │   (WebSocket)   │  │    (Pion)       │  │   (WebSocket)   │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │  Room Manager   │  │  TURN Server    │  │   REST API      │  │
│  │                 │  │  (embedded)     │  │                 │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### Why SFU?

- **Mesh topology**: Each participant sends N-1 streams (doesn't scale past ~4 people)
- **SFU topology**: Each participant sends 1 stream, receives N-1 (scales to 50+)

## Project Structure

```
airmeet/
├── cmd/server/
│   ├── main.go           # Entry point
│   └── web/              # Embedded static files
├── internal/
│   ├── server/           # HTTP/WebSocket server
│   ├── signaling/        # WebRTC signaling protocol
│   ├── sfu/              # Selective Forwarding Unit
│   ├── room/             # Room management
│   ├── chat/             # Chat service
│   └── turn/             # Embedded TURN server
└── config.yaml
```

## Production Deployment

For production use:

1. **Enable TLS** - WebRTC requires HTTPS in production
2. **Set public IP** - Configure `turn.public_ip` for NAT traversal
3. **Use a reverse proxy** - nginx or Caddy for TLS termination
4. **Open firewall ports** - 8080 (HTTP), 3478 (TURN UDP/TCP)

Example with Caddy:

```
meet.example.com {
    reverse_proxy localhost:8080
}
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Web UI |
| `/ws` | WebSocket | Signaling connection |
| `/api/rooms` | GET | List active rooms |
| `/api/room/create` | POST | Create a new room |
| `/api/ice-servers` | GET | Get ICE server config |

## License

MIT
