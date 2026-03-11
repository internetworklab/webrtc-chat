package ws_runner

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"

	"time"

	pkghandlers "webrtc-agents/pkg/handlers"

	pkgconnreg "example.com/webrtcserver/pkg/connreg"
	pkgframing "example.com/webrtcserver/pkg/framing"

	"github.com/gorilla/websocket"
)

type WebSocketRunner struct {
	// URL is the URL of signalling WebSocket server
	URL                   url.URL
	ReconnectDelay        time.Duration
	ReconnectOnDisconnect bool
	PingIntv              time.Duration
	Debug                 bool
	NodeName              string
	TLSConfig             *tls.Config
	Resolver              *net.Resolver
	PreferIPv6            bool
	JWTEnvName            string
}

func (runner *WebSocketRunner) getDialer() *websocket.Dialer {
	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 45 * time.Second,
	}

	dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		resolver := net.DefaultResolver
		if runner.Resolver != nil {
			resolver = runner.Resolver
		}

		// Use the custom resolver to lookup IP addresses
		nwPreference := "ip"
		if runner.PreferIPv6 {
			nwPreference = "ip6"
		}
		ips, err := resolver.LookupIP(ctx, nwPreference, host)
		if err != nil {
			return nil, err
		}

		if len(ips) == 0 {
			return nil, fmt.Errorf("failed to resolve IP addresses for %s, network: %s", host, nwPreference)
		}

		// Try connecting to each resolved IP until one succeeds
		var lastErr error
		for _, ip := range ips {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				ipAddr := net.JoinHostPort(ip.String(), port)
				conn, err := (&net.Dialer{}).DialContext(ctx, network, ipAddr)
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
		}

		return nil, fmt.Errorf("failed to connect to %s: %v", addr, lastErr)
	}

	// Check if the URL scheme is wss (WebSocket Secure) and set TLS config
	if runner.URL.Scheme == "wss" && runner.TLSConfig != nil {
		dialer.TLSClientConfig = runner.TLSConfig
	}

	return dialer
}

func (runner *WebSocketRunner) Run(ctx context.Context, handler pkghandlers.GenericWebRTCHandler) {
	u := runner.URL
	txChannel := make(chan pkgframing.MessagePayload)
	outputDataCh := make(chan pkgframing.MessagePayload)
	go func(ctx context.Context) {
		defer close(outputDataCh)
		errLogger := log.New(os.Stderr, "", 0)

		for {
			log.Printf("Connecting to %s", u.String())

			// Establish WebSocket connection

			wsConn, _, err := runner.getDialer().Dial(u.String(), nil)
			if err == nil {
				func(wsConn *websocket.Conn, ctx context.Context) {
					defer wsConn.Close()
					log.Printf("Dialed to ws server %+v", wsConn.RemoteAddr().String())

					registerer := &WebSocketRegisterer{}
					if err := registerer.Register(wsConn, runner.NodeName); err != nil {
						errLogger.Println("Failed to send registration message:", err)
						return
					}

					log.Printf("Sent registration message for node: %s", runner.NodeName)

					wsPinger := &BasicWSMsgHandler{
						Intv:  runner.PingIntv,
						Debug: runner.Debug,
					}
					dataCh, errCh := wsPinger.startMessagesLoop(ctx, wsConn, txChannel)
					log.Println("Ping/pong loop started")

					go func() {
						for item := range dataCh {
							outputDataCh <- item
						}
					}()

					err, ok := <-errCh
					if ok && err != nil {
						errLogger.Printf("Error on ws connection: %+v", err)
						return
					}
				}(wsConn, ctx)
			} else {
				errLogger.Println("Failed to dial:", err)
			}

			if !runner.ReconnectOnDisconnect {
				return
			}

			log.Printf("Reconnecting to %s in %s", u.String(), runner.ReconnectDelay.String())
			select {
			case <-ctx.Done():
				errLogger.Println("Reconnect cancelled (context cancelled):", ctx.Err())
				return
			case <-time.After(runner.ReconnectDelay):
			}
		}
	}(ctx)

	handler.Serve(ctx, txChannel, outputDataCh)
}

type WebSocketRegisterer struct{}

// Send registration message
func (reg *WebSocketRegisterer) Register(wsConn *websocket.Conn, nodeName string) error {
	registerMsg := pkgframing.MessagePayload{
		Register: &pkgconnreg.RegisterPayload{
			NodeName: nodeName,
		},
	}

	return wsConn.WriteJSON(registerMsg)
}

type BasicWSMsgHandler struct {
	Intv  time.Duration
	Debug bool
}

// Start ping goroutine to maintain WebSocket connection
func (pinger *BasicWSMsgHandler) startMessagesLoop(ctx context.Context, wsConn *websocket.Conn, txChannel <-chan pkgframing.MessagePayload) (chan pkgframing.MessagePayload, chan error) {
	period := pinger.Intv
	debug := pinger.Debug

	seqID := uint64(0)

	errCh := make(chan error, 1)
	dataCh := make(chan pkgframing.MessagePayload, 1)

	go func(ctx context.Context) {
		defer close(errCh)

		readErrCh := make(chan error, 1)
		go func() {
			defer close(readErrCh)
			defer close(dataCh)

			for {
				var msg pkgframing.MessagePayload
				if err := wsConn.ReadJSON(&msg); err != nil {
					log.Println("Failed to get message from ws connection:", err)
					errCh <- err
					return
				}

				if msg.Echo != nil {
					if msg.Echo.Direction == pkgconnreg.EchoDirectionS2C && pinger.Debug {
						rtt := time.Since(time.UnixMilli(int64(msg.Echo.Timestamp)))
						log.Printf("Pong received - RTT: %v, CorrelationID: %s, SeqID: %d",
							rtt, msg.Echo.CorrelationID, msg.Echo.SeqID)
					}
					continue
				}

				dataCh <- msg
			}
		}()

		ticker := time.NewTicker(period)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case txItem := <-txChannel:
				if err := wsConn.WriteJSON(txItem); err != nil {
					log.Println("Failed to send message:", err)
					errCh <- err
					return
				}
				continue
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

				if err := wsConn.WriteJSON(pingMsg); err != nil {
					log.Println("Failed to send ping:", err)
					errCh <- err
					return
				}

				if debug {
					log.Printf("Sent ping - SeqID: %d, CorrelationID: ping-%d", seqID, seqID)
				}
			case err, ok := <-readErrCh:
				if ok && err != nil {
					log.Println("Failed to read message:", err)
					errCh <- err
				}
				return
			}
		}
	}(ctx)

	return dataCh, errCh
}
