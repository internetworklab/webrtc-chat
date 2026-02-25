package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	pkgframing "example.com/webrtcserver/pkg/framing"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

type ClockBotDCHandler struct {
	APIKey string

	generalProperties sync.Map
}

// NewClockBotDCHandler creates a new ClockBotDCHandler
func NewClockBotDCHandler(apiKey string) *ClockBotDCHandler {
	return &ClockBotDCHandler{
		APIKey: apiKey,
	}
}

// Serve starts the WebRTC handler
func (h *ClockBotDCHandler) Serve(ctx context.Context, dc *webrtc.DataChannel, signallingTx chan<- pkgframing.MessagePayload) {
	remoteNodeID := ctx.Value(DCHandlerCtxRemoteNodeID).(string)
	log.Printf("[webrtc] Received data channel: %s from peer %s", dc.Label(), remoteNodeID)

	switch dc.Label() {
	case PredefinedDCLabelChat:
		h.setupChatDataChannel(ctx, dc, remoteNodeID, signallingTx)
	default:
		log.Printf("[webrtc] Unknown (or unsupported) data channel label: %s", dc.Label())
	}
}

// setupChatDataChannel sets up the chat data channel for handling messages
func (h *ClockBotDCHandler) setupChatDataChannel(ctx context.Context, dc *webrtc.DataChannel, remoteNodeID string, signallingTx chan<- pkgframing.MessagePayload) {
	dc.OnOpen(func() {
		log.Printf("[webrtc] Chat data channel opened with peer %s", remoteNodeID)
	})

	dc.OnClose(func() {
		log.Printf("[webrtc] Chat data channel closed with peer %s", remoteNodeID)
		// Stop any running clock updater for this peer
		h.stopClockUpdater(remoteNodeID)
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
			msgText := *chatMsg.Message

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
			if msgText == "/start" {
				log.Printf("[webrtc] Received /start command from peer %s", remoteNodeID)
				h.sendChatResponse(dc, &chatMsg, h.formatHelp())
				return
			}

			// Handle /now command - start clock updates
			if msgText == "/now" {
				log.Printf("[webrtc] Received /now command from peer %s", remoteNodeID)
				h.startClock(ctx, dc, &chatMsg, remoteNodeID, signallingTx)
				return
			}
		}
	})

	dc.OnError(func(err error) {
		log.Printf("[webrtc] Chat data channel error with peer %s: %v", remoteNodeID, err)
	})
}

// formatHelp returns a formatted string with usage information
func (h *ClockBotDCHandler) formatHelp() string {
	return `🕐 Clock Bot - Available Commands:

/start - Show this help message
/now - Display a real-time clock that updates every second

The clock will continue updating until you send another command or disconnect.`
}

// startClock initiates the clock update functionality
func (h *ClockBotDCHandler) startClock(ctx context.Context, chatDC *webrtc.DataChannel, originalMsg *ChatMessage, remoteNodeID string, signallingTx chan<- pkgframing.MessagePayload) {
	// Stop any existing clock updater for this peer
	h.stopClockUpdater(remoteNodeID)

	// Create a cancellable context for the clock updater
	msgStreamCtx, cancel := context.WithCancel(ctx)

	// Store the cancel function in generalProperties
	h.generalProperties.Store(remoteNodeID+"_cancel_func", cancel)

	// Generate a message ID for the clock message that we'll update
	messageID := uuid.New().String()

	// Send initial clock message via chat
	initialTime := fmt.Sprintf("🕐 Current Time: %s", time.Now().Format("15:04:05"))
	h.sendChatResponseWithID(chatDC, originalMsg, initialTime, messageID)

	// Get peer connection from context
	peerConnStore := ctx.Value(DCHandlerCtxPeerConnStore).(*PeerConnStore)
	if peerConnStore == nil {
		log.Printf("[webrtc] PeerConnStore not found in context for peer %s", remoteNodeID)
		cancel()
		return
	}

	entry, found := peerConnStore.GetPeerConnEntry(remoteNodeID)
	if !found {
		log.Printf("[webrtc] No peer connection found for peer %s", remoteNodeID)
		cancel()
		return
	}

	// Create a new data channel for message stream
	msgStreamDC, err := entry.PeerConnection.CreateDataChannel(PredefinedDCLabelMsgStream, nil)
	if err != nil {
		log.Printf("[webrtc] Failed to create msgstream data channel for peer %s: %v", remoteNodeID, err)
		cancel()
		return
	}

	// Set up the message stream data channel
	h.setupMsgStreamDataChannel(msgStreamCtx, msgStreamDC, remoteNodeID, messageID)
}

// setupMsgStreamDataChannel sets up the message stream data channel for sending clock updates
func (h *ClockBotDCHandler) setupMsgStreamDataChannel(ctx context.Context, dc *webrtc.DataChannel, remoteNodeID string, messageID string) {
	dc.OnOpen(func() {
		log.Printf("[webrtc] MsgStream data channel opened with peer %s", remoteNodeID)
		// Start sending clock updates
		go h.sendClockUpdates(ctx, dc, messageID)
	})

	dc.OnClose(func() {
		log.Printf("[webrtc] MsgStream data channel closed with peer %s", remoteNodeID)
		h.stopClockUpdater(remoteNodeID)
	})

	dc.OnError(func(err error) {
		log.Printf("[webrtc] MsgStream data channel error with peer %s: %v", remoteNodeID, err)
		h.stopClockUpdater(remoteNodeID)
	})
}

// sendClockUpdates sends clock updates every second via the message stream data channel
func (h *ClockBotDCHandler) sendClockUpdates(ctx context.Context, dc *webrtc.DataChannel, messageID string) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[webrtc] Clock updates stopped for message %s", messageID)
			return
		case <-ticker.C:
			// Create a MessagePatchOrder to replace the clock message
			patch := MessagePatchOrder{
				MessageID: messageID,
				Kind:      MessagePatchOrderKindReplace,
				Value:     fmt.Sprintf("🕐 Current Time: %s", time.Now().Format(time.RFC3339)),
			}

			// Marshal the patch to binary
			data, err := patch.Marshal()
			if err != nil {
				log.Printf("[webrtc] Failed to marshal MessagePatchOrder: %v", err)
				continue
			}

			// Send the binary data
			if err := dc.Send(data); err != nil {
				log.Printf("[webrtc] Failed to send clock update: %v", err)
				return
			}

			log.Printf("[webrtc] Sent clock update for message %s", messageID)
		}
	}
}

// stopClockUpdater stops the clock updater for a given remote node ID
func (h *ClockBotDCHandler) stopClockUpdater(remoteNodeID string) {
	key := remoteNodeID + "_cancel_func"
	if value, exists := h.generalProperties.Load(key); exists {
		if cancel, ok := value.(context.CancelFunc); ok {
			cancel()
			h.generalProperties.Delete(key)
			log.Printf("[webrtc] Stopped clock updater for peer %s", remoteNodeID)
		}
	}
}

// sendChatResponse sends a chat message response back to the peer
func (h *ClockBotDCHandler) sendChatResponse(dc *webrtc.DataChannel, originalMsg *ChatMessage, responseText string) {
	messageID := uuid.New().String()
	h.sendChatResponseWithID(dc, originalMsg, responseText, messageID)
}

// sendChatResponseWithID sends a chat message response with a specific message ID
func (h *ClockBotDCHandler) sendChatResponseWithID(dc *webrtc.DataChannel, originalMsg *ChatMessage, responseText string, messageID string) {
	responseMsg := ChatMessage{
		MessageID:  messageID,
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
