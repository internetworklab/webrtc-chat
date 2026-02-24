package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	pkgtracks "webrtc-agents/pkg/tracks"
	pkgoggfile "webrtc-agents/pkg/tracks/oggfile"
	pkgsine "webrtc-agents/pkg/tracks/sine"
	pkgwn "webrtc-agents/pkg/tracks/wn"

	pkgconnreg "example.com/webrtcserver/pkg/connreg"
	pkgframing "example.com/webrtcserver/pkg/framing"

	"github.com/google/uuid"
	"github.com/hraban/opus"
	"github.com/pion/webrtc/v4"
)

type TrackDCHandler struct {
	debug      bool
	generators []pkgtracks.RTPPacketGenerator
	oggFiles   []string // OGG files to load as tracks
}

func NewTrackDCHandler(oggFiles []string, debug bool) (*TrackDCHandler, error) {
	h := &TrackDCHandler{
		debug:      debug,
		generators: []pkgtracks.RTPPacketGenerator{},
		oggFiles:   oggFiles,
	}

	// Create opus encoder for the generators
	enc, err := opus.NewEncoder(
		pkgtracks.DefaultSampleRate,
		pkgtracks.DefaultChannelsCount,
		opus.AppAudio,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create opus encoder: %v", err)
	}

	samplesPerPacket := int(float64(pkgtracks.DefaultFrameIntv.Seconds()) * float64(pkgtracks.DefaultSampleRate))

	// Create C4 sine wave generator (261.63 Hz)
	c4Gen, err := pkgsine.NewOpusSineWaveformGenerator(
		"C4",
		enc,
		pkgtracks.DefaultChannelsCount,
		samplesPerPacket,
		pkgtracks.DefaultMaxPayloadSize,
		261.63,
		pkgtracks.DefaultSampleRate,
	)
	if err != nil {
		log.Printf("failed to create C4 generator: %v", err)
		return nil, fmt.Errorf("failed to create C4 generator: %v", err)
	}
	h.generators = append(h.generators, c4Gen)

	// Create A4 sine wave generator (440 Hz)
	a4Gen, err := pkgsine.NewOpusSineWaveformGenerator(
		"A4",
		enc,
		pkgtracks.DefaultChannelsCount,
		samplesPerPacket,
		pkgtracks.DefaultMaxPayloadSize,
		440.0,
		pkgtracks.DefaultSampleRate,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create A4 generator: %v", err)
	}
	h.generators = append(h.generators, a4Gen)

	// Create White Noise generator
	whiteNoiseGen, err := pkgwn.NewOpusWhiteNoiseGenerator(
		"WhiteNoise",
		enc,
		pkgtracks.DefaultChannelsCount,
		samplesPerPacket,
		pkgtracks.DefaultMaxPayloadSize,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create WhiteNoise generator: %v", err)
	}
	h.generators = append(h.generators, whiteNoiseGen)

	// Load OGG files
	h.loadOggFiles()

	return h, nil
}

// loadOggFile loads a single OGG file as a generator
// Validates that the file has the correct sample rate (48000) and channel count (2)
func (h *TrackDCHandler) loadOggFile(filePath string) error {
	// Create generator - name is automatically set from filename
	gen, err := pkgoggfile.NewOggFileGenerator("", filePath)
	if err != nil {
		return fmt.Errorf("failed to load OGG file %s: %w", filePath, err)
	}

	// Validate sample rate and channels
	sampleRate := gen.GetSampleRate()
	channels := gen.GetChannels()
	if sampleRate != pkgtracks.DefaultSampleRate {
		return fmt.Errorf("OGG file %s has sample rate %d, expected %d", filePath, sampleRate, pkgtracks.DefaultSampleRate)
	}
	if channels != pkgtracks.DefaultChannelsCount {
		return fmt.Errorf("OGG file %s has %d channels, expected %d", filePath, channels, pkgtracks.DefaultChannelsCount)
	}

	h.generators = append(h.generators, gen)
	log.Printf("[webrtc] Loaded OGG track: %s (sampleRate=%d, channels=%d)", gen.GetName(), sampleRate, channels)
	return nil
}

// loadOggFiles loads all configured OGG files as generators
func (h *TrackDCHandler) loadOggFiles() {
	for _, filePath := range h.oggFiles {
		if err := h.loadOggFile(filePath); err != nil {
			log.Printf("[webrtc] %v", err)
		}
	}
}

