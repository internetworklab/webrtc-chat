package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	pkgframing "example.com/webrtcserver/pkg/framing"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

type ChatBotDCHandler struct {
	APIKey string
}

// Serve starts the WebRTC handler
func (h *ChatBotDCHandler) Serve(ctx context.Context, dc *webrtc.DataChannel, signallingTx chan<- pkgframing.MessagePayload) {

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
func (h *ChatBotDCHandler) setupChatDataChannel(dc *webrtc.DataChannel, remoteNodeID string) {
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
func (h *ChatBotDCHandler) formatHelp() string {
	return `🔢 Counter Bot - Available Commands:

/start - Show this help message
/increase - Increase your personal counter by 1
/reset - Reset your personal counter to 0

Your counter is personal and independent from other users!`
}

// increaseCounter increments the counter for the given remote node ID and returns the new value
func (h *ChatBotDCHandler) increaseCounter(remoteNodeID string) int {
	return 0
}

// resetCounter resets the counter for the given remote node ID to 0
func (h *ChatBotDCHandler) resetCounter(remoteNodeID string) {
	// todo
}

// getCounter returns the current counter value for the given remote node ID
func (h *ChatBotDCHandler) getCounter(remoteNodeID string) int {
	// todo
	return 0
}

// sendChatResponse sends a chat message response back to the peer
func (h *ChatBotDCHandler) sendChatResponse(dc *webrtc.DataChannel, originalMsg *ChatMessage, responseText string) {
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

const exampleOutput string = `
{
"id": "gen-1772067607-bx2G4GM6pDOS5SyWQOnO",
"provider": "AtlasCloud",
"model": "deepseek/deepseek-v3.2",
"object": "chat.completion",
"created": 1772067607,
"choices": [
  {
    "logprobs": null,
    "finish_reason": "stop",
    "native_finish_reason": "stop",
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "The word \"strawberry\" contains three r's.",
      "refusal": null,
      "reasoning": "reasoning ...",
      "reasoning_details": [
        {
          "format": "unknown",
          "index": 0,
          "type": "reasoning.text",
          "text": "thinking ..."
        }
      ]
    }
  }
],
"system_fingerprint": null,
"usage": {
  "prompt_tokens": 18,
  "completion_tokens": 400,
  "total_tokens": 418,
  "cost": 0.00015668,
  "is_byok": false,
  "prompt_tokens_details": {
    "cached_tokens": 0,
    "audio_tokens": 0
  },
  "cost_details": {
    "upstream_inference_cost": 0.00015668,
    "upstream_inference_prompt_cost": 0.00000468,
    "upstream_inference_completions_cost": 0.000152
  },
  "completion_tokens_details": {
    "reasoning_tokens": 386,
    "audio_tokens": 0
  }
}
}
`
