package main

import (
	"log"
	"net/http"
	"os"
	"slices"
	"time"

	pkgconnreg "example.com/webrtcserver/pkg/connreg"
	pkggithub "example.com/webrtcserver/pkg/github"
	pkghandler "example.com/webrtcserver/pkg/handler"
	pkgsafemap "example.com/webrtcserver/pkg/safemap"
	pkgsession "example.com/webrtcserver/pkg/session"

	"github.com/alecthomas/kong"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

type CLI struct {
	ListenAddr                string        `name:"listen-addr" help:"Address to listen on" default:":3001"`
	WsTimeout                 time.Duration `name:"ws-timeout" help:"WebSocket timeout duration" default:"30s"`
	WsPath                    string        `name:"ws-path" help:"WebSocket path" default:"/ws"`
	AllowedOrigins            []string      `name:"allowed-origin" help:"Allowed origins for CORS (may be specified multiple times)"`
	DefaultCorsAllowed        bool          `name:"default-cors-allowed" help:"Allow requests with absent Origin header" default:"true"`
	InjectAllowAllCorsHeaders bool          `name:"inject-allow-all-cors-headers" help:"Inject CORS headers that allow all origins (for debugging purposes)"`
	GithubLoginRedirectURL    string        `name:"github-login-redir-url" help:"The redirect_uri parameter that will be pass to Github OAuth login API" default:"http://localhost:3000/api/login/auth"`
	Debug                     bool          `name:"debug" help:"Toggle this to make it print extra verbose logs in stdout" default:"false"`
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

	// Load .env file if it exists (ignore error if file doesn't exist)
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found or error loading .env file, continuing with existing environment variables")
	}

	gh_cli_id := os.Getenv("GH_LOGIN_CLIENT_ID")
	if gh_cli_id == "" {
		log.Fatalf("Github OAuth Client Id not found")
	}

	gh_cli_sec := os.Getenv("GH_LOGIN_CLIENT_SECRET")
	if gh_cli_sec == "" {
		log.Fatalf("Github OAuth Client Secret not found")
	}

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

	ghTokenManager := &pkggithub.MemoryGithubLoginManager{
		Debug: cli.Debug,
	}

	loginHandler := &pkghandler.LoginHandler{
		GithubOAuthClientId:  gh_cli_id,
		GithubOAuthAppSecret: gh_cli_sec,
		GithubOAuthRedirURL:  cli.GithubLoginRedirectURL,
		GithubLoginManager:   ghTokenManager,
	}
	mux.Handle("/login/", loginHandler)

	profileHandler := &pkghandler.ProfileHandler{
		GithubTokenRetriever: ghTokenManager,
	}
	mux.Handle("/profile", profileHandler)

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
