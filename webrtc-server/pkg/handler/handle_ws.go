package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	pkgconnreg "example.com/webrtcserver/pkg/connreg"
	pkgframing "example.com/webrtcserver/pkg/framing"
	"example.com/webrtcserver/pkg/ws_proxy"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type WebsocketHandler struct {
	upgrader *websocket.Upgrader
	cr       *pkgconnreg.ConnRegistry
	timeout  time.Duration
	counter  int
}

func NewWebsocketHandler(upgrader *websocket.Upgrader, cr *pkgconnreg.ConnRegistry, timeout time.Duration) *WebsocketHandler {
	return &WebsocketHandler{
		upgrader: upgrader,
		cr:       cr,
		timeout:  timeout,
	}
}

func (handler *WebsocketHandler) sendBroadcastMsg(payload pkgframing.MessagePayload) error {
	for key, connent := range handler.cr.Dump() {
		if wsConn := connent.WSConn; wsConn != nil {
			log.Printf("Broadcasting message to %s", key)
			if err := wsConn.WriteJSON(payload); err != nil {
				return fmt.Errorf("failed to send response message to %s: %v", key, err)
			}
		}
	}
	return nil
}

func (handler *WebsocketHandler) sendMsgTo(nodeId string, payload pkgframing.MessagePayload) error {
	nodeEntry, err := handler.cr.GetByNodeId(nodeId)

	if err != nil {
		return fmt.Errorf("failed to get node entry by node id: %v", err)
	}

	if nodeEntry != nil {
		if wsConn := nodeEntry.WSConn; wsConn != nil {
			if err := wsConn.WriteJSON(payload); err != nil {
				return fmt.Errorf("failed to send SDP offer message to %s: %v", payload.SDPOffer.ToNodeId, err)
			}
		}
	}
	return nil
}

func (handler *WebsocketHandler) getDefaultUserName(numId int) string {
	return fmt.Sprintf("user%04d", numId)
}

func (handler *WebsocketHandler) handleTextMessage(key string, conn *ws_proxy.WebsocketWriteProxy, msg []byte, numId int) error {
	cr := handler.cr
	if cr == nil {
		return fmt.Errorf("connection registry is not set")
	}

	var payload pkgframing.MessagePayload
	err := json.Unmarshal(msg, &payload)
	if err != nil {
		return fmt.Errorf("failed to unmarshal message from %s: %v", key, err)
	}

	if payload.Register != nil {
		if payload.Register.NodeName == "" {
			payload.Register.NodeName = handler.getDefaultUserName(numId)
		}
		cr.Register(key, *payload.Register, nil, conn)
		responsePayload := pkgframing.MessagePayload{
			Online: &pkgconnreg.NodeGoesOnline{
				NodeId: key,
			},
		}
		if err := handler.sendBroadcastMsg(responsePayload); err != nil {
			return fmt.Errorf("failed to send response message to %s: %v", key, err)
		}
		responsePayload = pkgframing.MessagePayload{
			Register: payload.Register,
			NodeId:   key,
		}
		if err := conn.WriteJSON(responsePayload); err != nil {
			return fmt.Errorf("failed to send response message to %s: %v", key, err)
		}
	}
	if payload.Echo != nil {
		if payload.Echo.Direction == pkgconnreg.EchoDirectionC2S {
			cr.UpdateHeartbeat(key)
			responsePayload := pkgframing.MessagePayload{
				Echo: &pkgconnreg.EchoPayload{
					Direction:       pkgconnreg.EchoDirectionS2C,
					CorrelationID:   payload.Echo.CorrelationID,
					ServerTimestamp: uint64(time.Now().UnixMilli()),
					Timestamp:       payload.Echo.Timestamp,
					SeqID:           payload.Echo.SeqID,
				},
				NodeId: key,
			}
			if err := conn.WriteJSON(responsePayload); err != nil {
				return fmt.Errorf("failed to send response message to %s: %v", key, err)
			}

		}
	}
	if payload.AttributesAnnouncement != nil {
		cr.SetAttributes(key, payload.AttributesAnnouncement)
	}
	if payload.Rename != nil {
		originName, err := cr.UpdatePreference(key, *payload.Rename)
		if err != nil {
			return fmt.Errorf("failed to rename connection from %s: %v", key, err)
		}
		responsePayload := pkgframing.MessagePayload{
			Rename: &pkgconnreg.RenamePayload{
				OriginNodeName: originName,
				NewPreference:  payload.Rename.NewPreference,
			},
		}
		if err := handler.sendBroadcastMsg(responsePayload); err != nil {
			return fmt.Errorf("failed to send response message to %s: %v", key, err)
		}
	}
	if payload.SDPOffer != nil {
		if err := handler.sendMsgTo(payload.SDPOffer.ToNodeId, payload); err != nil {
			return fmt.Errorf("failed to send SDP offer message to %s: %v", payload.SDPOffer.ToNodeId, err)
		}
	}
	if payload.ICEOffer != nil {
		if err := handler.sendMsgTo(payload.ICEOffer.ToNodeId, payload); err != nil {
			return fmt.Errorf("failed to send ICE offer message to %s: %v", payload.ICEOffer.ToNodeId, err)
		}
	}

	return nil
}

func (h *WebsocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upgrader := h.upgrader
	cr := h.cr
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade to WebSocket: %v", err)
		return
	}

	remoteKey := uuid.New().String()
	numId := cr.OpenConnection(remoteKey, nil)
	log.Printf("Connection opened for %s, total connections: %d", remoteKey, cr.Count())

	defer func() {
		log.Printf("Closing WebSocket connection: %s", remoteKey)
		err := conn.Close()
		if err != nil {
			log.Printf("Failed to close WebSocket connection for %s: %v", remoteKey, err)
		}
		cr.CloseConnection(remoteKey)
		log.Printf("Connection closed for %s, remaining connections: %d", remoteKey, cr.Count())
	}()

	var gcTimer *time.Timer = nil
	if int64(h.timeout) == 0 {
		panic("timeout is not set")
	}
	gcTimer = time.NewTimer(h.timeout)
	defer func() {
		if gcTimer != nil {
			gcTimer.Stop()
			gcTimer = nil
		}
	}()

	connErrCh := make(chan error)

	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create ONE proxy per connection, not per message.
		// This ensures only one goroutine writes to the websocket connection.
		connProxy := ws_proxy.NewWebsocketWriteProxy(conn)
		connProxy.Run(ctx)

		for {
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				connErrCh <- fmt.Errorf("failed to read message from %s: %v", remoteKey, err)
				break
			}

			switch msgType {
			case websocket.TextMessage:
				if err := h.handleTextMessage(remoteKey, connProxy, msg, numId); err != nil {
					log.Printf("Failed to handle text message from %s: %v", remoteKey, err)
					continue
				}
				gcTimer.Reset(h.timeout)
			default:
				log.Printf("Received unknown message type from %s: %d", remoteKey, msgType)
			}
		}
	}()

	select {
	case <-gcTimer.C:
		log.Printf("Garbage collection timeout for %s, closing connection", remoteKey)
	case err := <-connErrCh:
		if err != nil {
			log.Printf("Connection error for %s: %v", remoteKey, err)
		}
	}
}
