package main

import (
	"log"
	"net/http"
	"slices"
	"time"

	pkgconnreg "example.com/webrtcserver/pkg/connreg"
	pkghandler "example.com/webrtcserver/pkg/handler"
	pkgsafemap "example.com/webrtcserver/pkg/safemap"
	pkgsession "example.com/webrtcserver/pkg/session"

	"github.com/alecthomas/kong"
	"github.com/gorilla/websocket"
)

type CLI struct {
	ListenAddr                string        `name:"listen-addr" help:"Address to listen on" default:":3001"`
	WsTimeout                 time.Duration `name:"ws-timeout" help:"WebSocket timeout duration" default:"30s"`
	WsPath                    string        `name:"ws-path" help:"WebSocket path" default:"/ws"`
	AllowedOrigins            []string      `name:"allowed-origin" help:"Allowed origins for CORS (may be specified multiple times)"`
	DefaultCorsAllowed        bool          `name:"default-cors-allowed" help:"Allow requests with absent Origin header" default:"true"`
	InjectAllowAllCorsHeaders bool          `name:"inject-allow-all-cors-headers" help:"Inject CORS headers that allow all origins (for debugging purposes)"`
}

var cli CLI

// getOriginValidator returns a function that validates the Origin header
// against a list of allowed origins.
func (c *CLI) getOriginValidator() func(r *http.Request) bool {
	return func(r *http.Request) bool {
		if origin := r.Header.Get("Origin"); origin != "" {
			return slices.Contains(c.AllowedOrigins, origin)
		}
		return c.DefaultCorsAllowed
	}
}

func main() {
	kong.Parse(&cli)

	upgrader := websocket.Upgrader{
		CheckOrigin: cli.getOriginValidator(),
	}

	sm := pkgsafemap.NewSafeMap()
	cr := pkgconnreg.NewConnRegistry(sm)
	wsHandler := pkghandler.NewWebsocketHandler(&upgrader, cr, cli.WsTimeout)

	var connsHandler http.Handler = pkghandler.NewConnsHandler(cr)

	mux := http.NewServeMux()
	mux.Handle(cli.WsPath, wsHandler)
	mux.Handle("/conns", connsHandler)

	cntHandler := &pkghandler.CounterHandler{}
	mux.Handle("/counter", cntHandler)

	loginHandler := &pkghandler.LoginHandler{}
	mux.Handle("/login/", loginHandler)

	sessMngr := &pkgsession.CookieSessionManager{}
	server := &http.Server{
		Addr:    cli.ListenAddr,
		Handler: pkghandler.WithSessionHandler(mux, sessMngr),
	}
	if cli.InjectAllowAllCorsHeaders {
		server.Handler = pkghandler.WithCORSAllowAny(server.Handler)
	}
	log.Printf("Starting server on %s", cli.ListenAddr)
	server.ListenAndServe()
}
