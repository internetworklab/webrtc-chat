package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"

	pkghandlers "webrtc-agents/pkg/handlers"
	pkgproxy "webrtc-agents/pkg/proxy"
	pkgsignalling "webrtc-agents/pkg/signalling"

	pkgconnreg "example.com/webrtcserver/pkg/connreg"
	pkgframing "example.com/webrtcserver/pkg/framing"
	"github.com/alecthomas/kong"
	"github.com/gorilla/websocket"
)

var cli struct {
	WsServer          string   `name:"ws-server" help:"WebSocket server URL" default:"ws://localhost:3001/ws"`
	NodeName          string   `name:"node-name" help:"Node name for registration" default:"webrtc-agent-1"`
	PingPeriodSeconds int      `name:"ping-period-seconds" help:"Ping period in seconds" default:"5"`
	Debug             bool     `name:"debug" help:"Show ping/pong messages in logs for debugging purposes"`
	ICEServer         []string `name:"ice-server" help:"To specify the ICE servers, might be specify multiple times" default:"stun:stun.l.google.com:19302"`
}

func main() {
	kong.Parse(&cli)

	pingPeriod := time.Duration(cli.PingPeriodSeconds) * time.Second

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// Parse WebSocket URL
	u, err := url.Parse(cli.WsServer)
	if err != nil {
		log.Fatal("Failed to parse WebSocket URL:", err)
	}

	log.Printf("Connecting to %s", u.String())

	// Establish WebSocket connection
	wsConn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("Failed to dial:", err)
	}
	defer wsConn.Close()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create WebSocket signalling proxy from existing connection
	signallingProxy := pkgsignalling.NewWebSocketProxy(wsConn, cli.Debug)

	// Create filtered signalling proxy that handles pong messages
	filteredProxy := newFilteredSignallingProxy(signallingProxy, cli.Debug)

	// Create WebRTC handler
	webrtcHandler := pkghandlers.NewWebRTCHandler(cli.ICEServer, cli.Debug)

	// Channel to signal when WebRTC handler is done
	handlerDone := make(chan struct{})

	// Start WebRTC handler in a goroutine
	go func() {
		defer close(handlerDone)
		webrtcHandler.Run(ctx, filteredProxy)
	}()

	// Send registration message
	registerMsg := pkgframing.MessagePayload{
		Register: &pkgconnreg.RegisterPayload{
			NodeName: cli.NodeName,
		},
	}

	if err := signallingProxy.Send(ctx, registerMsg); err != nil {
		log.Fatal("Failed to send registration:", err)
	}
	log.Printf("Sent registration message for node: %s", cli.NodeName)

	// Start ping goroutine to maintain WebSocket connection
	go startPingLoop(ctx, signallingProxy, pingPeriod, cli.Debug)

	// Wait for interrupt signal or handler done
	select {
	case <-handlerDone:
		log.Println("WebRTC handler stopped")
	case <-interrupt:
		log.Println("Interrupt received, closing connection...")
	}

	// Cancel context to stop all goroutines
	cancel()

	// Close signalling proxy
	if err := signallingProxy.Close(); err != nil {
		log.Printf("Failed to close signalling proxy: %v", err)
	}

	// Cleanly close the WebSocket connection
	err = wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		log.Println("Failed to write close message:", err)
		return
	}

	// Wait for handler to finish
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		log.Println("Timeout waiting for handler to stop")
	}
}

// startPingLoop sends periodic ping messages to maintain the WebSocket connection
func startPingLoop(ctx context.Context, signallingProxy *pkgsignalling.WebSocketProxy, period time.Duration, debug bool) {
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	seqID := uint64(0)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			seqID++
			now := uint64(time.Now().UnixMilli())
			pingMsg := pkgframing.MessagePayload{
				Echo: &pkgconnreg.EchoPayload{
					Direction:     pkgconnreg.EchoDirectionC2S,
					CorrelationID: fmt.Sprintf("ping-%d", seqID),
					Timestamp:     now,
					SeqID:         seqID,
				},
			}

			if err := signallingProxy.Send(ctx, pingMsg); err != nil {
				log.Println("Failed to send ping:", err)
				return
			}

			if debug {
				log.Printf("Sent ping - SeqID: %d, CorrelationID: ping-%d", seqID, seqID)
			}
		}
	}
}

// filteredSignallingProxy wraps a SignallingServerProxy and filters out pong messages
type filteredSignallingProxy struct {
	proxy       pkgproxy.SignallingServerProxy
	receiveChan chan pkgframing.MessagePayload
	debug       bool
}

// newFilteredSignallingProxy creates a new filtered signalling proxy
func newFilteredSignallingProxy(proxy pkgproxy.SignallingServerProxy, debug bool) *filteredSignallingProxy {
	f := &filteredSignallingProxy{
		proxy:       proxy,
		receiveChan: make(chan pkgframing.MessagePayload, 100),
		debug:       debug,
	}

	// Start goroutine to filter messages
	go f.filterLoop()

	return f
}

// filterLoop reads from the underlying proxy and filters out pong messages
func (f *filteredSignallingProxy) filterLoop() {
	for msg := range f.proxy.Receive() {
		// Handle pong messages
		if msg.Echo != nil && msg.Echo.Direction == pkgconnreg.EchoDirectionS2C {
			if f.debug {
				rtt := time.Since(time.UnixMilli(int64(msg.Echo.Timestamp)))
				log.Printf("Pong received - RTT: %v, CorrelationID: %s, SeqID: %d",
					rtt, msg.Echo.CorrelationID, msg.Echo.SeqID)
			}
			continue
		}

		// Pass through non-pong messages
		f.receiveChan <- msg
	}
	close(f.receiveChan)
}

// Send sends a message through the underlying proxy
func (f *filteredSignallingProxy) Send(ctx context.Context, msg pkgframing.MessagePayload) error {
	return f.proxy.Send(ctx, msg)
}

// Receive returns a channel that filters out pong messages
func (f *filteredSignallingProxy) Receive() <-chan pkgframing.MessagePayload {
	return f.receiveChan
}
