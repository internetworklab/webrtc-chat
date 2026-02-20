package proxy

import (
	"context"

	pkgframing "example.com/webrtcserver/pkg/framing"
)

// SignallingServerProxy defines the interface for signalling communication
type SignallingServerProxy interface {
	// Send sends a message to the signalling server
	Send(ctx context.Context, msg pkgframing.MessagePayload) error
	// Receive returns a channel for receiving messages from the signalling server
	Receive() <-chan pkgframing.MessagePayload
}
