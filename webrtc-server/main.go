package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	pkgconnreg "example.com/webrtcserver/pkg/connreg"
	pkghandler "example.com/webrtcserver/pkg/handler"
	pkgsafemap "example.com/webrtcserver/pkg/safemap"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func main() {
	fmt.Println("Hello, World!")

	listenAddr := ":3001"
	wsTimeout := 10 * time.Second
	wsPath := "/ws"
	sm := pkgsafemap.NewSafeMap()
	cr := pkgconnreg.NewConnRegistry(sm)
	wsHandler := pkghandler.NewWebsocketHandler(&upgrader, cr, wsTimeout)
	mux := http.NewServeMux()
	mux.Handle(wsPath, wsHandler)
	server := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}
	log.Printf("Starting server on %s", listenAddr)
	server.ListenAndServe()
}
