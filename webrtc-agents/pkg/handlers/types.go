package handlers

import (
	"context"

	pkgframing "example.com/webrtcserver/pkg/framing"
	"github.com/pion/webrtc/v4"
)

type GenericWebRTCHandler interface {
	Serve(ctx context.Context, signallingTx chan<- pkgframing.MessagePayload, signallingRx <-chan pkgframing.MessagePayload)
}

// Predefined data channel labels
const (
	PredefinedDCLabelChat      = "chat"
	PredefinedDCLabelFile      = "file"
	PredefinedDCLabelPing      = "ping"
	PredefinedDCLabelMsgStream = "msgpatch"
)

// ChatMessageFileCategory represents the category of a file
type ChatMessageFileCategory string

const (
	ChatMessageFileCategoryFile     ChatMessageFileCategory = "file"
	ChatMessageFileCategoryImage    ChatMessageFileCategory = "image"
	ChatMessageFileCategoryVideo    ChatMessageFileCategory = "video"
	ChatMessageFileCategoryAudio    ChatMessageFileCategory = "audio"
	ChatMessageFileCategoryDocument ChatMessageFileCategory = "document"
)

// ChatMessageFileThumbnail represents a thumbnail for image/video files
type ChatMessageFileThumbnail struct {
	DataURL string `json:"dataURL"`
	Mime    string `json:"mime"`
}

// ChatMessageFile represents a file in a chat message
type ChatMessageFile struct {
	Category  ChatMessageFileCategory   `json:"category"`
	Thumbnail *ChatMessageFileThumbnail `json:"thumbnail,omitempty"`
	Name      *string                   `json:"name,omitempty"`
	Size      *int64                    `json:"size,omitempty"`
	Type      *string                   `json:"type,omitempty"`
	DCID      string                    `json:"dcId"`
	URL       *string                   `json:"url,omitempty"`
}

// ChatMessagePingDirection represents the direction of a ping message
type ChatMessagePingDirection string

const (
	ChatMessagePingDirectionPing ChatMessagePingDirection = "ping"
	ChatMessagePingDirectionPong ChatMessagePingDirection = "pong"
)

// ChatMessagePing represents a ping/pong message
type ChatMessagePing struct {
	Direction ChatMessagePingDirection `json:"direction"`
	Seq       int                      `json:"seq"`
}

// ChatMessageAmend represents a request to amend a message
type ChatMessageAmend struct {
	MessageID      string `json:"messageId"`
	NewMessageJSON string `json:"newMessageJSON"`
}

// ChatMessageDelete represents a request to delete a message
type ChatMessageDelete struct {
	MessageID string `json:"messageId"`
}

// ChatMessageText represents rich text content
type ChatMessageText struct {
	Content string `json:"content"`
	Type    string `json:"type"`
}

// ChatMessageACK represents an acknowledgment for a chat message
type ChatMessageACK struct {
	MessageID string `json:"messageId"`
}

// ChatMessage represents a chat message sent over the data channel
type ChatMessage struct {
	MessageID  string `json:"messageId"`
	FromNodeID string `json:"fromNodeId"`
	ToNodeID   string `json:"toNodeId"`
	Timestamp  int64  `json:"timestamp"`

	File     *ChatMessageFile   `json:"file,omitempty"`
	Ping     *ChatMessagePing   `json:"ping,omitempty"`
	Delete   *ChatMessageDelete `json:"delete,omitempty"`
	Amend    *ChatMessageAmend  `json:"amend,omitempty"`
	ACK      *ChatMessageACK    `json:"ack,omitempty"`
	ACKed    *bool              `json:"acked,omitempty"`
	Unread   *bool              `json:"unread,omitempty"`
	Message  *string            `json:"message,omitempty"`
	RichText *ChatMessageText   `json:"richText,omitempty"`
}

type DCHandler interface {
	Serve(ctx context.Context, dc *webrtc.DataChannel, signallingTx chan<- pkgframing.MessagePayload)
}

type DCHandlerCtxKey string

const (
	DCHandlerCtxRemoteNodeID  DCHandlerCtxKey = "remoteNodeID"
	DCHandlerCtxOurNodeID     DCHandlerCtxKey = "ourNodeID"
	DCHandlerCtxPeerConnStore DCHandlerCtxKey = "peerConnStore"
)

type GeneralPropertyKey string

const (
	GP_Key_NodeId     = "node_id"
	GP_Key_CancelFunc = "cancel_func"
)
