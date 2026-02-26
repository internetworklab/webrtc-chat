package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	pkgllm "webrtc-agents/pkg/llm"
	"webrtc-agents/pkg/msgs_store"

	pkgframing "example.com/webrtcserver/pkg/framing"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

// Index by session id
type IndexedMsgsCollection struct {
	store map[string][]interface{}
}

func NewIndexedMsgsCollection() *IndexedMsgsCollection {
	return &IndexedMsgsCollection{
		store: make(map[string][]interface{}),
	}
}

type IdentifiableMessage interface {
	GetSessionId() string
}

func (indexColl *IndexedMsgsCollection) DeepClone() msgs_store.MsgsCollection {
	newMap := make(map[string][]interface{})
	for sessionId, li := range indexColl.store {
		newList := make([]interface{}, len(li))
		copy(newList, li)
		newMap[sessionId] = newList
	}
	newIndexColl := new(IndexedMsgsCollection)
	newIndexColl.store = newMap
	return newIndexColl
}

func (indexColl *IndexedMsgsCollection) Append(msg interface{}) {
	sessionId := ""
	if identifiable, ok := msg.(IdentifiableMessage); ok {
		sessionId = identifiable.GetSessionId()
	}
	indexColl.store[sessionId] = append(indexColl.store[sessionId], msg)
}

// GetMessagesBySessionId returns messages for a specific session
func (indexColl *IndexedMsgsCollection) GetMessagesBySessionId(sessionId string) []interface{} {
	return indexColl.store[sessionId]
}

// ChatHistoryMessage represents a message in the chat history
type ChatHistoryMessage struct {
	MessageID string
	SenderID  string

	// session id should be form by our id (bot's node id) then peer id
	// e.g. "<ourid>-<peerid>"
	SessionID string
	Content   string
	Timestamp int64
}

// GetSessionId implements IdentifiableMessage interface
func (m *ChatHistoryMessage) GetSessionId() string {
	return m.SessionID
}

// ChatBotDCHandler handles WebRTC data channel for chatbot functionality
type ChatBotDCHandler struct {
	llmGen pkgllm.CompletionGenerator
	store  *msgs_store.SyncMsgsStore
}

// NewChatBotDCHandler creates a new ChatBotDCHandler
func NewChatBotDCHandler(llmGen pkgllm.CompletionGenerator) *ChatBotDCHandler {
	return &ChatBotDCHandler{
		llmGen: llmGen,
		store: msgs_store.NewSyncMsgsStore(func() msgs_store.MsgsCollection {
			return NewIndexedMsgsCollection()
		}),
	}
}

// Serve starts the WebRTC handler
func (h *ChatBotDCHandler) Serve(ctx context.Context, dc *webrtc.DataChannel, signallingTx chan<- pkgframing.MessagePayload) {
	remoteNodeID := ctx.Value(DCHandlerCtxRemoteNodeID).(string)
	ourNodeID := ctx.Value(DCHandlerCtxOurNodeID).(string)
	log.Printf("[webrtc] ChatBot received data channel: %s from peer %s", dc.Label(), remoteNodeID)

	switch dc.Label() {
	case PredefinedDCLabelChat:
		h.setupChatDataChannel(dc, remoteNodeID, ourNodeID)
	default:
		log.Printf("[webrtc] Unknown (or unsupported) data channel label: %s", dc.Label())
	}
}

// setupChatDataChannel sets up the chat data channel for handling messages
func (h *ChatBotDCHandler) setupChatDataChannel(dc *webrtc.DataChannel, remoteNodeID string, ourNodeID string) {
	dc.OnOpen(func() {
		log.Printf("[webrtc] ChatBot data channel opened with peer %s", remoteNodeID)
	})

	dc.OnClose(func() {
		log.Printf("[webrtc] ChatBot data channel closed with peer %s", remoteNodeID)
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		// Parse the message as ChatMessage
		var chatMsg ChatMessage
		if err := json.Unmarshal(msg.Data, &chatMsg); err != nil {
			log.Printf("[webrtc] Failed to parse chat message: %v", err)
			return
		}

		// Skip ACK messages
		if chatMsg.ACK != nil {
			return
		}

		// Skip non-text messages
		if chatMsg.Message == nil && chatMsg.RichText == nil {
			return
		}

		// Get message content
		var content string
		if chatMsg.Message != nil {
			content = *chatMsg.Message
		} else if chatMsg.RichText != nil {
			content = chatMsg.RichText.Content
		}

		// Send ACK for the received message
		h.sendACK(dc, &chatMsg)

		// Build session ID: "<ourid>-<peerid>"
		sessionID := fmt.Sprintf("%s-%s", ourNodeID, remoteNodeID)

		// Store the user's message
		userMsg := &ChatHistoryMessage{
			MessageID: chatMsg.MessageID,
			SenderID:  remoteNodeID,
			SessionID: sessionID,
			Content:   content,
			Timestamp: chatMsg.Timestamp,
		}
		if err := h.store.Append(userMsg); err != nil {
			log.Printf("[webrtc] Failed to store user message: %v", err)
			return
		}

		// Generate LLM response
		responseText := h.generateLLMResponse(context.Background(), remoteNodeID, ourNodeID)
		h.sendChatResponse(dc, &chatMsg, responseText, ourNodeID, remoteNodeID)
	})

	dc.OnError(func(err error) {
		log.Printf("[webrtc] ChatBot data channel error with peer %s: %v", remoteNodeID, err)
	})
}

