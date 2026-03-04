package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type GithubTokenResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope,omitempty"`
	TokenType   string `json:"token_type,omitempty"`
}

type GithubLoginManager interface {
	Login(ctx context.Context, sessionId string, ghToken GithubTokenResponse) error
}

type LoginHandler struct {
	nonceMap                 sync.Map
	NonceLifespan            time.Duration
	GithubOAuthClientId      string
	GithubOAuthAppSecret     []byte
	GithubOAuthRedirURL      string
	GithubOAuthLoginPage     string
	GithubOAuthScope         string
	GithubOAuthTokenEndpoint string
	GithubLoginManager       GithubLoginManager
}

func (h *LoginHandler) getNonceLifespan() time.Duration {
	const defaultNonceLifespan = 5 * time.Minute
	if h.NonceLifespan == 0 {
		log.Printf("NonceLifespan is not set, using default: %+v", defaultNonceLifespan.String())
		return defaultNonceLifespan
	}
	return h.NonceLifespan
}

func (h *LoginHandler) createNonceFor(sessionId string) string {
	nonce := uuid.NewString()
	h.nonceMap.Store(nonce, sessionId)
	go func() {
		<-time.After(h.getNonceLifespan())
		if h != nil {
			h.nonceMap.Delete(nonce)
			log.Printf("Nonce %+v is deleted now", nonce)
		}
	}()
	return nonce
}

func (h *LoginHandler) getSessionIdByNonce(nonce string) string {
	if sess, ok := h.nonceMap.Load(nonce); ok {
		return sess.(string)
	}
	return ""
}

func (h *LoginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if url := r.URL; url != nil {
		if strings.HasSuffix(url.Path, "/login/start") {
			h.handleStart(w, r)
			return
		} else if strings.HasSuffix(url.Path, "/login/auth") {
			h.handleAuthorizationCode(w, r)
			return
		}
	}
	h.handleNotFoundForThis(w, r)
}

func (h *LoginHandler) getGithubOAuthRedirectURL(nonce string) string {
	urlVals := url.Values{}
	urlVals.Set("client_id", h.GithubOAuthClientId)
	urlVals.Set("redirect_uri", h.GithubOAuthRedirURL)
	urlVals.Set("scope", h.GithubOAuthScope)
	urlVals.Set("state", nonce)
	urlObj, err := url.Parse(h.GithubOAuthLoginPage)
	if err != nil {
		log.New(os.Stderr, "LoginHandler", 0).Println("Invalid github login page url:", err)
		return ""
	}
	urlObj.RawQuery = urlVals.Encode()
	return urlObj.String()
}

func (h *LoginHandler) handleStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionId := ctx.Value(CtxSessionKeySessionId)
	if sessionId == nil {
		json.NewEncoder(w).Encode(&ErrResponse{Err: "No session id is found"})
		return
	}
	nonce := h.createNonceFor(sessionId.(string))
	redirURL := h.getGithubOAuthRedirectURL(nonce)
	http.Redirect(w, r, redirURL, http.StatusTemporaryRedirect)
}

func (h *LoginHandler) handleAuthorizationCode(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, r.URL.Path)
	nonce := r.URL.Query().Get("state")
	ctx := r.Context()
	if nonce == "" {
		json.NewEncoder(w).Encode(&ErrResponse{Err: "No nonce is found in the request"})
		return
	}

	sessionId := ctx.Value(CtxSessionKeySessionId)
	if sessionId == nil {
		json.NewEncoder(w).Encode(&ErrResponse{Err: "No session id is found"})
		return
	}

	if s := h.getSessionIdByNonce(nonce); s == "" || s != sessionId.(string) {
		json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("Invalid nonce %+v, no session is bound", nonce)})
		return
	}

	authZCode := r.URL.Query().Get("code")
	if authZCode == "" {
		json.NewEncoder(w).Encode(&ErrResponse{Err: "No authorization code is found in the request"})
		return
	}

	urlVals := url.Values{}
	urlVals.Set("client_id", h.GithubOAuthClientId)
	urlVals.Set("client_secret", string(h.GithubOAuthAppSecret))
	urlVals.Set("code", authZCode)
	urlVals.Set("redirect_uri", h.GithubOAuthRedirURL)

	tokenUrlObj, err := url.Parse(h.GithubOAuthTokenEndpoint)
	if err != nil {
		json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("Invalid github token endpoint: %+v", h.GithubOAuthTokenEndpoint)})
		return
	}

	tokenUrlObj.RawQuery = urlVals.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenUrlObj.String(), nil)
	if err != nil {
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to create request to github token endpoint"})
		return
	}
	req.Header.Set("Accept", "application/json")
	cli := http.DefaultClient
	resp, err := cli.Do(req)
	if err != nil {
		json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("Failed to get token from github: %+v", err)})
		return
	}

	tokenResp := new(GithubTokenResponse)
	if err := json.NewDecoder(resp.Body).Decode(tokenResp); err != nil {
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to decode github token api response"})
		return
	}

	if err := h.GithubLoginManager.Login(ctx, sessionId.(string), *tokenResp); err != nil {
		json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("Failed to login: %+v", err)})
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *LoginHandler) handleNotFoundForThis(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("Path %s has no handler attached", r.URL.Path)})
}
