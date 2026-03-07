// Airmeet WebRTC Client

// Audio Processor using RNNoise for ML-based noise suppression
class AudioProcessor {
    constructor() {
        this.audioContext = null;
        this.rnnoiseModule = null;
        this.rnnoiseState = null;
        this.processorNode = null;
        this.sourceNode = null;
        this.destinationNode = null;
        this.enabled = false;
        this.rnnoiseLoaded = false;
        this.frameSize = 480; // RNNoise frame size (10ms at 48kHz)
    }

    async init() {
        // Load RNNoise WASM module
        try {
            // Dynamically load the RNNoise module
            const script = document.createElement('script');
            script.src = '/rnnoise-wasm.js';
            document.head.appendChild(script);

            await new Promise((resolve, reject) => {
                script.onload = resolve;
                script.onerror = reject;
            });

            // Initialize the WASM module
            if (typeof createRNNWasmModule === 'function') {
                this.rnnoiseModule = await createRNNWasmModule({
                    locateFile: (path) => '/' + path
                });
                await this.rnnoiseModule.ready;

                // Create RNNoise state
                this.rnnoiseState = this.rnnoiseModule._rnnoise_create();
                this.rnnoiseLoaded = true;
                console.log('RNNoise WASM loaded successfully');
            }
        } catch (err) {
            console.warn('RNNoise not available, using browser defaults:', err);
            this.rnnoiseLoaded = false;
        }
    }

    async processStream(stream) {
        if (!this.rnnoiseLoaded || !this.enabled) {
            return stream;
        }

        try {
            // Get audio track
            const audioTrack = stream.getAudioTracks()[0];
            if (!audioTrack) return stream;

            // Create audio context at 48kHz (required by RNNoise)
            this.audioContext = new AudioContext({ sampleRate: 48000 });

            // Create source from stream
            this.sourceNode = this.audioContext.createMediaStreamSource(stream);

            // Create script processor (buffer size must accommodate 480-sample frames)
            // Using 4096 for stability, will process in 480-sample chunks
            this.processorNode = this.audioContext.createScriptProcessor(4096, 1, 1);

            // Allocate WASM memory for audio processing
            const inputPtr = this.rnnoiseModule._malloc(this.frameSize * 4);
            const outputPtr = this.rnnoiseModule._malloc(this.frameSize * 4);

            // Process audio through RNNoise
            this.processorNode.onaudioprocess = (e) => {
                const input = e.inputBuffer.getChannelData(0);
                const output = e.outputBuffer.getChannelData(0);

                // Process in 480-sample chunks
                for (let i = 0; i < input.length; i += this.frameSize) {
                    const chunkSize = Math.min(this.frameSize, input.length - i);

                    // Copy input to WASM memory (convert to int16 range that RNNoise expects)
                    for (let j = 0; j < chunkSize; j++) {
                        this.rnnoiseModule.HEAPF32[(inputPtr >> 2) + j] = input[i + j] * 32768;
                    }

                    // Process through RNNoise
                    this.rnnoiseModule._rnnoise_process_frame(
                        this.rnnoiseState,
                        outputPtr,
                        inputPtr
                    );

                    // Copy output from WASM memory (convert back from int16 range)
                    for (let j = 0; j < chunkSize; j++) {
                        output[i + j] = this.rnnoiseModule.HEAPF32[(outputPtr >> 2) + j] / 32768;
                    }
                }
            };

            // Create destination
            this.destinationNode = this.audioContext.createMediaStreamDestination();

            // Connect: source -> processor -> destination
            this.sourceNode.connect(this.processorNode);
            this.processorNode.connect(this.destinationNode);

            // Also connect to audio context destination to keep it running
            this.processorNode.connect(this.audioContext.destination);

            // Create new stream with processed audio + original video
            const processedStream = new MediaStream();

            // Add processed audio track
            this.destinationNode.stream.getAudioTracks().forEach(track => {
                processedStream.addTrack(track);
            });

            // Add original video tracks
            stream.getVideoTracks().forEach(track => {
                processedStream.addTrack(track);
            });

            console.log('Audio processing enabled with RNNoise');
            return processedStream;
        } catch (err) {
            console.error('Failed to setup audio processing:', err);
            return stream;
        }
    }

    setEnabled(enabled) {
        this.enabled = enabled;
    }