// sendACK sends an acknowledgment for the received message
func (h *ChatBotDCHandler) sendACK(dc *webrtc.DataChannel, originalMsg *ChatMessage) {
	ackMsg := ChatMessage{
		MessageID:  uuid.New().String(),
		FromNodeID: originalMsg.ToNodeID,
		ToNodeID:   originalMsg.FromNodeID,
		Timestamp:  time.Now().UnixMilli(),
		ACK: &ChatMessageACK{
			MessageID: originalMsg.MessageID,
		},
	}
	ackData, err := json.Marshal(ackMsg)
	if err != nil {
		log.Printf("[webrtc] Failed to marshal ACK message: %v", err)
		return
	}
	if err := dc.SendText(string(ackData)); err != nil {
		log.Printf("[webrtc] Failed to send ACK: %v", err)
	}
}

// generateLLMResponse generates a response using the LLM based on conversation history
func (h *ChatBotDCHandler) generateLLMResponse(ctx context.Context, remoteNodeID string, ourNodeID string) string {
	// Build session ID: "<ourid>-<peerid>"
	sessionID := fmt.Sprintf("%s-%s", ourNodeID, remoteNodeID)

	// Get conversation history
	messages := h.getConversationHistory(sessionID, ourNodeID)

	// Build the LLM request
	request := pkgllm.OpenRouterCompletionRequest{
		Model: "openai/gpt-3.5-turbo",
		Messages: []pkgllm.OpenRouterCompletionRequestMessage{
			{
				Role:    pkgllm.LLMRoleSystem,
				Content: "You are a helpful assistant. Respond concisely and helpfully to user messages.",
			},
		},
		Reasoning: pkgllm.OpenRouterCompletionRequestReasoning{
			Enabled: false,
		},
	}

	// Add conversation history
	request.Messages = append(request.Messages, messages...)

	// Generate response from LLM
	response := h.llmGen.Generate(ctx, request)

	// Extract the response text
	if len(response.Choices) > 0 && response.Choices[0].Message != nil {
		return response.Choices[0].Message.Content
	}

	return "I apologize, but I couldn't generate a response at this time."
}

// getConversationHistory retrieves and formats the conversation history for the LLM
func (h *ChatBotDCHandler) getConversationHistory(sessionID string, ourNodeID string) []pkgllm.OpenRouterCompletionRequestMessage {
	store := h.store.Load()
	if store == nil {
		return nil
	}

	coll := store.Load().(*IndexedMsgsCollection)
	messages := coll.GetMessagesBySessionId(sessionID)

	// Sort messages by timestamp
	type TimestampedMessage struct {
		timestamp int64
		senderID  string
		content   string
	}

	allMessages := make([]TimestampedMessage, 0)
	for _, msg := range messages {
		if chatMsg, ok := msg.(*ChatHistoryMessage); ok {
			allMessages = append(allMessages, TimestampedMessage{
				timestamp: chatMsg.Timestamp,
				senderID:  chatMsg.SenderID,
				content:   chatMsg.Content,
			})
		}
	}

	// Sort by timestamp
	sort.Slice(allMessages, func(i, j int) bool {
		return allMessages[i].timestamp < allMessages[j].timestamp
	})

	// Convert to LLM message format
	var llmMessages []pkgllm.OpenRouterCompletionRequestMessage
	for _, msg := range allMessages {
		var role pkgllm.LLMRole = pkgllm.LLMRoleUser
		if msg.senderID == ourNodeID {
			role = pkgllm.LLMRoleAssistant
		}
		llmMessages = append(llmMessages, pkgllm.OpenRouterCompletionRequestMessage{
			Role:    role,
			Content: msg.content,
		})
	}

	return llmMessages
}

// sendChatResponse sends a chat message response back to the peer and stores it
func (h *ChatBotDCHandler) sendChatResponse(dc *webrtc.DataChannel, originalMsg *ChatMessage, responseText string, ourNodeID string, remoteNodeID string) {
	// Build session ID: "<ourid>-<peerid>"
	sessionID := fmt.Sprintf("%s-%s", ourNodeID, remoteNodeID)

	responseMsg := ChatMessage{
		MessageID:  uuid.New().String(),
		FromNodeID: originalMsg.ToNodeID,
		ToNodeID:   originalMsg.FromNodeID,
		Timestamp:  time.Now().UnixMilli(),
		Message:    &responseText,
	}

	// Store our response
	botMsg := &ChatHistoryMessage{
		MessageID: responseMsg.MessageID,
		SenderID:  ourNodeID,
		SessionID: sessionID,
		Content:   responseText,
		Timestamp: responseMsg.Timestamp,
	}
	if err := h.store.Append(botMsg); err != nil {
		log.Printf("[webrtc] Failed to store bot message: %v", err)
		return
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
