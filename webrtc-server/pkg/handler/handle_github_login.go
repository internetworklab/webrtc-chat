package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	pkggithub "example.com/webrtcserver/pkg/github"
	pkglogin "example.com/webrtcserver/pkg/models/login"
	pkguser "example.com/webrtcserver/pkg/models/user"
	"github.com/google/uuid"
)

type GithubOAuthLoginNonceState struct {
	SessionId   string
	CurrentPage string
}

type GithubOAuthLoginHandler struct {
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

	LoginSuccessRedirectURL string
	UserManager             pkguser.UserManager
	UserSessionManager      pkglogin.UserSessionManager
}

func (h *GithubOAuthLoginHandler) getGithubLoginPage() string {
	if h.GithubOAuthLoginPage != "" {
		return h.GithubOAuthLoginPage
	}
	return "https://github.com/login/oauth/authorize"
}

func (h *GithubOAuthLoginHandler) getGithubTokenEndpoint() string {
	if h.GithubOAuthTokenEndpoint != "" {
		return h.GithubOAuthTokenEndpoint
	}
	return "https://github.com/login/oauth/access_token"
}

func (h *GithubOAuthLoginHandler) getGithubOAuthScope() string {
	if h.GithubOAuthScope != "" {
		return h.GithubOAuthScope
	}
	scopes := []string{"read:user"}
	return strings.Join(scopes, " ")
}

func (h *GithubOAuthLoginHandler) getNonceLifespan() time.Duration {
	const defaultNonceLifespan = 5 * time.Minute
	if h.NonceLifespan == 0 {
		log.Printf("NonceLifespan is not set, using default: %+v", defaultNonceLifespan.String())
		return defaultNonceLifespan
	}
	return h.NonceLifespan
}

func (h *GithubOAuthLoginHandler) createNonceFor(sessionId string, currentPage string) string {
	nonce := uuid.NewString()
	h.nonceMap.Store(nonce, &GithubOAuthLoginNonceState{
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

func (h *GithubOAuthLoginHandler) getSessionIdByNonce(nonce string) string {
	if nonceState, ok := h.nonceMap.Load(nonce); ok && nonceState != nil {
		return nonceState.(*GithubOAuthLoginNonceState).SessionId
	}
	return ""
}

func (h *GithubOAuthLoginHandler) getInitialPageByNonce(nonce string) string {
	if nonceState, ok := h.nonceMap.Load(nonce); ok && nonceState != nil {
		return nonceState.(*GithubOAuthLoginNonceState).CurrentPage
	}
	return ""
}

func (h *GithubOAuthLoginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func (h *GithubOAuthLoginHandler) getGithubOAuthRedirectURL(nonce string) string {
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

func (h *GithubOAuthLoginHandler) handleStart(w http.ResponseWriter, r *http.Request) {
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

func (h *GithubOAuthLoginHandler) handleAuthorizationCode(w http.ResponseWriter, r *http.Request) {
	// See rfc6749 section-4.1.2 and section-4.1.2.1
	// https://datatracker.ietf.org/doc/html/rfc6749#section-4.1.2.1
	if err := r.URL.Query().Get("error"); err != "" {
		errDesc := r.URL.Query().Get("error_description")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("%s: %s", err, errDesc)})
		return
	}

	nonce := r.URL.Query().Get("state")
	ctx := r.Context()
	if nonce == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "No nonce is found in the request"})
		return
	}

	sessionId := ctx.Value(CtxSessionKeySessionId)
	if sessionId == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "No session id is found"})
		return
	}

	if s := h.getSessionIdByNonce(nonce); s == "" || s != sessionId.(string) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("Invalid nonce %+v, no session is bound", nonce)})
		return
	}

	authZCode := r.URL.Query().Get("code")
	if authZCode == "" {
		w.WriteHeader(http.StatusUnauthorized)
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
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("Invalid github token endpoint: %+v", h.GithubOAuthTokenEndpoint)})
		return
	}

	tokenUrlObj.RawQuery = urlVals.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenUrlObj.String(), nil)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to create request to github token endpoint"})
		return
	}
	req.Header.Set("Accept", "application/json")
	cli := http.DefaultClient
	resp, err := cli.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("Failed to get token from github: %+v", err)})
		return
	}
	defer resp.Body.Close()

	tokenResp := new(pkggithub.GithubTokenResponse)
	if err := json.NewDecoder(resp.Body).Decode(tokenResp); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to decode github token api response"})
		return
	}

	if err := h.GithubLoginManager.Login(ctx, sessionId.(string), *tokenResp); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("Failed to login: %+v", err)})
		return
	}

	profile, err := pkggithub.GetGithubProfileByToken(ctx, tokenResp.AccessToken)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("Failed to get Github user profile: %v", err)})
		return
	}

	githubId := profile.Id
	if githubId == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to get Id of github user"})
		return
	}

	ghIdStr := strconv.Itoa(*githubId)
	originalUsername := profile.Login
	newUser := pkguser.User{
		Username:    originalUsername,
		DisplayName: profile.Name,
		AvatarURL:   profile.AvatarURL,
		GithubId:    ghIdStr,
	}

	const maxRetries = 5
	var userObject *pkguser.User

	for i := 0; i < maxRetries; i++ {
		userObject, _, err = h.UserManager.LoadOrCreateNewUserByGithubId(ctx, ghIdStr, newUser)
		if err == nil {
			break
		}
		if errors.Is(err, pkguser.ErrUsernameDuplicated) {
			// Generate random suffix (e.g., -a1b2) and retry
			randomSuffix := fmt.Sprintf("-%04x", rand.Intn(0x10000))
			newUser.Username = originalUsername + randomSuffix
			continue
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to load user from store"})
		return
	}

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to create user with unique username after maximum retries"})
		return
	}

	if err := h.UserSessionManager.LogIn(ctx, userObject.Id, sessionId.(string)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to log user in"})
		return
	}

	if initialPage := h.getInitialPageByNonce(nonce); initialPage != "" {
		http.Redirect(w, r, initialPage, http.StatusTemporaryRedirect)
		return
	}

	if u := h.LoginSuccessRedirectURL; u != "" {
		log.Printf("User %s has been successfully logged in, redirecting to %s", sessionId.(string), u)
		http.Redirect(w, r, u, http.StatusTemporaryRedirect)
		return
	}

	if originURL, err := url.Parse(r.URL.String()); err != nil && originURL != nil {
		originURL.RawFragment = ""
		originURL.RawQuery = ""
		originURL.RawPath = "/"
		log.Printf("User %s has been successfully logged in, redirecting to %s", sessionId.(string), originURL.String())
		http.Redirect(w, r, originURL.String(), http.StatusTemporaryRedirect)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *GithubOAuthLoginHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessId := ctx.Value(CtxSessionKeySessionId)
	if sessId == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "No valid session id is found"})
		return
	}

	if err := h.UserSessionManager.LogOut(ctx, sessId.(string)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Unable to log user out, internal problem"})
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
		if err != nil || (resp != nil && resp.StatusCode >= 400) {
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

func (h *GithubOAuthLoginHandler) handleNotFoundForThis(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(&ErrResponse{Err: fmt.Sprintf("Path %s has no handler attached", r.URL.Path)})
}