func (h *TrackDCHandler) Serve(ctx context.Context, dc *webrtc.DataChannel, signallingTx chan<- pkgframing.MessagePayload) {
	if dc.Label() == PredefinedDCLabelChat {
		h.setupChatDataChannel(ctx, dc, signallingTx)
	}
}

// setupChatDataChannel sets up event handlers for chat data channel
func (h *TrackDCHandler) setupChatDataChannel(ctx context.Context, dc *webrtc.DataChannel, signallingTx chan<- pkgframing.MessagePayload) {
	remoteNodeID := ctx.Value(DCHandlerCtxRemoteNodeID).(string)

	dc.OnOpen(func() {
		log.Printf("[webrtc] Chat data channel opened with peer %s", remoteNodeID)
	})

	dc.OnClose(func() {
		log.Printf("[webrtc] Chat data channel closed with peer %s", remoteNodeID)
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		// Parse the message as ChatMessage
		var chatMsg ChatMessage
		if err := json.Unmarshal(msg.Data, &chatMsg); err != nil {
			log.Printf("[webrtc] Failed to parse chat message: %v", err)
			return
		}

		if chatMsg.ACK != nil {
			return
		}

		// Handle commands
		if chatMsg.Message != nil {

			msg := *chatMsg.Message

			// Send ACK for the received message first
			ackMsg := ChatMessage{
				MessageID:  uuid.New().String(),
				FromNodeID: chatMsg.ToNodeID,
				ToNodeID:   chatMsg.FromNodeID,
				Timestamp:  time.Now().UnixMilli(),
				ACK: &ChatMessageACK{
					MessageID: chatMsg.MessageID,
				},
			}
			ackData, err := json.Marshal(ackMsg)
			if err != nil {
				log.Printf("[webrtc] Failed to marshal ACK message: %v", err)
			} else if err := dc.SendText(string(ackData)); err != nil {
				log.Printf("[webrtc] Failed to send ACK: %v", err)
			}

			// Handle /start command - show usage/help
			if msg == "/start" {
				log.Printf("[webrtc] Received /start command from peer %s", remoteNodeID)
				h.sendChatResponse(dc, &chatMsg, h.formatHelp())
				return
			}

			// Handle /list command - list all available tracks
			if msg == "/list" {
				log.Printf("[webrtc] Received /list command from peer %s", remoteNodeID)
				h.sendChatResponse(dc, &chatMsg, h.formatTrackList())
				return
			}

			// Handle /play command - play a specific track
			if len(msg) > 6 && msg[:6] == "/play " {
				trackName := msg[6:]
				log.Printf("[webrtc] Received /play command from peer %s for track: %s", remoteNodeID, trackName)

				// Check if track exists
				found := false
				for _, gen := range h.generators {
					if gen.GetName() == trackName {
						found = true
						break
					}
				}

				if !found {
					errMsg := fmt.Sprintf("Error: Track '%s' not found. Use /list to see available tracks.", trackName)
					h.sendChatResponse(dc, &chatMsg, errMsg)
					return
				}

				peerConnStore := ctx.Value(DCHandlerCtxPeerConnStore).(*PeerConnStore)
				if peerConnStore == nil {
					panic("peerConnStore is not provided in the context")
				}

				entry, found := peerConnStore.GetPeerConnEntry(remoteNodeID)
				if !found {
					log.Printf("[webrtc] No peer connection found for peer %s", remoteNodeID)
					return
				}
				if err := h.createAndAddTrack(ctx, entry, remoteNodeID, signallingTx, trackName); err != nil {
					log.Printf("[webrtc] Failed to create track for peer %s: %v", remoteNodeID, err)
					h.sendChatResponse(dc, &chatMsg, fmt.Sprintf("Error: Failed to play track: %v", err))
				} else {
					h.sendChatResponse(dc, &chatMsg, fmt.Sprintf("Now playing: %s", trackName))
				}
				return
			}

		}

	})

	dc.OnError(func(err error) {
		log.Printf("[webrtc] Chat data channel error with peer %s: %v", remoteNodeID, err)
	})
}

// formatTrackList returns a formatted string listing all available tracks
func (h *TrackDCHandler) formatTrackList() string {
	if len(h.generators) == 0 {
		return "No tracks available."
	}
	result := "Available tracks:\n"
	for _, gen := range h.generators {
		result += fmt.Sprintf("  - %s\n", gen.GetName())
	}
	result += "\nUse /play <track_name> to play a track."
	return result
}

