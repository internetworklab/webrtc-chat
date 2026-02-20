package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"

	pkgconnreg "example.com/webrtcserver/pkg/connreg"
	pkgframing "example.com/webrtcserver/pkg/framing"
	"github.com/alecthomas/kong"
	"github.com/gorilla/websocket"
)

var cli struct {
	WsServer          string `name:"ws-server" help:"WebSocket server URL" default:"ws://localhost:3001/ws"`
	NodeName          string `name:"node-name" help:"Node name for registration" default:"webrtc-agent-1"`
	PingPeriodSeconds int    `name:"ping-period-seconds" help:"Ping period in seconds" default:"5"`
	Debug             bool   `name:"debug" help:"Show ping/pong messages in logs for debugging purposes"`
}

func main() {
	kong.Parse(&cli)

	pingPeriod := time.Duration(cli.PingPeriodSeconds) * time.Second

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u, err := url.Parse(cli.WsServer)
	if err != nil {
		log.Fatal("Failed to parse WebSocket URL:", err)
	}

	log.Printf("Connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("Failed to dial:", err)
	}
	defer c.Close()

	// Channel to signal when connection is closed
	done := make(chan struct{})

	// Start goroutine to read messages from server
	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("Read error:", err)
				return
			}
			if cli.Debug {
				log.Printf("Received: %s", message)
			}

			// Parse the message to check if it's a pong
			var payload pkgframing.MessagePayload
			if err := json.Unmarshal(message, &payload); err != nil {
				log.Printf("Failed to parse message: %v", err)
				continue
			}

			if payload.Echo != nil && payload.Echo.Direction == pkgconnreg.EchoDirectionS2C {
				if cli.Debug {
					rtt := time.Since(time.UnixMilli(int64(payload.Echo.Timestamp)))
					log.Printf("Pong received - RTT: %v, CorrelationID: %s, SeqID: %d",
						rtt, payload.Echo.CorrelationID, payload.Echo.SeqID)
				}
			}
		}
	}()

	// Send registration message
	registerMsg := pkgframing.MessagePayload{
		Register: &pkgconnreg.RegisterPayload{
			NodeName: cli.NodeName,
		},
	}
	registerData, err := json.Marshal(registerMsg)
	if err != nil {
		log.Fatal("Failed to marshal registration message:", err)
	}

	err = c.WriteMessage(websocket.TextMessage, registerData)
	if err != nil {
		log.Fatal("Failed to send registration:", err)
	}
	log.Printf("Sent registration message for node: %s", cli.NodeName)

	// Ticker for sending ping messages
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	seqID := uint64(0)

	for {
		select {
		case <-done:
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
			pingData, err := json.Marshal(pingMsg)
			if err != nil {
				log.Println("Failed to marshal ping message:", err)
				continue
			}

			if err := c.WriteMessage(websocket.TextMessage, pingData); err != nil {
				log.Println("Failed to send ping:", err)
				return
			}
			if cli.Debug {
				log.Printf("Sent ping - SeqID: %d, CorrelationID: ping-%d", seqID, seqID)
			}

		case <-interrupt:
			log.Println("Interrupt received, closing connection...")

			// Cleanly close the connection
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("Failed to write close message:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}
