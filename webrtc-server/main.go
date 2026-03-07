package main

import (
	"log"
	"net/http"
	"os"
	"slices"
	"time"

	pkgconnreg "example.com/webrtcserver/pkg/connreg"
	pkghandler "example.com/webrtcserver/pkg/handler"
	pkglogin "example.com/webrtcserver/pkg/models/login"
	pkguser "example.com/webrtcserver/pkg/models/user"
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
	GithubLoginRedirectURL    string        `name:"github-login-redir-url" help:"The redirect_uri parameter that will be pass to Github OAuth login API" default:"http://localhost:3000/api/github/login/auth"`
	KioubitLoginRedirectURL   string        `name:"kioubit-login-redir-url" help:"The page the user will be redirect to once the authorization grants" default:"http://localhost:3000/api/kioubit/login/auth"`
	Debug                     bool          `name:"debug" help:"Toggle this to make it print extra verbose logs in stdout" default:"false"`
	LoginSuccessRedirectURL   string        `name:"login-success-redir-url" help:"The page to which the user will be redirect to once oauth login is successful, this usually should be the home page for a typical SPA web app" default:"http://localhost:3000/"`
	KioubitLoginPubkey        string        `name:"kioubit-login-pubkey" help:"The path to the PEM pubkey file in order to use the Sign in with Kioubit service, this is optional"`
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

	// initiate UserManager and UserSessionManager, and
	// inject them to where is needed
	userMgr := &pkguser.MemoryUserManager{}
	userSessionMgr := &pkglogin.MemoryUserSessionManager{}

	upgrader := websocket.Upgrader{
		CheckOrigin: cli.getOriginValidator(),
	}

	sm := pkgsafemap.NewSafeMap()
	cr := pkgconnreg.NewConnRegistry(sm)
	wsHandler := &pkghandler.WebsocketHandler{
		Upgrader:           &upgrader,
		ConnectionRegistry: cr,
		ClientTimeout:      cli.WsTimeout,
		UserManager:        userMgr,
		UserSessionManager: userSessionMgr,
	}

	var connsHandler http.Handler = pkghandler.NewConnsHandler(cr)

	mux := http.NewServeMux()
	mux.Handle(cli.WsPath, wsHandler)
	mux.Handle("/conns", connsHandler)

	cntHandler := &pkghandler.CounterHandler{}
	mux.Handle("/counter", cntHandler)

	if pubkeyPath := cli.KioubitLoginPubkey; pubkeyPath != "" {
		pubkey, err := os.ReadFile(pubkeyPath)
		if err != nil {
			log.Fatalln("Failed to read path", pubkeyPath, err)
		}
		mux.Handle("/kioubit/login/", &pkghandler.KioubitLoginHandler{
			KioubitRedirURL:         cli.KioubitLoginRedirectURL,
			LoginSuccessRedirectURL: cli.LoginSuccessRedirectURL,
			UserManager:             userMgr,
			UserSessionManager:      userSessionMgr,
			KioubitPubkey:           pubkey,
		})
	}

	mux.Handle("/logout", &pkghandler.LogoutHandler{
		UserSessionManager: userSessionMgr,
	})

	mux.Handle("/github/login/", &pkghandler.GithubOAuthLoginHandler{
		GithubOAuthClientId:     gh_cli_id,
		GithubOAuthAppSecret:    gh_cli_sec,
		GithubOAuthRedirURL:     cli.GithubLoginRedirectURL,
		LoginSuccessRedirectURL: cli.LoginSuccessRedirectURL,
		UserManager:             userMgr,
		UserSessionManager:      userSessionMgr,
	})

	mux.Handle("/profile/avatar", &pkghandler.ProfileAvatarHandler{
		UserManager:        userMgr,
		UserSessionManager: userSessionMgr,
	})
	mux.Handle("/profile/status", &pkghandler.ProfileStatusHandler{
		UserManager:        userMgr,
		UserSessionManager: userSessionMgr,
	})
	mux.Handle("/profile", &pkghandler.ProfileHandler{
		UserManager:        userMgr,
		UserSessionManager: userSessionMgr,
	})

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