// formatHelp returns a formatted string with usage information
func (h *TrackDCHandler) formatHelp() string {
	return `🎵 WebRTC Audio Bot - Available Commands:

/start - Show this help message
/list - List all available audio tracks
/play <track_name> - Play a specific audio track

Example: /play ambient_music`
}

// sendChatResponse sends a chat message response back to the peer
func (h *TrackDCHandler) sendChatResponse(dc *webrtc.DataChannel, originalMsg *ChatMessage, responseText string) {
	responseMsg := ChatMessage{
		MessageID:  uuid.New().String(),
		FromNodeID: originalMsg.ToNodeID,
		ToNodeID:   originalMsg.FromNodeID,
		Timestamp:  time.Now().UnixMilli(),
		Message:    &responseText,
	}
	responseData, err := json.Marshal(responseMsg)
	if err != nil {
		log.Printf("[webrtc] Failed to marshal response message: %v", err)
		return
	}
	if err := dc.SendText(string(responseData)); err != nil {
		log.Printf("[webrtc] Failed to send response: %v", err)
	}
}

// createAndAddTrack creates a new audio track and adds it to the peer connection
func (h *TrackDCHandler) createAndAddTrack(ctx context.Context, entry *PeerConnEntry, remoteNodeID string, signallingTx chan<- pkgframing.MessagePayload, generatorName string) error {
	// Find the generator by name
	var selectedGenerator pkgtracks.RTPPacketGenerator
	for _, gen := range h.generators {
		if gen.GetName() == generatorName {
			selectedGenerator = gen
			break
		}
	}
	if selectedGenerator == nil {
		return fmt.Errorf("generator not found: %s", generatorName)
	}

	frameIntv := pkgtracks.DefaultFrameIntv
	sampleRate := pkgtracks.DefaultSampleRate
	channels := pkgtracks.DefaultChannelsCount

	streamId := "stream-" + uuid.NewString()
	trackId := "track-" + uuid.NewString()

	// Check if user already has an unclosed track and remove it
	entry.mu.Lock()
	if entry.CurrentTrackSender != nil {
		log.Printf("[webrtc] Removing existing track for peer %s before adding new track", remoteNodeID)
		entry.CurrentTrackSender.Stop()
		if err := entry.PeerConnection.RemoveTrack(entry.CurrentTrackSender); err != nil {
			log.Printf("[webrtc] Warning: failed to remove existing track for peer %s: %v", remoteNodeID, err)
		}
		entry.CurrentTrackSender = nil
	}
	entry.mu.Unlock()

	// Create a new audio track
	track, err := pkgtracks.NewTrackHandle(
		streamId,
		trackId,
		frameIntv,
		sampleRate,
		channels,
		selectedGenerator)
	if err != nil {
		return fmt.Errorf("failed to create track: %w", err)
	}

	// Add the track to the peer connection
	sender, err := entry.PeerConnection.AddTrack(track)
	if err != nil {
		return fmt.Errorf("failed to add track to peer connection: %w", err)
	}

	// Store the sender for this user
	entry.mu.Lock()
	entry.CurrentTrackSender = sender
	entry.mu.Unlock()

	log.Printf("[webrtc] Created and added track %s for peer %s", track.ID(), remoteNodeID)

	// Create a new offer since we added a track
	offer, err := entry.PeerConnection.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("failed to create offer: %w", err)
	}

	// Set local description
	if err := entry.PeerConnection.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("failed to set local description: %w", err)
	}

	// Marshal the offer to JSON
	offerJSON, err := json.Marshal(offer)
	if err != nil {
		return fmt.Errorf("failed to marshal offer: %w", err)
	}

	ourNodeID := ctx.Value(DCHandlerCtxOurNodeID).(string)
	if ourNodeID == "" {
		panic("ourNodeID is not provided in the context")
	}

	// Send the offer via signalling server
	offerMsg := pkgframing.MessagePayload{
		SDPOffer: &pkgconnreg.SDPOfferPayload{
			Type:       pkgconnreg.OfferTypeOffer,
			OfferJSON:  string(offerJSON),
			FromNodeId: ourNodeID,
			ToNodeId:   remoteNodeID,
		},
	}

	signallingTx <- offerMsg

	log.Printf("[webrtc] Sent offer with new track to peer %s", remoteNodeID)

	return nil
}
