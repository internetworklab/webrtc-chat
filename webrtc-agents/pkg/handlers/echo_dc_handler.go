package handlers

import (
	"context"
	"encoding/json"
	"log"
	"time"

	pkgframing "example.com/webrtcserver/pkg/framing"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

// EchoHandler handles WebRTC peer connections
type EchoDCHandler struct{}

func (_ *EchoDCHandler) Serve(ctx context.Context, dc *webrtc.DataChannel, signallingTx chan<- pkgframing.MessagePayload) {

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		// Parse the message as ChatMessage
		var chatMsg ChatMessage
		if err := json.Unmarshal(msg.Data, &chatMsg); err != nil {
			log.Printf("[webrtc] Failed to parse chat message: %v", err)
			return
		}

		remoteNodeID := chatMsg.FromNodeID

		// Skip control messages that shouldn't be echoed
		// - ack: acknowledgment messages
		// - delete: delete request messages
		// - amend: amend request messages
		// - ping: ping/pong messages
		// - file: file transfer metadata
		if chatMsg.ACK != nil || chatMsg.Delete != nil || chatMsg.Amend != nil || chatMsg.Ping != nil || chatMsg.File != nil {
			return
		}

		// Only echo messages that have actual text content
		if chatMsg.Message == nil && chatMsg.RichText == nil {
			log.Printf("[webrtc] Skipping non-text message from peer %s", remoteNodeID)
			return
		}

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

		// Swap sender and receiver for echo
		chatMsg.FromNodeID, chatMsg.ToNodeID = chatMsg.ToNodeID, chatMsg.FromNodeID
		chatMsg.ACK = nil
		chatMsg.ACKed = nil
		chatMsg.MessageID = uuid.New().String()

		// Marshal the modified message
		responseData, err := json.Marshal(chatMsg)
		if err != nil {
			log.Printf("[webrtc] Failed to marshal chat message: %v", err)
			return
		}

		// Send the modified message back
		if err := dc.SendText(string(responseData)); err != nil {
			log.Printf("[webrtc] Failed to send chat response: %v", err)
		}
	})
}
