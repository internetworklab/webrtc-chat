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

type GeneralPropertyKey string

const (
	GP_Key_NodeId = "node_id"
)

// CounterHandler handles WebRTC peer connections with per-user counter functionality
type CounterDCHandler struct {
	counters sync.Map
}

func NewCounterDCHandler() *CounterDCHandler {
	return &CounterDCHandler{
		counters: sync.Map{},
	}
}

// Serve starts the WebRTC handler
func (h *CounterDCHandler) Serve(ctx context.Context, dc *webrtc.DataChannel, signallingTx chan<- pkgframing.MessagePayload) {

	remoteNodeID := ctx.Value(DCHandlerCtxRemoteNodeID).(string)
	log.Printf("[webrtc] Received data channel: %s from peer %s", dc.Label(), remoteNodeID)

	switch dc.Label() {
	case PredefinedDCLabelChat:
		h.setupChatDataChannel(dc, remoteNodeID)
	default:
		log.Printf("[webrtc] Unknown (or unsupported) data channel label: %s", dc.Label())
	}
}

// setupChatDataChannel sets up the chat data channel for handling messages
func (h *CounterDCHandler) setupChatDataChannel(dc *webrtc.DataChannel, remoteNodeID string) {
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

			// Handle /increase command - increment the counter
			if msg == "/increase" {
				log.Printf("[webrtc] Received /increase command from peer %s", remoteNodeID)
				newValue := h.increaseCounter(remoteNodeID)
				h.sendChatResponse(dc, &chatMsg, fmt.Sprintf("Counter increased! Current value: %d", newValue))
				return
			}

			// Handle /reset command - reset the counter
			if msg == "/reset" {
				log.Printf("[webrtc] Received /reset command from peer %s", remoteNodeID)
				h.resetCounter(remoteNodeID)
				h.sendChatResponse(dc, &chatMsg, "Counter reset! Current value: 0")
				return
			}

		}

	})

	dc.OnError(func(err error) {
		log.Printf("[webrtc] Chat data channel error with peer %s: %v", remoteNodeID, err)
	})
}

// formatHelp returns a formatted string with usage information
func (h *CounterDCHandler) formatHelp() string {
	return `🔢 Counter Bot - Available Commands:

/start - Show this help message
/increase - Increase your personal counter by 1
/reset - Reset your personal counter to 0

Your counter is personal and independent from other users!`
}

// increaseCounter increments the counter for the given remote node ID and returns the new value
func (h *CounterDCHandler) increaseCounter(remoteNodeID string) int {
	for {
		// Load the current value
		val, _ := h.counters.LoadOrStore(remoteNodeID, 0)
		newValue := val.(int) + 1

		// Try to atomically swap the old value with the new value
		// If the key doesn't exist yet, val will be nil, which is correct for CompareAndSwap
		if h.counters.CompareAndSwap(remoteNodeID, val, newValue) {
			return newValue
		}
		// If CompareAndSwap failed, another goroutine modified the value, so retry
	}
}

// resetCounter resets the counter for the given remote node ID to 0
func (h *CounterDCHandler) resetCounter(remoteNodeID string) {
	h.counters.Store(remoteNodeID, 0)
}

// getCounter returns the current counter value for the given remote node ID
func (h *CounterDCHandler) getCounter(remoteNodeID string) int {
	if val, ok := h.counters.Load(remoteNodeID); ok {
		if intVal, isValid := val.(int); isValid {
			return intVal
		}
	}
	return 0
}

// sendChatResponse sends a chat message response back to the peer
func (h *CounterDCHandler) sendChatResponse(dc *webrtc.DataChannel, originalMsg *ChatMessage, responseText string) {
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
