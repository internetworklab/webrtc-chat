package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	pkghandlers "webrtc-agents/pkg/handlers"
	pkgllm "webrtc-agents/pkg/llm"
	pkgwsrunner "webrtc-agents/pkg/ws_runner"

	"github.com/alecthomas/kong"
	"github.com/joho/godotenv"
	"github.com/pion/webrtc/v4"
)

type CLI struct {
	WsServer              string        `name:"ws-server" help:"WebSocket server URL" default:"ws://localhost:3001/ws"`
	PingPeriod            time.Duration `name:"ping-period-seconds" help:"Ping period in seconds" default:"3s"`
	Debug                 bool          `name:"debug" help:"Show ping/pong messages in logs for debugging purposes"`
	ICEServer             []string      `name:"ice-server" help:"To specify the ICE servers, might be specify multiple times" default:"stun:stun.l.google.com:19302"`
	CustomCA              []string      `name:"custom-ca" help:"Custom CA certificates to trust (the --custom-ca= argument can appear multiple times)"`
	ReconnectOnDisconnect bool          `name:"reconnect-on-disconnect" help:"Reconnect on WebSocket disconnect"`
	ReconnectDelay        time.Duration `name:"reconnect-delay" help:"Delay between reconnect attempts" default:"3s"`
	OggFiles              []string      `name:"ogg-file" help:"OGG files to load as audio tracks (must be 48kHz stereo)" placeholder:"FILE.ogg"`
	OpenRouterAPIKeyEnv   string        `name:"openrouter-apikey-env" help:"Environment variable name that stores the OpenRouter API key" default:"OPENROUTER_APIKEY"`
	ChatBotModel          string        `name:"chatbot-model" help:"The id of the model use for chatbot"`
	CustomResolver        string        `name:"custom-resolver" help:"Use specified resolver instead of system's default resolver, example like [fd42:d42:d42:54::1]:53 or 172.20.0.53"`
	PreferIPv6            bool          `name:"prefer-ipv6" help:"Use IPv6-only"`
	BotsEnvFile           string        `name:"bots-secret-env-file" help:"When presented, ENVs defined in this file will be read to enable the bots authenticate themselves to the server." default:".env.bots"`
}

func (cli *CLI) getCustomResolver() *net.Resolver {
	if cli.CustomResolver == "" {
		return nil
	}

	var resolverAddr string

	// Try to split host and port first
	host, port, err := net.SplitHostPort(cli.CustomResolver)
	if err != nil {
		// No port specified, use default DNS port 53
		resolverAddr = net.JoinHostPort(cli.CustomResolver, "53")
	} else {
		// Port was specified
		resolverAddr = net.JoinHostPort(host, port)
	}

	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, network, resolverAddr)
		},
	}
}

func (cli *CLI) getTLSConfig() *tls.Config {
	if len(cli.CustomCA) == 0 {
		return nil
	}

	logger := log.New(os.Stderr, "", 0)

	var certPool *x509.CertPool
	// Get system cert pool as base (to trust both system and custom CAs)
	sysCertPool, err := x509.SystemCertPool()
	if err != nil {
		logger.Printf("Warning: Failed to get system cert pool: %v", err)
		// If we can't get system cert pool, create a new empty one
		certPool = x509.NewCertPool()
	} else {
		certPool = sysCertPool
	}

	// Append custom CA certificates to the pool
	for _, certFile := range cli.CustomCA {
		certData, err := os.ReadFile(certFile)
		if err != nil {
			logger.Printf("Warning: Failed to read custom CA certificate %s: %v", certFile, err)
			continue
		}
		if !certPool.AppendCertsFromPEM(certData) {
			logger.Printf("Warning: Failed to parse custom CA certificate %s", certFile)
		}
	}

	return &tls.Config{
		RootCAs: certPool,
	}
}

var cli CLI

