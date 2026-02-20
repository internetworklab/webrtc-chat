package main

import (
	"context"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	pkghandlers "webrtc-agents/pkg/handlers"
	pkgsignalling "webrtc-agents/pkg/signalling"

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

	registerer := &pkghandlers.WebSocketRegisterer{}
	if err := registerer.Register(wsConn, cli.NodeName); err != nil {
		log.Fatal("Failed to send registration message:", err)
	}

	log.Printf("Sent registration message for node: %s", cli.NodeName)

	wsPinger := &pkghandlers.WebSocketPinger{
		Intv:  pingPeriod,
		Debug: cli.Debug,
	}
	go wsPinger.StartPingLoop(ctx, wsConn)
	log.Println("Ping/pong loop started")

	// Create WebSocket signalling proxy from existing connection
	signallingProxy := pkgsignalling.NewWebSocketProxy(wsConn, &pkgsignalling.WebSocketProxyOptions{
		Debug:                 cli.Debug,
		ReconnectOnDisconnect: true,
		ReconnectDelay:        time.Second * 3,
	})
	defer signallingProxy.Close()

	// Create filtered signalling proxy that handles pong messages
	filteredProxy := pkgsignalling.NewFilteredSignallingProxy(signallingProxy, cli.Debug)

	// Create WebRTC handler
	webrtcHandler := pkghandlers.NewWebRTCHandler(cli.ICEServer, cli.Debug)

	webrtcHandler.Run(ctx, filteredProxy)

	sigsCh := make(chan os.Signal, 1)
	signal.Notify(sigsCh, syscall.SIGINT)
	signal := <-sigsCh
	log.Printf("Received signal %+v, exitting", signal.String())
}
