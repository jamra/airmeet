# Simulcast

## Overview

Simulcast allows senders to encode video at multiple quality levels simultaneously. The SFU selects which layer to forward to each receiver based on their conditions.

## How It Works

```
Sender encodes 3 layers:
┌─────────────┐
│   Camera    │
└──────┬──────┘
       │
       ▼
┌─────────────┐     ┌────────────────────────────────┐
│   Encoder   │────►│ High:   1080p @ 2.5 Mbps       │
│  (VP8/H264) │────►│ Medium: 720p  @ 1.0 Mbps       │
│             │────►│ Low:    360p  @ 0.3 Mbps       │
└─────────────┘     └────────────────────────────────┘
                                  │
                                  ▼
                    ┌─────────────────────────────┐
                    │            SFU              │
                    │  Selects layer per receiver │
                    └─────────────────────────────┘
                         │         │         │
                    High │    Med  │    Low  │
                         ▼         ▼         ▼
                    ┌───────┐ ┌───────┐ ┌───────┐
                    │Active │ │ Small │ │Mobile │
                    │Speaker│ │Tile   │ │User   │
                    └───────┘ └───────┘ └───────┘
```

## Benefits

| Without Simulcast | With Simulcast |
|-------------------|----------------|
| Everyone gets same quality | Quality adapts per receiver |
| Bandwidth wasted on thumbnails | Low quality for small tiles |
| Can't handle varying connections | Graceful degradation |
| One slow receiver affects all | Isolated impact |

## Layer Selection Logic

The SFU decides which layer to forward based on:

1. **Viewport size** - No point sending 1080p to a 200px thumbnail
2. **Available bandwidth** - Detected via REMB/TWCC feedback
3. **Active speaker** - High quality for current speaker
4. **User preference** - "Low bandwidth mode" option

```go
func selectLayer(receiver *Peer, sender *Peer) Layer {
    // Active speaker gets high quality
    if sender.IsActiveSpeaker() && receiver.ViewportSize > 720 {
        return LayerHigh
    }

    // Small tiles get low quality
    if receiver.TileSize < 300 {
        return LayerLow
    }

    // Check bandwidth
    if receiver.AvailableBandwidth < 1_000_000 {
        return LayerLow
    }

    return LayerMedium
}
```

## Implementation in Pion

### Client-side (JavaScript)

```javascript
// Enable simulcast when creating offer
const transceiver = peerConnection.addTransceiver(videoTrack, {
    direction: 'sendonly',
    sendEncodings: [
        { rid: 'low', maxBitrate: 300000, scaleResolutionDownBy: 4 },
        { rid: 'medium', maxBitrate: 1000000, scaleResolutionDownBy: 2 },
        { rid: 'high', maxBitrate: 2500000 }
    ]
});
```

### Server-side (Pion)

```go
// Handle incoming simulcast track
peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
    // track.RID() returns "low", "medium", or "high"
    rid := track.RID()

    // Store all layers
    sender.AddSimulcastLayer(rid, track)

    // Forward appropriate layer to each receiver
    for _, peer := range room.GetOtherPeers(sender.ID) {
        layer := selectLayerForPeer(peer)
        forwardTrack(sender.GetLayer(layer), peer)
    }
})
```

## Implementation Plan

### Phase 1: Basic Simulcast
- [ ] Enable simulcast encodings in client
- [ ] Handle multiple RIDs on server
- [ ] Store all layers per sender
- [ ] Forward highest available layer

### Phase 2: Adaptive Selection
- [ ] Track receiver viewport sizes (client reports)
- [ ] Implement bandwidth estimation (TWCC)
- [ ] Select layer based on viewport + bandwidth
- [ ] Smooth layer switching (avoid flicker)

### Phase 3: Active Speaker
- [ ] Detect active speaker (audio levels)
- [ ] Prioritize high quality for active speaker
- [ ] Lower quality for inactive participants

### Phase 4: Client Controls
- [ ] "Low bandwidth mode" toggle
- [ ] Quality indicator per participant
- [ ] Manual quality override option

## Bandwidth Savings Example

**10-person call, one active speaker:**

| Without Simulcast | With Simulcast |
|-------------------|----------------|
| 9 × 2.5 Mbps = 22.5 Mbps | 1 × 2.5 + 8 × 0.3 = 4.9 Mbps |

**78% bandwidth reduction!**

## Codec Support

| Codec | Simulcast Support |
|-------|-------------------|
| VP8 | Yes (widely supported) |
| VP9 | Yes (SVC preferred) |
| H.264 | Yes (some browser quirks) |
| AV1 | Yes (SVC preferred) |

## References

- [WebRTC Simulcast](https://webrtc.github.io/samples/src/content/peerconnection/simulcast/)
- [Pion Simulcast Example](https://github.com/pion/webrtc/tree/master/examples/simulcast)
- [RFC 8853 - Simulcast](https://datatracker.ietf.org/doc/html/rfc8853)