    cleanup() {
        if (this.sourceNode) {
            this.sourceNode.disconnect();
        }
        if (this.processorNode) {
            this.processorNode.disconnect();
        }
        if (this.rnnoiseState && this.rnnoiseModule) {
            this.rnnoiseModule._rnnoise_destroy(this.rnnoiseState);
        }
        if (this.audioContext) {
            this.audioContext.close();
        }
    }
}

class AirmeetClient {
    constructor() {
        this.ws = null;
        this.peerConnection = null;
        this.localStream = null;
        this.screenStream = null;
        this.peerId = null;
        this.roomId = null;
        this.password = null;
        this.displayName = null;
        this.isHost = false;
        this.peers = new Map(); // peerId -> { displayName, videoElement }
        this.iceServers = [];

        this.audioEnabled = true;
        this.videoEnabled = true;
        this.screenSharing = false;
        this.chatOpen = false;
        this.settingsOpen = false;

        // Audio settings
        this.audioSettings = {
            noiseSuppression: true,
            echoCancellation: true,
            autoGainControl: true,
            enhancedNoiseSuppression: false  // RNNoise
        };

        // Audio processor for RNNoise
        this.audioProcessor = new AudioProcessor();

        this.init();
    }

    init() {
        // DOM Elements
        this.joinScreen = document.getElementById('join-screen');
        this.meetingScreen = document.getElementById('meeting-screen');
        this.joinForm = document.getElementById('join-form');
        this.displayNameInput = document.getElementById('display-name');
        this.roomIdInput = document.getElementById('room-id');
        this.videoGrid = document.getElementById('video-grid');
        this.chatPanel = document.getElementById('chat-panel');
        this.chatMessages = document.getElementById('chat-messages');
        this.chatForm = document.getElementById('chat-form');
        this.chatInput = document.getElementById('chat-input');
        this.participantsList = document.getElementById('participants-list');
        this.participantCount = document.getElementById('participant-count');
        this.roomIdDisplay = document.getElementById('room-id-display');

        // Control buttons
        this.toggleAudioBtn = document.getElementById('toggle-audio');
        this.toggleVideoBtn = document.getElementById('toggle-video');
        this.toggleScreenBtn = document.getElementById('toggle-screen');
        this.toggleChatBtn = document.getElementById('toggle-chat');
        this.toggleSettingsBtn = document.getElementById('toggle-settings');
        this.leaveBtn = document.getElementById('leave-btn');
        this.copyLinkBtn = document.getElementById('copy-link');
        this.closeChatBtn = document.getElementById('close-chat');
        this.closeSettingsBtn = document.getElementById('close-settings');
        this.settingsPanel = document.getElementById('settings-panel');

        // Event listeners
        this.joinForm.addEventListener('submit', (e) => this.handleJoin(e));
        this.chatForm.addEventListener('submit', (e) => this.handleChatSubmit(e));
        this.toggleAudioBtn.addEventListener('click', () => this.toggleAudio());
        this.toggleVideoBtn.addEventListener('click', () => this.toggleVideo());
        this.toggleScreenBtn.addEventListener('click', () => this.toggleScreenShare());
        this.toggleChatBtn.addEventListener('click', () => this.toggleChat());
        this.toggleSettingsBtn.addEventListener('click', () => this.toggleSettings());
        this.leaveBtn.addEventListener('click', () => this.leave());
        this.copyLinkBtn.addEventListener('click', () => this.copyInviteLink());
        this.closeChatBtn.addEventListener('click', () => this.toggleChat());
        this.closeSettingsBtn.addEventListener('click', () => this.toggleSettings());

        // Audio settings listeners
        this.setupAudioSettingsListeners();

        // Check URL for room ID and password
        const urlParams = new URLSearchParams(window.location.search);
        const roomFromUrl = urlParams.get('room');
        const pwdFromUrl = urlParams.get('pwd');
        if (roomFromUrl) {
            this.roomIdInput.value = roomFromUrl;
            this.password = pwdFromUrl;
        }

        // Fetch ICE servers
        this.fetchICEServers();

        // Initialize audio processor
        this.audioProcessor.init();
    }

