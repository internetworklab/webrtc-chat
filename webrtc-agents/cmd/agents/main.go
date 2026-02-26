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
	"github.com/joho/godotenv"
	"github.com/pion/webrtc/v4"
)

var cli struct {
	WsServer              string        `name:"ws-server" help:"WebSocket server URL" default:"ws://localhost:3001/ws"`
	PingPeriod            time.Duration `name:"ping-period-seconds" help:"Ping period in seconds" default:"3s"`
	Debug                 bool          `name:"debug" help:"Show ping/pong messages in logs for debugging purposes"`
	ICEServer             []string      `name:"ice-server" help:"To specify the ICE servers, might be specify multiple times" default:"stun:stun.l.google.com:19302"`
	ReconnectOnDisconnect bool          `name:"reconnect-on-disconnect" help:"Reconnect on WebSocket disconnect"`
	ReconnectDelay        time.Duration `name:"reconnect-delay" help:"Delay between reconnect attempts" default:"3s"`
	OggFiles              []string      `name:"ogg-file" help:"OGG files to load as audio tracks (must be 48kHz stereo)" placeholder:"FILE.ogg"`
	OpenRouterAPIKeyEnv   string        `name:"openrouter-apikey-env" help:"Environment variable name that stores the OpenRouter API key" default:"OPENROUTER_APIKEY"`
}

func main() {
	kong.Parse(&cli)

	// Load .env file if it exists (ignore error if file doesn't exist)
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found or error loading .env file, continuing with existing environment variables")
	}

	// Read OpenRouter API key from environment variable if specified
	var openrouterAPIKey string
	if cli.OpenRouterAPIKeyEnv != "" {
		openrouterAPIKey = os.Getenv(cli.OpenRouterAPIKeyEnv)
	}

	// Parse WebSocket URL
	u, err := url.Parse(cli.WsServer)
	if err != nil {
		log.Fatal("Failed to parse WebSocket URL:", err)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var echoBotHandler pkghandlers.GenericWebRTCHandler
	echoBotHandler = pkghandlers.NewSignallingHandler(pkghandlers.WithPingHandler(&pkghandlers.EchoDCHandler{}), cli.ICEServer, cli.Debug, nil)
	echoBotRunner := &pkgwsrunner.WebSocketRunner{
		URL:                   *u,
		PingIntv:              cli.PingPeriod,
		Debug:                 cli.Debug,
		ReconnectOnDisconnect: cli.ReconnectOnDisconnect,
		ReconnectDelay:        cli.ReconnectDelay,
		NodeName:              "EchoBot",
	}
	go echoBotRunner.Run(ctx, echoBotHandler)
	log.Println("Echo bot started!")
	<-time.After(1 * time.Second)

	// MediaEngine Configuration:
	// The handler creates a MediaEngine that registers only the Opus codec with the following parameters:
	//   - MimeType:    webrtc.MimeTypeOpus ("audio/opus")
	//   - ClockRate:   48000 Hz
	//   - Channels:    2 (stereo)
	//   - SDPFmtpLine: "minptime=10;useinbandfec=1"
	//   - PayloadType: 111 (standard for Opus)
	//
	// This constraint ensures that:
	// 1. SDP offers/answers only include Opus codec
	// 2. Negotiation will fail if the peer doesn't support Opus 48kHz stereo
	// 3. Audio tracks can safely assume Opus encoding parameters
	//
	// The MediaEngine is used by the webrtc.API when creating new peer connections,
	// which automatically constrains all codec negotiations to the registered codecs.

	// Create MediaEngine and register only Opus codec (48kHz stereo)
	// This constrains negotiation to only support what we can stream
	mediaEngine := &webrtc.MediaEngine{}

	// Register Opus codec with specific parameters matching our audio pipeline
	// see: https://github.com/pion/webrtc/blob/96cbf971e272f466aaa68cbdfe927fe947426869/mediaengine.go#L69
	opusCodec := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1",
		},
		PayloadType: 111, // Standard payload type for Opus
	}

	if err := mediaEngine.RegisterCodec(opusCodec, webrtc.RTPCodecTypeAudio); err != nil {
		log.Fatalf("Failed to register Opus codec: %v", err)
	}

	// Create API with the constrained MediaEngine
	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))

	var musicBotHandler pkghandlers.GenericWebRTCHandler
	trackDCHandler, err := pkghandlers.NewTrackDCHandler(cli.OggFiles, cli.Debug)
	if err != nil {
		log.Fatalf("Failed to create track data channel handler: %v", err)
	}
	musicBotHandler = pkghandlers.NewSignallingHandler(pkghandlers.WithPingHandler(trackDCHandler), cli.ICEServer, cli.Debug, api)
	musicBotRunner := &pkgwsrunner.WebSocketRunner{
		URL:                   *u,
		PingIntv:              cli.PingPeriod,
		Debug:                 cli.Debug,
		ReconnectOnDisconnect: cli.ReconnectOnDisconnect,
		ReconnectDelay:        cli.ReconnectDelay,
		NodeName:              "MusicBot",
	}
	go musicBotRunner.Run(ctx, musicBotHandler)
	log.Println("Music bot started!")
	<-time.After(1 * time.Second)

	var counterBotHandler pkghandlers.GenericWebRTCHandler
	counterBotHandler = pkghandlers.NewSignallingHandler(pkghandlers.WithPingHandler(pkghandlers.NewCounterDCHandler()), cli.ICEServer, cli.Debug, nil)
	counterBotRunner := &pkgwsrunner.WebSocketRunner{
		URL:                   *u,
		PingIntv:              cli.PingPeriod,
		Debug:                 cli.Debug,
		ReconnectOnDisconnect: cli.ReconnectOnDisconnect,
		ReconnectDelay:        cli.ReconnectDelay,
		NodeName:              "CounterBot",
	}
	go counterBotRunner.Run(ctx, counterBotHandler)
	log.Println("Counter bot started!")
	<-time.After(1 * time.Second)

	var clockBotHandler pkghandlers.GenericWebRTCHandler
	clockBotHandler = pkghandlers.NewSignallingHandler(pkghandlers.WithPingHandler(pkghandlers.NewClockBotDCHandler(openrouterAPIKey)), cli.ICEServer, cli.Debug, nil)
	clockBotRunner := &pkgwsrunner.WebSocketRunner{
		URL:                   *u,
		PingIntv:              cli.PingPeriod,
		Debug:                 cli.Debug,
		ReconnectOnDisconnect: cli.ReconnectOnDisconnect,
		ReconnectDelay:        cli.ReconnectDelay,
		NodeName:              "ClockBot",
	}
	go clockBotRunner.Run(ctx, clockBotHandler)
	log.Println("Clock bot started!")
	<-time.After(1 * time.Second)

	sigsCh := make(chan os.Signal, 1)
	signal.Notify(sigsCh, syscall.SIGINT)
	signal := <-sigsCh
	log.Printf("Received signal %+v, exitting", signal.String())
}
