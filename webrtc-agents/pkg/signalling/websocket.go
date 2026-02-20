package signalling

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	pkgframing "example.com/webrtcserver/pkg/framing"

	"github.com/gorilla/websocket"
)

// WebSocketProxy implements SignallingServerProxy using WebSocket
type WebSocketProxy struct {
	conn        *websocket.Conn
	nodeID      string
	nodeIDMu    sync.RWMutex
	receiveChan chan pkgframing.MessagePayload
	sendChan    chan pkgframing.MessagePayload
	done        chan struct{}
	wg          sync.WaitGroup
	debug       bool
}

// NewWebSocketProxy creates a new WebSocket signalling proxy from an existing connection
func NewWebSocketProxy(conn *websocket.Conn, debug bool) *WebSocketProxy {
	proxy := &WebSocketProxy{
		conn:        conn,
		receiveChan: make(chan pkgframing.MessagePayload, 100),
		sendChan:    make(chan pkgframing.MessagePayload, 100),
		done:        make(chan struct{}),
		debug:       debug,
	}

	// Start read and write goroutines
	proxy.wg.Add(2)
	go proxy.readLoop()
	go proxy.writeLoop()

	return proxy
}

// Send sends a message to the signalling server
func (p *WebSocketProxy) Send(ctx context.Context, msg pkgframing.MessagePayload) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case p.sendChan <- msg:
		return nil
	case <-p.done:
		return fmt.Errorf("connection closed")
	}
}

// Receive returns a channel for receiving messages from the signalling server
func (p *WebSocketProxy) Receive() <-chan pkgframing.MessagePayload {
	return p.receiveChan
}

// Close closes the WebSocket proxy and stops all goroutines
func (p *WebSocketProxy) Close() error {
	close(p.done)

	// Send close message
	err := p.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		log.Println("Failed to write close message:", err)
	}

	p.conn.Close()
	p.wg.Wait()

	close(p.sendChan)
	close(p.receiveChan)

	return nil
}

// readLoop reads messages from WebSocket and sends to receiveChan
func (p *WebSocketProxy) readLoop() {
	defer p.wg.Done()

	for {
		select {
		case <-p.done:
			return
		default:
			_, message, err := p.conn.ReadMessage()
			if err != nil {
				log.Println("Read error:", err)
				return
			}

			if p.debug {
				log.Printf("Received: %s", message)
			}

			var payload pkgframing.MessagePayload
			if err := json.Unmarshal(message, &payload); err != nil {
				log.Printf("Failed to parse message: %v", err)
				continue
			}

			// Send to receive channel
			select {
			case p.receiveChan <- payload:
			case <-p.done:
				return
			}
		}
	}
}

// writeLoop writes messages from sendChan to WebSocket
func (p *WebSocketProxy) writeLoop() {
	defer p.wg.Done()

	for {
		select {
		case <-p.done:
			return
		case msg, ok := <-p.sendChan:
			if !ok {
				return
			}

			data, err := json.Marshal(msg)
			if err != nil {
				log.Printf("Failed to marshal message: %v", err)
				continue
			}

			if err := p.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Printf("Failed to write message: %v", err)
				return
			}
		}
	}
}