    setupAudioSettingsListeners() {
        const noiseSuppressionCheckbox = document.getElementById('noise-suppression');
        const echoCancellationCheckbox = document.getElementById('echo-cancellation');
        const autoGainCheckbox = document.getElementById('auto-gain');
        const enhancedNoiseCheckbox = document.getElementById('enhanced-noise');

        if (noiseSuppressionCheckbox) {
            noiseSuppressionCheckbox.checked = this.audioSettings.noiseSuppression;
            noiseSuppressionCheckbox.addEventListener('change', (e) => {
                this.audioSettings.noiseSuppression = e.target.checked;
                this.updateAudioConstraints();
            });
        }

        if (echoCancellationCheckbox) {
            echoCancellationCheckbox.checked = this.audioSettings.echoCancellation;
            echoCancellationCheckbox.addEventListener('change', (e) => {
                this.audioSettings.echoCancellation = e.target.checked;
                this.updateAudioConstraints();
            });
        }

        if (autoGainCheckbox) {
            autoGainCheckbox.checked = this.audioSettings.autoGainControl;
            autoGainCheckbox.addEventListener('change', (e) => {
                this.audioSettings.autoGainControl = e.target.checked;
                this.updateAudioConstraints();
            });
        }

        if (enhancedNoiseCheckbox) {
            enhancedNoiseCheckbox.checked = this.audioSettings.enhancedNoiseSuppression;
            enhancedNoiseCheckbox.addEventListener('change', (e) => {
                this.audioSettings.enhancedNoiseSuppression = e.target.checked;
                this.audioProcessor.setEnabled(e.target.checked);
                // Note: Enhanced noise suppression requires rejoining to take effect
                if (this.localStream) {
                    this.showNotification('Rejoin the meeting for enhanced noise suppression to take effect');
                }
            });
        }
    }

    async updateAudioConstraints() {
        if (!this.localStream) return;

        // Get current audio track
        const audioTrack = this.localStream.getAudioTracks()[0];
        if (!audioTrack) return;

        try {
            await audioTrack.applyConstraints({
                noiseSuppression: this.audioSettings.noiseSuppression,
                echoCancellation: this.audioSettings.echoCancellation,
                autoGainControl: this.audioSettings.autoGainControl
            });
            console.log('Audio constraints updated');
        } catch (err) {
            console.warn('Failed to update audio constraints:', err);
        }
    }

    showNotification(message) {
        // Simple notification - could be enhanced with a toast UI
        const notification = document.createElement('div');
        notification.className = 'notification';
        notification.textContent = message;
        document.body.appendChild(notification);
        setTimeout(() => notification.remove(), 3000);
    }

    showInviteLinkDialog() {
        const inviteUrl = `${window.location.origin}?room=${this.roomId}&pwd=${this.password}`;

        // Create modal overlay
        const overlay = document.createElement('div');
        overlay.className = 'modal-overlay';
        overlay.innerHTML = `
            <div class="modal">
                <h3>Share this link to invite others</h3>
                <div class="invite-link-container">
                    <input type="text" readonly value="${inviteUrl}" class="invite-link-input">
                    <button class="copy-btn">Copy</button>
                </div>
                <p class="invite-note">Others need this link to join. The room is password-protected.</p>
                <button class="close-modal-btn">Got it</button>
            </div>
        `;

        document.body.appendChild(overlay);

        // Copy button handler
        const copyBtn = overlay.querySelector('.copy-btn');
        const input = overlay.querySelector('.invite-link-input');
        copyBtn.addEventListener('click', () => {
            navigator.clipboard.writeText(inviteUrl).then(() => {
                copyBtn.textContent = 'Copied!';
                setTimeout(() => copyBtn.textContent = 'Copy', 2000);
            });
        });

        // Close button handler
        const closeBtn = overlay.querySelector('.close-modal-btn');
        closeBtn.addEventListener('click', () => {
            overlay.remove();
        });

        // Select input text on focus
        input.addEventListener('focus', () => input.select());
    }

    toggleSettings() {
        this.settingsOpen = !this.settingsOpen;
        this.settingsPanel.classList.toggle('hidden', !this.settingsOpen);
        this.toggleSettingsBtn.classList.toggle('active', this.settingsOpen);
    }

    async fetchICEServers() {
        try {
            const response = await fetch('/api/ice-servers');
            this.iceServers = await response.json();
        } catch (err) {
            console.warn('Failed to fetch ICE servers, using defaults:', err);
            this.iceServers = [{ urls: 'stun:stun.l.google.com:19302' }];
        }
    }

