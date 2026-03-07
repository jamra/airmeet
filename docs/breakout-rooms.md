# Breakout Rooms

## Overview

Breakout rooms allow hosts to split participants into smaller groups for focused discussions, then bring everyone back to the main room.

## User Stories

- As a host, I can create multiple breakout rooms from my main meeting
- As a host, I can assign participants to breakout rooms (manual or automatic)
- As a host, I can broadcast a message to all breakout rooms
- As a host, I can set a timer that auto-closes breakout rooms
- As a host, I can end all breakout rooms and bring everyone back
- As a participant, I can see which breakout room I'm assigned to
- As a participant, I can ask the host for help from within a breakout room
- As a participant, I can return to the main room early (if allowed)

## Data Model

```go
type BreakoutSession struct {
    ID            string
    MainRoomID    string
    BreakoutRooms []BreakoutRoom
    CreatedAt     time.Time
    EndsAt        *time.Time  // optional timer
    Status        string      // "active", "closing", "closed"
}

type BreakoutRoom struct {
    ID           string
    Name         string      // "Room 1", "Room 2", or custom
    Participants []string    // peer IDs
    RoomID       string      // actual room ID for WebRTC
}

type BreakoutAssignment struct {
    PeerID        string
    BreakoutID    string
    OriginalRoom  string
}
```

## Signaling Messages

```typescript
// Host → Server
{ type: "breakout-create", rooms: [{ name: "Room 1" }, { name: "Room 2" }] }
{ type: "breakout-assign", assignments: [{ peerId: "x", roomName: "Room 1" }] }
{ type: "breakout-broadcast", message: "5 minutes remaining" }
{ type: "breakout-end" }

// Server → Participants
{ type: "breakout-assigned", roomId: "xyz", roomName: "Room 1" }
{ type: "breakout-ending", secondsRemaining: 60 }
{ type: "breakout-ended", returnToRoom: "main-room-id" }

// Participant → Server
{ type: "breakout-help" }  // request host assistance
{ type: "breakout-leave" } // return to main room early
```

## Implementation Plan

### Phase 1: Core Infrastructure
- [ ] Add `BreakoutSession` to room manager
- [ ] Add host role/permissions to rooms
- [ ] Implement breakout room creation (creates child rooms linked to parent)
- [ ] Implement participant assignment

### Phase 2: Room Transitions
- [ ] Handle participant movement between rooms
- [ ] Preserve WebRTC connections during transition (or graceful reconnect)
- [ ] Update UI to show breakout status

### Phase 3: Host Controls
- [ ] Broadcast message to all breakout rooms
- [ ] Timer with countdown warnings (5 min, 1 min, 30 sec)
- [ ] "End breakout" that moves everyone back
- [ ] Help request notifications

### Phase 4: UI/UX
- [ ] Host panel for managing breakout rooms
- [ ] Participant view showing current breakout room
- [ ] Visual countdown timer
- [ ] Drag-and-drop assignment (optional)

## UI Wireframes

### Host View - Create Breakouts
```
┌─────────────────────────────────────┐
│ Create Breakout Rooms               │
├─────────────────────────────────────┤
│ Number of rooms: [3] [▼]            │
│                                     │
│ ○ Assign automatically              │
│ ● Assign manually                   │
│                                     │
│ Timer: [10] minutes (0 = no limit)  │
│                                     │
│ [Create Breakout Rooms]             │
└─────────────────────────────────────┘
```

### Host View - Manage Breakouts
```
┌─────────────────────────────────────┐
│ Breakout Rooms          ⏱ 8:32     │
├─────────────────────────────────────┤
│ Room 1 (3)     Room 2 (3)    + Add  │
│ ├ Alice        ├ Dan                │
│ ├ Bob          ├ Eve                │
│ └ Carol        └ Frank              │
│                                     │
│ [Broadcast Message] [End Breakouts] │
└─────────────────────────────────────┘
```

## Edge Cases

- Host disconnects while breakouts active → co-host takes over, or auto-end
- Participant joins meeting during breakout → assign to main room or specific breakout
- Breakout room becomes empty → keep open or auto-close?
- Network issues during room transition → retry logic with exponential backoff

## Dependencies

- Host/co-host role system (not yet implemented)
- Room-to-room participant transfer
- Timer service

## Future Enhancements

- Pre-assign breakouts before meeting starts
- Let participants choose their own breakout room
- Breakout room recordings
- Persistent breakout room assignments across sessions
