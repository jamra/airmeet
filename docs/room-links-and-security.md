# Room Links and Security

## Overview

Shareable room links with embedded passwords, plus optional waiting room for host approval.

## Link Format

```
https://meet.example.com/join/{roomId}?pwd={base64Password}
```

**Example:**
```
https://meet.example.com/join/a1b2c3?pwd=dGVhbS1tZWV0aW5n
```

The password is base64-encoded for URL safety, not for encryption. The actual security comes from the password being random and unguessable.

## Password Generation

- Auto-generated when room is created
- 16 bytes of cryptographically random data
- Encoded as base64url (URL-safe)
- Stored server-side with the room

```go
func generatePassword() string {
    bytes := make([]byte, 16)
    rand.Read(bytes)
    return base64.URLEncoding.EncodeToString(bytes)
}
```

## Data Model Changes

```go
type Room struct {
    ID              string
    Password        string    // auto-generated
    HostPeerID      string    // first person to join, or explicit
    WaitingRoom     bool      // if true, host must approve joiners
    WaitingPeers    []*Peer   // peers waiting for approval
    // ... existing fields
}

type RoomSettings struct {
    WaitingRoomEnabled bool
}
```

## Join Flow

### Without Waiting Room
```
1. User clicks invite link
2. Client extracts roomId and pwd from URL
3. Client connects to WebSocket
4. Client sends: { type: "join", roomId, password, displayName }
5. Server validates password
   - If valid: join room, send "joined" message
   - If invalid: send "error" with "Invalid password"
```

### With Waiting Room
```
1. User clicks invite link
2. Client extracts roomId and pwd from URL
3. Client connects to WebSocket
4. Client sends: { type: "join", roomId, password, displayName }
5. Server validates password
   - If invalid: send "error"
   - If valid and user is host: join immediately
   - If valid and waiting room enabled: add to waiting room
6. Server sends to joiner: { type: "waiting", position: 2 }
7. Server sends to host: { type: "waiting-peer", peerId, displayName }
8. Host sends: { type: "admit", peerId } or { type: "deny", peerId }
9. Server sends to peer: { type: "admitted" } or { type: "denied" }
```

## Signaling Messages

```typescript
// Join with password
{ type: "join", roomId: "abc", password: "base64pwd", displayName: "Alice" }

// Waiting room
{ type: "waiting", position: 2, message: "Waiting for host to let you in" }
{ type: "waiting-peer", peerId: "xyz", displayName: "Bob" }  // to host
{ type: "admit", peerId: "xyz" }      // host admits
{ type: "admit-all" }                  // host admits everyone
{ type: "deny", peerId: "xyz" }        // host denies

// Responses
{ type: "admitted" }
{ type: "denied", message: "Host denied your request" }
```

## API Changes

### Create Room
```
POST /api/room/create
Request:  { "waitingRoom": true }
Response: { "roomId": "abc123", "password": "base64pwd", "inviteLink": "https://..." }
```

### Room Info (for hosts)
```
GET /api/room/{id}/info
Response: { "id": "abc", "participants": 5, "waiting": 2, "waitingRoom": true }
```

## UI Changes

### Join Screen
- Parse URL for roomId and password
- If link has password, auto-fill and skip to name entry
- Show "Joining..." then "Waiting for host..." if in waiting room

### In-Meeting (Host)
- "Copy Invite Link" button generates full link with password
- If waiting room enabled, show waiting room panel:
  ```
  ┌─────────────────────────────┐
  │ Waiting Room (3)            │
  │ ├ Bob         [Admit][Deny] │
  │ ├ Carol       [Admit][Deny] │
  │ └ Dan         [Admit][Deny] │
  │         [Admit All]         │
  └─────────────────────────────┘
  ```

### Room Settings (Host)
- Toggle: "Enable waiting room"
- Button: "Regenerate invite link" (creates new password)

## Implementation Plan

### Phase 1: Password Protection
- [ ] Add password field to Room struct
- [ ] Generate password on room creation
- [ ] Update join flow to validate password
- [ ] Update client to send password with join
- [ ] Parse password from URL on page load
- [ ] Update invite link to include password

### Phase 2: Waiting Room
- [ ] Add waiting room fields to Room struct
- [ ] Add waiting room signaling messages
- [ ] Implement admit/deny logic
- [ ] Add waiting room UI for pending joiners
- [ ] Add host panel for managing waiting room
- [ ] Add room settings toggle

### Phase 3: Host Controls
- [ ] First joiner becomes host (or explicit host assignment)
- [ ] Host can regenerate password (invalidates old links)
- [ ] Host can toggle waiting room mid-meeting
- [ ] Co-host support (can admit from waiting room)

## Security Considerations

- Passwords are 128 bits of entropy (16 random bytes)
- Brute force is impractical (~3.4 × 10^38 possibilities)
- Rate limit join attempts per IP
- Passwords not logged or exposed in API responses (except to creator)
- Consider password expiry for long-running rooms

## Edge Cases

- Host leaves while people in waiting room → next participant becomes host, or auto-admit all
- Password in URL but room doesn't exist → show "Room not found or expired"
- Someone has old link after password regenerated → show "Invalid link, request a new one"
- Waiting room disabled mid-meeting → auto-admit all waiting peers
