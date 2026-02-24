package ws_runner

import (
	"context"

	pkghandlers "webrtc-agents/pkg/handlers"
)

type WebSocketSignallingSessionRunner interface {
	Run(ctx context.Context, handler pkghandlers.GenericWebRTCHandler)
}
