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

type PingDCHandler struct{}

// Serve starts the WebRTC handler
func (h *PingDCHandler) Serve(ctx context.Context, dc *webrtc.DataChannel, signallingTx chan<- pkgframing.MessagePayload) {

	remoteNodeID := ctx.Value(DCHandlerCtxRemoteNodeID).(string)

	if dc.Label() == PredefinedDCLabelPing {
		h.setupPingDataChannel(dc, remoteNodeID)
	}
}

// setupPingDataChannel sets up the ping data channel for handling ping/pong messages
func (h *PingDCHandler) setupPingDataChannel(dc *webrtc.DataChannel, remoteNodeID string) {
	dc.OnOpen(func() {
		log.Printf("[webrtc] Ping data channel opened with peer %s", remoteNodeID)
	})

	dc.OnClose(func() {
		log.Printf("[webrtc] Ping data channel closed with peer %s", remoteNodeID)
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		// Parse the message as ChatMessage
		var chatMsg ChatMessage
		if err := json.Unmarshal(msg.Data, &chatMsg); err != nil {
			log.Printf("[webrtc] Failed to parse ping message: %v", err)
			return
		}

		// Handle ping message
		if chatMsg.Ping != nil && chatMsg.Ping.Direction == ChatMessagePingDirectionPing {
			// Send pong response
			pongMsg := ChatMessage{
				MessageID:  uuid.New().String(),
				FromNodeID: chatMsg.ToNodeID,
				ToNodeID:   chatMsg.FromNodeID,
				Timestamp:  time.Now().UnixMilli(),
				Ping: &ChatMessagePing{
					Direction: ChatMessagePingDirectionPong,
					Seq:       chatMsg.Ping.Seq,
				},
			}
			pongData, err := json.Marshal(pongMsg)
			if err != nil {
				log.Printf("[webrtc] Failed to marshal pong message: %v", err)
				return
			}
			if err := dc.SendText(string(pongData)); err != nil {
				log.Printf("[webrtc] Failed to send pong: %v", err)
			}
		}
	})

	dc.OnError(func(err error) {
		log.Printf("[webrtc] Ping data channel error with peer %s: %v", remoteNodeID, err)
	})
}

type WrappedDCHandler struct {
	next DCHandler
}

func WithPingHandler(next DCHandler) DCHandler {
	wrappedHandler := &WrappedDCHandler{
		next: next,
	}
	return wrappedHandler
}

func (h *WrappedDCHandler) Serve(ctx context.Context, dc *webrtc.DataChannel, signallingTx chan<- pkgframing.MessagePayload) {
	pingHandler := &PingDCHandler{}
	pingHandler.Serve(ctx, dc, signallingTx)
	h.next.Serve(ctx, dc, signallingTx)
}
