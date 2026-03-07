# Audio Enhancements

## Overview

Improve audio quality by suppressing background noise, echo, and other unwanted sounds using ML-based audio processing.

## Features

### 1. Background Noise Suppression
Remove keyboard typing, fans, AC, street noise, etc.

### 2. Echo Cancellation
Prevent feedback loops when not using headphones.

### 3. Auto Gain Control
Normalize volume levels across participants.

## Implementation Options

### Option A: RNNoise (Recommended for MVP)
- Open source, BSD licensed
- Recurrent Neural Network trained on real noise
- Runs in browser via WebAssembly
- ~5% CPU overhead
- Already used by: Discord, Jitsi, OBS

**Integration:**
```javascript
// Using rnnoise-wasm
import { RNNoise } from 'rnnoise-wasm';

const rnnoise = await RNNoise.create();
const audioContext = new AudioContext();
const processor = audioContext.createScriptProcessor(512, 1, 1);

processor.onaudioprocess = (e) => {
    const input = e.inputBuffer.getChannelData(0);
    const output = e.outputBuffer.getChannelData(0);
    rnnoise.process(input, output);
};
```

**Links:**
- https://github.com/nickarls/nickarls/rnnoise-wasm
- https://jmvalin.ca/demo/rnnoise/

### Option B: Web Audio API + TensorFlow.js
- More flexible, can train custom models
- Higher CPU usage
- Larger bundle size (~2MB for TF.js)

### Option C: Browser Built-in
- Chrome/Edge have `noiseSuppression` constraint
- Inconsistent quality across browsers
- No control over aggressiveness

```javascript
// Built-in (limited)
navigator.mediaDevices.getUserMedia({
    audio: {
        noiseSuppression: true,
        echoCancellation: true,
        autoGainControl: true
    }
});
```

### Option D: Server-side Processing
- Process audio on SFU before forwarding
- Higher latency
- More server CPU
- Works for all clients equally

## Recommended Approach

**Phase 1: Enable browser defaults + RNNoise option**
1. Enable `noiseSuppression`, `echoCancellation`, `autoGainControl` by default
2. Add toggle in UI: "Enhanced noise suppression" (uses RNNoise)
3. RNNoise runs client-side in WebAssembly

**Phase 2: Adaptive processing**
1. Detect noise level, auto-enable suppression when needed
2. Voice activity detection (VAD) - mute when not speaking
3. Visual indicator showing noise suppression is active

## UI Design

```
┌─────────────────────────────────────┐
│ Audio Settings                      │
├─────────────────────────────────────┤
│ ☑ Noise suppression                 │
│ ☑ Echo cancellation                 │
│ ☑ Auto gain control                 │
│                                     │
│ Advanced:                           │
│ ☑ AI noise suppression (RNNoise)    │
│   Removes keyboard, fans, etc.      │
│                                     │
│ Input level: ████████░░ Good        │
└─────────────────────────────────────┘
```

## Implementation Plan

### Phase 1: Browser Defaults
- [ ] Enable noiseSuppression, echoCancellation, autoGainControl in getUserMedia
- [ ] Add audio settings panel in UI
- [ ] Allow users to toggle each setting

### Phase 2: RNNoise Integration
- [ ] Add rnnoise-wasm dependency
- [ ] Create AudioProcessor class that wraps RNNoise
- [ ] Add "Enhanced noise suppression" toggle
- [ ] Process local audio before sending to peer connection
- [ ] Add visual indicator when active

### Phase 3: Voice Activity Detection
- [ ] Detect silence/speech using RNNoise VAD output
- [ ] Auto-mute when not speaking (optional setting)
- [ ] Show speaking indicator in participant list
- [ ] Highlight active speaker in video grid

### Phase 4: Audio Visualization
- [ ] Input level meter in settings
- [ ] Speaking indicator on video tiles
- [ ] Waveform/spectrum visualization (optional)

## Performance Considerations

- RNNoise WASM: ~5% CPU on modern devices
- Process at 48kHz sample rate for best quality
- Use AudioWorklet instead of ScriptProcessor (deprecated)
- Consider disabling on low-power devices

## Testing

1. Test with various noise sources (keyboard, fan, music)
2. Test CPU usage on different devices
3. Test latency impact (should be <10ms)
4. A/B test with users to validate quality improvement

## Dependencies

```json
{
  "rnnoise-wasm": "^1.0.0"
}
```

## References

- [RNNoise: Learning Noise Suppression](https://jmvalin.ca/demo/rnnoise/)
- [Web Audio API](https://developer.mozilla.org/en-US/docs/Web/API/Web_Audio_API)
- [AudioWorklet](https://developer.mozilla.org/en-US/docs/Web/API/AudioWorklet)