func main() {
	kong.Parse(&cli)
	var resolverUsed *net.Resolver = net.DefaultResolver
	if resolver := cli.getCustomResolver(); resolver != nil {
		resolverUsed = resolver
	}

	var tlsConfig *tls.Config = cli.getTLSConfig()

	if botsEnvPath := cli.BotsEnvFile; botsEnvPath != "" {
		log.Printf("Loading %s", botsEnvPath)
		if err := godotenv.Load(botsEnvPath); err != nil {
			log.Fatalln("Failed to load env", botsEnvPath, err)
		}
	}

	// Load .env file if it exists (ignore error if file doesn't exist)
	log.Printf("Loading default .env file for ENVs")
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found or error loading .env file, continuing with existing environment variables")
	}

	// Parse WebSocket URL
	u, err := url.Parse(cli.WsServer)
	if err != nil {
		log.Fatal("Failed to parse WebSocket URL:", err)
	}

	getWsRunnerByDerivedCfg := func(nodeName string, jwtEnvName string) *pkgwsrunner.WebSocketRunner {
		return &pkgwsrunner.WebSocketRunner{
			URL:                   *u,
			PingIntv:              cli.PingPeriod,
			Debug:                 cli.Debug,
			ReconnectOnDisconnect: cli.ReconnectOnDisconnect,
			ReconnectDelay:        cli.ReconnectDelay,
			NodeName:              nodeName,
			Resolver:              resolverUsed,
			TLSConfig:             tlsConfig,
			PreferIPv6:            cli.PreferIPv6,
			JWTEnvName:            jwtEnvName,
		}
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	echoBotDCHandler := pkghandlers.WithPingHandler(&pkghandlers.EchoDCHandler{})
	echoBotHandler := pkghandlers.NewSignallingHandler(echoBotDCHandler, cli.ICEServer, cli.Debug, nil)
	echoBotRunner := getWsRunnerByDerivedCfg("EchoBot", "ECHOBOT_JWT_TOKEN")
	go echoBotRunner.Run(ctx, echoBotHandler)
	log.Println("Echo bot started!")

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
	musicBotRunner := getWsRunnerByDerivedCfg("MusicBot", "MUSICBOT_JWT_TOKEN")
	go musicBotRunner.Run(ctx, musicBotHandler)
	log.Println("Music bot started!")

	var counterBotHandler pkghandlers.GenericWebRTCHandler
	counterBotHandler = pkghandlers.NewSignallingHandler(pkghandlers.WithPingHandler(pkghandlers.NewCounterDCHandler()), cli.ICEServer, cli.Debug, nil)
	counterBotRunner := getWsRunnerByDerivedCfg("CounterBot", "COUNTERBOT_JWT_TOKEN")
	go counterBotRunner.Run(ctx, counterBotHandler)
	log.Println("Counter bot started!")

	var clockBotHandler pkghandlers.GenericWebRTCHandler
	clockBotHandler = pkghandlers.NewSignallingHandler(pkghandlers.WithPingHandler(pkghandlers.NewClockBotDCHandler()), cli.ICEServer, cli.Debug, nil)
	clockBotRunner := getWsRunnerByDerivedCfg("ClockBot", "CLOCKBOT_JWT_TOKEN")
	go clockBotRunner.Run(ctx, clockBotHandler)
	log.Println("Clock bot started!")

	// ChatBot using OpenRouterCompletionProxy
	var chatBotHandler pkghandlers.GenericWebRTCHandler
	chatBotLLM := &pkgllm.OpenRouterCompletionProxy{
		APIKeyFromEnv: cli.OpenRouterAPIKeyEnv,
	}
	chatBotDCHandler, err := pkghandlers.NewChatBotDCHandler(chatBotLLM, cli.ChatBotModel)
	if err != nil {
		log.Fatalf("Failed to create chat bot handler: %v", err)
	}
	chatBotHandler = pkghandlers.NewSignallingHandler(pkghandlers.WithPingHandler(chatBotDCHandler), cli.ICEServer, cli.Debug, nil)
	chatBotRunner := getWsRunnerByDerivedCfg("ChatBot", "CHATBOT_JWT_TOKEN")
	go chatBotRunner.Run(ctx, chatBotHandler)
	log.Println("Chat bot started!")

	sigsCh := make(chan os.Signal, 1)
	signal.Notify(sigsCh, syscall.SIGINT)
	signal := <-sigsCh
	log.Printf("Received signal %+v, exitting", signal.String())
}