    async handleJoin(e) {
        e.preventDefault();

        this.displayName = this.displayNameInput.value.trim();
        this.roomId = this.roomIdInput.value.trim();

        if (!this.displayName) {
            alert('Please enter your name');
            return;
        }

        // Generate room ID if not provided
        if (!this.roomId) {
            try {
                const response = await fetch('/api/room/create', { method: 'POST' });
                const data = await response.json();
                this.roomId = data.roomId;
                this.password = data.password;
            } catch (err) {
                console.error('Failed to create room:', err);
                alert('Failed to create room');
                return;
            }
        }

        // Get local media stream with audio processing
        try {
            const rawStream = await navigator.mediaDevices.getUserMedia({
                video: true,
                audio: {
                    noiseSuppression: this.audioSettings.noiseSuppression,
                    echoCancellation: this.audioSettings.echoCancellation,
                    autoGainControl: this.audioSettings.autoGainControl
                }
            });

            // Apply enhanced noise suppression if enabled
            if (this.audioSettings.enhancedNoiseSuppression) {
                this.localStream = await this.audioProcessor.processStream(rawStream);
            } else {
                this.localStream = rawStream;
            }
        } catch (err) {
            console.error('Failed to get media:', err);
            alert('Failed to access camera/microphone. Please check permissions.');
            return;
        }

        // Connect to signaling server
        this.connect();
    }

    connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;

        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            console.log('Connected to signaling server');
            this.sendMessage({
                type: 'join',
                roomId: this.roomId,
                password: this.password || '',
                displayName: this.displayName
            });
        };

        this.ws.onmessage = (event) => {
            const message = JSON.parse(event.data);
            this.handleMessage(message);
        };

        this.ws.onclose = () => {
            console.log('Disconnected from signaling server');
        };

        this.ws.onerror = (err) => {
            console.error('WebSocket error:', err);
        };
    }

    handleMessage(message) {
        switch (message.type) {
            case 'joined':
                this.handleJoined(message);
                break;
            case 'peer-joined':
                this.handlePeerJoined(message);
                break;
            case 'peer-left':
                this.handlePeerLeft(message);
                break;
            case 'offer':
                this.handleOffer(message);
                break;
            case 'answer':
                this.handleAnswer(message);
                break;
            case 'ice-candidate':
                this.handleICECandidate(message);
                break;
            case 'chat':
                this.handleChatMessage(message);
                break;
            case 'mute':
                this.handleMuteMessage(message);
                break;
            case 'error':
                this.handleError(message);
                break;
        }
    }

    async handleJoined(message) {
        this.peerId = message.peerId;
        this.roomId = message.roomId;
        this.isHost = message.isHost;

        // Store password if host (for invite link)
        if (message.password) {
            this.password = message.password;
        }

        // Show meeting screen
        this.joinScreen.classList.add('hidden');
        this.meetingScreen.classList.remove('hidden');

        // Update URL (without password for security)
        const url = new URL(window.location);
        url.searchParams.set('room', this.roomId);
        url.searchParams.delete('pwd');
        window.history.replaceState({}, '', url);

        // Display room ID and host status
        this.roomIdDisplay.textContent = `Room: ${this.roomId}${this.isHost ? ' (Host)' : ''}`;

        // Show invite link dialog for host
        if (this.isHost && this.password) {
            this.showInviteLinkDialog();
        }

        // Add local video
        this.addLocalVideo();

        // Add existing peers
        for (const peer of message.peers) {
            this.peers.set(peer.peerId, {
                displayName: peer.displayName,
                videoElement: null
            });
            this.addPeerVideo(peer.peerId, peer.displayName);
        }

        // Update participants
        this.updateParticipants();

        // Create peer connection and send offer
        await this.createPeerConnection();
        await this.sendOffer();
    }

    async handlePeerJoined(message) {
        console.log('Peer joined:', message.peerId, message.displayName);

        this.peers.set(message.peerId, {
            displayName: message.displayName,
            videoElement: null
        });

        this.addPeerVideo(message.peerId, message.displayName);
        this.updateParticipants();
    }

    handlePeerLeft(message) {
        console.log('Peer left:', message.peerId);

        const peer = this.peers.get(message.peerId);
        if (peer && peer.videoElement) {
            peer.videoElement.parentElement.remove();
        }
        this.peers.delete(message.peerId);
        this.updateParticipants();
    }

    async createPeerConnection() {
        const config = {
            iceServers: this.iceServers
        };

        this.peerConnection = new RTCPeerConnection(config);

        // Add local tracks
        this.localStream.getTracks().forEach(track => {
            this.peerConnection.addTrack(track, this.localStream);
        });

        // Handle incoming tracks
        this.peerConnection.ontrack = (event) => {
            console.log('Received track:', event.track.kind);
            this.handleRemoteTrack(event);
        };

        // Handle ICE candidates
        this.peerConnection.onicecandidate = (event) => {
            if (event.candidate) {
                this.sendMessage({
                    type: 'ice-candidate',
                    candidate: event.candidate.candidate,
                    sdpMid: event.candidate.sdpMid,
                    sdpMLineIndex: event.candidate.sdpMLineIndex
                });
            }
        };

        // Handle connection state changes
        this.peerConnection.onconnectionstatechange = () => {
            console.log('Connection state:', this.peerConnection.connectionState);
        };

        // Handle ICE connection state changes
        this.peerConnection.oniceconnectionstatechange = () => {
            console.log('ICE connection state:', this.peerConnection.iceConnectionState);
        };
    }

    async sendOffer() {
        const offer = await this.peerConnection.createOffer();
        await this.peerConnection.setLocalDescription(offer);

        this.sendMessage({
            type: 'offer',
            sdp: offer.sdp
        });
    }

    async handleOffer(message) {
        if (!this.peerConnection) {
            await this.createPeerConnection();
        }

        await this.peerConnection.setRemoteDescription({
            type: 'offer',
            sdp: message.sdp
        });

        const answer = await this.peerConnection.createAnswer();
        await this.peerConnection.setLocalDescription(answer);

        this.sendMessage({
            type: 'answer',
            sdp: answer.sdp
        });
    }

    async handleAnswer(message) {
        await this.peerConnection.setRemoteDescription({
            type: 'answer',
            sdp: message.sdp
        });
    }

    async handleICECandidate(message) {
        if (this.peerConnection && message.candidate) {
            await this.peerConnection.addIceCandidate({
                candidate: message.candidate,
                sdpMid: message.sdpMid,
                sdpMLineIndex: message.sdpMLineIndex
            });
        }
    }

    handleRemoteTrack(event) {
        // Get or create a video element for remote streams
        const stream = event.streams[0];
        if (!stream) return;

        // Find or create video container for remote peers
        let container = document.querySelector('.video-container.remote');
        if (!container) {
            container = document.createElement('div');
            container.className = 'video-container remote';

            const video = document.createElement('video');
            video.autoplay = true;
            video.playsInline = true;

            const label = document.createElement('div');
            label.className = 'video-label';
            label.textContent = 'Participant';

            container.appendChild(video);
            container.appendChild(label);
            this.videoGrid.appendChild(container);
        }

        const video = container.querySelector('video');
        if (video.srcObject !== stream) {
            video.srcObject = stream;
        }

        this.updateVideoGrid();
    }

    addLocalVideo() {
        const container = document.createElement('div');
        container.className = 'video-container local';
        container.id = 'local-video-container';

        const video = document.createElement('video');
        video.id = 'local-video';
        video.autoplay = true;
        video.playsInline = true;
        video.muted = true;
        video.srcObject = this.localStream;

        const label = document.createElement('div');
        label.className = 'video-label';
        label.textContent = `${this.displayName} (You)`;

        container.appendChild(video);
        container.appendChild(label);
        this.videoGrid.appendChild(container);
        this.updateVideoGrid();
    }

    addPeerVideo(peerId, displayName) {
        // Peer videos will be handled via remote tracks
        // This just sets up the display name association
        const peer = this.peers.get(peerId);
        if (peer) {
            peer.displayName = displayName;
        }
    }

    updateVideoGrid() {
        const count = this.videoGrid.children.length;
        this.videoGrid.className = '';

        if (count === 1) {
            this.videoGrid.classList.add('single-video');
        } else if (count === 2) {
            this.videoGrid.classList.add('two-videos');
        } else {
            this.videoGrid.classList.add('many-videos');
        }
    }

    updateParticipants() {
        this.participantsList.innerHTML = '';

        // Add self
        const selfLi = document.createElement('li');
        selfLi.innerHTML = `<span class="status-indicator"></span>${this.displayName} (You)`;
        this.participantsList.appendChild(selfLi);

        // Add peers
        for (const [peerId, peer] of this.peers) {
            const li = document.createElement('li');
            li.innerHTML = `<span class="status-indicator"></span>${peer.displayName}`;
            this.participantsList.appendChild(li);
        }

        this.participantCount.textContent = this.peers.size + 1;
    }

    toggleAudio() {
        this.audioEnabled = !this.audioEnabled;

        this.localStream.getAudioTracks().forEach(track => {
            track.enabled = this.audioEnabled;
        });

        this.toggleAudioBtn.classList.toggle('muted', !this.audioEnabled);

        this.sendMessage({
            type: 'mute',
            kind: 'audio',
            muted: !this.audioEnabled
        });
    }

    toggleVideo() {
        this.videoEnabled = !this.videoEnabled;

        this.localStream.getVideoTracks().forEach(track => {
            track.enabled = this.videoEnabled;
        });

        this.toggleVideoBtn.classList.toggle('muted', !this.videoEnabled);

        this.sendMessage({
            type: 'mute',
            kind: 'video',
            muted: !this.videoEnabled
        });
    }

    async toggleScreenShare() {
        if (!this.screenSharing) {
            try {
                this.screenStream = await navigator.mediaDevices.getDisplayMedia({
                    video: true
                });

                // Replace video track
                const screenTrack = this.screenStream.getVideoTracks()[0];
                const sender = this.peerConnection.getSenders()
                    .find(s => s.track && s.track.kind === 'video');

                if (sender) {
                    await sender.replaceTrack(screenTrack);
                }

                // Update local video
                const localVideo = document.getElementById('local-video');
                localVideo.srcObject = this.screenStream;

                // Handle screen share end
                screenTrack.onended = () => {
                    this.stopScreenShare();
                };

                this.screenSharing = true;
                this.toggleScreenBtn.classList.add('active');
            } catch (err) {
                console.error('Failed to start screen share:', err);
            }
        } else {
            this.stopScreenShare();
        }
    }

    async stopScreenShare() {
        if (this.screenStream) {
            this.screenStream.getTracks().forEach(track => track.stop());
        }

        // Restore video track
        const videoTrack = this.localStream.getVideoTracks()[0];
        const sender = this.peerConnection.getSenders()
            .find(s => s.track && s.track.kind === 'video');

        if (sender && videoTrack) {
            await sender.replaceTrack(videoTrack);
        }

        // Update local video
        const localVideo = document.getElementById('local-video');
        localVideo.srcObject = this.localStream;

        this.screenSharing = false;
        this.toggleScreenBtn.classList.remove('active');
    }

    toggleChat() {
        this.chatOpen = !this.chatOpen;
        this.chatPanel.classList.toggle('hidden', !this.chatOpen);
        this.toggleChatBtn.classList.toggle('active', this.chatOpen);
    }

    handleChatSubmit(e) {
        e.preventDefault();

        const message = this.chatInput.value.trim();
        if (!message) return;

        this.sendMessage({
            type: 'chat',
            message: message
        });

        this.chatInput.value = '';
    }

    handleChatMessage(message) {
        const div = document.createElement('div');
        div.className = 'chat-message';

        const isOwn = message.fromPeerId === this.peerId;
        const peer = this.peers.get(message.fromPeerId);
        const senderName = isOwn ? 'You' : (peer ? peer.displayName : 'Unknown');

        const time = message.timestamp ?
            new Date(message.timestamp).toLocaleTimeString() :
            new Date().toLocaleTimeString();

        div.innerHTML = `
            <div class="sender">${senderName}</div>
            <div class="text">${this.escapeHtml(message.message)}</div>
            <div class="time">${time}</div>
        `;

        this.chatMessages.appendChild(div);
        this.chatMessages.scrollTop = this.chatMessages.scrollHeight;
    }

    handleMuteMessage(message) {
        console.log('Mute status:', message);
        // Could update UI to show mute indicators on peer videos
    }

    handleError(message) {
        console.error('Server error:', message.message);
        alert('Error: ' + message.message);
    }

    leave() {
        if (this.peerConnection) {
            this.peerConnection.close();
        }

        if (this.localStream) {
            this.localStream.getTracks().forEach(track => track.stop());
        }

        if (this.screenStream) {
            this.screenStream.getTracks().forEach(track => track.stop());
        }

        if (this.ws) {
            this.ws.close();
        }

        // Cleanup audio processor
        this.audioProcessor.cleanup();

        // Reset state
        this.peers.clear();
        this.videoGrid.innerHTML = '';

        // Show join screen
        this.meetingScreen.classList.add('hidden');
        this.joinScreen.classList.remove('hidden');

        // Clear URL params
        window.history.replaceState({}, '', window.location.pathname);
    }

    copyInviteLink() {
        if (!this.password) {
            alert('Only the host can share the invite link');
            return;
        }
        const url = `${window.location.origin}?room=${this.roomId}&pwd=${this.password}`;
        navigator.clipboard.writeText(url).then(() => {
            alert('Invite link copied to clipboard!');
        }).catch(() => {
            prompt('Copy this invite link:', url);
        });
    }

    sendMessage(message) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify(message));
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    window.airmeet = new AirmeetClient();
});
