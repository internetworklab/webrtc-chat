package main

import (
	"context"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"

	pkghandlers "webrtc-agents/pkg/handlers"
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

	// Create WebRTC handler
	webrtcHandler := pkghandlers.NewWebRTCHandler(cli.ICEServer, cli.Debug)

	// Channel to signal when WebRTC handler is done
	handlerDone := make(chan struct{})

	// Start WebRTC handler in a goroutine
	go func() {
		defer close(handlerDone)
		webrtcHandler.Run(ctx, signallingProxy)
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

	// Start ping goroutine
	signallingProxy.StartPing(ctx, pingPeriod)

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
