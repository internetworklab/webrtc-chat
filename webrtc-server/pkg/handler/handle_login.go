package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	pkggithub "example.com/webrtcserver/pkg/github"
	"github.com/google/uuid"
)

const QueryParamCurrentPage string = "current_page"

type NonceState struct {
	SessionId   string
	CurrentPage string
}

type LoginHandler struct {
	nonceMap sync.Map

	// If this is empty, we would use default value (5m) for it.
	NonceLifespan time.Duration

	GithubOAuthClientId  string
	GithubOAuthAppSecret string
	GithubOAuthRedirURL  string

	// If this is empty, we would use default value (see github docs) for it.
	GithubOAuthLoginPage string

	// If this is empty, we would use default value ("read:user")) for it.
	GithubOAuthScope string

	// If this is empty, we would use default value (see github docs) for it.
	GithubOAuthTokenEndpoint string
	GithubLoginManager       pkggithub.GithubLoginManager
}

func (h *LoginHandler) getGithubLoginPage() string {
	if h.GithubOAuthLoginPage != "" {
		return h.GithubOAuthLoginPage
	}
	return "https://github.com/login/oauth/authorize"
}

func (h *LoginHandler) getGithubTokenEndpoint() string {
	if h.GithubOAuthTokenEndpoint != "" {
		return h.GithubOAuthTokenEndpoint
	}
	return "https://github.com/login/oauth/access_token"
}

func (h *LoginHandler) getGithubOAuthScope() string {
	if h.GithubOAuthScope != "" {
		return h.GithubOAuthScope
	}
	scopes := []string{"read:user"}
	return strings.Join(scopes, " ")
}

func (h *LoginHandler) getNonceLifespan() time.Duration {
	const defaultNonceLifespan = 5 * time.Minute
	if h.NonceLifespan == 0 {
		log.Printf("NonceLifespan is not set, using default: %+v", defaultNonceLifespan.String())
		return defaultNonceLifespan
	}
	return h.NonceLifespan
}

func (h *LoginHandler) createNonceFor(sessionId string, currentPage string) string {
	nonce := uuid.NewString()
	h.nonceMap.Store(nonce, &NonceState{
		SessionId:   sessionId,
		CurrentPage: currentPage,
	})
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
	if nonceState, ok := h.nonceMap.Load(nonce); ok && nonceState != nil {
		return nonceState.(*NonceState).SessionId
	}
	return ""
}

func (h *LoginHandler) getInitialPageByNonce(nonce string) string {
	if nonceState, ok := h.nonceMap.Load(nonce); ok && nonceState != nil {
		return nonceState.(*NonceState).CurrentPage
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
		} else if strings.HasSuffix(url.Path, "/login/delete") {
			h.handleLogout(w, r)
			return
		}
	}
	h.handleNotFoundForThis(w, r)
}

func (h *LoginHandler) getGithubOAuthRedirectURL(nonce string) string {
	urlVals := url.Values{}
	urlVals.Set("client_id", h.GithubOAuthClientId)
	urlVals.Set("redirect_uri", h.GithubOAuthRedirURL)
	urlVals.Set("scope", h.getGithubOAuthScope())
	urlVals.Set("state", nonce)
	urlObj, err := url.Parse(h.getGithubLoginPage())
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
	nonce := h.createNonceFor(sessionId.(string), r.URL.Query().Get(QueryParamCurrentPage))
	redirURL := h.getGithubOAuthRedirectURL(nonce)
	if redirURL == "" {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to determine redir url (internal error)"})
		return
	}
	http.Redirect(w, r, redirURL, http.StatusTemporaryRedirect)
}

func (h *LoginHandler) handleAuthorizationCode(w http.ResponseWriter, r *http.Request) {
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
	urlVals.Set("client_secret", h.GithubOAuthAppSecret)
	urlVals.Set("code", authZCode)
	urlVals.Set("redirect_uri", h.GithubOAuthRedirURL)

	tokenUrlObj, err := url.Parse(h.getGithubTokenEndpoint())
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
	defer resp.Body.Close()

	tokenResp := new(pkggithub.GithubTokenResponse)
	if err := json.NewDecoder(resp.Body).Decode(tokenResp); err != nil {
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to decode github token api response"})
		return
	}

	if err := h.GithubLoginManager.Login(ctx, sessionId.(string), *tokenResp); err != nil {
		json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("Failed to login: %+v", err)})
		return
	}

	if initialPage := h.getInitialPageByNonce(nonce); initialPage != "" {
		http.Redirect(w, r, initialPage, http.StatusTemporaryRedirect)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *LoginHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessId := ctx.Value(CtxSessionKeySessionId)
	if sessId == nil {
		json.NewEncoder(w).Encode(&ErrResponse{Err: "No valid session id is found"})
		return
	}

	if tokenObj, err := h.GithubLoginManager.GetToken(ctx, sessId.(string)); err == nil && tokenObj != nil {
		u := fmt.Sprintf("https://api.github.com/applications/%s/token", h.GithubOAuthClientId)
		var reqBody bytes.Buffer
		json.NewEncoder(&reqBody).Encode(map[string]string{"access_token": tokenObj.AccessToken})
		req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, u, &reqBody)
		req.SetBasicAuth(h.GithubOAuthClientId, h.GithubOAuthAppSecret)
		req.Header.Set("Accept", "application/vnd.github+json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to revoke Github access_token"})
			return
		}
		if err := h.GithubLoginManager.DeleteToken(ctx, sessId.(string)); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to relete token from memory"})
			return
		}
		w.WriteHeader(resp.StatusCode)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *LoginHandler) handleNotFoundForThis(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("Path %s has no handler attached", r.URL.Path)})
}
