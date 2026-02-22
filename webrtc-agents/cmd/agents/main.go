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
	pkgwsrunner "webrtc-agents/pkg/ws_runner"

	"github.com/alecthomas/kong"
)

var cli struct {
	WsServer              string        `name:"ws-server" help:"WebSocket server URL" default:"ws://localhost:3001/ws"`
	PingPeriod            time.Duration `name:"ping-period-seconds" help:"Ping period in seconds" default:"3s"`
	Debug                 bool          `name:"debug" help:"Show ping/pong messages in logs for debugging purposes"`
	ICEServer             []string      `name:"ice-server" help:"To specify the ICE servers, might be specify multiple times" default:"stun:stun.l.google.com:19302"`
	ReconnectOnDisconnect bool          `name:"reconnect-on-disconnect" help:"Reconnect on WebSocket disconnect"`
	ReconnectDelay        time.Duration `name:"reconnect-delay" help:"Delay between reconnect attempts" default:"3s"`
}

func main() {
	kong.Parse(&cli)

	// Parse WebSocket URL
	u, err := url.Parse(cli.WsServer)
	if err != nil {
		log.Fatal("Failed to parse WebSocket URL:", err)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var echoBotHandler pkghandlers.GenericWebRTCHandler
	echoBotHandler = pkghandlers.NewEchoHandler(cli.ICEServer, cli.Debug)
	go echoBotHandler.Run(ctx, &pkgwsrunner.WebSocketRunner{
		URL:                   *u,
		PingIntv:              cli.PingPeriod,
		Debug:                 cli.Debug,
		ReconnectOnDisconnect: cli.ReconnectOnDisconnect,
		ReconnectDelay:        cli.ReconnectDelay,
		NodeName:              "EchoBot",
	})
	log.Println("Echo bot started!")

	var musicBotHandler pkghandlers.GenericWebRTCHandler
	musicBotHandler = pkghandlers.NewTrackHandler(cli.ICEServer, cli.Debug)
	go musicBotHandler.Run(ctx, &pkgwsrunner.WebSocketRunner{
		URL:                   *u,
		PingIntv:              cli.PingPeriod,
		Debug:                 cli.Debug,
		ReconnectOnDisconnect: cli.ReconnectOnDisconnect,
		ReconnectDelay:        cli.ReconnectDelay,
		NodeName:              "MusicBot",
	})
	log.Println("Music bot started!")

	sigsCh := make(chan os.Signal, 1)
	signal.Notify(sigsCh, syscall.SIGINT)
	signal := <-sigsCh
	log.Printf("Received signal %+v, exitting", signal.String())
}
